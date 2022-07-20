package lib

import (
	"fmt"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/cni/config"
)

// Common bevavior used by both the host-side and container-side Setup functions
type Common struct {
	NetlinkAdapter netlinkAdapter
	LinkOperations linkOperations
	Logger         lager.Logger
}

const MAX_ATTEMPTS = 10

// BasicSetup configures a veth device for point-to-point communication with its peer.
// It is meant to be called by either Host.Setup or Container.Setup
func (s *Common) BasicSetup(deviceName string, local, peer config.DualAddress) error {
	s.Logger.Debug("basic-device-setup", lager.Data{"deviceName": deviceName, "local": local.Hardware.String(), "peer": peer.Hardware.String()})
	defer s.Logger.Debug("done")
	link, err := s.NetlinkAdapter.LinkByName(deviceName)
	if err != nil {
		return fmt.Errorf("failed to find link %q: %s", deviceName, err)
	}

	// Starting with Ubuntu 22.04 (jammy), we encountered cases where interfaces were
	// not actually getting the hardware addr being set here. So we added this loop
	// to make sure that when we set it, it gets set to what we set it to. After we observed
	// occasional failures even after introducing this loop, we modified VethPairCreator.Create
	// to set the hardware addresses on interface creation, and that seems to have fixed the issue.
	// For example: s-1234 had MAC addr bb:bb:bb:bb:bb:bb initially. We call LinkSetHardwarAddr
	// to set it to aa:aa:aa:aa:aa:aa, but afterwards its MAC addr is ff:dd:23:f1:9a:43

	l, _ := s.NetlinkAdapter.LinkByName(deviceName)
	got := l.Attrs().HardwareAddr.String()
	expected := local.Hardware.String()
	for i := 0; got != expected && i < MAX_ATTEMPTS; i++ {
		s.Logger.Debug("hardware-addr-incorrect-retrying", lager.Data{"expected": expected, "found": got})
		err = s.NetlinkAdapter.LinkSetHardwareAddr(link, local.Hardware)
		if err != nil {
			return fmt.Errorf("setting hardware address: %s", err)
		}
		l, _ := s.NetlinkAdapter.LinkByName(deviceName)
		got = l.Attrs().HardwareAddr.String()
	}
	if got != expected {
		return fmt.Errorf("failed to set hardware addr after %d attempts", MAX_ATTEMPTS)
	} else {
		s.Logger.Debug("hardware-addr-set-correctly", lager.Data{"addr": l.Attrs().HardwareAddr.String()})
	}

	s.LinkOperations.DisableIPv6(deviceName)

	if err := s.LinkOperations.StaticNeighborNoARP(link, peer.IP, peer.Hardware); err != nil {
		return fmt.Errorf("replace ARP with permanent neighbor rule: %s", err)
	}

	if err := s.LinkOperations.SetPointToPointAddress(link, local.IP, peer.IP); err != nil {
		return fmt.Errorf("setting point to point address: %s", err)
	}

	if err := s.LinkOperations.EnableReversePathFiltering(deviceName); err != nil {
		return fmt.Errorf("enable reverse path filtering: %s", err)
	}

	if err := s.NetlinkAdapter.LinkSetUp(link); err != nil {
		return fmt.Errorf("setting link %s up: %s", deviceName, err)
	}

	return nil
}
