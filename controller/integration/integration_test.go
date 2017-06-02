package integration_test

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"code.cloudfoundry.org/cf-networking-helpers/db"
	"code.cloudfoundry.org/cf-networking-helpers/metrics"
	"code.cloudfoundry.org/cf-networking-helpers/testsupport"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/config"
	"code.cloudfoundry.org/silk/controller/integration/helpers"
	"code.cloudfoundry.org/silk/controller/leaser"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/types"
)

var (
	dbConfig       db.Config
	session        *gexec.Session
	conf           config.Config
	testClient     *controller.Client
	configFilePath string
	baseURL        string
	fakeMetron     metrics.FakeMetron
)

var _ = BeforeEach(func() {
	fakeMetron = metrics.NewFakeMetron()
	dbConfig = testsupport.GetDBConfig()
	dbConfig.DatabaseName = fmt.Sprintf("test_%d", testsupport.PickAPort())
	testsupport.CreateDatabase(dbConfig)

	conf = helpers.DefaultTestConfig(dbConfig, "fixtures")
	conf.MetronPort = fakeMetron.Port()
	testClient = helpers.TestClient(conf, "fixtures")
	session = helpers.StartAndWaitForServer(controllerBinaryPath, conf, testClient)
})

var _ = AfterEach(func() {
	helpers.StopServer(session)
	Expect(testsupport.RemoveDatabase(dbConfig)).To(Succeed())
})

