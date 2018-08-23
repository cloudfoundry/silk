package vtep

import (
	"fmt"
	"net"
	"syscall"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
	"github.com/vishvananda/netlink"
)

type Converger struct {
	OverlayNetwork   *net.IPNet
	LocalSubnet      *net.IPNet
	LocalVTEP        net.Interface
	NetlinkAdapter   netlinkAdapter
	Logger           lager.Logger
	underlayAdresses map[string]net.IP
}

func (c *Converger) Converge(leases []controller.Lease) error {
	previousRoutes, previousNeighs, err := c.getPreviousState(c.LocalVTEP.Index)
	if err != nil {
		return err
	}

	nonRoutableLeaseCount := 0
	var currentRoutes []netlink.Route
	var currentNeighs []netlink.Neigh
	for _, lease := range leases {
		destAddr, destNet, err := net.ParseCIDR(lease.OverlaySubnet)
		if err != nil {
			return fmt.Errorf("parse lease: %s", err)
		}

		if c.isLocal(destNet) {
			continue
		}

		if !c.OverlayNetwork.Contains(destNet.IP) {
			nonRoutableLeaseCount++
			continue
		}

		route, err := c.addRoute(destNet, destAddr)
		if err != nil {
			return err
		}
		currentRoutes = append(currentRoutes, route)

		underlayIP := net.ParseIP(lease.UnderlayIP)
		if underlayIP == nil {
			return fmt.Errorf("invalid underlay ip: %s", lease.UnderlayIP)
		}

		remoteMac, err := net.ParseMAC(lease.OverlayHardwareAddr)
		if err != nil {
			return fmt.Errorf("invalid hardware addr: %s", lease.OverlayHardwareAddr)
		}

		neighs, err := c.addNeighs(underlayIP, destAddr, remoteMac)
		if err != nil {
			return err
		}
		currentNeighs = append(currentNeighs, neighs...)
	}

	routesForDeletion := getDeletedRoutes(previousRoutes, currentRoutes)
	for _, route := range routesForDeletion {
		if route.LinkIndex == c.LocalVTEP.Index && c.OverlayNetwork.Contains(route.Gw) {
			err = c.NetlinkAdapter.RouteDel(&route)
			if err != nil {
				return fmt.Errorf("del route: %s", err)
			}
		}
	}

	neighsForDeletion := getDeletedNeighs(previousNeighs, currentNeighs)
	c.Logger.Debug("converger", lager.Data{"neighsForDeletion count": len(neighsForDeletion)})

	for _, neigh := range neighsForDeletion {
		if neigh.LinkIndex == c.LocalVTEP.Index {
			err = c.NetlinkAdapter.NeighDel(&neigh)
			if err != nil {
				return fmt.Errorf("del neigh with ip/hwaddr %s: %s", &neigh, err)
			}
		}
	}

	if nonRoutableLeaseCount > 0 {
		c.Logger.Info("converger", lager.Data{"non-routable-lease-count": nonRoutableLeaseCount})
	}

	return nil
}

func (c *Converger) isLocal(destNet *net.IPNet) bool {
	return destNet.String() == c.LocalSubnet.String()
}

func getDeletedRoutes(previous, current []netlink.Route) []netlink.Route {
	var deletedRoutes []netlink.Route
	for _, previousRoute := range previous {
		isRemoved := true
		for _, currentRoute := range current {
			isRemoved = isRemoved && !routeEqual(previousRoute, currentRoute)
		}

		if isRemoved {
			deletedRoutes = append(deletedRoutes, previousRoute)
		}
	}

	return deletedRoutes

}

func getDeletedNeighs(previous, current []netlink.Neigh) []netlink.Neigh {
	var deletedNeighs []netlink.Neigh
	for _, previousNeigh := range previous {
		isRemoved := true
		for _, currentNeigh := range current {
			isRemoved = isRemoved && !neighEqual(previousNeigh, currentNeigh)
		}

		if isRemoved {
			deletedNeighs = append(deletedNeighs, previousNeigh)
		}
	}

	return deletedNeighs
}

