package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"code.cloudfoundry.org/cf-networking-helpers/db"
	"code.cloudfoundry.org/cf-networking-helpers/httperror"
	"code.cloudfoundry.org/cf-networking-helpers/marshal"
	"code.cloudfoundry.org/cf-networking-helpers/metrics"
	"code.cloudfoundry.org/cf-networking-helpers/mutualtls"
	"code.cloudfoundry.org/debugserver"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller/config"
	"code.cloudfoundry.org/silk/controller/database"
	"code.cloudfoundry.org/silk/controller/handlers"
	"code.cloudfoundry.org/silk/controller/leaser"
	"code.cloudfoundry.org/silk/controller/server_metrics"
	"github.com/cloudfoundry/dropsonde"
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
	flag.StringVar(&configFilePath, "config", "", "path to config file")
	flag.Parse()

	conf, err := config.ReadFromFile(configFilePath)
	if err != nil {
		return fmt.Errorf("loading config: %s", err)
	}
	logger.Info("parsed-config")

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
	logger.Info("db-connection-established")

	databaseHandler := database.NewDatabaseHandler(&database.MigrateAdapter{}, sqlDB)
	cidrPool := leaser.NewCIDRPool(conf.Network, conf.SubnetPrefixLength)
	leaseController := &leaser.LeaseController{
		DatabaseHandler:            databaseHandler,
		HardwareAddressGenerator:   &leaser.HardwareAddressGenerator{},
		LeaseValidator:             &leaser.LeaseValidator{},
		AcquireSubnetLeaseAttempts: 10,
		CIDRPool:                   cidrPool,
		LeaseExpirationSeconds:     conf.LeaseExpirationSeconds,
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

	errorResponse := &httperror.ErrorResponse{
		Logger:        logger,
		MetricsSender: &metrics.NoOpMetricsSender{},
	}

	leasesIndex := &handlers.LeasesIndex{
		Logger:          logger,
		Marshaler:       marshal.MarshalFunc(json.Marshal),
		LeaseRepository: leaseController,
		ErrorResponse:   errorResponse,
	}

	leasesAcquire := &handlers.LeasesAcquire{
		Logger:        logger,
		Marshaler:     marshal.MarshalFunc(json.Marshal),
		Unmarshaler:   marshal.UnmarshalFunc(json.Unmarshal),
		LeaseAcquirer: leaseController,
		ErrorResponse: errorResponse,
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

	metronAddress := fmt.Sprintf("127.0.0.1:%d", conf.MetronPort)
	err = dropsonde.Initialize(metronAddress, "silk-controller")
	if err != nil {
		return fmt.Errorf("initializing dropsonde: %s", err)
	}

	logger.Info("starting-servers")
	httpServer := http_server.NewTLSServer(mainServerAddress, router, tlsConfig)

	// Metrics sources
	uptimeSource := metrics.NewUptimeSource()
	totalLeasesSource := server_metrics.NewTotalLeasesSource(databaseHandler)
	freeLeasesSource := server_metrics.NewFreeLeasesSource(databaseHandler, cidrPool)
	staleLeasesSource := server_metrics.NewStaleLeasesSource(databaseHandler, conf.StalenessThresholdSeconds)

	metricsEmitter := metrics.NewMetricsEmitter(logger, time.Duration(conf.MetricsEmitSeconds)*time.Second, uptimeSource, totalLeasesSource, freeLeasesSource, staleLeasesSource)
	members := grouper.Members{
		{"http_server", httpServer},
		{"debug-server", debugserver.Runner(debugServerAddress, reconfigurableSink)},
		{"metrics-emitter", metricsEmitter},
	}

	group := grouper.NewOrdered(os.Interrupt, members)
	monitor := ifrit.Invoke(sigmon.New(group))

	logger.Info("running")
	err = <-monitor.Wait()
	if err != nil {
		return fmt.Errorf("wait returned error: %s", err)
	}

	logger.Info("exited")
	return nil
}
