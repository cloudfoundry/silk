package lib

import (
	"fmt"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/cni/config"
	"github.com/containernetworking/plugins/pkg/ns"
)

type Container struct {
	Common         common
	LinkOperations linkOperations
	Logger         lager.Logger
}

// Teardown deletes the named device from the container.
// The kernel should automatically cleanup the other end of the veth pair
// and any associated addresses, neighbor rules, etc
func (c *Container) Teardown(containerNS ns.NetNS, deviceName string) error {
	c.Logger.Debug("start")
	defer c.Logger.Debug("done")
	return containerNS.Do(func(_ ns.NetNS) error {
		if err := c.LinkOperations.DeleteLinkByName(deviceName); err != nil {
			return fmt.Errorf("deleting link: %s", err)
		}
		return nil
	})
}

// Setup will configure the network stack within the container
// A veth pair must already have been created, with one end given the
// TemporaryDeviceName and moved into the container.  See VethPairCreator.
func (c *Container) Setup(cfg *config.Config) error {
	c.Logger.Debug("start")
	defer c.Logger.Debug("done")
	deviceName := cfg.Container.DeviceName

	local := cfg.Container.Address
	peer := cfg.Host.Address

	return cfg.Container.Namespace.Do(func(_ ns.NetNS) error {
		if err := c.LinkOperations.RenameLink(cfg.Container.TemporaryDeviceName, deviceName); err != nil {
			return fmt.Errorf("renaming link in container: %s", err)
		}

		if err := c.Common.BasicSetup(deviceName, local, peer); err != nil {
			return fmt.Errorf("setting up device in container: %s", err)
		}

		if err := c.LinkOperations.RouteAddAll(cfg.Container.Routes, cfg.Container.Address.IP); err != nil {
			return fmt.Errorf("adding route in container: %s", err)
		}

		return nil
	})
}