func (c *Converger) getPreviousState(index int) ([]netlink.Route, []netlink.Neigh, error) {
	link, err := c.NetlinkAdapter.LinkByIndex(c.LocalVTEP.Index)
	if err != nil {
		return nil, nil, fmt.Errorf("link by index: %s", err)
	}

	previousRoutes, err := c.NetlinkAdapter.RouteList(link, netlink.FAMILY_V4)
	if err != nil {
		return nil, nil, fmt.Errorf("list routes: %s", err)
	}

	previousFDBNeighs, err := c.NetlinkAdapter.FDBList(c.LocalVTEP.Index)
	if err != nil {
		return nil, nil, fmt.Errorf("list fdb: %s", err)
	}

	previousARPNeighs, err := c.NetlinkAdapter.ARPList(c.LocalVTEP.Index)
	if err != nil {
		return nil, nil, fmt.Errorf("list arp: %s", err)
	}

	/*
		on trusty, FDB entries are found with the `FDBList` command above
		on xenial, the FDB entries are not found.
		As a workaround, generate the state of the fdb based on known data
	*/
	if len(previousFDBNeighs) != len(previousARPNeighs) {
		for _, previousARPNeigh := range previousARPNeighs {
			if underlayIP, ok := c.underlayAdresses[previousARPNeigh.HardwareAddr.String()]; ok {
				previousFDBNeighs = append(previousFDBNeighs, netlink.Neigh{
					LinkIndex:    previousARPNeigh.LinkIndex,
					State:        previousARPNeigh.State,
					Family:       syscall.AF_BRIDGE,
					Flags:        netlink.NTF_SELF,
					IP:           underlayIP,
					HardwareAddr: previousARPNeigh.HardwareAddr,
				})
			} else {
				c.Logger.Info("failedFindFDBNeighbor", lager.Data{
					"hardware-addr": previousARPNeigh.HardwareAddr.String(),
					"error-msg":     "unable to resolve the underlayIP using the arp entry hardware address",
				})
			}
		}
	}

	previousNeighs := append(previousARPNeighs, previousFDBNeighs...)

	return previousRoutes, previousNeighs, nil
}

func (c *Converger) addRoute(destNet *net.IPNet, destAddr net.IP) (netlink.Route, error) {
	route := netlink.Route{
		LinkIndex: c.LocalVTEP.Index,
		Scope:     netlink.SCOPE_UNIVERSE,
		Dst:       destNet,
		Gw:        destAddr,
		Src:       c.LocalSubnet.IP,
	}

	err := c.NetlinkAdapter.RouteReplace(&route)
	if err != nil {
		return netlink.Route{}, fmt.Errorf("add route: %s", err)
	}

	return route, nil
}

func (c *Converger) addNeighs(underlayIP, destAddr net.IP, remoteMac net.HardwareAddr) ([]netlink.Neigh, error) {
	neighs := []*netlink.Neigh{
		{ // ARP
			LinkIndex:    c.LocalVTEP.Index,
			State:        netlink.NUD_PERMANENT,
			Type:         syscall.RTN_UNICAST,
			IP:           destAddr,
			HardwareAddr: remoteMac,
		},
		{ // FDB
			LinkIndex:    c.LocalVTEP.Index,
			State:        netlink.NUD_PERMANENT,
			Family:       syscall.AF_BRIDGE,
			Flags:        netlink.NTF_SELF,
			IP:           underlayIP,
			HardwareAddr: remoteMac,
		},
	}

	c.underlayAdresses[remoteMac.String()] = underlayIP

	var currentNeighs []netlink.Neigh
	for _, neigh := range neighs {
		err := c.NetlinkAdapter.NeighSet(neigh)
		if err != nil {
			return nil, fmt.Errorf("set neigh: %s", err)
		}

		currentNeighs = append(currentNeighs, *neigh)
	}

	return currentNeighs, nil
}

func routeEqual(r1, r2 netlink.Route) bool {
	return r1.LinkIndex == r2.LinkIndex &&
		r1.Scope == r2.Scope &&
		r1.Dst.String() == r2.Dst.String() &&
		r1.Gw.String() == r2.Gw.String() &&
		r1.Src.String() == r2.Src.String()
}

func neighEqual(n1, n2 netlink.Neigh) bool {
	return n1.LinkIndex == n2.LinkIndex &&
		n1.State == n2.State &&
		n1.IP.String() == n2.IP.String() &&
		n1.HardwareAddr.String() == n2.HardwareAddr.String()
}
