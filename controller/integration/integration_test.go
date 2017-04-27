package integration_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"code.cloudfoundry.org/go-db-helpers/testsupport"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/config"
	"code.cloudfoundry.org/silk/controller/leaser"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var (
	testDatabase   *testsupport.TestDatabase
	session        *gexec.Session
	conf           config.Config
	testClient     *controller.Client
	configFilePath string
	baseURL        string
)
var _ = BeforeEach(func() {
	dbName := fmt.Sprintf("test_database_%x", GinkgoParallelNode())
	dbConnectionInfo := testsupport.GetDBConnectionInfo()
	testDatabase = dbConnectionInfo.CreateDatabase(dbName)
})

var _ = AfterEach(func() {
	if testDatabase != nil {
		testDatabase.Destroy()
	}
})

var _ = Describe("Silk Controller", func() {

	BeforeEach(func() {
		conf = config.Config{
			ListenHost:             "127.0.0.1",
			ListenPort:             50000 + GinkgoParallelNode(),
			DebugServerPort:        60000 + GinkgoParallelNode(),
			CACertFile:             "fixtures/ca.crt",
			ServerCertFile:         "fixtures/server.crt",
			ServerKeyFile:          "fixtures/server.key",
			Network:                "10.255.0.0/16",
			SubnetPrefixLength:     24,
			Database:               testDatabase.DBConfig(),
			LeaseExpirationSeconds: 60,
		}
		baseURL = fmt.Sprintf("https://%s:%d", conf.ListenHost, conf.ListenPort)

		startAndWaitForServer()
	})

	AfterEach(func() {
		stopServer()
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

	It("provides an endpoint to acquire a subnet leases", func() {
		lease, err := testClient.AcquireSubnetLease("10.244.4.5")
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.UnderlayIP).To(Equal("10.244.4.5"))
		_, subnet, err := net.ParseCIDR(lease.OverlaySubnet)
		Expect(err).NotTo(HaveOccurred())
		_, network, err := net.ParseCIDR(conf.Network)
		Expect(network.Contains(subnet.IP)).To(BeTrue())
		expectedHardwareAddr, err := (&leaser.HardwareAddressGenerator{}).GenerateForVTEP(subnet.IP)
		Expect(err).NotTo(HaveOccurred())
		Expect(lease.OverlayHardwareAddr).To(Equal(expectedHardwareAddr.String()))
	})

	Describe("releasing", func() {
		It("provides an endpoint to release a subnet lease", func() {
			By("getting a valid lease")
			lease, err := testClient.AcquireSubnetLease("10.244.4.5")
			Expect(err).NotTo(HaveOccurred())

			By("checking that the lease is present in the list of routable leases")
			leases, err := testClient.GetRoutableLeases()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(leases)).To(Equal(1))
			Expect(leases[0]).To(Equal(lease))

			By("attempting to release it")
			err = testClient.ReleaseSubnetLease("10.244.4.5")
			Expect(err).NotTo(HaveOccurred())

			By("checking that the lease is not present in the list of routable leases")
			leases, err = testClient.GetRoutableLeases()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(leases)).To(Equal(0))
		})

		Context("when lease is not present in database", func() {
			It("does not error", func() {
				err := testClient.ReleaseSubnetLease("9.9.9.9")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("lease expiration", func() {
		BeforeEach(func() {
			stopServer()
			conf = config.Config{
				ListenHost:             "127.0.0.1",
				ListenPort:             50000 + GinkgoParallelNode(),
				DebugServerPort:        60000 + GinkgoParallelNode(),
				CACertFile:             "fixtures/ca.crt",
				ServerCertFile:         "fixtures/server.crt",
				ServerKeyFile:          "fixtures/server.key",
				Network:                "10.255.0.0/29",
				SubnetPrefixLength:     30,
				Database:               testDatabase.DBConfig(),
				LeaseExpirationSeconds: 1,
			}
			baseURL = fmt.Sprintf("https://%s:%d", conf.ListenHost, conf.ListenPort)

			startAndWaitForServer()
		})

		It("reclaims expired leases", func() {
			oldLease, err := testClient.AcquireSubnetLease("10.244.4.5")
			Expect(err).NotTo(HaveOccurred())

			_, err = testClient.AcquireSubnetLease("10.244.4.15")
			Expect(err).To(MatchError(ContainSubstring("No lease available")))

			// wait for lease to expire
			time.Sleep(2 * time.Second)

			newLease, err := testClient.AcquireSubnetLease("10.244.4.15")
			Expect(err).NotTo(HaveOccurred())
			Expect(newLease.OverlaySubnet).To(Equal(oldLease.OverlaySubnet))
		})
	})

	Describe("renewal", func() {
		It("successfully renews", func() {
			By("getting a valid lease")
			lease, err := testClient.AcquireSubnetLease("10.244.4.5")
			Expect(err).NotTo(HaveOccurred())

			By("attempting to renew it")
			err = testClient.RenewSubnetLease(lease)
			Expect(err).NotTo(HaveOccurred())

			By("checking that the lease is present in the list of routable leases")
			leases, err := testClient.GetRoutableLeases()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(leases)).To(Equal(1))
			Expect(leases[0]).To(Equal(lease))
		})

		Context("when the lease is not valid for some reason", func() {
			It("returns a non-retriable error", func() {
				By("getting a valid lease")
				validLease, err := testClient.AcquireSubnetLease("10.244.4.5")
				Expect(err).NotTo(HaveOccurred())

				By("corrupting it somehow")
				invalidLease := controller.Lease{
					UnderlayIP:          validLease.UnderlayIP,
					OverlaySubnet:       "10.9.9.9/24",
					OverlayHardwareAddr: validLease.OverlayHardwareAddr,
				}

				By("attempting to renew it")
				err = testClient.RenewSubnetLease(invalidLease)
				Expect(err).To(BeAssignableToTypeOf(controller.NonRetriableError("")))
				typedError := err.(controller.NonRetriableError)
				Expect(typedError.Error()).To(Equal("non-retriable: renew-subnet-lease: lease mismatch"))

				By("checking that the corrupted lease is not present in the list of routable leases")
				leases, err := testClient.GetRoutableLeases()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(leases)).To(Equal(1))
				Expect(leases[0]).To(Equal(validLease))
			})
		})

		Context("when the local lease is not present in the database", func() {
			It("the renew succeeds (even though its really more of an acquire)", func() {
				lease := controller.Lease{
					UnderlayIP:          "10.244.9.9",
					OverlaySubnet:       "10.255.9.0/24",
					OverlayHardwareAddr: "ee:ee:0a:ff:09:00",
				}

				By("attempting to renew something new but ok")
				err := testClient.RenewSubnetLease(lease)
				Expect(err).NotTo(HaveOccurred())

				By("checking that the lease is present in the list of routable leases")
				leases, err := testClient.GetRoutableLeases()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(leases)).To(Equal(1))
				Expect(leases[0]).To(Equal(lease))
			})
		})
	})

	Describe("listing leases", func() {
		It("provides an endpoint to get the current routable leases", func() {
			lease, err := testClient.AcquireSubnetLease("10.244.4.5")
			Expect(err).NotTo(HaveOccurred())

			leases, err := testClient.GetRoutableLeases()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(leases)).To(Equal(1))
			Expect(leases[0]).To(Equal(lease))
		})

		Context("when a lease expires", func() {
			BeforeEach(func() {
				stopServer()
				conf = config.Config{
					ListenHost:             "127.0.0.1",
					ListenPort:             50000 + GinkgoParallelNode(),
					DebugServerPort:        60000 + GinkgoParallelNode(),
					CACertFile:             "fixtures/ca.crt",
					ServerCertFile:         "fixtures/server.crt",
					ServerKeyFile:          "fixtures/server.key",
					Network:                "10.255.0.0/16",
					SubnetPrefixLength:     24,
					Database:               testDatabase.DBConfig(),
					LeaseExpirationSeconds: 1,
				}
				baseURL = fmt.Sprintf("https://%s:%d", conf.ListenHost, conf.ListenPort)

				startAndWaitForServer()
			})

			It("does not return expired leases", func() {
				lease1, err := testClient.AcquireSubnetLease("10.244.4.5")
				Expect(err).NotTo(HaveOccurred())
				lease2, err := testClient.AcquireSubnetLease("10.244.4.6")
				Expect(err).NotTo(HaveOccurred())

				leases, err := testClient.GetRoutableLeases()
				Expect(err).NotTo(HaveOccurred())

				Expect(leases).To(ConsistOf(lease1, lease2))

				renewAndCheck := func() []controller.Lease {
					Expect(testClient.RenewSubnetLease(lease2)).To(Succeed())
					leases, err := testClient.GetRoutableLeases()
					Expect(err).NotTo(HaveOccurred())
					return leases
				}

				Eventually(renewAndCheck, 2).Should(ConsistOf(lease2))
				Consistently(renewAndCheck).Should(ConsistOf(lease2))
			})
		})
	})

	It("assigns unique leases from the whole network to multiple clients acquiring subnets concurrently", func() {
		parallelRunner := &testsupport.ParallelRunner{
			NumWorkers: 4,
		}
		nHosts := 255
		underlayIPs := []string{}
		for i := 0; i < nHosts; i++ {
			underlayIPs = append(underlayIPs, fmt.Sprintf("10.244.42.%d", i))
		}

		leases := make(chan (controller.Lease), nHosts)
		go func() {
			parallelRunner.RunOnSliceStrings(underlayIPs, func(underlayIP string) {
				lease, err := testClient.AcquireSubnetLease(underlayIP)
				Expect(err).NotTo(HaveOccurred())
				leases <- lease
			})
			close(leases)
		}()

		leaseIPs := make(map[string]struct{})
		leaseSubnets := make(map[string]struct{})
		_, network, err := net.ParseCIDR(conf.Network)
		Expect(err).NotTo(HaveOccurred())

		for lease := range leases {
			_, subnet, err := net.ParseCIDR(lease.OverlaySubnet)
			Expect(err).NotTo(HaveOccurred())
			Expect(network.Contains(subnet.IP)).To(BeTrue())

			leaseIPs[lease.UnderlayIP] = struct{}{}
			leaseSubnets[lease.OverlaySubnet] = struct{}{}
		}
		Expect(len(leaseIPs)).To(Equal(nHosts))
		Expect(len(leaseSubnets)).To(Equal(nHosts))
	})

})

func startAndWaitForServer() {
	configFile, err := ioutil.TempFile("", "config-")
	Expect(err).NotTo(HaveOccurred())
	configFilePath = configFile.Name()
	Expect(configFile.Close()).To(Succeed())
	Expect(conf.WriteToFile(configFilePath)).To(Succeed())

	session = startServer(configFilePath)

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: makeClientTLSConfig(),
		},
	}

	testClient = controller.NewClient(lagertest.NewTestLogger("test"), httpClient, baseURL)

	By("waiting for the http server to boot")
	serverIsUp := func() error {
		_, err := testClient.GetRoutableLeases()
		return err
	}
	Eventually(serverIsUp, DEFAULT_TIMEOUT).Should(Succeed())
}

func startServer(configFilePath string) *gexec.Session {
	cmd := exec.Command(controllerBinaryPath, "-config", configFilePath)
	s, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	session = s
	return s
}

func stopServer() {
	session.Interrupt()
	Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(0))
	Expect(os.Remove(configFilePath)).To(Succeed())
}

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
