package veth

import (
	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/ns"
)

type Creator struct{}

func (c *Creator) Pair(ifname string, mtu int, hostNS, containerNS ns.NetNS) (ip.Link, ip.Link, error) {
	var err error
	var hostVeth, containerVeth ip.Link
	err = containerNS.Do(func(_ ns.NetNS) error {
		hostVeth, containerVeth, err = ip.SetupVeth(ifname, mtu, hostNS)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return hostVeth, containerVeth, nil
}

type Destroyer struct{}

func (d *Destroyer) Destroy(ifname string, containerNS ns.NetNS) error {
	err := containerNS.Do(func(_ ns.NetNS) error {
		err := ip.DelLinkByName(ifname)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
