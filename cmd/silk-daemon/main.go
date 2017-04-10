package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/client/state"
	"code.cloudfoundry.org/silk/daemon/lib"

	"github.com/containernetworking/cni/pkg/utils/hwaddr"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
	"github.com/vishvananda/netlink"
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

	lease, err := state.LoadSubnetLease(cfg.LocalStateFile)
	if err != nil {
		return fmt.Errorf("loading state file: %s", err)
	}

	_, err = lib.NewLeaseController(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating lease controller: %s", err)
	}

	leaseBytes, err := json.Marshal(lease)
	if err != nil {
		return fmt.Errorf("unmarshaling lease: %s", err) // not possible
	}

	if cfg.HealthCheckPort == 0 {
		return fmt.Errorf("invalid healthcheck port: %d", cfg.HealthCheckPort)
	}

	vtepDeviceName := "silk-vxlan"
	overlayIP, _, err := net.ParseCIDR(lease.Subnet)
	if err != nil {
		return fmt.Errorf("cannot parse subnetlease: %d", cfg.HealthCheckPort) // not tested
	}

	localIP := net.ParseIP(cfg.UnderlayIP)
	if localIP == nil {
		return fmt.Errorf("could not parse underlay ip: %s", cfg.UnderlayIP) // not tested
	}

	device, err := locateInterface(localIP)
	if err != nil {
		return fmt.Errorf("could not find device from ip %s: %s", localIP, err) // not tested
	}

	vxlan := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: vtepDeviceName,
		},
		VxlanId:      42,
		SrcAddr:      localIP,
		GBP:          true,
		Port:         4789,
		VtepDevIndex: device.Index,
	}
	err = netlink.LinkAdd(vxlan)
	if err != nil {
		return fmt.Errorf("cannot create link") // not tested
	}
	err = netlink.LinkSetUp(vxlan)
	if err != nil {
		return fmt.Errorf("cannot up link") // not tested
	}

	macAddr, err := hwaddr.GenerateHardwareAddr4(overlayIP, []byte{0xee, 0xee}) // not tested
	if err != nil {
		return fmt.Errorf("cannot generate MAC address: %d", cfg.HealthCheckPort) // not tested
	}

	err = netlink.LinkSetHardwareAddr(vxlan, macAddr)
	if err != nil {
		return fmt.Errorf("cannot set MAC address") // not tested
	}

	server := http_server.New(
		fmt.Sprintf("127.0.0.1:%d", cfg.HealthCheckPort),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write(leaseBytes)
		}),
	)

	members := grouper.Members{
		{"server", server},
	}
	group := grouper.NewOrdered(os.Interrupt, members)
	monitor := ifrit.Invoke(sigmon.New(group))

	err = <-monitor.Wait()
	return err
}

func locateInterface(toFind net.IP) (net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, err
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return net.Interface{}, err
		}

		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				return net.Interface{}, err
			}
			if ip.String() == toFind.String() {
				return iface, nil
			}
		}
	}

	return net.Interface{}, fmt.Errorf("no interface with address %s", toFind.String())
}
