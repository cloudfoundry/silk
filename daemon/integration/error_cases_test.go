package integration_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/cloudfoundry-incubator/silk/daemon/config"
	"github.com/cloudfoundry-incubator/silk/daemon/testsupport"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("error cases", func() {
	var (
		testDatabase *testsupport.TestDatabase
	)

	BeforeEach(func() {
		dbName := fmt.Sprintf("test_database_%x", GinkgoParallelNode())
		dbConnectionInfo, err := testsupport.GetDBConnectionInfo()
		Expect(err).NotTo(HaveOccurred())
		testDatabase = dbConnectionInfo.CreateDatabase(dbName)
	})

	AfterEach(func() {
		if testDatabase != nil {
			testDatabase.Destroy()
		}
	})

	Context("when the path to the config is bad", func() {
		It("exits with status 1", func() {
			startCmd := exec.Command(daemonPath, "--config", "some/bad/path")
			session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("could not read config file"))

			session.Interrupt()
		})
	})

	Context("when the contents of the config file cannot be unmarshaled", func() {
		It("exits with status 1", func() {
			configFile, err := ioutil.TempFile("", "test-config")
			Expect(err).NotTo(HaveOccurred())

			err = ioutil.WriteFile(configFile.Name(), []byte("some-bad-contents"), os.ModePerm)
			Expect(err).NotTo(HaveOccurred())

			startCmd := exec.Command(daemonPath, "--config", configFile.Name())
			session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("could not unmarshal config file contents"))

			session.Interrupt()
		})
	})

	Context("when the database handler fails to be created", func() {
		It("exits with status 1", func() {
			conf := config.Config{
				SubnetRange: "10.255.0.0/16",
				SubnetMask:  "24",
				UnderlayIP:  "10.244.4.6",
				Database: config.DatabaseConfig{
					Type:             "bad-type",
					ConnectionString: "some-bad-connection-string",
				},
			}

			configFilePath := writeConfigFile(conf)

			startCmd := exec.Command(daemonPath, "--config", configFilePath)
			session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("could not create database handler:"))

			session.Interrupt()
		})
	})

	Context("when the lease controller fails to migrate the database", func() {
		It("exits with status 1", func() {
			conf := config.Config{
				SubnetRange: "10.255.0.0/16",
				SubnetMask:  "24",
				UnderlayIP:  "10.244.4.6",
				Database: config.DatabaseConfig{
					Type:             "postgres",
					ConnectionString: "some-bad-connection-string",
				},
			}

			configFilePath := writeConfigFile(conf)

			startCmd := exec.Command(daemonPath, "--config", configFilePath)
			session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			Eventually(session, 10*time.Second).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("could not migrate database:"))

			session.Interrupt()
		})
	})

	Context("when the lease controller fails to acquire a subnet lease", func() {
		It("exits with status 1", func() {
			conf := CreateTestConfig(testDatabase)
			conf.UnderlayIP = "10.244.4.5"

			configFilePath := writeConfigFile(conf)
			startCmd := exec.Command(daemonPath, "--config", configFilePath)
			session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			Eventually(session.Out, "4s").Should(gbytes.Say("subnet-acquired.*subnet.*underlay ip.*"))

			failingCmd := exec.Command(daemonPath, "--config", configFilePath)
			failingSession, err := gexec.Start(failingCmd, GinkgoWriter, GinkgoWriter)
			Eventually(failingSession, 20*time.Second).Should(gexec.Exit(1))
			Expect(failingSession.Err.Contents()).To(ContainSubstring("could not acquire subnet:"))

			session.Interrupt()
			failingSession.Interrupt()
		})
	})
})
