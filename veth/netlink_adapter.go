package veth

import "github.com/vishvananda/netlink"

//go:generate counterfeiter -o fakes/netlinkAdapter.go --fake-name NetlinkAdapter . netlinkAdapter
type netlinkAdapter interface {
	LinkByName(string) (netlink.Link, error)
	ParseAddr(string) (*netlink.Addr, error)
	AddrAdd(netlink.Link, *netlink.Addr) error
}

type NetlinkAdapter struct{}

func (n *NetlinkAdapter) LinkByName(name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

func (n *NetlinkAdapter) ParseAddr(addr string) (*netlink.Addr, error) {
	return netlink.ParseAddr(addr)
}

func (n *NetlinkAdapter) AddrAdd(link netlink.Link, addr *netlink.Addr) error {
	return netlink.AddrAdd(link, addr)
}
