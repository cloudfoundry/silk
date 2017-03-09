package veth

import (
	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/ns"
)

type Creator struct{}

func (c *Creator) Pair(ifname string, mtu int, hostNSPath string) error {
	netns, err := ns.GetNS(hostNSPath)
	if err != nil {
		panic(err)
	}

	_, _, err = ip.SetupVeth(ifname, mtu, netns)
	if err != nil {
		panic(err)
	}

	return nil
}
