package integration_test

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"code.cloudfoundry.org/go-db-helpers/mutualtls"
	"code.cloudfoundry.org/localip"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/daemon/vtep"
	"code.cloudfoundry.org/silk/lib/adapter"
	"code.cloudfoundry.org/silk/testsupport"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/vishvananda/netlink"
)

var (
	DEFAULT_TIMEOUT = "5s"

	localIP               string
	daemonConf            config.Config
	daemonLease           controller.Lease
	fakeServer            *testsupport.FakeController
	serverListenAddr      string
	serverTLSConfig       *tls.Config
	session               *gexec.Session
	daemonHealthCheckURL  string
	daemonDebugServerPort int
	datastorePath         string
	vtepFactory           *vtep.Factory
	vtepName              string
	vni                   int
)

var _ = BeforeEach(func() {
	var err error
	localIP, err = localip.LocalIP()
	Expect(err).NotTo(HaveOccurred())
	daemonLease = controller.Lease{
		UnderlayIP:          localIP,
		OverlaySubnet:       "10.255.30.0/24",
		OverlayHardwareAddr: "ee:ee:0a:ff:1e:00",
	}
	vni = GinkgoParallelNode()
	vtepName = fmt.Sprintf("silk-vtep-%d", GinkgoParallelNode())
	daemonHealthCheckPort := 4000 + GinkgoParallelNode()
	daemonHealthCheckURL = fmt.Sprintf("http://127.0.0.1:%d/health", daemonHealthCheckPort)
	daemonDebugServerPort = 20000 + GinkgoParallelNode()
	serverListenAddr = fmt.Sprintf("127.0.0.1:%d", 40000+GinkgoParallelNode())
	datastoreDir, err := ioutil.TempDir("", "")
	Expect(err).NotTo(HaveOccurred())
	datastorePath = filepath.Join(datastoreDir, "container-metadata.json")
	daemonConf = config.Config{
		UnderlayIP:                 localIP,
		SubnetPrefixLength:         24,
		OverlayNetworkPrefixLength: 16,
		HealthCheckPort:            uint16(daemonHealthCheckPort),
		VTEPName:                   vtepName,
		ConnectivityServerURL:      fmt.Sprintf("https://%s", serverListenAddr),
		ServerCACertFile:           paths.ServerCACertFile,
		ClientCertFile:             paths.ClientCertFile,
		ClientKeyFile:              paths.ClientKeyFile,
		VNI:                        vni,
		PollInterval:               1,
		DebugServerPort:            daemonDebugServerPort,
		Datastore:                  datastorePath,
	}

	vtepFactory = &vtep.Factory{&adapter.NetlinkAdapter{}}

	serverTLSConfig, err = mutualtls.NewServerTLSConfig(paths.ServerCertFile, paths.ServerKeyFile, paths.ClientCACertFile)
	Expect(err).NotTo(HaveOccurred())
	fakeServer = testsupport.StartServer(serverListenAddr, serverTLSConfig)

	acquireHandler := &testsupport.FakeHandler{
		ResponseCode: 200,
		ResponseBody: &controller.Lease{
			UnderlayIP:          localIP,
			OverlaySubnet:       "10.255.30.0/24",
			OverlayHardwareAddr: "ee:ee:0a:ff:1e:00",
		},
	}

	leases := map[string][]controller.Lease{
		"leases": []controller.Lease{
			{
				UnderlayIP:          localIP,
				OverlaySubnet:       "10.255.30.0/24",
				OverlayHardwareAddr: "ee:ee:0a:ff:1e:00",
			}, {
				UnderlayIP:          "172.17.0.5",
				OverlaySubnet:       "10.255.40.0/24",
				OverlayHardwareAddr: "ee:ee:0a:ff:28:00",
			},
		},
	}
	indexHandler := &testsupport.FakeHandler{
		ResponseCode: 200,
		ResponseBody: leases,
	}
	fakeServer.SetHandler("/leases/acquire", acquireHandler)
	fakeServer.SetHandler("/leases", indexHandler)
})

