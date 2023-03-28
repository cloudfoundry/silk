package lib

import (
	"fmt"
	"net"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/silk/cni/config"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

type VethPairCreator struct {
	NetlinkAdapter netlinkAdapter
	Logger         lager.Logger
}

// Create will create a pair of virtual ethernet devices and move one end into the container
// The container-side will have a temporary name.
func (c *VethPairCreator) Create(cfg *config.Config) error {
	c.Logger.Debug("start")
	defer c.Logger.Debug("done")
	hostName := cfg.Host.DeviceName
	containerName := cfg.Container.TemporaryDeviceName

	// Starting with Ubuntu 22.04 (jammy), we encountered cases where interfaces were
	// not actually getting the hardware addr being set here. Originally we were not setting
	// hardware addrs on the interface during VethPairCreator.Create, so we added a loop
	// around the LinkSetHardwareAddr call in Common.BasicSetup to make sure that when we set it,
	// it gets set to what we set it to. That still did not always work, so we started to
	// set the HardwareAddr here as well. This has successfully quelled the problem.
	vethDeviceRequest := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name:         hostName,
			Flags:        net.FlagUp,
			MTU:          cfg.Container.MTU,
			HardwareAddr: cfg.Host.Address.Hardware,
		},
		PeerName:         containerName,
		PeerHardwareAddr: cfg.Container.Address.Hardware,
	}
	c.Logger.Debug("create", lager.Data{"hostName": hostName, "containerName": containerName, "vethDeviceRequest": vethDeviceRequest})

	// Note: this Do is only necessary while we're doing container namespace switching elsewhere in this process
	err := cfg.Host.Namespace.Do(func(_ ns.NetNS) error {
		if err := c.NetlinkAdapter.LinkAdd(vethDeviceRequest); err != nil {
			return fmt.Errorf("creating veth pair: %s", err)
		}

		containerVeth, err := c.NetlinkAdapter.LinkByName(containerName)
		if err != nil {
			return fmt.Errorf("failed to find newly-created veth device %q: %v", containerName, err)
		}
		c.Logger.Debug("create.result", lager.Data{"containerVeth": containerVeth})

		err = c.NetlinkAdapter.LinkSetNsFd(containerVeth, int(cfg.Container.Namespace.Fd()))
		if err != nil {
			return fmt.Errorf("failed to move veth to container namespace: %s", err)
		}
		return nil
	})

	return err
}
