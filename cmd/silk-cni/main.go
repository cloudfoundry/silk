package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"code.cloudfoundry.org/lager"

	"code.cloudfoundry.org/silk/cni/adapter"
	"code.cloudfoundry.org/silk/cni/config"
	"code.cloudfoundry.org/silk/cni/legacy_flannel"
	"code.cloudfoundry.org/silk/cni/lib"
	"code.cloudfoundry.org/silk/daemon"
	libAdapter "code.cloudfoundry.org/silk/lib/adapter"
	"code.cloudfoundry.org/silk/lib/datastore"
	"code.cloudfoundry.org/silk/lib/filelock"
	"code.cloudfoundry.org/silk/lib/serial"
	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
)

type CNIPlugin struct {
	HostNSPath      string
	HostNS          ns.NetNS
	ConfigCreator   *config.ConfigCreator
	VethPairCreator *lib.VethPairCreator
	Host            *lib.Host
	Container       *lib.Container
	Store           *datastore.Store
	Logger          lager.Logger
}

func main() {
	hostNS, err := ns.GetCurrentNS()
	if err != nil {
		log.Fatal(err)
	}

	logger := lager.NewLogger("silk-cni")
	sink := lager.NewWriterSink(os.Stderr, lager.INFO)
	logger.RegisterSink(sink)

	netlinkAdapter := &libAdapter.NetlinkAdapter{}
	linkOperations := &lib.LinkOperations{
		SysctlAdapter:  &adapter.SysctlAdapter{},
		NetlinkAdapter: netlinkAdapter,
		Logger:         logger,
	}
	commonSetup := &lib.Common{
		NetlinkAdapter: netlinkAdapter,
		LinkOperations: linkOperations,
	}
	store := &datastore.Store{
		Serializer: &serial.Serial{},
		Locker:     &filelock.Locker{},
	}

	plugin := &CNIPlugin{
		HostNSPath: hostNS.Path(),
		HostNS:     hostNS,
		ConfigCreator: &config.ConfigCreator{
			HardwareAddressGenerator: &config.HardwareAddressGenerator{},
			DeviceNameGenerator:      &config.DeviceNameGenerator{},
			NamespaceAdapter:         &adapter.NamespaceAdapter{},
		},
		VethPairCreator: &lib.VethPairCreator{
			NetlinkAdapter: netlinkAdapter,
		},
		Host: &lib.Host{
			Common:         commonSetup,
			LinkOperations: linkOperations,
		},
		Container: &lib.Container{
			Common:         commonSetup,
			LinkOperations: linkOperations,
		},
		Logger: logger,
		Store:  store,
	}

	skel.PluginMain(plugin.cmdAdd, plugin.cmdDel, version.PluginSupports("0.3.0"))
}

type NetConf struct {
	types.NetConf
	DataDir    string `json:"dataDir"`
	SubnetFile string `json:"subnetFile"`
	MTU        int    `json:"mtu"`
	Datastore  string `json:"datastore"`
	DaemonPort int    `json:"daemonPort"`
}

type HostLocalIPAM struct {
	Type    string         `json:"type"`
	Subnet  string         `json:"subnet"`
	Gateway string         `json:"gateway"`
	Routes  []*types.Route `json:"routes"`
	DataDir string         `json:"dataDir"`
}

type ConfForHostLocal struct {
	CNIVersion string        `json:"cniVersion"`
	Name       string        `json:"name"`
	IPAM       HostLocalIPAM `json:"ipam"`
}

func typedError(msg string, err error) *types.Error {
	return &types.Error{
		Code:    100,
		Msg:     msg,
		Details: err.Error(),
	}
}

