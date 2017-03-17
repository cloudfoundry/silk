package veth

import (
	"fmt"
	"log"
	"net"

	"github.com/containernetworking/cni/pkg/ns"
	"github.com/vishvananda/netlink"
)

type Manager struct {
	HostNS           ns.NetNS
	ContainerNS      ns.NetNS
	ContainerNSPath  string
	HostNSPath       string
	IPAdapter        ipAdapter
	NetlinkAdapter   netlinkAdapter
	HWAddrAdapter    hwAddrAdapter
	NamespaceAdapter namespaceAdapter
	SysctlAdapter    sysctlAdapter
}

type Pair struct {
	Host      Peer
	Container Peer
}

type Peer struct {
	Link      net.Interface
	Namespace ns.NetNS
}

func NewManager(hostNSPath, containerNSPath string) *Manager {
	vethManager := &Manager{
		HostNSPath:       hostNSPath,
		ContainerNSPath:  containerNSPath,
		NamespaceAdapter: &NamespaceAdapter{},
		NetlinkAdapter:   &NetlinkAdapter{},
		IPAdapter:        &IPAdapter{},
		HWAddrAdapter:    &HWAddrAdapter{},
		SysctlAdapter:    &SysctlAdapter{},
	}
	err := vethManager.Init()
	if err != nil {
		log.Fatal(err)
	}

	return vethManager
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
	var hostVeth, containerVeth net.Interface
	err = m.ContainerNS.Do(func(_ ns.NetNS) error {
		hostVeth, containerVeth, err = m.IPAdapter.SetupVeth(ifname, mtu, m.HostNS)
		if err != nil {
			return fmt.Errorf("Setting up veth: %s", err)
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
		err := m.IPAdapter.DelLinkByName(ifname)
		if err != nil {
			return fmt.Errorf("Deleting link: %s", err)
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
		_, err := m.SysctlAdapter.Sysctl(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", vethPair.Host.Link.Name), "1")
		return err
	})
	if err != nil {
		return fmt.Errorf("Disabling IPv6 on host: %s", err)
	}

	err = vethPair.Container.Namespace.Do(func(_ ns.NetNS) error {
		_, err := m.SysctlAdapter.Sysctl(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", vethPair.Container.Link.Name), "1")
		return err
	})
	if err != nil {
		return fmt.Errorf("Disabling IPv6 in container: %s", err)
	}

	return nil
}

func (m *Manager) AssignIP(vethPair *Pair, containerIP net.IP) error {
	hostHardwareAddr, err := m.HWAddrAdapter.GenerateHardwareAddr4(containerIP, []byte{0xaa, 0xaa})
	if err != nil {
		return fmt.Errorf("generating MAC address for host: %s", err)
	}
	containerHardwareAddr, err := m.HWAddrAdapter.GenerateHardwareAddr4(containerIP, []byte{0xee, 0xee})
	if err != nil {
		return fmt.Errorf("generating MAC address for container: %s", err)
	}
	hostIP := net.IPv4(169, 254, 0, 1)

	err = vethPair.Host.Namespace.Do(func(_ ns.NetNS) error {
		link, err := m.setPointToPointAddress(vethPair.Host.Link.Name, hostIP, containerIP, hostHardwareAddr)
		if err != nil {
			return err
		}
		vethPair.Host.Link = link
		return nil
	})
	if err != nil {
		return err
	}

	err = vethPair.Container.Namespace.Do(func(_ ns.NetNS) error {
		link, err := m.setPointToPointAddress(vethPair.Container.Link.Name, containerIP, hostIP, containerHardwareAddr)
		if err != nil {
			return err
		}
		vethPair.Container.Link = link
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (m *Manager) setPointToPointAddress(deviceName string, localIP, peerIP net.IP, hardwareAddr net.HardwareAddr) (net.Interface, error) {
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
		return net.Interface{}, fmt.Errorf("parsing address %s: %s", localAddr, err)
	}

	addr.Scope = int(netlink.SCOPE_LINK)
	addr.Peer = peerAddr

	link, err := m.NetlinkAdapter.LinkByName(deviceName)
	if err != nil {
		return net.Interface{}, fmt.Errorf("find link by name %s: %s", deviceName, err)
	}

	err = m.NetlinkAdapter.AddrAdd(link, addr)
	if err != nil {
		return net.Interface{}, fmt.Errorf("adding IP address %s: %s", localAddr, err)
	}

	err = m.NetlinkAdapter.LinkSetHardwareAddr(link, hardwareAddr)
	if err != nil {
		return net.Interface{}, fmt.Errorf("adding MAC address %s: %s", hardwareAddr, err)
	}

	nlLink, err := m.NetlinkAdapter.LinkByName(deviceName)
	if err != nil {
		return net.Interface{}, fmt.Errorf("find new link by name %s: %s", deviceName, err)
	}
	return net.Interface{
		Index:        nlLink.Attrs().Index,
		MTU:          nlLink.Attrs().MTU,
		Name:         nlLink.Attrs().Name,
		HardwareAddr: nlLink.Attrs().HardwareAddr,
		Flags:        nlLink.Attrs().Flags,
	}, nil
}
