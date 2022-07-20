package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/validator.v2"

	"code.cloudfoundry.org/cf-networking-helpers/json_client"
	"code.cloudfoundry.org/filelock"
	"code.cloudfoundry.org/lager"

	"code.cloudfoundry.org/silk/cni/adapter"
	"code.cloudfoundry.org/silk/cni/config"
	"code.cloudfoundry.org/silk/cni/lib"
	"code.cloudfoundry.org/silk/cni/netinfo"
	"code.cloudfoundry.org/silk/daemon"
	libAdapter "code.cloudfoundry.org/silk/lib/adapter"
	"code.cloudfoundry.org/silk/lib/datastore"
	"code.cloudfoundry.org/silk/lib/serial"
	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	uuid "github.com/google/uuid"
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

const (
	jobPrefix = "silk-cni"
	logPrefix = "cfnetworking"
)

// used as a compile-time flag to disable logging during integration tests
var LoggingDevice = "vcapLog"

func main() {
	hostNS, err := ns.GetCurrentNS()
	if err != nil {
		log.Fatal(err)
	}

	requestId := uuid.New()
	logger := lager.NewLogger(logPrefix).Session(jobPrefix, lager.Data{"cni-request-id": requestId})

	logWriter := os.Stdout
	if LoggingDevice != "stdout" {
		vcapLog, err := os.OpenFile("/var/vcap/sys/log/silk-cni/silk-cni.stdout.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, os.FileMode(0644))
		if err != nil {
			log.Fatalf("can't open log file: %s", err)
		}
		logWriter = vcapLog
	}
	logLevel := lager.ERROR
	if _, err := os.Stat("/var/vcap/jobs/silk-cni/config/enable_debug"); err == nil {
		logLevel = lager.DEBUG
	}

	inSink := lager.NewPrettySink(logWriter, logLevel)
	sink := lager.NewReconfigurableSink(inSink, logLevel)
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
		Logger:         logger.Session("common-setup"),
	}
	store := &datastore.Store{
		Serializer: &serial.Serial{},
		LockerNew:  filelock.NewLocker,
	}

	plugin := &CNIPlugin{
		HostNSPath: hostNS.Path(),
		HostNS:     hostNS,
		ConfigCreator: &config.ConfigCreator{
			HardwareAddressGenerator: &config.HardwareAddressGenerator{},
			DeviceNameGenerator:      &config.DeviceNameGenerator{},
			NamespaceAdapter:         &adapter.NamespaceAdapter{},
			Logger:                   logger.Session("config-creator"),
		},
		VethPairCreator: &lib.VethPairCreator{
			NetlinkAdapter: netlinkAdapter,
			Logger:         logger.Session("veth-pair-creator"),
		},
		Host: &lib.Host{
			Common:         commonSetup,
			LinkOperations: linkOperations,
			Logger:         logger.Session("host-setup"),
		},
		Container: &lib.Container{
			Common:         commonSetup,
			LinkOperations: linkOperations,
			Logger:         logger.Session("container-setup"),
		},
		Logger: logger,
		Store:  store,
	}

	skel.PluginMain(plugin.cmdAdd, plugin.cmdDel, version.PluginSupports("0.3.1"))
}

