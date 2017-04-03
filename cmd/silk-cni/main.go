package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"code.cloudfoundry.org/lager"

	"code.cloudfoundry.org/silk/cni/adapter"
	"code.cloudfoundry.org/silk/cni/config"
	"code.cloudfoundry.org/silk/cni/legacy_flannel"
	"code.cloudfoundry.org/silk/cni/lib"
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

	netlinkAdapter := &adapter.NetlinkAdapter{}
	linkOperations := &lib.LinkOperations{
		SysctlAdapter:  &adapter.SysctlAdapter{},
		NetlinkAdapter: netlinkAdapter,
		Logger:         logger,
	}
	commonSetup := &lib.Common{
		NetlinkAdapter: netlinkAdapter,
		LinkOperations: linkOperations,
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
	}

	skel.PluginMain(plugin.cmdAdd, plugin.cmdDel, version.PluginSupports("0.3.0"))
}

type NetConf struct {
	types.NetConf
	DataDir    string `json:"dataDir"`
	SubnetFile string `json:"subnetFile"`
	MTU        int    `json:"mtu"`
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

func (p *CNIPlugin) cmdAdd(args *skel.CmdArgs) error {
	var netConf NetConf
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		return err // impossible, skel package asserts JSON is valid
	}

	networkInfo, err := legacy_flannel.DiscoverNetworkInfo(netConf.SubnetFile, netConf.MTU)
	if err != nil {
		return typedError("discovering network info", err)
	}

	generator := config.IPAMConfigGenerator{}
	ipamConfig := generator.GenerateConfig(networkInfo.Subnet, netConf.Name, netConf.DataDir)
	ipamConfigBytes, _ := json.Marshal(ipamConfig) // untestable

	result, err := ipam.ExecAdd("host-local", ipamConfigBytes)
	if err != nil {
		return typedError("ipam plugin failed", err)
	}

	cniResult, err := current.NewResultFromResult(result)
	if err != nil {
		return fmt.Errorf("unable to convert result to current CNI version: %s", err) // not tested
	}

	cfg, err := p.ConfigCreator.Create(p.HostNS, args, cniResult, networkInfo.MTU)
	if err != nil {
		return typedError("creating config", err)
	}

	err = p.VethPairCreator.Create(cfg)
	if err != nil {
		return typedError("creating veth pair", err)
	}

	err = p.Host.Setup(cfg)
	if err != nil {
		return typedError("setting up host", err)
	}

	err = p.Container.Setup(cfg)
	if err != nil {
		return typedError("setting up container", err)
	}

	return types.PrintResult(cfg.AsCNIResult(), netConf.CNIVersion)
}

func (p *CNIPlugin) cmdDel(args *skel.CmdArgs) error {
	var netConf NetConf
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		return err // impossible, skel package asserts JSON is valid
	}

	networkInfo, err := legacy_flannel.DiscoverNetworkInfo(netConf.SubnetFile, netConf.MTU)
	if err != nil {
		return typedError("discovering network info", err)
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
		p.Logger.Error("opening-netns", err)
		return nil // can't do teardown if no netns
	}

	err = p.Container.Teardown(containerNS, args.IfName)
	if err != nil {
		return typedError("teardown failed", err)
	}

	return nil
}
