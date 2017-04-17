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
	"strconv"

	"code.cloudfoundry.org/go-db-helpers/mutualtls"
	"code.cloudfoundry.org/localip"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/controller"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
	"github.com/vishvananda/netlink"
)

var (
	DEFAULT_TIMEOUT = "5s"

	localIP          string
	daemonConf       config.Config
	daemonLease      controller.Lease
	fakeServer       ifrit.Process
	serverListenAddr string
	serverTLSConfig  *tls.Config
	session          *gexec.Session
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
	serverListenAddr = fmt.Sprintf("127.0.0.1:%d", 40000+GinkgoParallelNode())
	daemonConf = config.Config{
		UnderlayIP:            localIP,
		SubnetRange:           "10.255.0.0/16",
		SubnetPrefixLength:    24,
		HealthCheckPort:       4000,
		VTEPName:              "silk-vxlan",
		ConnectivityServerURL: fmt.Sprintf("https://%s", serverListenAddr),
		ServerCACertFile:      paths.ServerCACertFile,
		ClientCertFile:        paths.ClientCertFile,
		ClientKeyFile:         paths.ClientKeyFile,
		VNI:                   42,
	}

	serverTLSConfig, err = mutualtls.NewServerTLSConfig(paths.ServerCertFile, paths.ServerKeyFile, paths.ClientCACertFile)
	Expect(err).NotTo(HaveOccurred())
	fakeServer = startServer(serverListenAddr, serverTLSConfig)
})

var _ = AfterEach(func() {
	stopServer(fakeServer)
})

var _ = Describe("Daemon Integration", func() {
	startAndWaitForDaemon := func(numSessions int) {
		session = startDaemon(writeConfigFile(daemonConf))

		By("waiting until the daemon is healthy before tests")
		callHealthcheck := func() (int, error) {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", 4000))
			if resp == nil {
				return -1, err
			}
			return resp.StatusCode, err
		}
		Eventually(callHealthcheck, "5s").Should(Equal(http.StatusOK))

	}

	AfterEach(func() {
		mustSucceed("ip", "link", "del", "silk-vxlan")
		stopDaemon()
	})

	Context("when one session is running", func() {
		BeforeEach(func() {
			startAndWaitForDaemon(1)
		})

		It("creates a vtep device ", func() {
			By("getting the device")
			link, err := netlink.LinkByName("silk-vxlan")
			Expect(err).NotTo(HaveOccurred())
			vtep := link.(*netlink.Vxlan)

			By("asserting on the device properties")
			Expect(vtep.Attrs().Flags & net.FlagUp).To(Equal(net.FlagUp))
			Expect(vtep.HardwareAddr.String()).To(Equal("ee:ee:0a:ff:1e:00"))
			Expect(vtep.SrcAddr.String()).To(Equal(localIP))
			defaultDevice, err := locateInterface(net.ParseIP(localIP))
			Expect(err).NotTo(HaveOccurred())
			Expect(vtep.VtepDevIndex).To(Equal(defaultDevice.Index))
			Expect(vtep.VxlanId).To(Equal(42))
			Expect(vtep.Port).To(Equal(4789))
			Expect(vtep.Learning).To(Equal(false))
			Expect(vtep.GBP).To(BeTrue())

			By("getting the addresses on the device")
			addresses, err := netlink.AddrList(vtep, netlink.FAMILY_V4)
			Expect(err).NotTo(HaveOccurred())
			Expect(addresses).To(HaveLen(1))
			Expect(addresses[0].IP.String()).To(Equal("10.255.30.0"))

			By("responding with a status code ok")
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", 4000))
			Expect(err).NotTo(HaveOccurred())
			responseBytes, err := ioutil.ReadAll(resp.Body)

			By("responding with its current lease")
			var responseLease controller.Lease
			err = json.Unmarshal(responseBytes, &responseLease)
			Expect(err).NotTo(HaveOccurred())
			Expect(responseLease).To(Equal(daemonLease))

			By("surviving a restart")
			stopDaemon()
			startAndWaitForDaemon(1)
		})
	})
})

func mustSucceed(binary string, args ...string) string {
	cmd := exec.Command(binary, args...)
	sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, "10s").Should(gexec.Exit(0))
	return string(sess.Out.Contents())
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
	session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	return session
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

func startServer(serverListenAddr string, tlsConfig *tls.Config) ifrit.Process {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/leases/acquire" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		bodyBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var payload struct {
			UnderlayIP string `json:"underlay_ip"`
		}
		err = json.Unmarshal(bodyBytes, &payload)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if payload.UnderlayIP != localIP {
			statusCode, err := strconv.Atoi(localIP)
			if err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(statusCode)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`
			{
				"underlay_ip": "%s",
				"overlay_subnet": "10.255.30.0/24",
				"overlay_hardware_addr": "ee:ee:0a:ff:1e:00"
			}
	  `, localIP)))
		return
	})

	someServer := http_server.NewTLSServer(serverListenAddr, testHandler, tlsConfig)

	members := grouper.Members{{
		Name:   "http_server",
		Runner: someServer,
	}}
	group := grouper.NewOrdered(os.Interrupt, members)
	monitor := ifrit.Invoke(sigmon.New(group))

	Eventually(monitor.Ready()).Should(BeClosed())
	return monitor
}

func stopServer(server ifrit.Process) {
	if server == nil {
		return
	}
	server.Signal(os.Interrupt)
	Eventually(server.Wait()).Should(Receive())
}
