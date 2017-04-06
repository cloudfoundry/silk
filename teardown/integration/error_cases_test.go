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
		teardownConfig config.Config
		configFilePath string
	)

	BeforeEach(func() {
		stateFile, err := ioutil.TempFile("", "")
		Expect(err).NotTo(HaveOccurred())

		teardownConfig = config.Config{
			SubnetRange:    "10.255.0.0/16",
			SubnetMask:     24,
			Database:       testDatabase.DBConfig(),
			UnderlayIP:     "10.244.4.6",
			LocalStateFile: stateFile.Name(),
		}
		configFilePath = writeConfigFile(teardownConfig)
	})

	AfterEach(func() {
		os.Remove(teardownConfig.LocalStateFile)
		os.Remove(configFilePath)
	})

	Context("when the path to the config is bad", func() {
		It("exits with status 1", func() {
			session := startSetup("some/bad/path")
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("loading config: reading file some/bad/path"))

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
			Expect(session.Err.Contents()).To(ContainSubstring("loading config: unmarshaling contents"))
		})
	})

	Context("when the config has an unsupported type", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			teardownConfig.Database.Type = "bad-type"
			configFilePath = writeConfigFile(teardownConfig)
		})

		It("exits with status 1", func() {
			session := startSetup(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("creating lease contoller: connecting to database:"))
		})
	})

	Context("when the config has a bad connection string", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			teardownConfig.Database.ConnectionString = "some-bad-connection-string"
			configFilePath = writeConfigFile(teardownConfig)
		})

		It("exits with status 1", func() {
			session := startSetup(configFilePath)
			Eventually(session, "10s").Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("creating lease contoller: connecting to database:"))
		})

		It("deletes the local state file", func() {
			session := startSetup(configFilePath)
			Eventually(session, "10s").Should(gexec.Exit(1))
			_, err := os.Stat(teardownConfig.LocalStateFile)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no such file or directory"))
		})
	})
})
