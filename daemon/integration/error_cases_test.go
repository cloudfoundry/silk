package integration_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"code.cloudfoundry.org/silk/testsupport"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("error cases", func() {
	var (
		configFilePath string
	)

	BeforeEach(func() {
		configFilePath = writeConfigFile(daemonConf)
	})

	AfterEach(func() {
		stopDaemon()
	})

	Context("when the path to the config is bad", func() {
		It("exits with status 1", func() {
			session = startDaemon("/some/bad/path")
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("cfnetworking.silk-daemon error: load config file: reading file /some/bad/path"))
		})
	})

	Context("when the tls config is invalid", func() {
		var configFilePath string
		BeforeEach(func() {
			clientConf := daemonConf
			clientConf.ServerCACertFile = "/dev/null"
			configFilePath = writeConfigFile(clientConf)
		})

		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("create tls config:"))
		})
	})

	Context("when the contents of the config file cannot be unmarshaled", func() {
		BeforeEach(func() {
			Expect(ioutil.WriteFile(configFilePath, []byte("some-bad-contents"), os.ModePerm)).To(Succeed())
		})

		It("exits with status 1", func() {
			session = startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("cfnetworking.silk-daemon error: load config file: unmarshaling contents"))
		})
	})

	Describe("failures to acquire a new lease", func() {
		Context("when acquire returns a 500", func() {
			BeforeEach(func() {
				fakeServer.SetHandler("/leases/acquire", &testsupport.FakeHandler{
					ResponseCode: 500,
					ResponseBody: struct{}{},
				})
			})

			It("exits with status 1", func() {
				session = startDaemon(configFilePath)
				Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
				Expect(string(session.Err.Contents())).To(ContainSubstring("potato-prefix.silk-daemon error: acquire subnet lease: http status 500"))
			})
		})

		Context("when the controller address is not reachable", func() {
			BeforeEach(func() {
				fakeServer.Stop()
			})
			It("exits with status 1", func() {
				session = startDaemon(configFilePath)
				Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
				Expect(string(session.Err.Contents())).To(MatchRegexp(`.*acquire subnet lease:.*dial tcp.*`))
			})
		})
	})

	Context("when a local lease is discovered", func() {
		BeforeEach(func() {
			By("ensuring a local lease is already present")
			startAndWaitForDaemon() // creates a new lease
			stopDaemon()            // stops daemon, but leaves local lease intact
		})

		Context("when the controller becomes unavailable for a long time", func() {
			It("allows containers to be scheduled until the partition tolerance duration has elapsed", func() {
				By("starting the daemon and waiting for it to become healthy")
				startAndWaitForDaemon()

				By("stopping the controller")
				fakeServer.Stop()

				partitionToleranceConfig := time.Duration(daemonConf.PartitionToleranceSeconds) * time.Second
				const partitionToleranceEpsilon = 2 * time.Second // an error margin
				By("verifying that the daemon remains alive during a partition shorter than the tolerance duration")
				Consistently(func() error {
					if exitCode := session.ExitCode(); exitCode != -1 {
						return fmt.Errorf("expected daemon to be alive, but it exited with code %d", exitCode)
					}
					if err := doHealthCheckWithErr(); err != nil {
						return err
					}
					return nil
				}, partitionToleranceConfig-partitionToleranceEpsilon).Should(Succeed())

				By("verifying that the daemon crashes once the partition lasts longer than the tolerance")
				Eventually(session, partitionToleranceEpsilon*2).Should(gexec.Exit(1))
			})
		})

		Context("when renewing the local lease fails due to a retriable error", func() {
			BeforeEach(func() {
				fakeServer.SetHandler("/leases/renew", &testsupport.FakeHandler{
					ResponseCode: 404,
					ResponseBody: map[string]interface{}{},
				})
			})

			Context("when reading the datastore fails", func() {
				BeforeEach(func() {
					daemonConf.Datastore = "/dev/urandom"
					configFilePath := writeConfigFile(daemonConf)
					startDaemon(configFilePath)
				})

				AfterEach(func() {
					err := vtepFactory.DeleteVTEP(vtepName)
					Expect(err).NotTo(HaveOccurred())
				})

				It("exits with status 1", func() {
					configFilePath := writeConfigFile(daemonConf)
					startDaemon(configFilePath)
					Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
					Expect(string(session.Err.Contents())).To(ContainSubstring("potato-prefix.silk-daemon error: read datastore"))
				})
			})

			Context("when no containers are running", func() {
				Context("when acquire returns a 500", func() {
					BeforeEach(func() {
						fakeServer.SetHandler("/leases/acquire", &testsupport.FakeHandler{
							ResponseCode: 500,
							ResponseBody: struct{}{},
						})
					})

					It("exits with status 1", func() {
						configFilePath := writeConfigFile(daemonConf)
						startDaemon(configFilePath)
						Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
						Expect(string(session.Err.Contents())).To(ContainSubstring("potato-prefix.silk-daemon error: acquire subnet lease: http status 500"))
					})
				})
			})

			Context("when containers are running", func() {
				BeforeEach(func() {
					err := ioutil.WriteFile(datastorePath, []byte(`{
	          "some-handle": {
	              "handle": "some-handle",
	              "ip": "192.168.0.100",
	              "metadata": {}
	          }
	      }`), os.FileMode(0600))
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					err := vtepFactory.DeleteVTEP(vtepName)
					Expect(err).NotTo(HaveOccurred())
				})

				It("exits with status 1", func() {
					configFilePath = writeConfigFile(daemonConf)
					startDaemon(configFilePath)
					Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
					Expect(session.Out).To(gbytes.Say(`renew-lease.*"error":"http status 404: "`))
					Expect(string(session.Err.Contents())).To(ContainSubstring(`renew subnet lease with containers: 1`))
				})
			})
		})

		Context("when renewing the local lease fails due to a non-retriable error", func() {
			BeforeEach(func() {
				fakeServer.SetHandler("/leases/renew", &testsupport.FakeHandler{
					ResponseCode: http.StatusConflict,
					ResponseBody: map[string]interface{}{},
				})
			})

			It("exits with status 1", func() {
				configFilePath := writeConfigFile(daemonConf)
				startDaemon(configFilePath)
				Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
				Expect(string(session.Err.Contents())).To(ContainSubstring(`This cell must be restarted (run "bosh restart <job>"): fatal: renew lease: non-retriable:`))
			})
		})
	})

	Context("when requests to the controller server time out", func() {
		BeforeEach(func() {
			mustSucceed("iptables", "-A", "INPUT", "-p", "tcp", "--dport", strconv.Itoa(serverListenPort), "-j", "DROP")
		})

		AfterEach(func() {
			mustSucceed("iptables", "-D", "INPUT", "-p", "tcp", "--dport", strconv.Itoa(serverListenPort), "-j", "DROP")
		})
		It("exits with status 1", func() {
			daemonConf.ClientTimeoutSeconds = 1
			configFilePath := writeConfigFile(daemonConf)
			startDaemon(configFilePath)
			Eventually(session, 3*time.Second).Should(gexec.Exit(1))
			Expect(string(session.Err.Contents())).To(MatchRegexp(`potato-prefix.silk-daemon error: acquire subnet lease: http client do:.* \(Client.Timeout exceeded while awaiting headers\)`))
		})
	})

	Context("when a local lease that is not part of the overlay network is discovered", func() {
		BeforeEach(func() {
			By("ensuring a local lease is already present")
			startAndWaitForDaemon() // creates a new lease
			stopDaemon()            // stops daemon, but leaves local lease intact
		})

		Context("when containers are running", func() {
			BeforeEach(func() {
				err := ioutil.WriteFile(datastorePath, []byte(`{
	          "some-handle": {
	              "handle": "some-handle",
	              "ip": "192.168.0.100",
	              "metadata": {}
	          }
	      }`), os.FileMode(0600))
				Expect(err).NotTo(HaveOccurred())

				daemonConf.OverlayNetwork = "10.254.0.0/16"
				configFilePath := writeConfigFile(daemonConf)
				writeConfigFile(daemonConf)
				startDaemon(configFilePath)
			})

			AfterEach(func() {
				err := vtepFactory.DeleteVTEP(vtepName)
				Expect(err).NotTo(HaveOccurred())
			})

			It("exits with status 1", func() {
				Eventually(session, 3*time.Second).Should(gexec.Exit(1))
				Expect(string(session.Err.Contents())).To(MatchRegexp(`potato-prefix.silk-daemon error: discovered lease is not in overlay network and has containers: 1`))
			})
		})

		Context("when reading the datastore fails", func() {
			BeforeEach(func() {
				daemonConf.Datastore = "/dev/urandom"
				daemonConf.OverlayNetwork = "10.254.0.0/16"
				configFilePath := writeConfigFile(daemonConf)
				startDaemon(configFilePath)
			})

			AfterEach(func() {
				err := vtepFactory.DeleteVTEP(vtepName)
				Expect(err).NotTo(HaveOccurred())
			})

			It("exits with status 1", func() {
				Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
				Expect(string(session.Err.Contents())).To(ContainSubstring("read datastore"))
			})
		})

		Context("no containers are running but acquiring a new lease fails", func() {
			BeforeEach(func() {
				fakeServer.SetHandler("/leases/acquire", &testsupport.FakeHandler{
					ResponseCode: 500,
					ResponseBody: struct{}{},
				})
			})

			It("exits with status 1", func() {
				daemonConf.OverlayNetwork = "10.254.0.0/16"
				configFilePath := writeConfigFile(daemonConf)
				startDaemon(configFilePath)
				Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
				Expect(string(session.Err.Contents())).To(ContainSubstring("acquire subnet lease: http status 500"))
			})
		})

		Context("when the vxlan interface does not exist", func() {
			BeforeEach(func() {
				daemonConf.VxlanInterfaceName = "non-existent-eth1"
				configFilePath = writeConfigFile(daemonConf)
			})
			It("exits with status 1", func() {
				session := startDaemon(configFilePath)
				Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
				Expect(string(session.Err.Contents())).To(ContainSubstring(`potato-prefix.silk-daemon error: create vtep config: find device from name non-existent-eth1: route ip+net: no such network interface`))
			})
		})
	})
})
