package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"strconv"
	"time"

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

	leaseController := LeaseController{
		DatabaseHandler:               databaseHandler,
		MaxMigrationAttempts:          5,
		MigrationAttemptSleepDuration: time.Second,
	}
	if err := leaseController.TryMigrations(); err != nil {
		panic(err)
	}

	subnet, err := leaseController.AcquireSubnetLease(cfg)
	if err != nil {
		panic(err)
	}

	fmt.Printf("acquired subnet %s for underlay ip %s", subnet, cfg.UnderlayIP)

	for {
		time.Sleep(10 * time.Second)
	}
}

type LeaseController struct {
	DatabaseHandler               *database.DatabaseHandler
	MaxMigrationAttempts          int
	MigrationAttemptSleepDuration time.Duration
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

func (c *LeaseController) AcquireSubnetLease(cfg config.Config) (string, error) {
	subnet, err := getFreeSubnet(c.DatabaseHandler, cfg)
	if err != nil {
		panic(err)
	}

	return subnet, c.DatabaseHandler.AddEntry(cfg.UnderlayIP, subnet)
}

func getFreeSubnet(databaseHandler *database.DatabaseHandler, cfg config.Config) (string, error) {
	i := 0
	for {
		subnet, err := getSubnet(cfg, i)
		if err != nil {
			panic(err)
		}

		subnetExists, err := databaseHandler.EntryExists("subnet", subnet)
		if err != nil {
			panic(err)
		}

		if !subnetExists {
			return subnet, nil
		}
		i++
	}

}

func getSubnet(cfg config.Config, index int) (string, error) {
	ip, ipCIDR, err := net.ParseCIDR(cfg.SubnetRange)
	if err != nil {
		panic(err)
	}
	cidrMask, _ := ipCIDR.Mask.Size()
	cidrMaskBlock, err := strconv.Atoi(cfg.SubnetMask)
	if err != nil {
		panic(err)
	}
	pool := lib.NewCIDRPool(ip.String(), uint(cidrMask), uint(cidrMaskBlock))

	return pool.Get(index)
}
