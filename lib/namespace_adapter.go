package lib

import "github.com/containernetworking/cni/pkg/ns"

//go:generate counterfeiter -o fakes/namespaceAdapter.go --fake-name NamespaceAdapter . namespaceAdapter
type namespaceAdapter interface {
	GetNS(string) (ns.NetNS, error)
	GetCurrentNS() (ns.NetNS, error)
}

type NamespaceAdapter struct{}

func (n *NamespaceAdapter) GetNS(path string) (ns.NetNS, error) {
	return ns.GetNS(path)
}

func (n *NamespaceAdapter) GetCurrentNS() (ns.NetNS, error) {
	return ns.GetCurrentNS()
}
