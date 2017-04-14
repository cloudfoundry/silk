package leaser

import (
	"fmt"
	"net"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/database"
)

//go:generate counterfeiter -o fakes/database_handler.go --fake-name DatabaseHandler . databaseHandler
type databaseHandler interface {
	AddEntry(*controller.Lease) error
	DeleteEntry(string) error
	LeaseForUnderlayIP(string) (*controller.Lease, error)
	Release(controller.Lease) error
	All() ([]controller.Lease, error)
}

//go:generate counterfeiter -o fakes/cidr_pool.go --fake-name CIDRPool . cidrPool
type cidrPool interface {
	GetAvailable([]string) string
}

//go:generate counterfeiter -o fakes/hardwareAddressGenerator.go --fake-name HardwareAddressGenerator . hardwareAddressGenerator
type hardwareAddressGenerator interface {
	GenerateForVTEP(containerIP net.IP) (net.HardwareAddr, error)
}

type LeaseController struct {
	DatabaseHandler            databaseHandler
	HardwareAddressGenerator   hardwareAddressGenerator
	AcquireSubnetLeaseAttempts int
	CIDRPool                   cidrPool
	Logger                     lager.Logger
}

func (c *LeaseController) ReleaseSubnetLease(lease controller.Lease) error {
	err := c.DatabaseHandler.Release(lease)
	if err == database.RecordNotAffectedError {
		c.Logger.Error("lease-not-found", err, lager.Data{"lease": lease})
		return nil
	}
	if err == database.MultipleRecordsAffectedError {
		c.Logger.Error("multiple-leases-deleted", err, lager.Data{"lease": lease})
		return nil
	}
	if err != nil {
		return fmt.Errorf("release lease: %s", err)
	}

	c.Logger.Info("lease-released", lager.Data{"lease": lease})
	return err
}

func (c *LeaseController) AcquireSubnetLease(underlayIP string) (*controller.Lease, error) {
	var err error
	var lease *controller.Lease

	if net.ParseIP(underlayIP).To4() == nil {
		return nil, fmt.Errorf("invalid ipv4 address: %s", underlayIP)
	}

	lease, err = c.DatabaseHandler.LeaseForUnderlayIP(underlayIP)
	if lease != nil {
		c.Logger.Info("lease-renewed", lager.Data{"lease": lease})
		return lease, nil
	}

	for numErrs := 0; numErrs < c.AcquireSubnetLeaseAttempts; numErrs++ {
		lease, err = c.tryAcquireLease(underlayIP)
		if lease != nil {
			c.Logger.Info("lease-acquired", lager.Data{"lease": lease})
			return lease, nil
		}
	}

	return nil, err
}

func (c *LeaseController) RoutableLeases() ([]controller.Lease, error) {
	leases, err := c.DatabaseHandler.All()
	if err != nil {
		return nil, fmt.Errorf("getting all leases: %s", err)
	}

	return leases, nil
}

func (c *LeaseController) tryAcquireLease(underlayIP string) (*controller.Lease, error) {
	var subnet string
	leases, err := c.DatabaseHandler.All()
	if err != nil {
		return nil, fmt.Errorf("getting all subnets: %s", err)
	}
	var taken []string
	for _, lease := range leases {
		taken = append(taken, lease.OverlaySubnet)
	}

	subnet = c.CIDRPool.GetAvailable(taken)
	if subnet == "" {
		return nil, nil
	}

	vtepIP, _, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("parse subnet: %s", err)
	}
	hwAddr, err := c.HardwareAddressGenerator.GenerateForVTEP(vtepIP)
	if err != nil {
		return nil, fmt.Errorf("generate hardware address: %s", err)
	}

	lease := &controller.Lease{
		UnderlayIP:          underlayIP,
		OverlaySubnet:       subnet,
		OverlayHardwareAddr: hwAddr.String(),
	}

	err = c.DatabaseHandler.AddEntry(lease)
	if err != nil {
		return nil, fmt.Errorf("adding lease entry: %s", err)
	}
	return lease, nil
}
