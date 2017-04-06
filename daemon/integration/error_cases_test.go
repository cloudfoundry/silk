package integration_test

import (
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/client/state"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("error cases", func() {
	var (
		daemonConfig   config.Config
		lease          state.SubnetLease
		configFilePath string
	)

	BeforeEach(func() {
		lease = state.SubnetLease{}
		leasePath := writeStateFile(lease)
		daemonConfig = config.Config{
			SubnetRange:    "10.255.0.0/16",
			SubnetMask:     24,
			Database:       testDatabase.DBConfig(),
			UnderlayIP:     "10.244.4.6",
			LocalStateFile: leasePath,
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

	Context("when the path to the state file is bad", func() {
		BeforeEach(func() {
			daemonConfig = config.Config{
				SubnetRange:    "10.255.0.0/16",
				SubnetMask:     24,
				Database:       testDatabase.DBConfig(),
				UnderlayIP:     "10.244.4.6",
				LocalStateFile: "/some/bad/path",
			}
			configFilePath = writeConfigFile(daemonConfig)
		})
		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("loading state file: reading file /some/bad/path"))
		})
	})

	Context("when the contents of the state file cannot be parsed", func() {
		BeforeEach(func() {
			Expect(ioutil.WriteFile(daemonConfig.LocalStateFile, []byte("some-bad-contents"), os.ModePerm)).To(Succeed())
		})
		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(string(session.Err.Contents())).To(ContainSubstring("loading state file: unmarshaling contents"))
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

	Context("when the port is invalid", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			daemonConfig.HealthCheckPort = 0
			configFilePath = writeConfigFile(daemonConfig)
		})
		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("invalid healthcheck port: 0"))
		})
	})
})
