package adapter

import (
	"net"

	"github.com/vishvananda/netlink"
)

type NetlinkAdapter struct{}

func (*NetlinkAdapter) LinkByName(name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

func (*NetlinkAdapter) ParseAddr(addr string) (*netlink.Addr, error) {
	return netlink.ParseAddr(addr)
}

func (*NetlinkAdapter) AddrAddScopeLink(link netlink.Link, addr *netlink.Addr) error {
	addr.Scope = int(netlink.SCOPE_LINK)
	return netlink.AddrAdd(link, addr)
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
