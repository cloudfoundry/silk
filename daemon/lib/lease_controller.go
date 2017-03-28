package lib

import (
	"fmt"
	"time"

	"code.cloudfoundry.org/lager"
)

//go:generate counterfeiter -o fakes/database_handler.go --fake-name DatabaseHandler . databaseHandler
type databaseHandler interface {
	Migrate() (int, error)
	AddEntry(string, string) error
	SubnetExists(string) (bool, error)
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

func (c *LeaseController) AcquireSubnetLease() (string, error) {
	var err error
	for numErrs := 0; numErrs < c.AcquireSubnetLeaseAttempts; numErrs++ {
		var subnet string
		subnet, err = c.tryAcquireLease()
		if err == nil {
			c.Logger.Info("subnet-acquired", lager.Data{"subnet": subnet,
				"underlay ip": c.UnderlayIP,
			})
			return subnet, nil
		}
	}

	return "", err
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
	for {
		subnet := c.CIDRPool.GetRandom()
		subnetExists, err := c.DatabaseHandler.SubnetExists(subnet)
		if err != nil {
			return "", fmt.Errorf("checking if subnet is available: %s", err)
		}

		if !subnetExists {
			return subnet, nil
		}
	}
}
