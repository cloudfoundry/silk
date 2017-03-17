package veth

import (
	"net"

	"github.com/vishvananda/netlink"
)

//go:generate counterfeiter -o fakes/netlinkAdapter.go --fake-name NetlinkAdapter . netlinkAdapter
type netlinkAdapter interface {
	LinkByName(string) (netlink.Link, error)
	ParseAddr(string) (*netlink.Addr, error)
	AddrAdd(netlink.Link, *netlink.Addr) error
	LinkSetHardwareAddr(netlink.Link, net.HardwareAddr) error
}

type NetlinkAdapter struct{}

func (*NetlinkAdapter) LinkByName(name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

func (*NetlinkAdapter) ParseAddr(addr string) (*netlink.Addr, error) {
	return netlink.ParseAddr(addr)
}

func (*NetlinkAdapter) AddrAdd(link netlink.Link, addr *netlink.Addr) error {
	return netlink.AddrAdd(link, addr)
}

func (*NetlinkAdapter) LinkSetHardwareAddr(link netlink.Link, hwaddr net.HardwareAddr) error {
	return netlink.LinkSetHardwareAddr(link, hwaddr)
}
