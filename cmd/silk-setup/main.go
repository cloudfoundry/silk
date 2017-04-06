package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/client/state"
	"code.cloudfoundry.org/silk/daemon/lib"
)

func main() {
	if err := mainWithError(); err != nil {
		log.Fatalf("silk-setup error: %s", err)
	}
}

func mainWithError() error {
	logger := lager.NewLogger("silk-setup")
	sink := lager.NewWriterSink(os.Stdout, lager.INFO)
	logger.RegisterSink(sink)

	configFilePath := flag.String("config", "", "path to config file")
	flag.Parse()
	cfg, err := config.LoadConfig(*configFilePath)
	if err != nil {
		return fmt.Errorf("loading config: %s", err) // not tested
	}

	os.Remove(cfg.LocalStateFile)

	leaseController, err := lib.NewLeaseController(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating lease contoller: %s", err) // not tested
	}

	err = leaseController.ReleaseSubnetLease()
	if err != nil {
		return fmt.Errorf("releasing subnet lease: %s", err) // not tested
	}

	subnet, err := leaseController.AcquireSubnetLease()
	if err != nil {
		return fmt.Errorf("acquiring subnet: %s", err) // not tested
	}

	subnetLease := state.SubnetLease{
		Subnet:     subnet,
		UnderlayIP: cfg.UnderlayIP,
	}

	subnetLeaseBytes, err := json.Marshal(subnetLease)
	if err != nil {
		return fmt.Errorf("marshaling subnet lease: %s", err) // not tested
	}

	err = ioutil.WriteFile(cfg.LocalStateFile, subnetLeaseBytes, 0644)
	if err != nil {
		return fmt.Errorf("writing local state file: %s", err)
	}
	return nil
}
