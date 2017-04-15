package integration_test

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"

	"code.cloudfoundry.org/go-db-helpers/mutualtls"
	"code.cloudfoundry.org/localip"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/client/state"
	"code.cloudfoundry.org/silk/controller"
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
	lease            state.SubnetLease
	fakeServer       *FakeServer
	serverListenAddr string
	serverTLSConfig  *tls.Config
)

var _ = BeforeEach(func() {
	var err error
	localIP, err = localip.LocalIP()
	Expect(err).NotTo(HaveOccurred())
	lease = state.SubnetLease{
		UnderlayIP: localIP,
		Subnet:     "10.255.30.0/24",
	}
	serverListenAddr = fmt.Sprintf("127.0.0.1:%d", 40000+GinkgoParallelNode())
	clientConf = config.Config{
		UnderlayIP:            localIP,
		SubnetRange:           "10.255.0.0/16",
		SubnetMask:            24,
		HealthCheckPort:       4000,
		VTEPName:              "silk-vxlan",
		ConnectivityServerURL: fmt.Sprintf("https://%s", serverListenAddr),
		ServerCACertFile:      paths.ServerCACertFile,
		ClientCertFile:        paths.ClientCertFile,
		ClientKeyFile:         paths.ClientKeyFile,
	}

	serverTLSConfig, err = mutualtls.NewServerTLSConfig(paths.ServerCertFile, paths.ServerKeyFile, paths.ClientCACertFile)
	Expect(err).NotTo(HaveOccurred())
	fakeServer = startServer(serverListenAddr, serverTLSConfig)
})

var _ = AfterEach(func() {
	stopServer(fakeServer)
})

var _ = Describe("Teardown Integration", func() {
	var (
		session *gexec.Session
	)
	BeforeEach(func() {
		session = startTeardown(writeConfigFile(clientConf))
		fakeServer.HandleRequest = func(lease controller.Lease) (int, interface{}) {
			return 200, map[string]string{}
		}
	})
	It("exits 0", func() {
		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(0))

		By("calling the release endpoint on the lease controller")

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

func startTeardown(configFilePath string) *gexec.Session {
	startCmd := exec.Command(paths.TeardownBin, "--config", configFilePath)
	session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	return session
}

type FakeServer struct {
	ifrit.Process
	HandleRequest func(controller.Lease) (int, interface{})
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
		if fakeServer.HandleRequest != nil {
			statusCode, responseData := fakeServer.HandleRequest(lease)
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
