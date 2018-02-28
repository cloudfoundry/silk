package vtep

import (
	"fmt"
	"net"

	clientConfig "code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/controller"
	"strings"
)

//go:generate counterfeiter -o fakes/netAdapter.go --fake-name NetAdapter . netAdapter
type netAdapter interface {
	Interfaces() ([]net.Interface, error)
	InterfaceAddrs(net.Interface) ([]net.Addr, error)
	InterfaceByName(name string) (*net.Interface, error)
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
	VTEPPort                   int
}

func (c *ConfigCreator) Create(clientConf clientConfig.Config, lease controller.Lease) (*Config, error) {
	if clientConf.VTEPName == "" {
		return nil, fmt.Errorf("empty vtep name")
	}

	if clientConf.VTEPPort < 1 {
		return nil, fmt.Errorf("vtep port must be greater than 0")
	}

	underlayIP := net.ParseIP(clientConf.UnderlayIP)
	if underlayIP == nil {
		return nil, fmt.Errorf("parse underlay ip: %s", clientConf.UnderlayIP)
	}

	var underlayInterface net.Interface
	var err error

	if clientConf.CustomUnderlayInterfaceName != "" {
		underlayInterface, err = c.locateInterfaceByName(clientConf.CustomUnderlayInterfaceName, clientConf.UnderlayIPs)
		if err != nil {
			return nil, err
		}
	} else {
		underlayInterface, err = c.locateInterface(underlayIP)
		if err != nil {
			return nil, fmt.Errorf("find device from ip %s: %s", underlayIP, err)
		}
	}

	overlayIP, _, err := net.ParseCIDR(lease.OverlaySubnet)
	if err != nil {
		return nil, fmt.Errorf("determine vtep overlay ip: %s", err)
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
		VTEPPort:                   clientConf.VTEPPort,
	}, nil
}

func (c *ConfigCreator) locateInterfaceByName(name string, availableIPs []string) (net.Interface, error) {
	underlayInterface, err := c.NetAdapter.InterfaceByName(name)
	if err != nil {
		return net.Interface{}, fmt.Errorf("find device from name %s: %s", name, err)
	}

	availableInterfaceNames := make([]string, len(availableIPs))
	found := false

	for i, ip := range availableIPs {
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			return net.Interface{}, fmt.Errorf("parse underlay ip: %s", ip)
		}

		availableUnderlayInterface, err := c.locateInterface(parsedIP)
		if err != nil {
			return net.Interface{}, fmt.Errorf("find device from ip %s: %s", parsedIP, err)
		}

		availableInterfaceNames[i] = availableUnderlayInterface.Name
		if underlayInterface.Index == availableUnderlayInterface.Index {
			found = true
		}
	}

	if !found {
		return net.Interface{}, fmt.Errorf("requested custom underlay interface name '%s' refers to a non underlay device. valid choices are [%s]", underlayInterface.Name, strings.Join(availableInterfaceNames, ", "))
	}

	return *underlayInterface, nil
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
