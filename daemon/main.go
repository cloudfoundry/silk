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

	nErrors := 0
	for nErrors < 5 {
		var n int
		n, err = databaseHandler.Migrate()
		if err == nil {
			fmt.Printf("db migration complete: applied %d migrations.\n", n)
			break
		}

		nErrors++
		time.Sleep(time.Second)
	}
	if err != nil {
		panic(err)
	}

	entryAdded := false
	i := 0
	for !entryAdded {
		subnet, err := getSubnet(cfg, i)
		if err != nil {
			panic(err)
		}

		subnetExists, err := databaseHandler.EntryExists("subnet", subnet)
		if err != nil {
			panic(err)
		}
		fmt.Printf("found the subnet exist is: %+v", subnetExists)

		if !subnetExists {
			err = databaseHandler.AddEntry(cfg.UnderlayIP, subnet)
			if err != nil {
				panic(err)
			}
			entryAdded = true
		}
		i++
	}
	fmt.Printf("set subnet for vm")

	for {
		time.Sleep(10 * time.Second)
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