var _ = AfterEach(func() {
	fakeServer.Stop()
})

var _ = Describe("Daemon Integration", func() {
	BeforeEach(func() {
		startAndWaitForDaemon()
	})

	AfterEach(func() {
		err := vtepFactory.DeleteVTEP(vtepName)
		Expect(err).NotTo(HaveOccurred())
		stopDaemon()
	})

	It("syncs with the controller and updates the local networking stack", func() {
		By("getting the device")
		link, err := netlink.LinkByName(vtepName)
		Expect(err).NotTo(HaveOccurred())
		vtep := link.(*netlink.Vxlan)

		By("asserting on the device properties")
		Expect(vtep.Attrs().Flags & net.FlagUp).To(Equal(net.FlagUp))
		Expect(vtep.HardwareAddr.String()).To(Equal("ee:ee:0a:ff:1e:00"))
		Expect(vtep.SrcAddr.String()).To(Equal(localIP))
		defaultDevice, err := locateInterface(net.ParseIP(localIP))
		Expect(err).NotTo(HaveOccurred())
		Expect(vtep.VtepDevIndex).To(Equal(defaultDevice.Index))
		Expect(vtep.VxlanId).To(Equal(vni))
		Expect(vtep.Port).To(Equal(4789))
		Expect(vtep.Learning).To(Equal(false))
		Expect(vtep.GBP).To(BeTrue())

		By("getting the addresses on the device")
		addresses, err := netlink.AddrList(vtep, netlink.FAMILY_V4)
		Expect(err).NotTo(HaveOccurred())
		Expect(addresses).To(HaveLen(1))
		Expect(addresses[0].IP.String()).To(Equal("10.255.30.0"))
		By("checking the daemon's healthcheck")
		doHealthCheck()

		By("inspecting the daemon's log to see that it acquired a new lease")
		Expect(session.Out).To(gbytes.Say(`acquired-lease.*overlay_subnet.*10.255.30.0/24.*overlay_hardware_addr.*ee:ee:0a:ff:1e:00`))

		By("stopping the daemon")
		stopDaemon()

		By("set up renew handler")
		handler := &testsupport.FakeHandler{
			ResponseCode: 200,
			ResponseBody: struct{}{},
		}
		fakeServer.SetHandler("/leases/renew", handler)

		By("restarting the daemon")
		startAndWaitForDaemon()

		By("renewing its lease")
		var renewRequest controller.Lease
		Expect(json.Unmarshal(handler.LastRequestBody, &renewRequest)).To(Succeed())
		Expect(renewRequest).To(Equal(daemonLease))

		By("checking the daemon's healthcheck")
		doHealthCheck()

		By("inspecting the daemon's log to see that it renewed a new lease")
		Expect(session.Out).To(gbytes.Say(`renewed-lease.*overlay_subnet.*10.255.30.0/24.*overlay_hardware_addr.*ee:ee:0a:ff:1e:00`))
	})

	It("polls for other leases and logs it at debug level", func() {
		By("turning on debug logging")
		setLogLevel("DEBUG", daemonDebugServerPort)

		By("checking that the correct leases are logged")
		Eventually(session.Out, 2).Should(gbytes.Say(`silk-daemon.get-routable-leases.*log_level.*0`))
		Eventually(session.Out, 2).Should(gbytes.Say(fmt.Sprintf(`underlay_ip.*%s.*overlay_subnet.*10.255.30.0/24.*overlay_hardware_addr.*ee:ee:0a:ff:1e:00`, localIP)))
		Eventually(session.Out, 2).Should(gbytes.Say(`underlay_ip.*172.17.0.5.*overlay_subnet.*10.255.40.0/24.*overlay_hardware_addr.*ee:ee:0a:ff:28:00`))

		By("checking the arp fdb and routing are correct")
		routes := mustSucceed("ip", "route", "list", "dev", vtepName)
		Expect(routes).To(ContainSubstring(`10.255.0.0/16  proto kernel  scope link  src 10.255.30.0`))
		Expect(routes).To(ContainSubstring(`10.255.40.0/24 via 10.255.40.0  src 10.255.30.0`))

		By("removing the leases from the controller")
		fakeServer.SetHandler("/leases", &testsupport.FakeHandler{
			ResponseCode: 200,
			ResponseBody: map[string][]controller.Lease{"leases": []controller.Lease{}}},
		)

		By("checking that no leases are logged")
		Eventually(session.Out, 2).Should(gbytes.Say(fmt.Sprintf(`silk-daemon.get-routable-leases.*"leases":\[]`)))
	})

	Context("when a local lease is discovered but it cannot be renewed", func() {
		BeforeEach(func() {
			stopDaemon()

			fakeServer.SetHandler("/leases/renew", &testsupport.FakeHandler{
				ResponseCode: 404,
				ResponseBody: map[string]interface{}{},
			})
		})

		Context("when no containers are running", func() {
			It("logs an error message and acquires a new lease", func() {
				startAndWaitForDaemon()
				Expect(session.Out).To(gbytes.Say(`renew-lease.*"error":"http status 404: "`))
				Expect(session.Out).To(gbytes.Say(`acquired-lease.*`))
			})

			Context("when renew returns a 500", func() {
				BeforeEach(func() {
					fakeServer.SetHandler("/leases/renew", &testsupport.FakeHandler{
						ResponseCode: 500,
						ResponseBody: struct{}{},
					})
				})

				It("logs the correct error message", func() {
					startAndWaitForDaemon()
					Expect(session.Out).To(gbytes.Say(`renew-lease.*"error":"http status 500: "`))
				})
			})

			Context("when renew returns a 409 Conflict", func() {
				BeforeEach(func() {
					fakeServer.SetHandler("/leases/renew", &testsupport.FakeHandler{
						ResponseCode: 409,
						ResponseBody: map[string]string{"error": "lease mismatch"},
					})
				})

				It("logs the correct error message", func() {
					startAndWaitForDaemon()
					Expect(session.Out).To(gbytes.Say(`renew-lease.*"error":"non-retriable: lease mismatch"`))
				})
			})
		})
	})
})

