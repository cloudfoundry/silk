package lib

import (
	"fmt"
	"time"

	"code.cloudfoundry.org/go-db-helpers/db"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/client/config"
)

//go:generate counterfeiter -o fakes/database_handler.go --fake-name DatabaseHandler . databaseHandler
type databaseHandler interface {
	Migrate() (int, error)
	AddEntry(string, string) error
	DeleteEntry(string) error
	SubnetExists(string) (bool, error)
	SubnetForUnderlayIP(string) (string, error)
}

//go:generate counterfeiter -o fakes/cidr_pool.go --fake-name CIDRPool . cidrPool
type cidrPool interface {
	GetRandom() string
}

type LeaseController struct {
	DatabaseHandler               databaseHandler
	MaxMigrationAttempts          int
	MigrationAttemptSleepDuration time.Duration
	AcquireSubnetLeaseAttempts    int
	CIDRPool                      cidrPool
	UnderlayIP                    string
	Logger                        lager.Logger
}

func NewLeaseController(cfg config.Config, logger lager.Logger) (*LeaseController, error) {
	sqlDB, err := db.GetConnectionPool(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %s", err)
	}

	databaseHandler := NewDatabaseHandler(&MigrateAdapter{}, sqlDB)
	leaseController := &LeaseController{
		DatabaseHandler:               databaseHandler,
		MaxMigrationAttempts:          5,
		MigrationAttemptSleepDuration: time.Second,
		AcquireSubnetLeaseAttempts:    10,
		CIDRPool:                      NewCIDRPool(cfg.SubnetRange, cfg.SubnetMask),
		UnderlayIP:                    cfg.UnderlayIP,
		Logger:                        logger,
	}
	if err = leaseController.TryMigrations(); err != nil {
		return nil, fmt.Errorf("migrating database: %s", err)
	}

	return leaseController, nil
}

func (c *LeaseController) TryMigrations() error {
	nErrors := 0
	var err error
	for nErrors < c.MaxMigrationAttempts {
		var n int
		n, err = c.DatabaseHandler.Migrate()
		if err == nil {
			c.Logger.Info("db-migration-complete", lager.Data{"num-applied": n})
			return nil
		}

		nErrors++
		time.Sleep(c.MigrationAttemptSleepDuration)
	}

	return fmt.Errorf("creating table: %s", err)
}

func (c *LeaseController) ReleaseSubnetLease() error {
	err := c.DatabaseHandler.DeleteEntry(c.UnderlayIP)
	if err != nil {
		return fmt.Errorf("releasing lease: %s", err)
	}
	c.Logger.Info("subnet-released", lager.Data{
		"underlay ip": c.UnderlayIP,
	})
	return nil
}

func (c *LeaseController) AcquireSubnetLease(underlayIP string) (string, error) {
	var err error
	var subnet string

	subnet, err = c.tryRenewLease()
	if subnet != "" {
		c.Logger.Info("subnet-renewed", lager.Data{"subnet": subnet,
			"underlay ip": underlayIP,
		})
		return subnet, nil
	}

	for numErrs := 0; numErrs < c.AcquireSubnetLeaseAttempts; numErrs++ {
		subnet, err = c.tryAcquireLease()
		if err == nil {
			c.Logger.Info("subnet-acquired", lager.Data{"subnet": subnet,
				"underlay ip": underlayIP,
			})
			return subnet, nil
		}
	}

	return "", err
}

func (c *LeaseController) tryRenewLease() (string, error) {
	subnet, err := c.DatabaseHandler.SubnetForUnderlayIP(c.UnderlayIP)
	if err != nil {
		return "", fmt.Errorf("checking if subnet exists for underlay: %s", err)
	}

	return subnet, nil
}

func (c *LeaseController) tryAcquireLease() (string, error) {
	subnet, err := c.getFreeSubnet()
	if err != nil {
		return "", err
	}

	err = c.DatabaseHandler.AddEntry(c.UnderlayIP, subnet)
	if err != nil {
		return "", fmt.Errorf("adding lease entry: %s", err)
	}

	return subnet, nil
}

func (c *LeaseController) getFreeSubnet() (string, error) {
	maxSubnetAttempts := 10
	for i := 0; i < maxSubnetAttempts; i++ {
		subnet := c.CIDRPool.GetRandom()
		subnetExists, err := c.DatabaseHandler.SubnetExists(subnet)
		if err != nil {
			return "", fmt.Errorf("checking if subnet is available: %s", err)
		}

		if !subnetExists {
			return subnet, nil
		}
	}
	return "", fmt.Errorf("unable to find a free subnet after %d attempts", maxSubnetAttempts)
}
