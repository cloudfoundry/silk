package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/cloudfoundry-incubator/silk/daemon/config"
	"github.com/cloudfoundry-incubator/silk/daemon/database"
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

	migrator, err := database.NewMigrator(cfg.Database)
	if err != nil {
		panic(err)
	}
	fmt.Println("connected to db")

	n, err := migrator.Migrate()
	if err != nil {
		panic(err)
	}
	fmt.Printf("db migration complete: applied %d migrations.\n", n)

	for {
		time.Sleep(10 * time.Second)
	}
}