func startAndWaitForDaemon() {
	session = startDaemon(writeConfigFile(daemonConf))

	By("waiting until the daemon is healthy before tests")
	callHealthcheck := func() (int, error) {
		resp, err := http.Get(daemonHealthCheckURL)
		if resp == nil {
			return -1, err
		}
		return resp.StatusCode, err
	}
	Eventually(callHealthcheck, "5s").Should(Equal(http.StatusOK))
}

func doHealthCheck() {
	resp, err := http.Get(daemonHealthCheckURL)
	Expect(err).NotTo(HaveOccurred())
	responseBytes, err := ioutil.ReadAll(resp.Body)

	var response controller.Lease
	err = json.Unmarshal(responseBytes, &response)
	Expect(err).NotTo(HaveOccurred())
	Expect(response).To(Equal(daemonLease))
}

func writeConfigFile(config config.Config) string {
	configFile, err := ioutil.TempFile("", "test-config")
	Expect(err).NotTo(HaveOccurred())

	configBytes, err := json.Marshal(config)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(configFile.Name(), configBytes, os.ModePerm)
	Expect(err).NotTo(HaveOccurred())

	return configFile.Name()
}

func startDaemon(configFilePath string) *gexec.Session {
	startCmd := exec.Command(paths.DaemonBin, "--config", configFilePath)
	s, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	session = s
	return s
}

func stopDaemon() {
	session.Interrupt()
	Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
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

func stopServer(server ifrit.Process) {
	if server == nil {
		return
	}
	server.Signal(os.Interrupt)
	Eventually(server.Wait()).Should(Receive())
}

func setLogLevel(level string, port int) {
	serverAddress := fmt.Sprintf("localhost:%d/log-level", port)
	curlCmd := exec.Command("curl", serverAddress, "-X", "POST", "-d", level)
	Expect(curlCmd.Start()).To(Succeed())
	Expect(curlCmd.Wait()).To(Succeed())
	return
}
