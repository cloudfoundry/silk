package integration_test

import (
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/silk/controller"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("error cases", func() {
	var (
		configFilePath string
	)

	BeforeEach(func() {
		configFilePath = writeConfigFile(clientConf)
	})

	AfterEach(func() {
		os.Remove(configFilePath)
	})

	Context("when the path to the config is bad", func() {
		It("exits with status 1", func() {
			session := runTeardown("/some/bad/path")
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(string(session.Err.Contents())).To(ContainSubstring("load config file: reading file /some/bad/path"))
		})
	})

	Context("when the contents of the config file cannot be unmarshaled", func() {
		BeforeEach(func() {
			Expect(ioutil.WriteFile(configFilePath, []byte("some-bad-contents"), os.ModePerm)).To(Succeed())
		})

		It("exits with status 1", func() {
			session := runTeardown(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("load config file: unmarshaling contents"))
		})
	})

	Context("when the tls config is invalid", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			clientConf.ServerCACertFile = ""
			configFilePath = writeConfigFile(clientConf)
		})
		It("exits with status 1", func() {
			session := runTeardown(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("create tls config:"))
		})
	})

	Context("when the controller address is not reachable", func() {
		BeforeEach(func() {
			stopServer(fakeServer)
		})
		It("exits with status 1", func() {
			session := runTeardown(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(string(session.Err.Contents())).To(MatchRegexp(`.*release subnet lease:.*dial tcp.*`))
		})
	})

	Context("when the controller is reachable but returns a 500", func() {
		BeforeEach(func() {
			fakeServer.InstallRequestHandler(func(_ controller.ReleaseLeaseRequest) (int, interface{}) {
				return 500, map[string]string{"error": "potato"}
			})
		})

		It("exits with status 1", func() {
			session := runTeardown(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(string(session.Err.Contents())).To(ContainSubstring("silk-teardown error: release subnet lease: http status 500: potato"))
		})
	})

})
