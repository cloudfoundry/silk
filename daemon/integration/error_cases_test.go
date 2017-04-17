package integration_test

import (
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/silk/testsupport"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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

	Context("when the port is invalid", func() {
		BeforeEach(func() {
			daemonConf.HealthCheckPort = 0
			configFilePath = writeConfigFile(daemonConf)
		})
		It("exits with status 1", func() {
			session = startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("invalid health check port: 0"))
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
				Expect(string(session.Err.Contents())).To(ContainSubstring("acquire subnet lease: 500"))
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

	Describe("failures to renew an existing lease", func() {
		BeforeEach(func() {
			startAndWaitForDaemon()
			stopDaemon()
		})
		AfterEach(func() {
			mustSucceed("ip", "link", "del", vtepName)
		})
		Context("when renew returns a 500", func() {
			BeforeEach(func() {
				fakeServer.SetHandler("/leases/renew", &testsupport.FakeHandler{
					ResponseCode: 500,
					ResponseBody: struct{}{},
				})
			})

			It("exits with status 1", func() {
				session = startDaemon(configFilePath)
				Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
				Expect(string(session.Err.Contents())).To(ContainSubstring("renew subnet lease: http status 500"))
			})
		})

		Context("when renew returns a 409 Conflict", func() {
			BeforeEach(func() {
				fakeServer.SetHandler("/leases/renew", &testsupport.FakeHandler{
					ResponseCode: 409,
					ResponseBody: map[string]string{"error": "lease mismatch"},
				})
			})

			It("exits with status 1", func() {
				session = startDaemon(configFilePath)
				Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
				Expect(string(session.Err.Contents())).To(ContainSubstring("renew subnet lease: non-retriable: lease mismatch"))
			})
		})
	})
})
