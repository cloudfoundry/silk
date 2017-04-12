package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/controller/lib"
)

func main() {
	if err := mainWithError(); err != nil {
		log.Fatalf("silk-teardown error: %s", err)
	}
}

func mainWithError() error {
	logger := lager.NewLogger("silk-teardown")
	sink := lager.NewWriterSink(os.Stdout, lager.INFO)
	logger.RegisterSink(sink)

	configFilePath := flag.String("config", "", "path to config file")
	flag.Parse()
	cfg, err := config.LoadConfig(*configFilePath)
	if err != nil {
		return fmt.Errorf("loading config: %s", err)
	}

	os.Remove(cfg.LocalStateFile)

	leaseController, err := lib.NewLeaseController(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating lease contoller: %s", err)
	}

	err = leaseController.ReleaseSubnetLease()
	if err != nil {
		return fmt.Errorf("releasing subnet lease: %s", err) // not tested
	}

	return nil
}
