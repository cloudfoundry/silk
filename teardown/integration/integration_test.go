package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"

	"code.cloudfoundry.org/cf-networking-helpers/mutualtls"
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

	clientConf       config.Config
	fakeServer       *testsupport.FakeController
	serverListenAddr string

	vtepConfig  *vtep.Config
	vtepFactory *vtep.Factory
	fakeHandler *testsupport.FakeHandler
)

var _ = BeforeEach(func() {
	localIP := "127.0.0.1"
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
		UnderlayIP:                localIP,
		SubnetPrefixLength:        24,
		OverlayNetwork:            "10.255.0.0/16", // unused by teardown, but config requires it
		HealthCheckPort:           4000,
		VTEPName:                  vtepConfig.VTEPName,
		ConnectivityServerURL:     fmt.Sprintf("https://%s", serverListenAddr),
		ServerCACertFile:          paths.ServerCACertFile,
		ClientCertFile:            paths.ClientCertFile,
		ClientKeyFile:             paths.ClientKeyFile,
		VNI:                       GinkgoParallelNode(),
		PollInterval:              5,                    // unused by teardown
		DebugServerPort:           GinkgoParallelNode(), // unused by teardown
		Datastore:                 datastoreFile.Name(),
		PartitionToleranceSeconds: 60,    // unused by teardown
		ClientTimeoutSeconds:      5,     // unused by teardown
		MetronPort:                1234,  // unused by teardown
		VTEPPort:                  12345, // unused by teardown
	}

	serverTLSConfig, err := mutualtls.NewServerTLSConfig(paths.ServerCertFile, paths.ServerKeyFile, paths.ClientCACertFile)
	Expect(err).NotTo(HaveOccurred())
	fakeServer = testsupport.StartServer(serverListenAddr, serverTLSConfig)

	vtepFactory = &vtep.Factory{NetlinkAdapter: &adapter.NetlinkAdapter{}}

	Expect(vtepFactory.CreateVTEP(vtepConfig)).To(Succeed())

	fakeHandler = &testsupport.FakeHandler{
		ResponseCode: 200,
		ResponseBody: struct{}{},
	}
	fakeServer.SetHandler("/leases/release", fakeHandler)
})

var _ = AfterEach(func() {
	fakeServer.Stop()
	removeVTEP()
})

var _ = Describe("Teardown", func() {
	It("releases the lease and destroys the VTEP", func() {
		By("running teardown")
		session := runTeardown(writeConfigFile(clientConf))
		Expect(session).To(gexec.Exit(0))

		By("verifying that the controller was called")
		var lastRequest controller.ReleaseLeaseRequest
		Expect(json.Unmarshal(fakeHandler.LastRequestBody, &lastRequest)).To(Succeed())
		Expect(lastRequest).To(Equal(controller.ReleaseLeaseRequest{
			UnderlayIP: vtepConfig.UnderlayIP.String(),
		}))

		By("verifying that the vtep is no longer present")
		_, _, _, err := vtepFactory.GetVTEPState(clientConf.VTEPName)
		Expect(err).To(MatchError("find link: Link not found"))
	})
})

func removeVTEP() {
	exec.Command("ip", "link", "del", vtepConfig.VTEPName).Run()
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

func runTeardown(configFilePath string) *gexec.Session {
	startCmd := exec.Command(paths.TeardownBin, "--config", configFilePath)
	session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
	return session
}
