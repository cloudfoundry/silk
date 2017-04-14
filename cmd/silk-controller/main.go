package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"code.cloudfoundry.org/debugserver"
	"code.cloudfoundry.org/go-db-helpers/db"
	"code.cloudfoundry.org/go-db-helpers/httperror"
	"code.cloudfoundry.org/go-db-helpers/marshal"
	"code.cloudfoundry.org/go-db-helpers/metrics"
	"code.cloudfoundry.org/go-db-helpers/mutualtls"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller/config"
	"code.cloudfoundry.org/silk/controller/database"
	"code.cloudfoundry.org/silk/controller/handlers"
	"code.cloudfoundry.org/silk/controller/leaser"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
	"github.com/tedsuo/rata"
)

func main() {
	if err := mainWithError(); err != nil {
		log.Fatalf("silk-controller error: %s", err)
	}
}

func mainWithError() error {
	logger := lager.NewLogger("silk-controller")
	reconfigurableSink := lager.NewReconfigurableSink(
		lager.NewWriterSink(os.Stdout, lager.DEBUG),
		lager.INFO)
	logger.RegisterSink(reconfigurableSink)
	logger.Info("starting")

	var configFilePath string
	flag.StringVar(&configFilePath, "config-file", "", "path to config file")
	flag.Parse()

	conf, err := config.ReadFromFile(configFilePath)
	if err != nil {
		return fmt.Errorf("loading config: %s", err)
	}

	debugServerAddress := fmt.Sprintf("127.0.0.1:%d", conf.DebugServerPort)
	mainServerAddress := fmt.Sprintf("%s:%d", conf.ListenHost, conf.ListenPort)
	tlsConfig, err := mutualtls.NewServerTLSConfig(conf.ServerCertFile, conf.ServerKeyFile, conf.CACertFile)
	if err != nil {
		return fmt.Errorf("mutual tls config: %s", err)
	}

	sqlDB, err := db.GetConnectionPool(conf.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %s", err)
	}

	databaseHandler := database.NewDatabaseHandler(&database.MigrateAdapter{}, sqlDB)
	leaseController := &leaser.LeaseController{
		DatabaseHandler:            databaseHandler,
		HardwareAddressGenerator:   &leaser.HardwareAddressGenerator{},
		LeaseValidator:             &leaser.LeaseValidator{},
		AcquireSubnetLeaseAttempts: 10,
		CIDRPool:                   leaser.NewCIDRPool(conf.Network, conf.SubnetPrefixLength),
		Logger:                     logger,
	}
	migrator := &database.Migrator{
		DatabaseMigrator:              databaseHandler,
		MaxMigrationAttempts:          5,
		MigrationAttemptSleepDuration: time.Second,
		Logger: logger,
	}
	if err = migrator.TryMigrations(); err != nil {
		return fmt.Errorf("migrating database: %s", err)
	}

	leasesIndex := &handlers.LeasesIndex{
		Logger:          logger,
		Marshaler:       marshal.MarshalFunc(json.Marshal),
		LeaseRepository: leaseController,
	}

	leasesAcquire := &handlers.LeasesAcquire{
		Logger:        logger,
		Marshaler:     marshal.MarshalFunc(json.Marshal),
		Unmarshaler:   marshal.UnmarshalFunc(json.Unmarshal),
		LeaseAcquirer: leaseController,
	}

	errorResponse := &httperror.ErrorResponse{
		Logger:        logger,
		MetricsSender: &metrics.NoOpMetricsSender{},
	}

	leasesRelease := &handlers.ReleaseLease{
		Logger:        logger,
		Marshaler:     marshal.MarshalFunc(json.Marshal),
		Unmarshaler:   marshal.UnmarshalFunc(json.Unmarshal),
		LeaseReleaser: leaseController,
		ErrorResponse: errorResponse,
	}

	leasesRenew := &handlers.RenewLease{
		Logger:        logger,
		Unmarshaler:   marshal.UnmarshalFunc(json.Unmarshal),
		LeaseRenewer:  leaseController,
		ErrorResponse: errorResponse,
	}

	router, err := rata.NewRouter(
		rata.Routes{
			{Name: "leases-index", Method: "GET", Path: "/leases"},
			{Name: "leases-acquire", Method: "PUT", Path: "/leases/acquire"},
			{Name: "leases-release", Method: "PUT", Path: "/leases/release"},
			{Name: "leases-renew", Method: "PUT", Path: "/leases/renew"},
		},
		rata.Handlers{
			"leases-index":   leasesIndex,
			"leases-acquire": leasesAcquire,
			"leases-release": leasesRelease,
			"leases-renew":   leasesRenew,
		},
	)
	if err != nil {
		return fmt.Errorf("creating router: %s", err)
	}

	httpServer := http_server.NewTLSServer(mainServerAddress, router, tlsConfig)
	members := grouper.Members{
		{"http_server", httpServer},
		{"debug-server", debugserver.Runner(debugServerAddress, reconfigurableSink)},
	}

	group := grouper.NewOrdered(os.Interrupt, members)
	monitor := ifrit.Invoke(sigmon.New(group))

	err = <-monitor.Wait()
	if err != nil {
		return fmt.Errorf("wait returned error: %s", err)
	}

	logger.Info("exited")
	return nil
}
