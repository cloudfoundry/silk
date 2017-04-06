package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/client/state"
	"code.cloudfoundry.org/silk/daemon/lib"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

func main() {
	if err := mainWithError(); err != nil {
		log.Fatalf("silk-daemon error: %s", err)
	}
}

func mainWithError() error {
	logger := lager.NewLogger("silk-daemon")
	sink := lager.NewWriterSink(os.Stdout, lager.INFO)
	logger.RegisterSink(sink)

	configFilePath := flag.String("config", "", "path to config file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configFilePath)
	if err != nil {
		return fmt.Errorf("loading config file: %s", err)
	}

	_, err = state.LoadSubnetLease(cfg.LocalStateFile)
	if err != nil {
		return fmt.Errorf("loading state file: %s", err)
	}

	_, err = lib.NewLeaseController(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating lease controller: %s", err)
	}

	return nil
}
