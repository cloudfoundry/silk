package lib

import (
	"fmt"
	"time"

	"github.com/cloudfoundry-incubator/silk/daemon/database"
)

type LeaseController struct {
	DatabaseHandler               *database.DatabaseHandler
	MaxMigrationAttempts          int
	MigrationAttemptSleepDuration time.Duration
	AcquireSubnetLeaseAttempts    int
	CIDRPool                      *CIDRPool
	UnderlayIP                    string
}

func (c *LeaseController) TryMigrations() error {
	nErrors := 0
	var err error
	for nErrors < c.MaxMigrationAttempts {
		var n int
		n, err = c.DatabaseHandler.Migrate()
		if err == nil {
			fmt.Printf("db migration complete: applied %d migrations.\n", n)
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
	subnet, err := c.getFreeSubnet(c.DatabaseHandler)
	if err != nil {
		panic(err)
	}

	return subnet, c.DatabaseHandler.AddEntry(c.UnderlayIP, subnet)
}

func (c *LeaseController) getFreeSubnet(databaseHandler *database.DatabaseHandler) (string, error) {
	i := 0
	for {
		subnet := c.CIDRPool.GetRandom()
		subnetExists, err := databaseHandler.SubnetExists(subnet)
		if err != nil {
			panic(err)
		}

		if !subnetExists {
			return subnet, nil
		}
		i++
	}
}
