package integration_test

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"

	"code.cloudfoundry.org/go-db-helpers/mutualtls"
	"code.cloudfoundry.org/localip"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/daemon/vtep"
	"code.cloudfoundry.org/silk/lib/adapter"
	"code.cloudfoundry.org/silk/testsupport"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var (
	DEFAULT_TIMEOUT = "5s"

	localIP          string
	clientConf       config.Config
	fakeServer       *testsupport.FakeController
	serverListenAddr string
	serverTLSConfig  *tls.Config

	vtepConfig  *vtep.Config
	vtepFactory *vtep.Factory
)

var _ = BeforeEach(func() {
	var err error
	localIP, err = localip.LocalIP()
	Expect(err).NotTo(HaveOccurred())
	vtepConfig = &vtep.Config{
		VTEPName:            fmt.Sprintf("t-v-%d", GinkgoParallelNode()),
		UnderlayIP:          net.ParseIP(localIP),
		OverlayIP:           net.IP{10, 255, byte(GinkgoParallelNode()), 0},
		OverlayHardwareAddr: net.HardwareAddr{0xee, 0xee, 0x0a, 0xff, byte(GinkgoParallelNode()), 0x00},
		VNI:                 GinkgoParallelNode(),
	}

	serverListenAddr = fmt.Sprintf("127.0.0.1:%d", 40000+GinkgoParallelNode())
	datastoreFile, _ := ioutil.TempFile("", "-datastore")
	datastoreFile.Close()
	clientConf = config.Config{
		UnderlayIP:                 localIP,
		SubnetPrefixLength:         24,
		OverlayNetworkPrefixLength: 16, // unused by teardown, but config requires it
		HealthCheckPort:            4000,
		VTEPName:                   vtepConfig.VTEPName,
		ConnectivityServerURL:      fmt.Sprintf("https://%s", serverListenAddr),
		ServerCACertFile:           paths.ServerCACertFile,
		ClientCertFile:             paths.ClientCertFile,
		ClientKeyFile:              paths.ClientKeyFile,
		VNI:                        GinkgoParallelNode(),
		PollInterval:               5,                    // unused by teardown
		DebugServerPort:            GinkgoParallelNode(), // unused by teardown
		Datastore:                  datastoreFile.Name(),
	}

	serverTLSConfig, err = mutualtls.NewServerTLSConfig(paths.ServerCertFile, paths.ServerKeyFile, paths.ClientCACertFile)
	Expect(err).NotTo(HaveOccurred())
	fakeServer = testsupport.StartServer(serverListenAddr, serverTLSConfig)

	vtepFactory = &vtep.Factory{NetlinkAdapter: &adapter.NetlinkAdapter{}}
})

var _ = AfterEach(func() {
	fakeServer.Stop()
})

var _ = Describe("Teardown Integration", func() {
	var handler *testsupport.FakeHandler
	BeforeEach(func() {
		By("setting up the controller")
		handler = &testsupport.FakeHandler{
			ResponseCode: 200,
			ResponseBody: struct{}{},
		}
		fakeServer.SetHandler("/leases/release", handler)

		By("creating the vtep to destroy")
		Expect(vtepFactory.CreateVTEP(vtepConfig)).To(Succeed())
	})
	It("releases the lease and destroys the VTEP", func() {
		By("running teardown")
		session := runTeardown(writeConfigFile(clientConf))
		Expect(session).To(gexec.Exit(0))

		By("calling the controller to release the lease")
		var lastRequest controller.ReleaseLeaseRequest
		Expect(json.Unmarshal(handler.LastRequestBody, &lastRequest)).To(Succeed())
		Expect(lastRequest).To(Equal(controller.ReleaseLeaseRequest{
			UnderlayIP: vtepConfig.UnderlayIP.String(),
		}))

		By("destroying the VTEP")
		_, _, _, err := vtepFactory.GetVTEPState(clientConf.VTEPName)
		Expect(err).To(MatchError("find link: Link not found"))
	})
})

func writeConfigFile(config config.Config) string {
	configFile, err := ioutil.TempFile("", "test-config")
	Expect(err).NotTo(HaveOccurred())

	configBytes, err := json.Marshal(config)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(configFile.Name(), configBytes, os.ModePerm)
	Expect(err).NotTo(HaveOccurred())

	return configFile.Name()
}

func runTeardown(configFilePath string) *gexec.Session {
	startCmd := exec.Command(paths.TeardownBin, "--config", configFilePath)
	session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
	return session
}

func mustSucceed(binary string, args ...string) string {
	cmd := exec.Command(binary, args...)
	sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, "10s").Should(gexec.Exit(0))
	return string(sess.Out.Contents())
}