var _ = Describe("Silk Controller", func() {
	It("gracefully terminates when sent an interrupt signal", func() {
		Consistently(session).ShouldNot(gexec.Exit())

		session.Interrupt()
		Eventually(session, "5s").Should(gexec.Exit(0))
	})

	It("runs the cf debug server on the configured port", func() {
		resp, err := http.Get(
			fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", conf.DebugServerPort),
		)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("runs the health check server on the configured port", func() {
		resp, err := http.Get(
			fmt.Sprintf("http://127.0.0.1:%d/health", conf.HealthCheckPort),
		)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	Describe("acquiring", func() {
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

			Eventually(fakeMetron.AllEvents, "5s").Should(ContainElement(
				HaveName("LeasesAcquireRequestTime"),
			))
		})

		Context("when there is an existing lease for the underlay IP", func() {
			var existingLease controller.Lease
			BeforeEach(func() {
				var err error
				existingLease, err = testClient.AcquireSubnetLease("10.244.4.5")
				Expect(err).NotTo(HaveOccurred())
			})
			It("returns the same lease", func() {
				lease, err := testClient.AcquireSubnetLease("10.244.4.5")
				Expect(err).NotTo(HaveOccurred())

				Expect(lease).To(Equal(existingLease))
			})

			Context("when the existing lease is in a different overlay network", func() {
				BeforeEach(func() {
					helpers.StopServer(session)
					conf.Network = "10.254.0.0/16"
					session = helpers.StartAndWaitForServer(controllerBinaryPath, conf, testClient)
				})
				It("returns a new lease in the new network", func() {
					lease, err := testClient.AcquireSubnetLease("10.244.4.5")
					Expect(err).NotTo(HaveOccurred())

					Expect(lease).NotTo(Equal(existingLease))
					_, subnet, err := net.ParseCIDR(lease.OverlaySubnet)
					Expect(err).NotTo(HaveOccurred())
					_, network, err := net.ParseCIDR(conf.Network)
					Expect(network.Contains(subnet.IP)).To(BeTrue())
				})
			})
		})
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

			Eventually(fakeMetron.AllEvents, "5s").Should(ContainElement(
				HaveName("LeasesReleaseRequestTime"),
			))
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
			helpers.StopServer(session)
			conf.Network = "10.255.0.0/29"
			conf.SubnetPrefixLength = 30
			conf.LeaseExpirationSeconds = 1
			session = helpers.StartAndWaitForServer(controllerBinaryPath, conf, testClient)
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

			Eventually(fakeMetron.AllEvents, "5s").Should(ContainElement(
				HaveName("LeasesRenewRequestTime"),
			))
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

		Context("when there is an existing lease for the underlay IP", func() {
			var existingLease controller.Lease
			BeforeEach(func() {
				var err error
				existingLease, err = testClient.AcquireSubnetLease("10.244.4.5")
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when the existing lease is in a different overlay network", func() {
				BeforeEach(func() {
					helpers.StopServer(session)
					conf.Network = "10.254.0.0/16"
					session = helpers.StartAndWaitForServer(controllerBinaryPath, conf, testClient)
				})
				It("renews the same lease in the old network", func() {
					err := testClient.RenewSubnetLease(existingLease)
					Expect(err).NotTo(HaveOccurred())

					By("checking that the lease is present in the list of routable leases")
					leases, err := testClient.GetRoutableLeases()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(leases)).To(Equal(1))
					Expect(leases[0]).To(Equal(existingLease))
				})
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

			Eventually(fakeMetron.AllEvents, "5s").Should(ContainElement(
				HaveName("LeasesIndexRequestTime"),
			))
		})

		Context("when a lease expires", func() {
			BeforeEach(func() {
				helpers.StopServer(session)
				conf.LeaseExpirationSeconds = 2
				session = helpers.StartAndWaitForServer(controllerBinaryPath, conf, testClient)
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

				Eventually(renewAndCheck, 4).Should(ConsistOf(lease2))
				Consistently(renewAndCheck).Should(ConsistOf(lease2))
			})
		})

		Context("when there are leases from different networks", func() {
			var oldNetworkLease controller.Lease
			var newNetworkLease controller.Lease
			BeforeEach(func() {
				var err error
				oldNetworkLease, err = testClient.AcquireSubnetLease("10.244.4.5")
				Expect(err).NotTo(HaveOccurred())

				helpers.StopServer(session)
				conf.Network = "10.254.0.0/16"
				session = helpers.StartAndWaitForServer(controllerBinaryPath, conf, testClient)

				newNetworkLease, err = testClient.AcquireSubnetLease("10.244.4.6")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns all the leases", func() {
				leases, err := testClient.GetRoutableLeases()
				Expect(err).NotTo(HaveOccurred())

				Expect(leases).To(ConsistOf(oldNetworkLease, newNetworkLease))
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

	withName := func(name string) types.GomegaMatcher {
		return WithTransform(func(ev metrics.Event) string {
			return ev.Name
		}, Equal(name))
	}

	withValue := func(value interface{}) types.GomegaMatcher {
		return WithTransform(func(ev metrics.Event) float64 {
			return ev.Value
		}, BeEquivalentTo(value))
	}

	hasMetricWithValue := func(name string, value interface{}) types.GomegaMatcher {
		return SatisfyAll(withName(name), withValue(value))
	}

	Describe("metrics", func() {
		It("emits an uptime metric", func() {
			Eventually(fakeMetron.AllEvents, "5s").Should(ContainElement(withName("uptime")))
		})
		Context("when some leases have been claimed", func() {
			BeforeEach(func() {
				_, err := testClient.AcquireSubnetLease("10.244.4.5")
				Expect(err).NotTo(HaveOccurred())
				_, err = testClient.AcquireSubnetLease("10.244.4.6")
				Expect(err).NotTo(HaveOccurred())
			})
			It("emits number of total leases", func() {
				Eventually(fakeMetron.AllEvents, "10s").Should(ContainElement(hasMetricWithValue("totalLeases", 2)))
			})
			It("emits number of free leases", func() {
				Eventually(fakeMetron.AllEvents, "10s").Should(ContainElement(hasMetricWithValue("freeLeases", 253)))
			})
			It("emits number of stale leases", func() {
				Eventually(fakeMetron.AllEvents, "2s").Should(ContainElement(hasMetricWithValue("staleLeases", 0)))
				Consistently(fakeMetron.AllEvents, "2s").Should(ContainElement(hasMetricWithValue("staleLeases", 0)))
				Eventually(fakeMetron.AllEvents, "10s").Should(ContainElement(hasMetricWithValue("staleLeases", 2)))
			})
		})

	})
})
