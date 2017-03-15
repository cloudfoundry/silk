package veth

import (
	"fmt"
	"net"

	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/utils/sysctl"
	"github.com/vishvananda/netlink"
)

type Manager struct {
	HostNS           ns.NetNS
	ContainerNS      ns.NetNS
	ContainerNSPath  string
	HostNSPath       string
	NetlinkAdapter   netlinkAdapter
	NamespaceAdapter namespaceAdapter
}

type Pair struct {
	Host      Peer
	Container Peer
}

type Peer struct {
	Link      ip.Link
	Namespace ns.NetNS
}

func (m *Manager) Init() error {
	hostNS, err := m.NamespaceAdapter.GetNS(m.HostNSPath)
	if err != nil {
		return fmt.Errorf("Getting host namespace: %s", err)
	}

	containerNS, err := m.NamespaceAdapter.GetNS(m.ContainerNSPath)
	if err != nil {
		return fmt.Errorf("Getting container namespace: %s", err)
	}

	m.HostNS = hostNS
	m.ContainerNS = containerNS
	return nil
}

func (m *Manager) CreatePair(ifname string, mtu int) (*Pair, error) {
	var err error
	var hostVeth, containerVeth ip.Link
	err = m.ContainerNS.Do(func(_ ns.NetNS) error {
		hostVeth, containerVeth, err = ip.SetupVeth(ifname, mtu, m.HostNS)
		if err != nil {
			return err // not tested
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
			return err // not tested
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (m *Manager) DisableIPv6(vethPair *Pair) error {
	err := vethPair.Host.Namespace.Do(func(_ ns.NetNS) error {
		_, err := sysctl.Sysctl(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", vethPair.Host.Link.Attrs().Name), "1")
		return err
	})
	if err != nil {
		panic(err) // not tested
	}

	err = vethPair.Container.Namespace.Do(func(_ ns.NetNS) error {
		_, err := sysctl.Sysctl(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", vethPair.Container.Link.Attrs().Name), "1")
		return err
	})
	if err != nil {
		panic(err) // not tested
	}

	return nil
}

func (m *Manager) AssignIP(vethPair *Pair, containerIP net.IP) error {
	hostIP := net.IPv4(169, 254, 0, 1)
	err := vethPair.Host.Namespace.Do(func(_ ns.NetNS) error {
		return m.setPointToPointAddress(vethPair.Host.Link.Attrs().Name, hostIP, containerIP)
	})
	if err != nil {
		return err
	}

	err = vethPair.Container.Namespace.Do(func(_ ns.NetNS) error {
		return m.setPointToPointAddress(vethPair.Container.Link.Attrs().Name, containerIP, hostIP)
	})
	if err != nil {
		return err
	}
	return nil
}

func (m *Manager) setPointToPointAddress(deviceName string, localIP, peerIP net.IP) error {
	localAddr := &net.IPNet{
		IP:   localIP,
		Mask: net.IPv4Mask(255, 255, 255, 255),
	}
	peerAddr := &net.IPNet{
		IP:   peerIP,
		Mask: net.IPv4Mask(255, 255, 255, 255),
	}

	addr, err := m.NetlinkAdapter.ParseAddr(localAddr.String())
	if err != nil {
		return fmt.Errorf("parsing address %s: %s", localAddr, err)
	}

	addr.Scope = int(netlink.SCOPE_LINK)
	addr.Peer = peerAddr

	link, err := m.NetlinkAdapter.LinkByName(deviceName)
	if err != nil {
		return fmt.Errorf("find link by name %s: %s", deviceName, err)
	}

	if err = m.NetlinkAdapter.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("adding address %s: %s", localAddr, err)
	}
	return nil
}