type NetConf struct {
	types.NetConf
	DataDir    string `json:"dataDir"`
	SubnetFile string `json:"subnetFile"`
	MTU        int    `json:"mtu" validate:"min=0"`
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

func getNetworkInfo(netConf NetConf) (daemon.NetworkInfo, error) {
	err := validator.Validate(netConf)
	if err != nil {
		return daemon.NetworkInfo{}, fmt.Errorf("invalid config: %s", err)
	}

	discoverer := netinfo.Discoverer{}
	if netConf.SubnetFile != "" {
		discoverer.NetInfo = &netinfo.Flannel{
			SubnetFilePath: netConf.SubnetFile,
		}
	} else {
		jsonClient := json_client.New(lager.NewLogger(""), http.DefaultClient, fmt.Sprintf("http://127.0.0.1:%d", netConf.DaemonPort))
		discoverer.NetInfo = &netinfo.Daemon{
			JSONClient: jsonClient,
		}
	}
	return discoverer.Discover(netConf.MTU)
}

func (p *CNIPlugin) cmdAdd(args *skel.CmdArgs) error {
	p.Logger = p.Logger.Session("plugin-add")

	var netConf NetConf
	p.Logger.Debug("json-unmarshal-stdin-as-netconf")
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		p.Logger.Error("json-unmarshal-stdin-as-netconf-failed", err)
		return err // impossible, skel package asserts JSON is valid
	}

	p.Logger.Debug("getting-network-info", lager.Data{"netConf": netConf})
	networkInfo, err := getNetworkInfo(netConf)
	if err != nil {
		p.Logger.Error("get-network-info-failed", err)
		return typedError("discover network info", err)
	}

	p.Logger.Debug("generate-ipam-config", lager.Data{"overlaySubnet": networkInfo.OverlaySubnet, "name": netConf.Name, "dataDir": netConf.DataDir})
	generator := config.IPAMConfigGenerator{}
	ipamConfig, err := generator.GenerateConfig(networkInfo.OverlaySubnet, netConf.Name, netConf.DataDir)
	if err != nil {
		p.Logger.Error("generate-ipam-config-failed", err)
		return typedError("generate ipam config", err)
	}
	ipamConfigBytes, _ := json.Marshal(ipamConfig) // untestable

	p.Logger.Debug("host-local-ipam", lager.Data{"action": "add", "ipamConfig": string(ipamConfigBytes)})
	result, err := invoke.DelegateAdd("host-local", ipamConfigBytes)
	if err != nil {
		p.Logger.Error("host-local-ipam-failed", err)
		return typedError("run ipam plugin", err)
	}

	p.Logger.Debug("convert-ipam-result", lager.Data{"result": result})
	cniResult, err := current.NewResultFromResult(result)
	if err != nil {
		p.Logger.Error("convert-ipam-result-failed", err)
		return fmt.Errorf("convert result to current CNI version: %s", err) // not tested
	}

	p.Logger.Debug("create-config", lager.Data{"hostNamespace": p.HostNS, "args": args, "result": cniResult, "mtu": networkInfo.MTU})
	cfg, err := p.ConfigCreator.Create(p.HostNS, args, cniResult, networkInfo.MTU)
	if err != nil {
		p.Logger.Error("create-config-failed", err)
		return typedError("create config", err)
	}

	p.Logger.Debug("create-veth-pair", lager.Data{"cfg": cfg})
	err = p.VethPairCreator.Create(cfg)
	if err != nil {
		p.Logger.Error("create-veth-pair-failed", err)
		return typedError("create veth pair", err)
	}

	p.Logger.Debug("setup-host", lager.Data{"cfg": cfg})
	err = p.Host.Setup(cfg)
	if err != nil {
		p.Logger.Error("setup-host-failed", err)
		return typedError("set up host", err)
	}

	p.Logger.Debug("setup-container", lager.Data{"cfg": cfg})
	err = p.Container.Setup(cfg)
	if err != nil {
		p.Logger.Error("setup-container-failed", err)
		return typedError("set up container", err)
	}

	// use args.Netns as the 'handle' for now
	p.Logger.Debug("write-container-metadata", lager.Data{"datastore": netConf.Datastore, "path": filepath.Base(args.Netns), "ip": cfg.Container.Address.IP.String()})
	err = p.Store.Add(netConf.Datastore, filepath.Base(args.Netns), cfg.Container.Address.IP.String(), nil)
	if err != nil {
		p.Logger.Error("write-container-metadata-failed", err)
		return typedError("write container metadata", err)
	}

	p.Logger.Debug("print-cni-result", lager.Data{"cfg": cfg.AsCNIResult(), "cniVersion": netConf.CNIVersion})
	err = types.PrintResult(cfg.AsCNIResult(), netConf.CNIVersion)
	if err != nil {
		p.Logger.Error("print-cni-result-failed", err)
	}
	return err
}

func (p *CNIPlugin) cmdDel(args *skel.CmdArgs) error {
	p.Logger = p.Logger.Session("plugin-del")

	var netConf NetConf
	p.Logger.Debug("json-unmarshal-stdin-as-netconf")
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		p.Logger.Error("json-unmarshal-stdin-as-netconf-failed", err)
		return err // impossible, skel package asserts JSON is valid
	}

	p.Logger.Debug("generate-ipam-config", lager.Data{"name": netConf.Name, "dataDir": netConf.DataDir})
	generator := config.IPAMConfigGenerator{}
	// use 0.0.0.0/0 for the IPAM subnet during delete so we don't need to discover the subnet.
	// this way, silk-daemon does not need to be up during deletes, and cleanup that takes place
	// on startup, after the subnet may have changed, will succeed.
	ipamConfig, err := generator.GenerateConfig("0.0.0.0/0", netConf.Name, netConf.DataDir)
	if err != nil {
		p.Logger.Error("generate-ipam-config-failed", err) // untestable
		// continue, keep trying to cleanup
	}
	ipamConfigBytes, _ := json.Marshal(ipamConfig) // untestable

	p.Logger.Debug("host-local-ipam", lager.Data{"action": "delete", "ipamConfig": string(ipamConfigBytes)})
	err = invoke.DelegateDel("host-local", ipamConfigBytes)
	if err != nil {
		p.Logger.Error("host-local-ipam-failed", err)
		// continue, keep trying to cleanup
	}

	p.Logger.Debug("open-netns", lager.Data{"namespace": args.Netns})
	containerNS, err := ns.GetNS(args.Netns)
	if err != nil {
		p.Logger.Error("open-netns-failed", err)
		return nil // can't do teardown if no netns
	}

	p.Logger.Debug("teardown-container", lager.Data{"namespace": containerNS, "interface": args.IfName})
	err = p.Container.Teardown(containerNS, args.IfName)
	if err != nil {
		p.Logger.Error("teardown-failed", err)
		return typedError("teardown failed", err)
	}

	p.Logger.Debug("delete-from-container-metadata", lager.Data{"datastore": netConf.Datastore, "path": filepath.Base(args.Netns)})
	_, err = p.Store.Delete(netConf.Datastore, filepath.Base(args.Netns))
	if err != nil {
		p.Logger.Error("delete-from-container-metadata-failed", err)
	}

	return nil
}