func getNetworkInfo(netConf NetConf) (legacy_flannel.NetworkInfo, error) {
	var networkInfo legacy_flannel.NetworkInfo
	var err error
	if netConf.SubnetFile != "" {
		networkInfo, err = legacy_flannel.DiscoverNetworkInfo(netConf.SubnetFile, netConf.MTU)
		if err != nil {
			return legacy_flannel.NetworkInfo{}, typedError("discover network info", err)
		}
	} else {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d", netConf.DaemonPort))
		if err != nil {
			return legacy_flannel.NetworkInfo{}, typedError("discover network info", err)
		}

		defer resp.Body.Close()

		contents, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return legacy_flannel.NetworkInfo{}, typedError("discover network info", fmt.Errorf("read response body: %s", err)) //not tested
		}

		var daemonNetInfo daemon.NetworkInfo
		err = json.Unmarshal(contents, &daemonNetInfo)
		if err != nil {
			return legacy_flannel.NetworkInfo{}, typedError("discover network info", fmt.Errorf("unmarshal network info: %s", err))
		}

		mtu := netConf.MTU
		if mtu == 0 {
			mtu = daemonNetInfo.MTU
		}

		networkInfo = legacy_flannel.NetworkInfo{
			Subnet: daemonNetInfo.OverlaySubnet,
			MTU:    mtu,
		}
	}
	return networkInfo, nil
}

func (p *CNIPlugin) cmdAdd(args *skel.CmdArgs) error {
	var netConf NetConf
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		return err // impossible, skel package asserts JSON is valid
	}

	networkInfo, err := getNetworkInfo(netConf)
	if err != nil {
		return err
	}

	generator := config.IPAMConfigGenerator{}
	ipamConfig := generator.GenerateConfig(networkInfo.Subnet, netConf.Name, netConf.DataDir)
	ipamConfigBytes, _ := json.Marshal(ipamConfig) // untestable

	result, err := ipam.ExecAdd("host-local", ipamConfigBytes)
	if err != nil {
		return typedError("run ipam plugin", err)
	}

	cniResult, err := current.NewResultFromResult(result)
	if err != nil {
		return fmt.Errorf("convert result to current CNI version: %s", err) // not tested
	}

	cfg, err := p.ConfigCreator.Create(p.HostNS, args, cniResult, networkInfo.MTU)
	if err != nil {
		return typedError("create config", err)
	}

	err = p.VethPairCreator.Create(cfg)
	if err != nil {
		return typedError("create veth pair", err)
	}

	err = p.Host.Setup(cfg)
	if err != nil {
		return typedError("set up host", err)
	}

	err = p.Container.Setup(cfg)
	if err != nil {
		return typedError("set up container", err)
	}

	// use args.Netns as the 'handle' for now
	err = p.Store.Add(netConf.Datastore, filepath.Base(args.Netns), cfg.Container.Address.IP.String(), nil)
	if err != nil {
		return typedError("write container metadata", err)
	}

	return types.PrintResult(cfg.AsCNIResult(), netConf.CNIVersion)
}

func (p *CNIPlugin) cmdDel(args *skel.CmdArgs) error {
	var netConf NetConf
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		return err // impossible, skel package asserts JSON is valid
	}

	networkInfo, err := getNetworkInfo(netConf)
	if err != nil {
		return err
	}

	generator := config.IPAMConfigGenerator{}
	ipamConfig := generator.GenerateConfig(networkInfo.Subnet, netConf.Name, netConf.DataDir)
	ipamConfigBytes, _ := json.Marshal(ipamConfig) // untestable

	err = ipam.ExecDel("host-local", ipamConfigBytes)
	if err != nil {
		p.Logger.Error("host-local-ipam", err)
		// continue, keep trying to cleanup
	}

	containerNS, err := ns.GetNS(args.Netns)
	if err != nil {
		p.Logger.Error("open-netns", err)
		return nil // can't do teardown if no netns
	}

	err = p.Container.Teardown(containerNS, args.IfName)
	if err != nil {
		return typedError("teardown failed", err)
	}

	_, err = p.Store.Delete(netConf.Datastore, filepath.Base(args.Netns))
	if err != nil {
		p.Logger.Error("write-container-metadata", err)
	}

	return nil
}
