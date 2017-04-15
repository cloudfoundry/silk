package vtep

import (
	"errors"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

//go:generate counterfeiter -o fakes/netlinkAdapter.go --fake-name NetlinkAdapter . netlinkAdapter
type netlinkAdapter interface {
	LinkSetUp(netlink.Link) error
	LinkAdd(netlink.Link) error
	LinkByName(string) (netlink.Link, error)
	LinkSetHardwareAddr(netlink.Link, net.HardwareAddr) error
	AddrAddScopeLink(link netlink.Link, addr *netlink.Addr) error
	AddrList(link netlink.Link, family int) ([]netlink.Addr, error)
}

type Factory struct {
	NetlinkAdapter netlinkAdapter
}

func (f *Factory) CreateVTEP(cfg *Config) error {
	vxlan := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: cfg.VTEPName,
		},
		VxlanId:      cfg.VNI,
		SrcAddr:      cfg.UnderlayIP,
		GBP:          true,
		Port:         4789,
		VtepDevIndex: cfg.UnderlayInterface.Index,
	}
	err := f.NetlinkAdapter.LinkAdd(vxlan)
	if err != nil {
		return fmt.Errorf("create link %s: %s", cfg.VTEPName, err)
	}
	err = f.NetlinkAdapter.LinkSetUp(vxlan)
	if err != nil {
		return fmt.Errorf("up link: %s", err)
	}

	err = f.NetlinkAdapter.LinkSetHardwareAddr(vxlan, cfg.OverlayHardwareAddr)
	if err != nil {
		return fmt.Errorf("set hardware addr: %s", err)
	}

	err = f.NetlinkAdapter.AddrAddScopeLink(vxlan, &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   cfg.OverlayIP,
			Mask: net.IPMask{0xff, 0xff, 0xff, 0xff},
		},
	})
	if err != nil {
		return fmt.Errorf("add address: %s", err)
	}

	return nil
}

func (f *Factory) GetVTEPState(vtepName string) (net.HardwareAddr, net.IP, error) {
	link, err := f.NetlinkAdapter.LinkByName(vtepName)
	if err != nil {
		return nil, nil, fmt.Errorf("find link: %s", err)
	}
	addresses, err := f.NetlinkAdapter.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return nil, nil, fmt.Errorf("list addresses: %s", err)
	}
	if len(addresses) == 0 {
		return nil, nil, errors.New("no addresses")
	}
	return link.Attrs().HardwareAddr, addresses[0].IP, nil
}
