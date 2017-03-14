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

func setPointToPointAddress(deviceName string, localAddr, peerAddr *net.IPNet) error {
	addr, err := netlink.ParseAddr(localAddr.String())
	if err != nil {
		log.Fatal(err)
	}

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

	containerNS, err := ns.GetNS(args.Netns)
	if err != nil {
		log.Fatal(err)
	}

	creator := &veth.Creator{}

	hostVeth, containerVeth, err := creator.Pair(args.IfName, 1500, currentNS, containerNS)
	if err != nil {
		log.Fatal(err)
	}

	hostVethAddr := &net.IPNet{
		IP:   net.IPv4(169, 254, 0, 1),
		Mask: net.IPv4Mask(255, 255, 255, 255),
	}
	containerVethAddr := &net.IPNet{
		IP:   cniResult.IPs[0].Address.IP,
		Mask: net.IPv4Mask(255, 255, 255, 255),
	}
	err = setPointToPointAddress(hostVeth.Attrs().Name, hostVethAddr, containerVethAddr)
	if err != nil {
		log.Fatal(err)
	}

	err = containerNS.Do(func(_ ns.NetNS) error {
		return setPointToPointAddress(containerVeth.Attrs().Name, containerVethAddr, hostVethAddr)
	})
	if err != nil {
		log.Fatal(err)
	}

	// disable IPv6 on host
	_, err = sysctl.Sysctl(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", hostVeth.Attrs().Name), "1")
	if err != nil {
		log.Fatal(err)
	}

	// disable IPv6 in container
	err = containerNS.Do(func(_ ns.NetNS) error {
		_, err = sysctl.Sysctl(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", containerVeth.Attrs().Name), "1")
		return err
	})
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

	containerNS, err := ns.GetNS(args.Netns)
	if err != nil {
		log.Fatal(err)
	}
	destroyer := &veth.Destroyer{}
	err = destroyer.Destroy(args.IfName, containerNS)
	if err != nil {
		log.Fatal(err)
	}

	return nil
}
