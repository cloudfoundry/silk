package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/cloudfoundry-incubator/silk/veth"
	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/utils/sysctl"
	"github.com/containernetworking/cni/pkg/version"
)

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.All)
}

type NetConf struct {
	types.NetConf
}

func disableIPv6(vethPair *veth.Pair) {
	_, err := sysctl.Sysctl(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", vethPair.Host.Link.Attrs().Name), "1")
	if err != nil {
		log.Fatal(err)
	}

	err = vethPair.Container.Namespace.Do(func(_ ns.NetNS) error {
		_, err = sysctl.Sysctl(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", vethPair.Container.Link.Attrs().Name), "1")
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
}

func cmdAdd(args *skel.CmdArgs) error {
	var netConf NetConf
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		log.Fatal(err)
	}
	result, err := ipam.ExecAdd(netConf.IPAM.Type, args.StdinData)
	if err != nil {
		log.Fatal(err)
	}

	cniResult, err := current.NewResultFromResult(result)
	if err != nil {
		log.Fatal(err)
	}

	vethManager, err := veth.NewManager(args.Netns)
	if err != nil {
		log.Fatal(err)
	}

	vethPair, err := vethManager.CreatePair(args.IfName, 1500)
	if err != nil {
		log.Fatal(err)
	}

	vethManager.AssignIP(vethPair, cniResult.IPs[0].Address.IP)

	// vethPair.DisableIPv6()
	disableIPv6(vethPair)

	cniResult.Interfaces = append(cniResult.Interfaces,
		&current.Interface{
			Name: vethPair.Host.Link.Attrs().Name,
			Mac:  vethPair.Host.Link.Attrs().HardwareAddr.String(),
		},
		&current.Interface{
			Name:    vethPair.Container.Link.Attrs().Name,
			Mac:     vethPair.Container.Link.Attrs().HardwareAddr.String(),
			Sandbox: args.Netns,
		},
	)

	cniResult.IPs[0].Interface = -1

	return types.PrintResult(cniResult, netConf.CNIVersion)

}

func cmdDel(args *skel.CmdArgs) error {
	var netConf NetConf
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		log.Fatal(err)
	}

	err = ipam.ExecDel(netConf.IPAM.Type, args.StdinData)
	if err != nil {
		log.Fatal(err)
	}

	vethManager, err := veth.NewManager(args.Netns)
	if err != nil {
		log.Fatal(err)
	}
	err = vethManager.Destroy(args.IfName)
	if err != nil {
		log.Fatal(err)
	}

	return nil
}
