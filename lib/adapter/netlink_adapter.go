package adapter

import (
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
)

type NetlinkAdapter struct{}

func (*NetlinkAdapter) LinkByName(name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

func (*NetlinkAdapter) LinkByIndex(index int) (netlink.Link, error) {
	return netlink.LinkByIndex(index)
}

func (*NetlinkAdapter) ParseAddr(addr string) (*netlink.Addr, error) {
	return netlink.ParseAddr(addr)
}

func (*NetlinkAdapter) AddrAddScopeLink(link netlink.Link, addr *netlink.Addr) error {
	addr.Scope = int(netlink.SCOPE_LINK)
	return netlink.AddrAdd(link, addr)
}

func (*NetlinkAdapter) AddrList(link netlink.Link, family int) ([]netlink.Addr, error) {
	return netlink.AddrList(link, family)
}

func (*NetlinkAdapter) LinkSetHardwareAddr(link netlink.Link, hwaddr net.HardwareAddr) error {
	return netlink.LinkSetHardwareAddr(link, hwaddr)
}

func (*NetlinkAdapter) NeighAddPermanentIPv4(index int, destIP net.IP, hwAddr net.HardwareAddr) error {
	return netlink.NeighAdd(&netlink.Neigh{
		LinkIndex:    index,
		Family:       netlink.FAMILY_V4,
		State:        netlink.NUD_PERMANENT,
		IP:           destIP,
		HardwareAddr: hwAddr,
	})
}

func (*NetlinkAdapter) NeighSet(neigh *netlink.Neigh) error {
	return netlink.NeighSet(neigh)
}

func (*NetlinkAdapter) ARPList(linkIndex int) ([]netlink.Neigh, error) {
	return netlink.NeighList(linkIndex, netlink.FAMILY_V4)
}

func (*NetlinkAdapter) FDBList(linkIndex int) ([]netlink.Neigh, error) {
	return netlink.NeighList(linkIndex, syscall.AF_BRIDGE)
}

func (*NetlinkAdapter) NeighDel(neigh *netlink.Neigh) error {
	return netlink.NeighDel(neigh)
}

func (*NetlinkAdapter) LinkSetARPOff(link netlink.Link) error {
	return netlink.LinkSetARPOff(link)
}

func (*NetlinkAdapter) LinkSetName(link netlink.Link, newName string) error {
	return netlink.LinkSetName(link, newName)
}

func (*NetlinkAdapter) LinkSetUp(link netlink.Link) error {
	return netlink.LinkSetUp(link)
}

func (*NetlinkAdapter) LinkDel(link netlink.Link) error {
	return netlink.LinkDel(link)
}

func (*NetlinkAdapter) LinkAdd(link netlink.Link) error {
	return netlink.LinkAdd(link)
}

func (*NetlinkAdapter) LinkSetNsFd(link netlink.Link, fd int) error {
	return netlink.LinkSetNsFd(link, fd)
}

func (*NetlinkAdapter) RouteAdd(route *netlink.Route) error {
	return netlink.RouteAdd(route)
}

func (*NetlinkAdapter) RouteReplace(route *netlink.Route) error {
	return netlink.RouteReplace(route)
}

func (*NetlinkAdapter) RouteList(link netlink.Link, family int) ([]netlink.Route, error) {
	return netlink.RouteList(link, family)
}

func (*NetlinkAdapter) RouteDel(route *netlink.Route) error {
	return netlink.RouteDel(route)
}

func (*NetlinkAdapter) QdiscAdd(qdisc netlink.Qdisc) error {
	return netlink.QdiscAdd(qdisc)
}
