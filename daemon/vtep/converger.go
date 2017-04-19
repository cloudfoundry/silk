package vtep

import (
	"fmt"
	"net"
	"syscall"

	"code.cloudfoundry.org/silk/controller"
	"github.com/containernetworking/cni/pkg/utils/hwaddr"
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
			return fmt.Errorf("parse lease: %s", err)
		}

		remoteUnderlayIP := net.ParseIP(lease.UnderlayIP)
		if remoteUnderlayIP == nil {
			return fmt.Errorf("%s is not a valid ip", lease.UnderlayIP)
		}

		if c.isLocal(destNet) {
			continue
		}

		err = c.NetlinkAdapter.RouteReplace(&netlink.Route{
			LinkIndex: c.LocalVTEP.Index,
			Scope:     netlink.SCOPE_UNIVERSE,
			Dst:       destNet,
			Gw:        destAddr,
			Src:       c.LocalSubnet.IP,
		})
		if err != nil {
			return fmt.Errorf("add route: %s", err)
		}

		remoteVTEPMac, err := hwaddr.GenerateHardwareAddr4(destAddr, []byte{0xee, 0xee})
		if err != nil {
			return fmt.Errorf("generate remote vtep mac: %s", err) // untested, should be impossible
		}

		neighs := []*netlink.Neigh{
			{ // ARP
				LinkIndex:    c.LocalVTEP.Index,
				State:        netlink.NUD_PERMANENT,
				Type:         syscall.RTN_UNICAST,
				IP:           destAddr,
				HardwareAddr: remoteVTEPMac,
			},
			{ // FDB
				LinkIndex:    c.LocalVTEP.Index,
				State:        netlink.NUD_PERMANENT,
				Family:       syscall.AF_BRIDGE,
				Flags:        netlink.NTF_SELF,
				IP:           remoteUnderlayIP,
				HardwareAddr: remoteVTEPMac,
			},
		}
		for _, neigh := range neighs {
			err = c.NetlinkAdapter.NeighSet(neigh)
			if err != nil {
				return fmt.Errorf("set neigh: %s", err)
			}
		}
	}
	return nil
}

func (c *Converger) isLocal(destNet *net.IPNet) bool {
	return destNet.String() == c.LocalSubnet.String()
}
