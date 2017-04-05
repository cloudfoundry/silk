package integration_test

import (
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/silk/daemon/config"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("error cases", func() {
	var (
		setupConfig    config.Config
		configFilePath string
	)

	BeforeEach(func() {
		setupConfig = config.Config{
			SubnetRange: "10.255.0.0/16",
			SubnetMask:  24,
			Database:    testDatabase.DBConfig(),
			UnderlayIP:  "10.244.4.6",
		}
		configFilePath = writeConfigFile(setupConfig)
	})

	AfterEach(func() {
		os.Remove(configFilePath)
	})

	Context("when the path to the config is bad", func() {
		It("exits with status 1", func() {
			session := startSetup("some/bad/path")
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("could not read config file"))

			session.Interrupt()
		})
	})

	Context("when the contents of the config file cannot be unmarshaled", func() {
		BeforeEach(func() {
			Expect(ioutil.WriteFile(configFilePath, []byte("some-bad-contents"), os.ModePerm)).To(Succeed())
		})

		It("exits with status 1", func() {
			session := startSetup(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("could not unmarshal config file contents"))
		})
	})

	Context("when the config has an unsupported type", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			setupConfig.Database.Type = "bad-type"
			configFilePath = writeConfigFile(setupConfig)
		})

		It("exits with status 1", func() {
			session := startSetup(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("could not connect to database:"))
		})
	})

	Context("when the config has a bad connection string", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			setupConfig.Database.ConnectionString = "some-bad-connection-string"
			configFilePath = writeConfigFile(setupConfig)
		})

		It("exits with status 1", func() {
			session := startSetup(configFilePath)
			Eventually(session, "10s").Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("could not connect to database:"))
		})
	})

	XContext("when the lease controller fails to acquire a subnet lease", func() {
		It("exits with status 1", func() {
			// TODO: unpend, figure out how to set up the test so that we can trigger
			// this sort of failure and actually test the behavior in that case
		})
	})

	Context("when the local state file cannot be written", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			setupConfig.LocalStateFile = "/some/path/that/does/not/exit"
			configFilePath = writeConfigFile(setupConfig)
		})
		It("exits with status 1", func() {
			session := startSetup(configFilePath)
			Eventually(session, "10s").Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("could not write local state file:"))
		})
	})
})
