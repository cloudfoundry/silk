package helpers

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"path/filepath"

	"code.cloudfoundry.org/cf-networking-helpers/db"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

const DEFAULT_TIMEOUT = "5s"

func DefaultTestConfig(dbConf db.Config, fixturesPath string) config.Config {
	return config.Config{
		ListenHost:                "127.0.0.1",
		ListenPort:                PickAPort(),
		DebugServerPort:           PickAPort(),
		CACertFile:                filepath.Join(fixturesPath, "ca.crt"),
		ServerCertFile:            filepath.Join(fixturesPath, "server.crt"),
		ServerKeyFile:             filepath.Join(fixturesPath, "server.key"),
		Network:                   "10.255.0.0/16",
		SubnetPrefixLength:        24,
		Database:                  dbConf,
		LeaseExpirationSeconds:    60,
		StalenessThresholdSeconds: 5,
		MetronPort:                5432,
		MetricsEmitSeconds:        1,
	}
}

func StartAndWaitForServer(controllerBinaryPath string, conf config.Config, client *controller.Client) *gexec.Session {
	configFile, err := ioutil.TempFile("", "config-")
	Expect(err).NotTo(HaveOccurred())
	configFilePath := configFile.Name()
	Expect(configFile.Close()).To(Succeed())
	Expect(conf.WriteToFile(configFilePath)).To(Succeed())

	session := startServer(controllerBinaryPath, configFilePath)

	By("waiting for the http server to boot")
	serverIsUp := func() error {
		_, err := client.GetRoutableLeases()
		return err
	}
	Eventually(serverIsUp, DEFAULT_TIMEOUT).Should(Succeed())
	return session
}

func startServer(controllerBinaryPath, configFilePath string) *gexec.Session {
	cmd := exec.Command(controllerBinaryPath, "-config", configFilePath)
	s, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	return s
}

func StopServer(session *gexec.Session) {
	session.Interrupt()
	Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(0))
}

func TestClient(conf config.Config, fixturesPath string) *controller.Client {
	baseURL := fmt.Sprintf("https://%s:%d", conf.ListenHost, conf.ListenPort)
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: makeClientTLSConfig(fixturesPath),
		},
	}
	return controller.NewClient(lagertest.NewTestLogger("test"), httpClient, baseURL)
}

func makeClientTLSConfig(fixturesPath string) *tls.Config {
	clientCertPath := filepath.Join(fixturesPath, "client.crt")
	clientKeyPath := filepath.Join(fixturesPath, "client.key")
	cert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	Expect(err).NotTo(HaveOccurred())

	clientCAPath := filepath.Join(fixturesPath, "ca.crt")
	clientCACert, err := ioutil.ReadFile(clientCAPath)
	Expect(err).NotTo(HaveOccurred())

	clientCertPool := x509.NewCertPool()
	clientCertPool.AppendCertsFromPEM(clientCACert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      clientCertPool,
	}
	tlsConfig.BuildNameToCertificate()
	return tlsConfig
}
