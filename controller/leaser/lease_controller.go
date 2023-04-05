package leaser

import (
	"fmt"
	"net"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/database"
)

//go:generate counterfeiter -o fakes/database_handler.go --fake-name DatabaseHandler . databaseHandler
type databaseHandler interface {
	AddEntry(controller.Lease) error
	DeleteEntry(string) error
	LeaseForUnderlayIP(string) (*controller.Lease, error)
	LastRenewedAtForUnderlayIP(string) (int64, error)
	RenewLeaseForUnderlayIP(string) error
	All() ([]controller.Lease, error)
	AllBlockSubnets() ([]controller.Lease, error)
	AllSingleIPSubnets() ([]controller.Lease, error)
	AllActive(int) ([]controller.Lease, error)
	OldestExpiredBlockSubnet(int) (*controller.Lease, error)
	OldestExpiredSingleIP(int) (*controller.Lease, error)
}

//go:generate counterfeiter -o fakes/lease_validator.go --fake-name LeaseValidator . leaseValidator
type leaseValidator interface {
	Validate(controller.Lease) error
}

//go:generate counterfeiter -o fakes/cidr_pool.go --fake-name CIDRPool . cidrPool
type cidrPool interface {
	GetAvailableBlock([]string) string
	GetAvailableSingleIP([]string) string
	IsMember(string) bool
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
	LeaseValidator             leaseValidator
	LeaseExpirationSeconds     int
	Logger                     lager.Logger
}

func (c *LeaseController) ReleaseSubnetLease(underlayIP string) error {
	err := c.DatabaseHandler.DeleteEntry(underlayIP)
	if err == database.RecordNotAffectedError {
		c.Logger.Debug("lease-not-found", lager.Data{"underlay_ip": underlayIP})
		return nil
	}
	if err != nil {
		return fmt.Errorf("release lease: %s", err)
	}

	c.Logger.Info("lease-released", lager.Data{"underlay_ip": underlayIP})
	return err
}

func (c *LeaseController) AcquireSubnetLease(underlayIP string, singleOverlayIP bool) (*controller.Lease, error) {
	var err error
	var lease *controller.Lease

	if net.ParseIP(underlayIP).To4() == nil {
		return nil, fmt.Errorf("invalid ipv4 address: %s", underlayIP)
	}

	lease, err = c.DatabaseHandler.LeaseForUnderlayIP(underlayIP)
	if err != nil {
		return nil, fmt.Errorf("getting lease for underlay ip: %s", err)
	}

	if lease != nil {
		if c.CIDRPool.IsMember(lease.OverlaySubnet) {
			c.Logger.Info("lease-renewed", lager.Data{"lease": lease})
			return lease, nil
		}
		err := c.DatabaseHandler.DeleteEntry(underlayIP)
		if err != nil {
			return nil, fmt.Errorf("deleting lease for underlay ip %s: %s", underlayIP, err)
		}
		c.Logger.Info("lease-deleted", lager.Data{"lease": lease})
	}

	for numErrs := 0; numErrs < c.AcquireSubnetLeaseAttempts; numErrs++ {
		lease, err = c.tryAcquireLease(underlayIP, singleOverlayIP)
		if lease != nil {
			c.Logger.Info("lease-acquired", lager.Data{"lease": lease})
			return lease, nil
		}
	}

	return nil, err
}

func (c *LeaseController) RenewSubnetLease(lease controller.Lease) error {
	err := c.LeaseValidator.Validate(lease)
	if err != nil {
		return controller.NonRetriableError(err.Error())
	}

	existingLease, err := c.DatabaseHandler.LeaseForUnderlayIP(lease.UnderlayIP)
	if err != nil {
		return fmt.Errorf("getting lease for underlay ip: %s", err)
	}
	if existingLease == nil {
		err := c.DatabaseHandler.AddEntry(lease)
		if err != nil {
			return controller.NonRetriableError(err.Error())
		}
	} else if lease != *existingLease {
		return controller.NonRetriableError("lease mismatch")
	}

	err = c.DatabaseHandler.RenewLeaseForUnderlayIP(lease.UnderlayIP)
	if err != nil {
		return fmt.Errorf("renewing lease for underlay ip: %s", err)
	}
	lastRenewedAt, err := c.DatabaseHandler.LastRenewedAtForUnderlayIP(lease.UnderlayIP)
	if err != nil {
		return fmt.Errorf("getting last renewed at: %s", err)
	}

	c.Logger.Debug("lease-renewed", lager.Data{"lease": lease, "last_renewed_at": lastRenewedAt})

	return nil
}

func (c *LeaseController) RoutableLeases() ([]controller.Lease, error) {
	leases, err := c.DatabaseHandler.AllActive(c.LeaseExpirationSeconds)
	if err != nil {
		return nil, fmt.Errorf("getting all leases: %s", err)
	}

	return leases, nil
}

func (c *LeaseController) tryAcquireLease(underlayIP string, singleOverlayIP bool) (*controller.Lease, error) {
	var subnet string
	if singleOverlayIP {
		var err error
		subnet, err = c.tryAcquireAvailableSingleIPSubnet(underlayIP)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		subnet, err = c.tryAcquireAvailableBlockSubnet(underlayIP)
		if err != nil {
			return nil, err
		}
	}
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

	lease := controller.Lease{
		UnderlayIP:          underlayIP,
		OverlaySubnet:       subnet,
		OverlayHardwareAddr: hwAddr.String(),
	}

	err = c.DatabaseHandler.AddEntry(lease)
	if err != nil {
		return nil, fmt.Errorf("adding lease entry: %s", err)
	}
	return &lease, nil
}

func (c *LeaseController) tryAcquireAvailableSingleIPSubnet(underlayIP string) (string, error) {
	var subnet string
	leases, err := c.DatabaseHandler.AllSingleIPSubnets()
	if err != nil {
		return "", fmt.Errorf("getting all single ip subnets: %s", err)
	}
	var taken []string
	for _, lease := range leases {
		taken = append(taken, lease.OverlaySubnet)
	}

	subnet = c.CIDRPool.GetAvailableSingleIP(taken)
	if subnet == "" {
		lease, err := c.DatabaseHandler.OldestExpiredSingleIP(c.LeaseExpirationSeconds)
		if err != nil {
			return "", fmt.Errorf("get oldest expired single ip: %s", err)
		} else if lease == nil {
			return "", nil
		} else {
			err := c.DatabaseHandler.DeleteEntry(lease.UnderlayIP)
			if err != nil {
				return "", fmt.Errorf("delete expired subnet: %s", err)
			}
			subnet = lease.OverlaySubnet
		}
	}

	return subnet, nil
}

func (c *LeaseController) tryAcquireAvailableBlockSubnet(underlayIP string) (string, error) {
	var subnet string
	leases, err := c.DatabaseHandler.AllBlockSubnets()
	if err != nil {
		return "", fmt.Errorf("getting all subnets: %s", err)
	}
	var taken []string
	for _, lease := range leases {
		taken = append(taken, lease.OverlaySubnet)
	}

	subnet = c.CIDRPool.GetAvailableBlock(taken)
	if subnet == "" {
		lease, err := c.DatabaseHandler.OldestExpiredBlockSubnet(c.LeaseExpirationSeconds)
		if err != nil {
			return "", fmt.Errorf("get oldest expired: %s", err)
		} else if lease == nil {
			return "", nil
		} else {
			err := c.DatabaseHandler.DeleteEntry(lease.UnderlayIP)
			if err != nil {
				return "", fmt.Errorf("delete expired subnet: %s", err) // test
			}
			subnet = lease.OverlaySubnet
		}
	}

	return subnet, nil
}
