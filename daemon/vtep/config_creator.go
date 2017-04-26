package vtep

import (
	"fmt"
	"net"

	clientConfig "code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/controller"
)

//go:generate counterfeiter -o fakes/netAdapter.go --fake-name NetAdapter . netAdapter
type netAdapter interface {
	Interfaces() ([]net.Interface, error)
	InterfaceAddrs(net.Interface) ([]net.Addr, error)
}

type ConfigCreator struct {
	NetAdapter netAdapter
}

type Config struct {
	VTEPName                   string
	UnderlayInterface          net.Interface
	UnderlayIP                 net.IP
	OverlayIP                  net.IP
	OverlayHardwareAddr        net.HardwareAddr
	VNI                        int
	OverlayNetworkPrefixLength int
}

func (c *ConfigCreator) Create(clientConf clientConfig.Config, lease controller.Lease) (*Config, error) {
	if clientConf.VTEPName == "" {
		return nil, fmt.Errorf("empty vtep name")
	}

	underlayIP := net.ParseIP(clientConf.UnderlayIP)
	if underlayIP == nil {
		return nil, fmt.Errorf("parse underlay ip: %s", clientConf.UnderlayIP)
	}

	overlayIP, _, err := net.ParseCIDR(lease.OverlaySubnet)
	if err != nil {
		return nil, fmt.Errorf("determine vtep overlay ip: %s", err)
	}

	underlayInterface, err := c.locateInterface(underlayIP)
	if err != nil {
		return nil, fmt.Errorf("find device from ip %s: %s", underlayIP, err)
	}

	overlayHardwareAddr, err := net.ParseMAC(lease.OverlayHardwareAddr)
	if err != nil {
		return nil, fmt.Errorf("parsing hardware address: %s", err)
	}

	_, overlayNetwork, err := net.ParseCIDR(clientConf.OverlayNetwork)
	if err != nil {
		return nil, fmt.Errorf("determine overlay network: %s", err)
	}

	overlayNetworkPrefixLength, _ := overlayNetwork.Mask.Size()

	if overlayNetworkPrefixLength >= clientConf.SubnetPrefixLength {
		return nil, fmt.Errorf("overlay prefix %d must be smaller than subnet prefix %d",
			overlayNetworkPrefixLength, clientConf.SubnetPrefixLength)
	}

	return &Config{
		VTEPName:            clientConf.VTEPName,
		UnderlayInterface:   underlayInterface,
		UnderlayIP:          underlayIP,
		OverlayIP:           overlayIP,
		OverlayHardwareAddr: overlayHardwareAddr,
		VNI:                 clientConf.VNI,
		OverlayNetworkPrefixLength: overlayNetworkPrefixLength,
	}, nil
}

func (c *ConfigCreator) locateInterface(toFind net.IP) (net.Interface, error) {
	ifaces, err := c.NetAdapter.Interfaces()
	if err != nil {
		return net.Interface{}, fmt.Errorf("find interfaces: %s", err)
	}
	for _, iface := range ifaces {
		addrs, err := c.NetAdapter.InterfaceAddrs(iface)
		if err != nil {
			return net.Interface{}, fmt.Errorf("get addresses: %s", err)
		}

		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				return net.Interface{}, fmt.Errorf("parse address: %s", err)
			}
			if ip.String() == toFind.String() {
				return iface, nil
			}
		}
	}

	return net.Interface{}, fmt.Errorf("no interface with address %s", toFind.String())
}
