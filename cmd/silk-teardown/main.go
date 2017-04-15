package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"code.cloudfoundry.org/go-db-helpers/mutualtls"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/daemon/vtep"
	"code.cloudfoundry.org/silk/lib/adapter"
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
		return fmt.Errorf("load config file: %s", err)
	}

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

	localLease, err := discoverLocalLease(cfg)
	if err != nil {
		return fmt.Errorf("discover local lease: %s", err) // UNTESTED
	}
	err = client.ReleaseSubnetLease(localLease)
	if err != nil {
		return fmt.Errorf("release subnet lease: %s", err)
	}

	return err
}

func discoverLocalLease(clientConfig config.Config) (controller.Lease, error) {
	vtepFactory := &vtep.Factory{
		NetlinkAdapter: &adapter.NetlinkAdapter{},
	}
	overlayHwAddr, overlayIP, err := vtepFactory.GetVTEPState(clientConfig.VTEPName)
	if err != nil {
		return controller.Lease{}, fmt.Errorf("get vtep overlay ip: %s", err)
	}
	overlaySubnet := &net.IPNet{
		IP:   overlayIP,
		Mask: net.CIDRMask(clientConfig.SubnetMask, 32),
	}
	return controller.Lease{
		UnderlayIP:          clientConfig.UnderlayIP,
		OverlaySubnet:       overlaySubnet.String(),
		OverlayHardwareAddr: overlayHwAddr.String(),
	}, nil
}
