package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"

	"github.com/cloudfoundry-incubator/silk/veth"
	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/utils/sysctl"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/vishvananda/netlink"
)

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.All)
}

type NetConf struct {
	types.NetConf
}

func setPointToPointAddress(deviceName string, localAddr *net.IPNet, peerAddr *net.IPNet) error {
	addr := &netlink.Addr{IPNet: localAddr}
	addr.Scope = int(netlink.SCOPE_LINK)
	addr.Peer = peerAddr

	link, err := netlink.LinkByName(deviceName)
	if err != nil {
		return fmt.Errorf("find link by name: %s", err)
	}

	if err = netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("adding address: %s", err)
	}
	return nil
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

	currentNS, err := ns.GetCurrentNS()
	if err != nil {
		log.Fatal(err)
	}

	creator := &veth.Creator{}

	hostVeth, containerVeth, err := creator.Pair(args.IfName, 1500, currentNS.Path(), args.Netns)
	if err != nil {
		log.Fatal(err)
	}

	hostVethAddr := &net.IPNet{
		IP:   net.ParseIP("169.254.0.1"),
		Mask: net.CIDRMask(32, 32),
	}
	if err != nil {
		log.Fatal(err)
	}
	err = setPointToPointAddress(hostVeth.Attrs().Name, hostVethAddr, &cniResult.IPs[0].Address)
	if err != nil {
		log.Fatal(err)
	}

	containerNS, err := ns.GetNS(args.Netns)
	if err != nil {
		log.Fatal(err)
	}

	// this is untested right now
	err = containerNS.Do(func(_ ns.NetNS) error {
		return setPointToPointAddress(containerVeth.Attrs().Name, &cniResult.IPs[0].Address, hostVethAddr)
	})
	if err != nil {
		log.Fatal(err)
	}

	// disable IPv6 on host
	_, err = sysctl.Sysctl(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", hostVeth.Attrs().Name), "1")
	if err != nil {
		log.Fatal(err)
	}

	cniResult.Interfaces = append(cniResult.Interfaces,
		&current.Interface{
			Name: hostVeth.Attrs().Name,
			Mac:  hostVeth.Attrs().HardwareAddr.String(),
		},
		&current.Interface{
			Name:    containerVeth.Attrs().Name,
			Mac:     containerVeth.Attrs().HardwareAddr.String(),
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

	destroyer := &veth.Destroyer{}
	err = destroyer.Destroy(args.IfName, args.Netns)
	if err != nil {
		log.Fatal(err)
	}

	return nil
}
