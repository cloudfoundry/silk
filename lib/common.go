package lib

import (
	"fmt"

	"github.com/cloudfoundry-incubator/silk/config"
)

// Common bevavior used by both the host-side and container-side Setup functions
type Common struct {
	NetlinkAdapter netlinkAdapter
	LinkOperations linkOperations
}

// BasicSetup configures a veth device for point-to-point communication with its peer.
// It is meant to be called by either Host.Setup or Container.Setup
func (s *Common) BasicSetup(deviceName string, local, peer config.DualAddress) error {
	link, err := s.NetlinkAdapter.LinkByName(deviceName)
	if err != nil {
		return fmt.Errorf("failed to find link %q: %s", deviceName, err)
	}

	err = s.NetlinkAdapter.LinkSetHardwareAddr(link, local.Hardware)
	if err != nil {
		return fmt.Errorf("setting hardware address: %s", err)
	}

	if err := s.LinkOperations.DisableIPv6(deviceName); err != nil {
		return fmt.Errorf("disable IPv6: %s", err)
	}

	if err := s.LinkOperations.StaticNeighborNoARP(link, peer.IP, peer.Hardware); err != nil {
		return fmt.Errorf("replace ARP with permanent neighbor rule: %s", err)
	}

	if err := s.LinkOperations.SetPointToPointAddress(link, local.IP, peer.IP); err != nil {
		return fmt.Errorf("setting point to point address: %s", err)
	}

	if err := s.NetlinkAdapter.LinkSetUp(link); err != nil {
		return fmt.Errorf("setting link %s up: %s", deviceName, err)
	}

	return nil
}
