package vtep

import (
	"fmt"
	"net"

	clientConfig "code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/client/state"
)

//go:generate counterfeiter -o fakes/netAdapter.go --fake-name NetAdapter . netAdapter
type netAdapter interface {
	Interfaces() ([]net.Interface, error)
	// ParseCIDR(s string) (net.IP, *net.IPNet, error)
	// ParseIP(s string) net.IP
	InterfaceAddrs(net.Interface) ([]net.Addr, error)
}

//go:generate counterfeiter -o fakes/hardwareAddressGenerator.go --fake-name HardwareAddressGenerator . hardwareAddressGenerator
type hardwareAddressGenerator interface {
	GenerateForVTEP(containerIP net.IP) (net.HardwareAddr, error)
}

type ConfigCreator struct {
	NetAdapter               netAdapter
	HardwareAddressGenerator hardwareAddressGenerator
}

type Config struct {
	VTEPName            string
	UnderlayInterface   net.Interface
	UnderlayIP          net.IP
	OverlayIP           net.IP
	OverlayHardwareAddr net.HardwareAddr
	VNI                 int
}

func (c *ConfigCreator) Create(clientConf clientConfig.Config, lease state.SubnetLease) (*Config, error) {
	if clientConf.VTEPName == "" {
		return nil, fmt.Errorf("empty vtep name")
	}

	underlayIP := net.ParseIP(clientConf.UnderlayIP)
	if underlayIP == nil {
		return nil, fmt.Errorf("parse underlay ip: %s", clientConf.UnderlayIP)
	}

	overlayIP, _, err := net.ParseCIDR(lease.Subnet)
	if err != nil {
		return nil, fmt.Errorf("determine vtep overlay ip: %s", err)
	}

	underlayInterface, err := c.locateInterface(underlayIP)
	if err != nil {
		return nil, fmt.Errorf("find device from ip %s: %s", underlayIP, err)
	}

	overlayHardwareAddr, err := c.HardwareAddressGenerator.GenerateForVTEP(overlayIP)
	if err != nil {
		return nil, fmt.Errorf("generate hardware address for ip %s: %s", overlayIP, err)
	}

	return &Config{
		VTEPName:            clientConf.VTEPName,
		UnderlayInterface:   underlayInterface,
		UnderlayIP:          underlayIP,
		OverlayIP:           overlayIP,
		OverlayHardwareAddr: overlayHardwareAddr,
		VNI:                 clientConf.VNI,
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
