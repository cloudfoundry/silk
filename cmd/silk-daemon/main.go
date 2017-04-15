package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"code.cloudfoundry.org/go-db-helpers/json_client"
	"code.cloudfoundry.org/go-db-helpers/mutualtls"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/client/state"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/leaser"
	"code.cloudfoundry.org/silk/daemon/vtep"
	"code.cloudfoundry.org/silk/lib/adapter"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
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
		return fmt.Errorf("load config file: %s", err)
	}

	tlsConfig, err := mutualtls.NewClientTLSConfig(cfg.ClientCertFile, cfg.ClientKeyFile, cfg.ServerCACertFile)
	if err != nil {
		return fmt.Errorf("create tls config: %s", err) // TODO not tested - see teardown
	}
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	client := controller.NewClient(logger, httpClient, cfg.ConnectivityServerURL)
	leaseResponse, err := client.AcquireSubnetLease(cfg.UnderlayIP)
	if err != nil {
		httpError, isHttpError := err.(*json_client.HttpResponseCodeError)
		if isHttpError && httpError.StatusCode == http.StatusInternalServerError {
			return fmt.Errorf("acquire subnet lease: %d", httpError.StatusCode)
		}
		return fmt.Errorf("acquire subnet lease: %s", err)
	}
	lease := state.SubnetLease{
		UnderlayIP: leaseResponse.UnderlayIP,
		Subnet:     leaseResponse.OverlaySubnet,
	}
	logger.Info("acquired-lease", lager.Data{"lease": lease})

	if cfg.HealthCheckPort == 0 {
		return fmt.Errorf("invalid health check port: %d", cfg.HealthCheckPort)
	}

	vtepConfigCreator := &vtep.ConfigCreator{
		NetAdapter:               &adapter.NetAdapter{},
		HardwareAddressGenerator: &leaser.HardwareAddressGenerator{},
	}
	vtepConf, err := vtepConfigCreator.Create(cfg, lease)
	if err != nil {
		return fmt.Errorf("create vtep config: %s", err)
	}

	vtepFactory := &vtep.Factory{
		NetlinkAdapter: &adapter.NetlinkAdapter{},
	}

	err = vtepFactory.CreateVTEP(vtepConf)
	if err != nil {
		return fmt.Errorf("create vtep: %s", err)
	}

	healthCheckServer, err := buildHealthCheckServer(cfg.HealthCheckPort, lease)
	if err != nil {
		return fmt.Errorf("create health check server: %s", err) // not tested
	}

	members := grouper.Members{
		{"server", healthCheckServer},
	}
	group := grouper.NewOrdered(os.Interrupt, members)
	monitor := ifrit.Invoke(sigmon.New(group))

	err = <-monitor.Wait()
	return err
}

func buildHealthCheckServer(healthCheckPort uint16, lease state.SubnetLease) (ifrit.Runner, error) {
	leaseBytes, err := json.Marshal(lease)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling lease: %s", err) // not possible
	}

	return http_server.New(
		fmt.Sprintf("127.0.0.1:%d", healthCheckPort),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write(leaseBytes)
		}),
	), nil
}
