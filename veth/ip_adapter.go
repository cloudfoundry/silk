package veth

import (
	"net"

	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/ns"
)

//go:generate counterfeiter -o fakes/ipAdapter.go --fake-name IPAdapter . ipAdapter
type ipAdapter interface {
	SetupVeth(string, int, ns.NetNS) (net.Interface, net.Interface, error)
	DelLinkByName(string) error
}

type IPAdapter struct{}

func (*IPAdapter) SetupVeth(ifname string, mtu int, namespace ns.NetNS) (net.Interface, net.Interface, error) {
	return ip.SetupVeth(ifname, mtu, namespace)
}

func (*IPAdapter) DelLinkByName(ifname string) error {
	return ip.DelLinkByName(ifname)
}
