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
	link, err := c.NetlinkAdapter.LinkByIndex(c.LocalVTEP.Index)
	if err != nil {
		panic(err)
	}

	previousRoutes, err := c.NetlinkAdapter.RouteList(link, 0)
	if err != nil {
		panic(err)
	}

	previousNeighs, err := c.NetlinkAdapter.NeighList(c.LocalVTEP.Index, 0)
	if err != nil {
		panic(err)
	}

	var currentRoutes []netlink.Route
	var currentNeighs []netlink.Neigh
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

		route := netlink.Route{
			LinkIndex: c.LocalVTEP.Index,
			Scope:     netlink.SCOPE_UNIVERSE,
			Dst:       destNet,
			Gw:        destAddr,
			Src:       c.LocalSubnet.IP,
		}
		err = c.NetlinkAdapter.RouteReplace(&route)
		if err != nil {
			return fmt.Errorf("add route: %s", err)
		}

		currentRoutes = append(currentRoutes, route)

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

			currentNeighs = append(currentNeighs, *neigh)
		}
	}

	routesForDeletion := getDeletedRoutes(previousRoutes, currentRoutes)
	for _, route := range routesForDeletion {
		if route.LinkIndex == c.LocalVTEP.Index && route.Protocol != 2 {
			fmt.Printf("route flags are : %+v/n/n", route.ListFlags())
			err = c.NetlinkAdapter.RouteDel(&route)
			if err != nil {
				panic(err)
			}
		}
	}

	neighsForDeletion := getDeletedNeighs(previousNeighs, currentNeighs)
	for _, neigh := range neighsForDeletion {
		if neigh.LinkIndex == c.LocalVTEP.Index {
			err = c.NetlinkAdapter.NeighDel(&neigh)
			if err != nil {
				panic(err)
			}
		}
	}

	return nil
}

func (c *Converger) isLocal(destNet *net.IPNet) bool {
	return destNet.String() == c.LocalSubnet.String()
}

func getDeletedRoutes(previous, current []netlink.Route) []netlink.Route {
	var deletedRoutes []netlink.Route
	isRemoved := true
	for _, previousRoute := range previous {
		for _, currentRoute := range current {
			if previousRoute.String() == currentRoute.String() {
				isRemoved = false
			}
		}

		if isRemoved {
			deletedRoutes = append(deletedRoutes, previousRoute)
		}

		isRemoved = true
	}

	return deletedRoutes

}

func getDeletedNeighs(previous, current []netlink.Neigh) []netlink.Neigh {
	var deletedNeighs []netlink.Neigh
	isRemoved := true
	for _, previousNeigh := range previous {
		for _, currentNeigh := range current {
			if previousNeigh.String() == currentNeigh.String() {
				isRemoved = false
			}
		}

		if isRemoved {
			deletedNeighs = append(deletedNeighs, previousNeigh)
		}

		isRemoved = true

	}

	return deletedNeighs
}
