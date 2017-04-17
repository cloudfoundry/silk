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

	"code.cloudfoundry.org/go-db-helpers/mutualtls"
	"code.cloudfoundry.org/localip"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/testsupport"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/vishvananda/netlink"
)

var (
	DEFAULT_TIMEOUT = "5s"

	localIP              string
	daemonConf           config.Config
	daemonLease          controller.Lease
	fakeServer           *testsupport.FakeController
	serverListenAddr     string
	serverTLSConfig      *tls.Config
	session              *gexec.Session
	daemonHealthCheckURL string
	vtepName             string
	vni                  int
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
	serverListenAddr = fmt.Sprintf("127.0.0.1:%d", 40000+GinkgoParallelNode())
	daemonConf = config.Config{
		UnderlayIP:            localIP,
		SubnetRange:           "10.255.0.0/16",
		SubnetPrefixLength:    24,
		HealthCheckPort:       uint16(daemonHealthCheckPort),
		VTEPName:              vtepName,
		ConnectivityServerURL: fmt.Sprintf("https://%s", serverListenAddr),
		ServerCACertFile:      paths.ServerCACertFile,
		ClientCertFile:        paths.ClientCertFile,
		ClientKeyFile:         paths.ClientKeyFile,
		VNI:                   vni,
	}

	serverTLSConfig, err = mutualtls.NewServerTLSConfig(paths.ServerCertFile, paths.ServerKeyFile, paths.ClientCACertFile)
	Expect(err).NotTo(HaveOccurred())
	fakeServer = testsupport.StartServer(serverListenAddr, serverTLSConfig)

	handler := &testsupport.FakeHandler{
		ResponseCode: 200,
		ResponseBody: &controller.Lease{
			UnderlayIP:          localIP,
			OverlaySubnet:       "10.255.30.0/24",
			OverlayHardwareAddr: "ee:ee:0a:ff:1e:00",
		},
	}
	fakeServer.SetHandler("/leases/acquire", handler)
})

var _ = AfterEach(func() {
	fakeServer.Stop()
})

var _ = Describe("Daemon Integration", func() {
	startAndWaitForDaemon := func(numSessions int) {
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

	AfterEach(func() {
		mustSucceed("ip", "link", "del", vtepName)
		stopDaemon()
	})

	Context("when one session is running", func() {
		BeforeEach(func() {
			startAndWaitForDaemon(1)
		})

		It("creates a vtep device ", func() {
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

			By("stopping the daemon")
			stopDaemon()

			By("set up renew handler")
			handler := &testsupport.FakeHandler{
				ResponseCode: 200,
				ResponseBody: struct{}{},
			}
			fakeServer.SetHandler("/leases/renew", handler)

			By("restarting the daemon")
			startAndWaitForDaemon(1)

			By("renewing it's lease")
			var renewRequest controller.Lease
			Expect(json.Unmarshal(handler.LastRequestBody, &renewRequest)).To(Succeed())
			Expect(renewRequest).To(Equal(daemonLease))

			By("checking the daemon's healthcheck")
			doHealthCheck()
		})
	})
})

func doHealthCheck() {
	resp, err := http.Get(daemonHealthCheckURL)
	Expect(err).NotTo(HaveOccurred())
	responseBytes, err := ioutil.ReadAll(resp.Body)

	var response controller.Lease
	err = json.Unmarshal(responseBytes, &response)
	Expect(err).NotTo(HaveOccurred())
	Expect(response).To(Equal(daemonLease))
}

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

func stopServer(server ifrit.Process) {
	if server == nil {
		return
	}
	server.Signal(os.Interrupt)
	Eventually(server.Wait()).Should(Receive())
}
