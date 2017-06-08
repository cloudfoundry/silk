package lib

import (
	"fmt"

	"code.cloudfoundry.org/silk/cni/config"
	"github.com/containernetworking/plugins/pkg/ns"
)

type Host struct {
	Common         common
	LinkOperations linkOperations
}

// Setup will configure the network stack on the host
// A veth pair must already have been created.  See VethPairCreator.
func (h *Host) Setup(cfg *config.Config) error {
	deviceName := cfg.Host.DeviceName
	local := cfg.Host.Address
	peer := cfg.Container.Address

	return cfg.Host.Namespace.Do(func(_ ns.NetNS) error {
		if err := h.Common.BasicSetup(deviceName, local, peer); err != nil {
			return fmt.Errorf("setting up device in host: %s", err)
		}

		if err := h.LinkOperations.EnableIPv4Forwarding(); err != nil {
			return fmt.Errorf("enabling packet forwarding on host: %s", err)
		}
		return nil
	})
}
