package veth

import (
	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/ns"
)

type Creator struct{}

func (c *Creator) Pair(ifname string, mtu int, hostNSPath, containerNSPath string) error {
	hostNS, err := ns.GetNS(hostNSPath)
	if err != nil {
		panic(err)
	}

	containerNS, err := ns.GetNS(containerNSPath)
	if err != nil {
		panic(err)
	}

	err = containerNS.Do(func(_ ns.NetNS) error {
		_, _, err = ip.SetupVeth(ifname, mtu, hostNS)
		if err != nil {
			panic(err)
		}

		return nil
	})
	if err != nil {
		panic(err)
	}

	return nil
}
