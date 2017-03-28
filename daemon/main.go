package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"time"

	"code.cloudfoundry.org/lager"

	"github.com/cloudfoundry-incubator/silk/daemon/config"
	"github.com/cloudfoundry-incubator/silk/daemon/database"
	"github.com/cloudfoundry-incubator/silk/daemon/lib"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

func main() {
	configFilePath := flag.String("config", "", "path to config file")
	flag.Parse()

	contents, err := ioutil.ReadFile(*configFilePath)
	if err != nil {
		panic(err)
	}

	var cfg config.Config
	err = json.Unmarshal(contents, &cfg)
	if err != nil {
		panic(err)
	}

	databaseHandler, err := database.NewDatabaseHandler(cfg.Database)
	if err != nil {
		panic(err)
	}
	fmt.Println("connected to db")

	leaseController := lib.LeaseController{
		DatabaseHandler:               databaseHandler,
		MaxMigrationAttempts:          5,
		MigrationAttemptSleepDuration: time.Second,
		AcquireSubnetLeaseAttempts:    10,
		CIDRPool:                      lib.NewCIDRPool(cfg.SubnetRange, cfg.SubnetMask),
		UnderlayIP:                    cfg.UnderlayIP,
		Logger:                        lager.NewLogger("silk-daemon"),
	}
	if err := leaseController.TryMigrations(); err != nil {
		panic(err)
	}

	subnet, err := leaseController.AcquireSubnetLease()
	if err != nil {
		panic(err)
	}

	fmt.Printf("acquired subnet %s for underlay ip %s\n", subnet, cfg.UnderlayIP)

	for {
		time.Sleep(10 * time.Second)
	}
}
