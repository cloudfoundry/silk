package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/hashicorp/go-multierror"

	"code.cloudfoundry.org/cf-networking-helpers/mutualtls"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagerflags"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/daemon/vtep"
	"code.cloudfoundry.org/silk/lib/adapter"
)

const (
	jobPrefix = "silk-teardown"
)

var logPrefix = "cfnetworking"

func main() {
	if err := mainWithError(); err != nil {
		log.Fatalf("silk-teardown error: %s", err)
	}
}

func mainWithError() error {
	configFilePath := flag.String("config", "", "path to config file")
	flag.Parse()
	cfg, err := config.LoadConfig(*configFilePath)
	if err != nil {
		return fmt.Errorf("load config file: %s", err)
	}

	if cfg.LogPrefix != "" {
		logPrefix = cfg.LogPrefix
	}

	logger, _ := lagerflags.NewFromConfig(fmt.Sprintf("%s.%s", logPrefix, jobPrefix), getLagerConfig())
	logger.Info("starting")

	tlsConfig, err := mutualtls.NewClientTLSConfig(cfg.ClientCertFile, cfg.ClientKeyFile, cfg.ServerCACertFile)
	if err != nil {
		return fmt.Errorf("create tls config: %s", err)
	}
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
	client := controller.NewClient(logger, httpClient, cfg.ConnectivityServerURL)

	var errList error
	if err := client.ReleaseSubnetLease(cfg.UnderlayIP); err != nil {
		errList = multierror.Append(errList, fmt.Errorf("release subnet lease: %s", err))
		logger.Error("release-subnet-lease", err, lager.Data{"underlay_ip": cfg.UnderlayIP})
	}

	vtepFactory := &vtep.Factory{NetlinkAdapter: &adapter.NetlinkAdapter{}}
	if err := vtepFactory.DeleteVTEP(cfg.VTEPName); err != nil {
		errList = multierror.Append(errList, fmt.Errorf("delete vtep: %s", err))
		logger.Error("delete-vtep", err, lager.Data{"vtep_name": cfg.VTEPName})
	}

	logger.Info("complete")

	return errList
}

func getLagerConfig() lagerflags.LagerConfig {
	lagerConfig := lagerflags.DefaultLagerConfig()
	lagerConfig.TimeFormat = lagerflags.FormatRFC3339
	return lagerConfig
}
