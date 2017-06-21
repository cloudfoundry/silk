package lib

import (
	"fmt"
	"net"

	"code.cloudfoundry.org/silk/cni/config"
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

func (ifb *IFBCreator) Teardown(ipAddr string) error {
	ifbDeviceName, err := ifb.DeviceNameGenerator.GenerateForHostIFB(net.ParseIP(ipAddr))
	if err != nil {
		return fmt.Errorf("generate ifb device name: %s", err)
	}

	err = ifb.NetlinkAdapter.LinkDel(&netlink.Ifb{
		LinkAttrs: netlink.LinkAttrs{
			Name: ifbDeviceName,
		},
	})
	if err != nil {
		return fmt.Errorf("delete link: %s", err)
	}

	return nil
}
