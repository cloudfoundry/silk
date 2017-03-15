package veth

import (
	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/ns"
)

//go:generate counterfeiter -o fakes/ipAdapter.go --fake-name IPAdapter . ipAdapter
type ipAdapter interface {
	SetupVeth(string, int, ns.NetNS) (ip.Link, ip.Link, error)
	DelLinkByName(string) error
}

type IPAdapter struct{}

func (n *IPAdapter) SetupVeth(ifname string, mtu int, namespace ns.NetNS) (ip.Link, ip.Link, error) {
	return ip.SetupVeth(ifname, mtu, namespace)
}

func (n *IPAdapter) DelLinkByName(ifname string) error {
	return ip.DelLinkByName(ifname)
}
