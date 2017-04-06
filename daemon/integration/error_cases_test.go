package integration_test

import (
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/silk/client/config"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("error cases", func() {
	var (
		daemonConfig   config.Config
		configFilePath string
	)

	BeforeEach(func() {
		daemonConfig = config.Config{
			SubnetRange: "10.255.0.0/16",
			SubnetMask:  24,
			Database:    testDatabase.DBConfig(),
			UnderlayIP:  "10.244.4.6",
		}
		configFilePath = writeConfigFile(daemonConfig)
	})

	AfterEach(func() {
		os.Remove(configFilePath)
	})

	Context("when the path to the config is bad", func() {
		It("exits with status 1", func() {
			session := startDaemon("/some/bad/path")
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("loading config file: reading file /some/bad/path"))

			session.Interrupt()
		})
	})

	Context("when the contents of the config file cannot be unmarshaled", func() {
		BeforeEach(func() {
			Expect(ioutil.WriteFile(configFilePath, []byte("some-bad-contents"), os.ModePerm)).To(Succeed())
		})

		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("loading config file: unmarshaling contents"))
		})
	})

	Context("when the config has an unsupported type", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			daemonConfig.Database.Type = "bad-type"
			configFilePath = writeConfigFile(daemonConfig)
		})

		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("creating lease controller: connecting to database:"))
		})
	})

	Context("when the config has a bad connection string", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			daemonConfig.Database.ConnectionString = "some-bad-connection-string"
			configFilePath = writeConfigFile(daemonConfig)
		})

		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, "10s").Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("creating lease controller: connecting to database:"))
		})
	})

	XContext("when the lease controller fails to acquire a subnet lease", func() {
		It("exits with status 1", func() {
			// TODO(gabe): unpend, figure out how to set up the test so that we can trigger
			// this sort of failure and actually test the behavior in that case
		})
	})
})
