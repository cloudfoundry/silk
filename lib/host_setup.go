package lib

import (
	"github.com/cloudfoundry-incubator/silk/config"
	"github.com/containernetworking/cni/pkg/ns"
)

type Host struct {
	Common
}

// Setup will configure the network stack on the host
// A veth pair must already have been created.  See VethPairCreator.
func (h *Host) Setup(cfg *config.Config) error {
	deviceName := cfg.Host.DeviceName
	local := cfg.Host.Address
	peer := cfg.Container.Address

	return cfg.Host.Namespace.Do(func(_ ns.NetNS) error {
		return h.Common.BasicSetup(deviceName, local, peer)
	})
}
