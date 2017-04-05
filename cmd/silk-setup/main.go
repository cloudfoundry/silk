package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"time"

	"code.cloudfoundry.org/go-db-helpers/db"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/client/state"
	"code.cloudfoundry.org/silk/daemon/lib"
)

func main() {
	configFilePath := flag.String("config", "", "path to config file")
	flag.Parse()

	contents, err := ioutil.ReadFile(*configFilePath)
	if err != nil {
		log.Fatalf("could not read config file %s: %s", *configFilePath, err)
	}

	var cfg config.Config
	err = json.Unmarshal(contents, &cfg)
	if err != nil {
		log.Fatalf("could not unmarshal config file contents")
	}

	os.Remove(cfg.LocalStateFile)

	sqlDB, err := db.GetConnectionPool(cfg.Database)
	if err != nil {
		log.Fatalf("could not connect to database: %s", err)
	}

	databaseHandler := lib.NewDatabaseHandler(&lib.MigrateAdapter{}, sqlDB)

	logger := lager.NewLogger("silk-setup")
	sink := lager.NewWriterSink(os.Stdout, lager.INFO)
	logger.RegisterSink(sink)

	leaseController := lib.LeaseController{
		DatabaseHandler:               databaseHandler,
		MaxMigrationAttempts:          5,
		MigrationAttemptSleepDuration: time.Second,
		AcquireSubnetLeaseAttempts:    10,
		CIDRPool:                      lib.NewCIDRPool(cfg.SubnetRange, cfg.SubnetMask),
		UnderlayIP:                    cfg.UnderlayIP,
		Logger:                        logger,
	}
	if err = leaseController.TryMigrations(); err != nil {
		log.Fatalf("could not migrate database: %s", err) // not tested
	}

	err = leaseController.ReleaseSubnetLease()
	if err != nil {
		log.Fatalf("unable to release subnet lease: %s", err) // not tested
	}

	subnet, err := leaseController.AcquireSubnetLease()
	if err != nil {
		log.Fatalf("could not acquire subnet: %s", err) // not tested
	}

	subnetLease := state.SubnetLease{
		Subnet:     subnet,
		UnderlayIP: cfg.UnderlayIP,
	}

	subnetLeaseBytes, err := json.Marshal(subnetLease)
	if err != nil {
		log.Fatalf("could not marshal subnet lease: %s", err) // not tested
	}

	err = ioutil.WriteFile(cfg.LocalStateFile, subnetLeaseBytes, 0644)
	if err != nil {
		log.Fatalf("could not write local state file: %s", err)
	}
}
