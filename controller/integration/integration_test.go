package integration_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"

	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Silk Controller", func() {

	var (
		session        *gexec.Session
		conf           config.Config
		configFilePath string
		baseURL        string
	)

	BeforeEach(func() {
		conf = config.Config{
			ListenHost:      "127.0.0.1",
			ListenPort:      50000 + GinkgoParallelNode(),
			DebugServerPort: 60000 + GinkgoParallelNode(),
			CACertFile:      "fixtures/ca.crt",
			ServerCertFile:  "fixtures/server.crt",
			ServerKeyFile:   "fixtures/server.key",
		}
		baseURL = fmt.Sprintf("https://%s:%d", conf.ListenHost, conf.ListenPort)

		configFile, err := ioutil.TempFile("", "config-file-")
		Expect(err).NotTo(HaveOccurred())
		configFilePath = configFile.Name()
		Expect(configFile.Close()).To(Succeed())
		Expect(conf.WriteToFile(configFilePath)).To(Succeed())

		cmd := exec.Command(controllerBinaryPath, "-config-file", configFilePath)
		session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		httpClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: makeClientTLSConfig(),
			},
		}

		testClient := controller.NewClient(lagertest.NewTestLogger("test"), httpClient, baseURL)

		By("waiting for the http server to boot")
		serverIsUp := func() error {
			_, err := testClient.GetRoutableLeases()
			return err
		}
		Eventually(serverIsUp, DEFAULT_TIMEOUT).Should(Succeed())
	})

	AfterEach(func() {
		session.Interrupt()
		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(0))
		Expect(os.Remove(configFilePath)).To(Succeed())
	})

	It("gracefully terminates when sent an interrupt signal", func() {
		Consistently(session).ShouldNot(gexec.Exit())

		session.Interrupt()
		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(0))
	})

	It("runs the cf debug server on the configured port", func() {
		resp, err := http.Get(
			fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", conf.DebugServerPort),
		)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})

func verifyHTTPConnection(httpClient *http.Client, baseURL string) error {
	resp, err := httpClient.Get(baseURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected server to respond %d but got %d", http.StatusOK, resp.StatusCode)
	}
	return nil
}

func makeClientTLSConfig() *tls.Config {
	cert, err := tls.LoadX509KeyPair("fixtures/client.crt", "fixtures/client.key")
	Expect(err).NotTo(HaveOccurred())

	clientCACert, err := ioutil.ReadFile("fixtures/ca.crt")
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
