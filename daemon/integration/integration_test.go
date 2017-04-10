package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"

	"code.cloudfoundry.org/go-db-helpers/testsupport"
	"code.cloudfoundry.org/localip"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/client/state"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/vishvananda/netlink"
)

var (
	DEFAULT_TIMEOUT = "5s"

	testDatabase *testsupport.TestDatabase
	localIP      string
	daemonLease  state.SubnetLease
	daemonConf   config.Config
)

var _ = BeforeEach(func() {
	dbName := fmt.Sprintf("test_database_%x", GinkgoParallelNode())
	dbConnectionInfo := testsupport.GetDBConnectionInfo()
	testDatabase = dbConnectionInfo.CreateDatabase(dbName)
	var err error
	localIP, err = localip.LocalIP()
	Expect(err).NotTo(HaveOccurred())
	daemonLease = state.SubnetLease{
		Subnet:     fmt.Sprintf("10.255.30.0/24"),
		UnderlayIP: localIP,
	}
	daemonConf = config.Config{
		UnderlayIP:      localIP,
		SubnetRange:     "10.255.0.0/16",
		SubnetMask:      24,
		Database:        testDatabase.DBConfig(),
		LocalStateFile:  writeStateFile(daemonLease),
		HealthCheckPort: 4000,
		VTEPName:        "silk-vxlan",
	}
})

var _ = AfterEach(func() {
	if testDatabase != nil {
		testDatabase.Destroy()
	}
})

var _ = Describe("Daemon Integration", func() {
	var (
		session *gexec.Session
		client  *http.Client
	)

	startTest := func(numSessions int) {
		session = startDaemon(writeConfigFile(daemonConf))

		client = http.DefaultClient

		By("waiting until the daemon is healthy before tests")
		callHealthcheck := func() (int, error) {
			resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", 4000))
			if resp == nil {
				return -1, err
			}
			return resp.StatusCode, err
		}
		Eventually(callHealthcheck, "5s").Should(Equal(http.StatusOK))

	}

	AfterEach(func() {
		mustSucceed("ip", "link", "del", "silk-vxlan")
		session.Interrupt()
		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
	})

	Context("when one session is running", func() {
		BeforeEach(func() {
			startTest(1)
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
			resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", 4000))
			Expect(err).NotTo(HaveOccurred())
			responseBytes, err := ioutil.ReadAll(resp.Body)

			By("responding with its current state")
			var responseLease state.SubnetLease
			err = json.Unmarshal(responseBytes, &responseLease)
			Expect(err).NotTo(HaveOccurred())
			Expect(responseLease).To(Equal(daemonLease))
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

func writeStateFile(lease state.SubnetLease) string {
	leaseFile, err := ioutil.TempFile("", "test-subnet-lease")
	Expect(err).NotTo(HaveOccurred())

	leaseBytes, err := json.Marshal(lease)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(leaseFile.Name(), leaseBytes, os.ModePerm)
	Expect(err).NotTo(HaveOccurred())

	return leaseFile.Name()
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
	startCmd := exec.Command(daemonPath, "--config", configFilePath)
	session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	return session
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
