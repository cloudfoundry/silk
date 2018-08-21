package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"code.cloudfoundry.org/cf-networking-helpers/metrics"
	"code.cloudfoundry.org/cf-networking-helpers/mutualtls"
	"code.cloudfoundry.org/debugserver"
	"code.cloudfoundry.org/filelock"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagerflags"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/daemon"
	"code.cloudfoundry.org/silk/daemon/planner"
	"code.cloudfoundry.org/silk/daemon/poller"
	"code.cloudfoundry.org/silk/daemon/vtep"
	"code.cloudfoundry.org/silk/lib/adapter"
	"code.cloudfoundry.org/silk/lib/datastore"
	"code.cloudfoundry.org/silk/lib/serial"

	"github.com/cloudfoundry/dropsonde"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

var logPrefix = "cfnetworking"

const (
	jobPrefix = "silk-daemon"
)

func main() {
	if err := mainWithError(); err != nil {
		log.Fatalf("%s.silk-daemon error: %s", logPrefix, err)
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

	logger, reconfigurableSink := lagerflags.NewFromConfig(fmt.Sprintf("%s.%s", logPrefix, jobPrefix), getLagerConfig())
	logger.Info("starting")

	tlsConfig, err := mutualtls.NewClientTLSConfig(cfg.ClientCertFile, cfg.ClientKeyFile, cfg.ServerCACertFile)
	if err != nil {
		return fmt.Errorf("create tls config: %s", err)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: time.Duration(cfg.ClientTimeoutSeconds) * time.Second,
	}

	metronAddress := fmt.Sprintf("127.0.0.1:%d", cfg.MetronPort)
	err = dropsonde.Initialize(metronAddress, "silk-daemon")
	if err != nil {
		return fmt.Errorf("initializing dropsonde: %s", err)
	}
	metricSender := &metrics.MetricsSender{
		Logger: logger,
	}

	vtepFactory := &vtep.Factory{
		NetlinkAdapter: &adapter.NetlinkAdapter{},
	}
	vtepConfigCreator := &vtep.ConfigCreator{
		NetAdapter: &adapter.NetAdapter{},
	}

	client := controller.NewClient(logger, httpClient, cfg.ConnectivityServerURL)

	store := &datastore.Store{
		Serializer: &serial.Serial{},
		LockerNew:  filelock.NewLocker,
	}

	_, overlayNetwork, err := net.ParseCIDR(cfg.OverlayNetwork)
	if err != nil {
		return fmt.Errorf("parse overlay network CIDR: %s", err) //TODO add test coverage
	}

	lease, err := discoverLocalLease(cfg, vtepFactory)
	if err != nil {
		lease, err = acquireLease(logger, client, vtepConfigCreator, vtepFactory, cfg)
		if err != nil {
			return err
		}
	} else {
		_, localSubnet, err := net.ParseCIDR(lease.OverlaySubnet)
		if err != nil {
			return fmt.Errorf("parse local subnet CIDR: %s", err) //TODO add test coverage
		}

		if !overlayNetwork.Contains(localSubnet.IP) {
			logger.Error("network-contains-lease", fmt.Errorf("discovered lease is not in overlay network"), lager.Data{
				"lease":   lease,
				"network": cfg.OverlayNetwork,
			})

			metadata, err := store.ReadAll(cfg.Datastore)
			if err != nil {
				return fmt.Errorf("read datastore: %s", err)
			}

			if len(metadata) != 0 {
				return fmt.Errorf("discovered lease is not in overlay network and has containers: %d", len(metadata))
			} else {
				lease, err = deleteAndAcquire(cfg, logger, client, vtepConfigCreator, vtepFactory)
				if err != nil {
					return err
				}
			}
		}

		err = client.RenewSubnetLease(lease)
		if err != nil {
			logger.Error("renew-lease", err, lager.Data{"lease": lease})

			metadata, err := store.ReadAll(cfg.Datastore)
			if err != nil {
				return fmt.Errorf("read datastore: %s", err)
			}

			if len(metadata) != 0 {
				return fmt.Errorf("renew subnet lease with containers: %d", len(metadata))
			} else {
				lease, err = deleteAndAcquire(cfg, logger, client, vtepConfigCreator, vtepFactory)
				if err != nil {
					return err
				}
			}
		}
		logger.Info("renewed-lease", lager.Data{"lease": lease})
	}

	debugServerAddress := fmt.Sprintf("127.0.0.1:%d", cfg.DebugServerPort)
	networkInfo, err := getNetworkInfo(vtepFactory, cfg, lease)
	if err != nil {
		return fmt.Errorf("get network info: %s", err) // not tested
	}

	healthCheckServer, err := buildHealthCheckServer(cfg.HealthCheckPort, networkInfo)
	if err != nil {
		return fmt.Errorf("create health check server: %s", err) // not tested
	}

	_, localSubnet, err := net.ParseCIDR(lease.OverlaySubnet)
	if err != nil {
		return fmt.Errorf("parse local subnet CIDR: %s", err) //TODO add test coverage
	}

	vxlanIface, err := net.InterfaceByName(cfg.VTEPName)
	if err != nil || vxlanIface == nil {
		return fmt.Errorf("find local VTEP: %s", err) //TODO add test coverage
	}

	vxlanPoller := &poller.Poller{
		Logger:       logger,
		PollInterval: time.Duration(cfg.PollInterval) * time.Second,
		SingleCycleFunc: (&planner.VXLANPlanner{
			Logger:           logger,
			ControllerClient: client,
			Lease:            lease,
			Converger: &vtep.Converger{
				OverlayNetwork: overlayNetwork,
				LocalSubnet:    localSubnet,
				LocalVTEP:      *vxlanIface,
				NetlinkAdapter: &adapter.NetlinkAdapter{},
				Logger:         logger,
			},
			ErrorDetector: planner.NewGracefulDetector(
				time.Duration(cfg.PartitionToleranceSeconds) * time.Second,
			),
			MetricSender: metricSender,
		}).DoCycle,
	}

	uptimeSource := metrics.NewUptimeSource()
	metricsEmitter := metrics.NewMetricsEmitter(logger, 30*time.Second, uptimeSource)
	members := grouper.Members{
		{"server", healthCheckServer},
		{"vxlan-poller", vxlanPoller},
		{"debug-server", debugserver.Runner(debugServerAddress, reconfigurableSink)},
		{"metrics-emitter", metricsEmitter},
	}
	group := grouper.NewOrdered(os.Interrupt, members)
	monitor := ifrit.Invoke(sigmon.New(group))

	err = <-monitor.Wait()
	return err
}

func acquireLease(logger lager.Logger, client *controller.Client, vtepConfigCreator *vtep.ConfigCreator, vtepFactory *vtep.Factory, cfg config.Config) (controller.Lease, error) {
	var lease controller.Lease
	if cfg.SingleIPOnly {
		var err error
		lease, err = client.AcquireSingleOverlayIPLease(cfg.UnderlayIP)
		if err != nil {
			return controller.Lease{}, fmt.Errorf("acquire subnet lease: %s", err)
		}
	} else {
		var err error
		lease, err = client.AcquireSubnetLease(cfg.UnderlayIP)
		if err != nil {
			return controller.Lease{}, fmt.Errorf("acquire subnet lease: %s", err)
		}
	}
	logger.Info("acquired-lease", lager.Data{"lease": lease})

	if cfg.HealthCheckPort == 0 {
		return controller.Lease{}, fmt.Errorf("invalid health check port: %d", cfg.HealthCheckPort)
	}

	vtepConf, err := vtepConfigCreator.Create(cfg, lease)
	if err != nil {
		return controller.Lease{}, fmt.Errorf("create vtep config: %s", err) // not tested
	}

	err = vtepFactory.CreateVTEP(vtepConf)
	if err != nil {
		return controller.Lease{}, fmt.Errorf("create vtep: %s", err) // not tested
	}

	return lease, nil
}

func buildHealthCheckServer(healthCheckPort uint16, networkInfo daemon.NetworkInfo) (ifrit.Runner, error) {
	networkBytes, err := json.Marshal(networkInfo)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling network info: %s", err) // not possible
	}

	return http_server.New(
		fmt.Sprintf("127.0.0.1:%d", healthCheckPort),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write(networkBytes)
		}),
	), nil
}

