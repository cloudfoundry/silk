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

	vtepConfig *vtep.Config
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
	clientConf = config.Config{
		UnderlayIP:            localIP,
		SubnetPrefixLength:    24,
		HealthCheckPort:       4000,
		VTEPName:              vtepConfig.VTEPName,
		ConnectivityServerURL: fmt.Sprintf("https://%s", serverListenAddr),
		ServerCACertFile:      paths.ServerCACertFile,
		ClientCertFile:        paths.ClientCertFile,
		ClientKeyFile:         paths.ClientKeyFile,
		VNI:                   GinkgoParallelNode(),
	}

	serverTLSConfig, err = mutualtls.NewServerTLSConfig(paths.ServerCertFile, paths.ServerKeyFile, paths.ClientCACertFile)
	Expect(err).NotTo(HaveOccurred())
	fakeServer = testsupport.StartServer(serverListenAddr, serverTLSConfig)

	By("setting up the vtep to reflect a local lease")
	vtepFactory := &vtep.Factory{NetlinkAdapter: &adapter.NetlinkAdapter{}}
	Expect(vtepFactory.CreateVTEP(vtepConfig)).To(Succeed())
})

var _ = AfterEach(func() {
	exec.Command("ip", "link", "del", vtepConfig.VTEPName).Run()
	fakeServer.Stop()
})

var _ = Describe("Teardown Integration", func() {
	It("discovers the local lease and tells the controller to release it", func() {
		handler := &testsupport.FakeHandler{
			ResponseCode: 200,
			ResponseBody: struct{}{},
		}
		fakeServer.SetHandler("/leases/release", handler)
		session := runTeardown(writeConfigFile(clientConf))
		Expect(session).To(gexec.Exit(0))

		var lastRequest controller.ReleaseLeaseRequest
		Expect(json.Unmarshal(handler.LastRequestBody, &lastRequest)).To(Succeed())
		Expect(lastRequest).To(Equal(controller.ReleaseLeaseRequest{
			UnderlayIP: vtepConfig.UnderlayIP.String(),
		}))
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
