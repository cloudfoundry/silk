package lib

import (
	"fmt"
	"net"

	"github.com/cloudfoundry-incubator/silk/cni/config"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/vishvananda/netlink"
)

type VethPairCreator struct {
	NetlinkAdapter netlinkAdapter
}

// Create will create a pair of virtual ethernet devices and move one end into the container
// The container-side will have a temporary name.
func (c *VethPairCreator) Create(cfg *config.Config) error {
	hostName := cfg.Host.DeviceName
	containerName := cfg.Container.TemporaryDeviceName

	vethDeviceRequest := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name:  hostName,
			Flags: net.FlagUp,
			MTU:   cfg.Container.MTU,
		},
		PeerName: containerName,
	}

	// Note: this Do is only necessary while we're doing container namespace switching elsewhere in this process
	err := cfg.Host.Namespace.Do(func(_ ns.NetNS) error {
		if err := c.NetlinkAdapter.LinkAdd(vethDeviceRequest); err != nil {
			return fmt.Errorf("creating veth pair: %s", err)
		}

		containerVeth, err := c.NetlinkAdapter.LinkByName(containerName)
		if err != nil {
			return fmt.Errorf("failed to find newly-created veth device %q: %v", containerName, err)
		}

		err = c.NetlinkAdapter.LinkSetNsFd(containerVeth, int(cfg.Container.Namespace.Fd()))
		if err != nil {
			return fmt.Errorf("failed to move veth to container namespace: %s", err)
		}
		return nil
	})

	return err
}
