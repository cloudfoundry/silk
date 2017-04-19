package vtep

import (
	"fmt"
	"net"

	"code.cloudfoundry.org/silk/controller"
	"github.com/vishvananda/netlink"
)

type Converger struct {
	LocalSubnet    *net.IPNet
	LocalVTEP      net.Interface
	NetlinkAdapter netlinkAdapter
}

func (c *Converger) Converge(leases []controller.Lease) error {
	for _, lease := range leases {
		destAddr, destNet, err := net.ParseCIDR(lease.OverlaySubnet)
		if err != nil {
			return fmt.Errorf("parsing lease: %s", err)
		}

		if c.isLocal(destNet) {
			continue
		}

		err = c.NetlinkAdapter.RouteAdd(&netlink.Route{
			LinkIndex: c.LocalVTEP.Index,
			Scope:     netlink.SCOPE_UNIVERSE,
			Dst:       destNet,
			Gw:        destAddr,
			Src:       c.LocalSubnet.IP,
		})
		if err != nil {
			return fmt.Errorf("adding route: %s", err)
		}
	}
	return nil
}

func (c *Converger) isLocal(destNet *net.IPNet) bool {
	return destNet.String() == c.LocalSubnet.String()
}
