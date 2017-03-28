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

type LeaseController struct {
	DatabaseHandler               databaseHandler
	MaxMigrationAttempts          int
	MigrationAttemptSleepDuration time.Duration
	AcquireSubnetLeaseAttempts    int
	CIDRPool                      *CIDRPool
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
			break
		}

		nErrors++
		time.Sleep(c.MigrationAttemptSleepDuration)
	}

	return err
}

func (c *LeaseController) AcquireSubnetLease() (string, error) {
	var err error
	for numErrs := 0; numErrs < c.AcquireSubnetLeaseAttempts; numErrs++ {
		var lease string
		lease, err = c.tryAcquireLease()
		if err == nil {
			return lease, nil
		}
		fmt.Printf("tried to acquire %s but failed, attempt %d for underlay ip %s\n", lease, numErrs, c.UnderlayIP)
	}

	return "", err
}

func (c *LeaseController) tryAcquireLease() (string, error) {
	subnet, err := c.getFreeSubnet()
	if err != nil {
		panic(err)
	}

	return subnet, c.DatabaseHandler.AddEntry(c.UnderlayIP, subnet)
}

func (c *LeaseController) getFreeSubnet() (string, error) {
	i := 0
	for {
		subnet := c.CIDRPool.GetRandom()
		subnetExists, err := c.DatabaseHandler.SubnetExists(subnet)
		if err != nil {
			panic(err)
		}

		if !subnetExists {
			return subnet, nil
		}
		i++
	}
}