func discoverLocalLease(clientConfig config.Config, vtepFactory *vtep.Factory) (controller.Lease, error) {
	overlayHwAddr, overlayIP, _, err := vtepFactory.GetVTEPState(clientConfig.VTEPName)
	if err != nil {
		return controller.Lease{}, fmt.Errorf("get vtep overlay ip: %s", err) // not tested
	}
	return leaseFromVTEPState(clientConfig, overlayHwAddr, overlayIP), nil
}

func leaseFromVTEPState(clientConfig config.Config, overlayHwAddr net.HardwareAddr, overlayIP net.IP) controller.Lease {
	overlaySubnet := &net.IPNet{
		IP:   overlayIP,
		Mask: net.CIDRMask(clientConfig.SubnetPrefixLength, 32),
	}
	return controller.Lease{
		UnderlayIP:          clientConfig.UnderlayIP,
		OverlaySubnet:       overlaySubnet.String(),
		OverlayHardwareAddr: overlayHwAddr.String(),
	}
}

func getNetworkInfo(vtepFactory *vtep.Factory, clientConfig config.Config, lease controller.Lease) (daemon.NetworkInfo, error) {
	_, _, mtu, err := vtepFactory.GetVTEPState(clientConfig.VTEPName)
	if err != nil {
		return daemon.NetworkInfo{}, fmt.Errorf("get vtep mtu: %s", err) // not tested
	}

	return daemon.NetworkInfo{
		OverlaySubnet: lease.OverlaySubnet,
		MTU:           mtu,
	}, nil
}

func deleteAndAcquire(cfg config.Config, logger lager.Logger, client *controller.Client, vtepConfigCreator *vtep.ConfigCreator, vtepFactory *vtep.Factory) (controller.Lease, error) {
	err := vtepFactory.DeleteVTEP(cfg.VTEPName)
	if err != nil {
		return controller.Lease{}, fmt.Errorf("delete vtep: %s", err) // not tested, should be impossible
	}
	return acquireLease(logger, client, vtepConfigCreator, vtepFactory, cfg)
}

func getLagerConfig() lagerflags.LagerConfig {
	lagerConfig := lagerflags.DefaultLagerConfig()
	lagerConfig.TimeFormat = lagerflags.FormatRFC3339
	return lagerConfig
}
