package integration_test

import (
	"io/ioutil"
	"net/http"
	"os"

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
			Expect(session.Err.Contents()).To(ContainSubstring("load config file: reading file /some/bad/path"))
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
			Expect(session.Err.Contents()).To(ContainSubstring("load config file: unmarshaling contents"))
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
				Expect(string(session.Err.Contents())).To(ContainSubstring("acquire subnet lease: http status 500"))
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
			startAndWaitForDaemon()
			stopDaemon()

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
					Expect(string(session.Err.Contents())).To(ContainSubstring("read datastore"))
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
						Expect(string(session.Err.Contents())).To(ContainSubstring("acquire subnet lease: http status 500"))
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
				Expect(string(session.Err.Contents())).To(ContainSubstring(`This cell must be restarted (run "bosh restart <job>"): non-retriable renew lease: non-retriable:`))
			})
		})
	})
})
