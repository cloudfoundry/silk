package vtep

import (
	"errors"
	"fmt"
	"net"

	"code.cloudfoundry.org/lager"
	"github.com/vishvananda/netlink"
)

//go:generate counterfeiter -o fakes/netlinkAdapter.go --fake-name NetlinkAdapter . netlinkAdapter
type netlinkAdapter interface {
	LinkSetUp(netlink.Link) error
	LinkAdd(netlink.Link) error
	LinkByName(string) (netlink.Link, error)
	LinkByIndex(int) (netlink.Link, error)
	LinkSetHardwareAddr(netlink.Link, net.HardwareAddr) error
	AddrAddScopeLink(link netlink.Link, addr *netlink.Addr) error
	AddrList(link netlink.Link, family int) ([]netlink.Addr, error)
	RouteAdd(*netlink.Route) error
	RouteReplace(*netlink.Route) error
	RouteList(netlink.Link, int) ([]netlink.Route, error)
	RouteDel(*netlink.Route) error
	LinkDel(netlink.Link) error
	NeighSet(*netlink.Neigh) error
	ARPList(index int) ([]netlink.Neigh, error)
	FDBList(index int) ([]netlink.Neigh, error)
	NeighDel(*netlink.Neigh) error
}

type Factory struct {
	NetlinkAdapter netlinkAdapter
	Logger         lager.Logger
}

func (f *Factory) CreateVTEP(cfg *Config) error {
	vxlan := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: cfg.VTEPName,
		},
		VxlanId:      cfg.VNI,
		SrcAddr:      cfg.UnderlayIP,
		Port:         cfg.VTEPPort,
		VtepDevIndex: cfg.UnderlayInterface.Index,
		GBP:          true,
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

	overlayNetworkMask := net.CIDRMask(cfg.OverlayNetworkPrefixLength, 32)

	err = f.NetlinkAdapter.AddrAddScopeLink(vxlan, &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   cfg.OverlayIP,
			Mask: overlayNetworkMask,
		},
	})
	if err != nil {
		return fmt.Errorf("add address: %s", err)
	}

	return nil
}

func (f *Factory) DeleteVTEP(deviceName string) error {
	link, err := f.NetlinkAdapter.LinkByName(deviceName)
	if err != nil {
		return fmt.Errorf("find link %s: %s", deviceName, err)
	}
	err = f.NetlinkAdapter.LinkDel(link)
	if err != nil {
		return fmt.Errorf("delete link %s: %s", deviceName, err)
	}
	return nil
}

func (f *Factory) GetVTEPState(vtepName string) (net.HardwareAddr, net.IP, int, error) {
	link, err := f.NetlinkAdapter.LinkByName(vtepName)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("find link: %s", err)
	}
	addresses, err := f.NetlinkAdapter.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("list addresses: %s", err)
	}
	if len(addresses) == 0 {
		return nil, nil, 0, errors.New("no addresses")
	}
	return link.Attrs().HardwareAddr, addresses[0].IP, link.Attrs().MTU, nil
}
