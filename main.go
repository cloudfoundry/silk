package main

import (
	"encoding/json"
	"net"

	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/vishvananda/netlink"
)

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.All)
}

type NetConf struct {
	types.NetConf
}

func cmdAdd(args *skel.CmdArgs) error {
	var netConf NetConf
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		panic(err)
	}
	result, err := ipam.ExecAdd(netConf.IPAM.Type, args.StdinData)
	if err != nil {
		panic(err)
	}

	veth := netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name:  "silk-veth",
			Flags: net.FlagUp,
			MTU:   1500,
		},
		PeerName: "silk-veth-peer",
	}
	err = netlink.LinkAdd(&veth)
	if err != nil {
		panic(err)
	}

	cniResult, err := current.NewResultFromResult(result)
	if err != nil {
		panic(err)
	}

	cniResult.IPs[0].Interface = -1

	return types.PrintResult(cniResult, netConf.CNIVersion)

}

func cmdDel(args *skel.CmdArgs) error {
	var netConf NetConf
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		panic(err)
	}
	err = ipam.ExecDel(netConf.IPAM.Type, args.StdinData)
	if err != nil {
		panic(err)
	}
	return nil
}
