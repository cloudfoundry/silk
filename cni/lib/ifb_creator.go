package lib

import (
	"fmt"
	"net"

	"code.cloudfoundry.org/silk/cni/config"
	"github.com/containernetworking/plugins/pkg/ns"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/vishvananda/netlink"
)

type IFBCreator struct {
	NetlinkAdapter      netlinkAdapter
	DeviceNameGenerator deviceNameGenerator
}

func (ifbCreator *IFBCreator) Create(cfg *config.Config) error {
	err := ifbCreator.NetlinkAdapter.LinkAdd(&netlink.Ifb{
		LinkAttrs: netlink.LinkAttrs{
			Name:  cfg.IFB.DeviceName,
			Flags: net.FlagUp,
			MTU:   cfg.Container.MTU,
		},
	})

	if err != nil {
		return fmt.Errorf("adding link: %s", err)
	}

	return nil
}

func (ifb *IFBCreator) Teardown(namespace ns.NetNS, deviceName string) error {
	var addrs []netlink.Addr
	err := namespace.Do(func(_ ns.NetNS) error {
		var err error
		var link netlink.Link
		link, err = ifb.NetlinkAdapter.LinkByName(deviceName)
		if err != nil {
			return fmt.Errorf("find link: %s", err)
		}

		addrs, err = ifb.NetlinkAdapter.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			return fmt.Errorf("list addresses: %s", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	var ifbDeviceName string
	var result *multierror.Error

	for _, addr := range addrs {
		ifbDeviceName, err = ifb.DeviceNameGenerator.GenerateForHostIFB(addr.IPNet.IP)

		if err != nil {
			result = multierror.Append(result, fmt.Errorf("generate ifb device name: %s", err))
			continue
		}
		err = ifb.NetlinkAdapter.LinkDel(&netlink.Ifb{
			LinkAttrs: netlink.LinkAttrs{
				Name: ifbDeviceName,
			},
		})

		if err != nil {
			result = multierror.Append(result, fmt.Errorf("delete link: %s", err))
		}
	}

	return result.ErrorOrNil()
}
