package lib

import (
	"fmt"

	"github.com/cloudfoundry-incubator/silk/config"
	"github.com/containernetworking/cni/pkg/ns"
)

type Container struct {
	Common
}

// Teardown deletes the named device from the container.
// The kernel should automatically cleanup the other end of the veth pair
// and any associated addresses, neighbor rules, etc
func (c *Container) Teardown(containerNS ns.NetNS, deviceName string) error {
	return containerNS.Do(func(_ ns.NetNS) error {
		return c.Common.LinkOperations.DeleteLinkByName(deviceName)
	})
}

// Setup will configure the network stack within the container
// A veth pair must already have been created, with one end given the
// TemporaryDeviceName and moved into the container.  See VethPairCreator.
func (c *Container) Setup(cfg *config.Config) error {
	deviceName := cfg.Container.DeviceName

	local := cfg.Container.Address
	peer := cfg.Host.Address

	return cfg.Container.Namespace.Do(func(_ ns.NetNS) error {
		if err := c.Common.LinkOperations.RenameLink(cfg.Container.TemporaryDeviceName, deviceName); err != nil {
			return fmt.Errorf("renaming link in container: %s", err)
		}

		return c.Common.BasicSetup(deviceName, local, peer)
	})
}
