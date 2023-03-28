package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"code.cloudfoundry.org/cf-networking-helpers/db"
	"code.cloudfoundry.org/cf-networking-helpers/httperror"
	"code.cloudfoundry.org/cf-networking-helpers/marshal"
	"code.cloudfoundry.org/cf-networking-helpers/metrics"
	"code.cloudfoundry.org/cf-networking-helpers/middleware"
	"code.cloudfoundry.org/cf-networking-helpers/mutualtls"
	"code.cloudfoundry.org/debugserver"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagerflags"
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

var (
	jobPrefix = "silk-controller"
	logPrefix = "cfnetworking"
)

func main() {
	if err := mainWithError(); err != nil {
		log.Fatalf("%s.silk-controller error: %s", logPrefix, err)
	}
}

func mainWithError() error {
	var configFilePath string
	flag.StringVar(&configFilePath, "config", "", "path to config file")
	flag.Parse()

	conf, err := config.ReadFromFile(configFilePath)
	if err != nil {
		return fmt.Errorf("load config: %s", err)
	}

	if conf.LogPrefix != "" {
		logPrefix = conf.LogPrefix
	}

	logger, reconfigurableSink := lagerflags.NewFromConfig(fmt.Sprintf("%s.%s", logPrefix, jobPrefix), getLagerConfig())
	logger.Info("starting")

	logger.Info("parsed-config")

	debugServerAddress := fmt.Sprintf("127.0.0.1:%d", conf.DebugServerPort)
	mainServerAddress := fmt.Sprintf("%s:%d", conf.ListenHost, conf.ListenPort)
	healthServerAddress := fmt.Sprintf("127.0.0.1:%d", conf.HealthCheckPort)
	tlsConfig, err := mutualtls.NewServerTLSConfig(conf.ServerCertFile, conf.ServerKeyFile, conf.CACertFile)
	if err != nil {
		return fmt.Errorf("mutual tls config: %s", err)
	}

	connectionPool, err := db.NewConnectionPool(
		conf.Database,
		conf.MaxOpenConnections,
		conf.MaxIdleConnections,
		time.Duration(conf.MaxConnectionsLifetimeSeconds)*time.Second,
		logPrefix,
		jobPrefix,
		logger,
	)
	if err != nil {
		return fmt.Errorf("connecting to database: %s", err)
	}

	databaseHandler := database.NewDatabaseHandler(&database.MigrateAdapter{}, connectionPool)
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
		Logger:                        logger,
	}
	if err = migrator.TryMigrations(); err != nil {
		return fmt.Errorf("migrating database: %s", err)
	}

	metricsSender := &metrics.MetricsSender{
		Logger: logger.Session("time-metric-emitter"),
	}

	errorResponse := &httperror.ErrorResponse{
		MetricsSender: metricsSender,
	}

	leasesIndex := &handlers.LeasesIndex{
		Marshaler:       marshal.MarshalFunc(json.Marshal),
		LeaseRepository: leaseController,
		ErrorResponse:   errorResponse,
	}

	leasesAcquire := &handlers.LeasesAcquire{
		Marshaler:     marshal.MarshalFunc(json.Marshal),
		Unmarshaler:   marshal.UnmarshalFunc(json.Unmarshal),
		LeaseAcquirer: leaseController,
		ErrorResponse: errorResponse,
	}

	leasesRelease := &handlers.ReleaseLease{
		Marshaler:     marshal.MarshalFunc(json.Marshal),
		Unmarshaler:   marshal.UnmarshalFunc(json.Unmarshal),
		LeaseReleaser: leaseController,
		ErrorResponse: errorResponse,
	}

	leasesRenew := &handlers.RenewLease{
		Unmarshaler:   marshal.UnmarshalFunc(json.Unmarshal),
		LeaseRenewer:  leaseController,
		ErrorResponse: errorResponse,
	}

	metricsWrap := func(name string, handle http.Handler) http.Handler {
		metricsWrapper := middleware.MetricWrapper{
			Name:          name,
			MetricsSender: metricsSender,
		}
		return metricsWrapper.Wrap(handle)
	}

	type loggableHandler interface {
		ServeHTTP(logger lager.Logger, w http.ResponseWriter, r *http.Request)
	}
	logWrap := func(handler loggableHandler) http.Handler {
		return handlers.LogWrap(logger, handler.ServeHTTP)
	}

	router, err := rata.NewRouter(
		rata.Routes{
			{Name: "leases-index", Method: "GET", Path: "/leases"},
			{Name: "leases-acquire", Method: "PUT", Path: "/leases/acquire"},
			{Name: "leases-release", Method: "PUT", Path: "/leases/release"},
			{Name: "leases-renew", Method: "PUT", Path: "/leases/renew"},
		},
		rata.Handlers{
			"leases-index":   metricsWrap("LeasesIndex", logWrap(leasesIndex)),
			"leases-acquire": metricsWrap("LeasesAcquire", logWrap(leasesAcquire)),
			"leases-release": metricsWrap("LeasesRelease", logWrap(leasesRelease)),
			"leases-renew":   metricsWrap("LeasesRenew", logWrap(leasesRenew)),
		},
	)
	if err != nil {
		return fmt.Errorf("creating router: %s", err)
	}

	health := &handlers.Health{
		DatabaseChecker: databaseHandler,
		ErrorResponse:   errorResponse,
	}

	healthRouter, err := rata.NewRouter(
		rata.Routes{
			{Name: "health", Method: "GET", Path: "/health"},
		},
		rata.Handlers{
			"health": metricsWrap("Health", logWrap(health)),
		},
	)
	if err != nil {
		return fmt.Errorf("creating health router: %s", err)
	}

	metronAddress := fmt.Sprintf("127.0.0.1:%d", conf.MetronPort)
	err = dropsonde.Initialize(metronAddress, "silk-controller")
	if err != nil {
		return fmt.Errorf("initializing dropsonde: %s", err)
	}

	logger.Info("starting-servers")
	httpServer := http_server.NewTLSServer(mainServerAddress, router, tlsConfig)
	healthServer := http_server.New(healthServerAddress, healthRouter)

	// Metrics sources
	metricSources := []metrics.MetricSource{
		metrics.NewUptimeSource(),
		server_metrics.NewTotalLeasesSource(databaseHandler),
		server_metrics.NewFreeLeasesSource(databaseHandler, cidrPool),
		server_metrics.NewStaleLeasesSource(databaseHandler, conf.StalenessThresholdSeconds),
	}
	metricSources = append(metricSources, metrics.NewDBMonitorSource(connectionPool, connectionPool.Monitor)...)
	metricsEmitter := metrics.NewMetricsEmitter(logger, time.Duration(conf.MetricsEmitSeconds)*time.Second, metricSources...)
	members := grouper.Members{
		{Name: "http_server", Runner: httpServer},
		{Name: "health-server", Runner: healthServer},
		{Name: "debug-server", Runner: debugserver.Runner(debugServerAddress, reconfigurableSink)},
		{Name: "metrics-emitter", Runner: metricsEmitter},
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

func getLagerConfig() lagerflags.LagerConfig {
	lagerConfig := lagerflags.DefaultLagerConfig()
	lagerConfig.TimeFormat = lagerflags.FormatRFC3339
	return lagerConfig
}
