package veth

import (
	"fmt"

	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/ns"
)

type Manager struct {
	HostNS      ns.NetNS
	ContainerNS ns.NetNS
}

type Pair struct {
	Host      Peer
	Container Peer
}

type Peer struct {
	Link      ip.Link
	Namespace ns.NetNS
}

func NewManager(containerNSPath string) (*Manager, error) {
	hostNS, err := ns.GetCurrentNS()
	if err != nil {
		panic(err) // not tested
	}

	containerNS, err := ns.GetNS(containerNSPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to create veth manager: %s", err)
	}

	return &Manager{
		HostNS:      hostNS,
		ContainerNS: containerNS,
	}, nil
}

func (m *Manager) CreatePair(ifname string, mtu int) (*Pair, error) {
	var err error
	var hostVeth, containerVeth ip.Link
	err = m.ContainerNS.Do(func(_ ns.NetNS) error {
		hostVeth, containerVeth, err = ip.SetupVeth(ifname, mtu, m.HostNS)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Pair{
		Host: Peer{
			Link:      hostVeth,
			Namespace: m.HostNS,
		},
		Container: Peer{
			Link:      containerVeth,
			Namespace: m.ContainerNS,
		},
	}, nil
}

func (m *Manager) Destroy(ifname string) error {
	err := m.ContainerNS.Do(func(_ ns.NetNS) error {
		err := ip.DelLinkByName(ifname)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
