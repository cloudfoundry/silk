package leaser

import (
	"fmt"
	"net"

	"code.cloudfoundry.org/silk/controller"
)

type LeaseValidator struct{}

func (v *LeaseValidator) Validate(lease controller.Lease) error {
	if net.ParseIP(lease.UnderlayIP) == nil {
		return fmt.Errorf("invalid underlay ip: %s", lease.UnderlayIP)
	}

	_, _, err := net.ParseCIDR(lease.OverlaySubnet)
	if err != nil {
		return err
	}

	_, err = net.ParseMAC(lease.OverlayHardwareAddr)
	if err != nil {
		return err
	}

	return nil
}
