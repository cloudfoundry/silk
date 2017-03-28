package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"time"

	"code.cloudfoundry.org/lager"

	"github.com/cloudfoundry-incubator/silk/daemon/config"
	"github.com/cloudfoundry-incubator/silk/daemon/lib"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
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

	databaseHandler, err := lib.NewDatabaseHandler(cfg.Database)
	if err != nil {
		log.Fatalf("could not create database handler: %s", err)
	}

	logger := lager.NewLogger("silk-daemon")
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
		log.Fatalf("could not migrate database: %s", err)
	}

	_, err = leaseController.AcquireSubnetLease()
	if err != nil {
		log.Fatalf("could not acquire subnet: %s", err)
	}

	for {
		time.Sleep(10 * time.Second)
	}
}
