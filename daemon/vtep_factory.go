package daemon

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

//go:generate counterfeiter -o fakes/netlinkAdapter.go --fake-name NetlinkAdapter . netlinkAdapter
type netlinkAdapter interface {
	LinkSetUp(netlink.Link) error
	LinkAdd(netlink.Link) error
	LinkSetHardwareAddr(netlink.Link, net.HardwareAddr) error
	AddrAddScopeLink(link netlink.Link, addr *netlink.Addr) error
}

//go:generate counterfeiter -o fakes/hardwareAddressGenerator.go --fake-name HardwareAddressGenerator . hardwareAddressGenerator
type hardwareAddressGenerator interface {
	GenerateForVTEP(containerIP net.IP) (net.HardwareAddr, error)
}

type VTEPFactory struct {
	NetlinkAdapter           netlinkAdapter
	HardwareAddressGenerator hardwareAddressGenerator
}

func (f *VTEPFactory) CreateVTEP(vtepDeviceName string, underlayInterface net.Interface, underlayIP, overlayIP net.IP) error {
	overlayHardwareAddr, err := f.HardwareAddressGenerator.GenerateForVTEP(overlayIP)
	if err != nil {
		return fmt.Errorf("generate vtep hardware address: %s", err)
	}

	vxlan := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: vtepDeviceName,
		},
		VxlanId:      42,
		SrcAddr:      underlayIP,
		GBP:          true,
		Port:         4789,
		VtepDevIndex: underlayInterface.Index,
	}
	err = f.NetlinkAdapter.LinkAdd(vxlan)
	if err != nil {
		return fmt.Errorf("create link: %s", err)
	}
	err = f.NetlinkAdapter.LinkSetUp(vxlan)
	if err != nil {
		return fmt.Errorf("up link: %s", err)
	}

	err = f.NetlinkAdapter.LinkSetHardwareAddr(vxlan, overlayHardwareAddr)
	if err != nil {
		return fmt.Errorf("set hardware addr: %s", err)
	}

	err = f.NetlinkAdapter.AddrAddScopeLink(vxlan, &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   overlayIP,
			Mask: net.IPMask{0xff, 0xff, 0xff, 0xff},
		},
	})
	if err != nil {
		return fmt.Errorf("add address: %s", err)
	}

	return nil
}

func LocateInterface(toFind net.IP) (net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, err
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return net.Interface{}, err
		}

		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				return net.Interface{}, err
			}
			if ip.String() == toFind.String() {
				return iface, nil
			}
		}
	}

	return net.Interface{}, fmt.Errorf("no interface with address %s", toFind.String())
}
