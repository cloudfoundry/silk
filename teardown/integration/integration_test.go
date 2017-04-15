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
	"sync"

	"code.cloudfoundry.org/go-db-helpers/mutualtls"
	"code.cloudfoundry.org/localip"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/daemon/vtep"
	"code.cloudfoundry.org/silk/lib/adapter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

var (
	DEFAULT_TIMEOUT = "5s"

	localIP          string
	clientConf       config.Config
	fakeServer       *FakeServer
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
		SubnetRange:           "10.255.0.0/16",
		SubnetMask:            24,
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
	fakeServer = startServer(serverListenAddr, serverTLSConfig)

	By("setting up the vtep to reflect a local lease")
	vtepFactory := &vtep.Factory{NetlinkAdapter: &adapter.NetlinkAdapter{}}
	Expect(vtepFactory.CreateVTEP(vtepConfig)).To(Succeed())
})

var _ = AfterEach(func() {
	exec.Command("ip", "link", "del", vtepConfig.VTEPName).Run()
	stopServer(fakeServer)
})

var _ = Describe("Teardown Integration", func() {
	It("discovers the local lease and tells the controller to release it", func() {
		var controllerReceivedLease controller.Lease
		fakeServer.InstallRequestHandler(func(lease controller.Lease) (int, interface{}) {
			controllerReceivedLease = lease
			return 200, map[string]string{}
		})

		session := runTeardown(writeConfigFile(clientConf))
		Expect(session).To(gexec.Exit(0))

		Expect(controllerReceivedLease).To(Equal(controller.Lease{
			UnderlayIP: vtepConfig.UnderlayIP.String(),
			OverlaySubnet: (&net.IPNet{
				IP:   vtepConfig.OverlayIP,
				Mask: net.CIDRMask(clientConf.SubnetMask, 32),
			}).String(),
			OverlayHardwareAddr: vtepConfig.OverlayHardwareAddr.String(),
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

type FakeServer struct {
	ifrit.Process
	handleRequest func(controller.Lease) (int, interface{})
	handlerLock   sync.Mutex
}

func (f *FakeServer) InstallRequestHandler(handler func(controller.Lease) (int, interface{})) {
	f.handlerLock.Lock()
	defer f.handlerLock.Unlock()
	f.handleRequest = handler
}

func startServer(serverListenAddr string, tlsConfig *tls.Config) *FakeServer {
	fakeServer := &FakeServer{}
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/leases/release" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		bodyBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var lease controller.Lease
		err = json.Unmarshal(bodyBytes, &lease)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fakeServer.handlerLock.Lock()
		defer fakeServer.handlerLock.Unlock()
		if fakeServer.handleRequest != nil {
			statusCode, responseData := fakeServer.handleRequest(lease)
			responseBytes, _ := json.Marshal(responseData)
			w.WriteHeader(statusCode)
			w.Write(responseBytes)
		} else {
			w.WriteHeader(http.StatusTeapot)
			w.Write([]byte(fmt.Sprintf(`{}`)))
		}
	})

	someServer := http_server.NewTLSServer(serverListenAddr, testHandler, tlsConfig)

	members := grouper.Members{{
		Name:   "http_server",
		Runner: someServer,
	}}
	group := grouper.NewOrdered(os.Interrupt, members)
	monitor := ifrit.Invoke(sigmon.New(group))

	Eventually(monitor.Ready()).Should(BeClosed())
	fakeServer.Process = monitor
	return fakeServer
}

func stopServer(server ifrit.Process) {
	if server == nil {
		return
	}
	server.Signal(os.Interrupt)
	Eventually(server.Wait()).Should(Receive())
}

func mustSucceed(binary string, args ...string) string {
	cmd := exec.Command(binary, args...)
	sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, "10s").Should(gexec.Exit(0))
	return string(sess.Out.Contents())
}
