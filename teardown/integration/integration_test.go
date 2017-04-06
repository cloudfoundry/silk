package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"code.cloudfoundry.org/go-db-helpers/testsupport"
	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/client/state"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var (
	DEFAULT_TIMEOUT = "5s"

	testDatabase *testsupport.TestDatabase
)

var _ = BeforeEach(func() {
	dbName := fmt.Sprintf("test_database_%x", GinkgoParallelNode())
	dbConnectionInfo := testsupport.GetDBConnectionInfo()
	testDatabase = dbConnectionInfo.CreateDatabase(dbName)
})

var _ = AfterEach(func() {
	if testDatabase != nil {
		testDatabase.Destroy()
	}
})

var _ = Describe("Teardown Integration", func() {
	var (
		conf config.Config
	)

	BeforeEach(func() {
		stateFilePath, err := ioutil.TempFile("", "")
		Expect(err).NotTo(HaveOccurred())
		conf = config.Config{
			SubnetRange:    "10.255.0.0/16",
			SubnetMask:     24,
			Database:       testDatabase.DBConfig(),
			UnderlayIP:     "10.244.4.0",
			LocalStateFile: stateFilePath.Name(),
		}
	})

	AfterEach(func() {
		os.Remove(conf.LocalStateFile)
	})

	It("releases the lease and removes the state file", func() {
		By("calling setup")
		configFilePath := writeConfigFile(conf)
		session := startSetup(configFilePath)

		By("verifying the setup exits with status 0")
		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())

		By("verifying the state file is updated")
		oldState := readStateFile(conf.LocalStateFile)
		Eventually(session.Out, "4s").Should(gbytes.Say(fmt.Sprintf("subnet-acquired.*subnet.*%s.*underlay ip.*", oldState.Subnet)))

		By("calling teardown")
		session = startTeardown(configFilePath)

		By("checking that the teardown released its lease")
		Eventually(session.Out, "4s").Should(gbytes.Say(fmt.Sprintf("subnet-released.*underlay ip.*")))

		By("verifying the teardown exits with status 0")
		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())

		By("verifying the state file is removed")
		_, err := os.Stat(conf.LocalStateFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no such file or directory"))

		By("calling teardown again")
		session = startTeardown(configFilePath)

		By("verifying the teardown exits with status 0")
		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
	})
})

func readStateFile(statePath string) state.SubnetLease {
	var subnetLease state.SubnetLease

	contents, err := ioutil.ReadFile(statePath)
	Expect(err).NotTo(HaveOccurred())

	err = json.Unmarshal(contents, &subnetLease)
	Expect(err).NotTo(HaveOccurred())

	return subnetLease
}

func writeConfigFile(config config.Config) string {
	configFile, err := ioutil.TempFile("", "test-config")
	Expect(err).NotTo(HaveOccurred())

	configBytes, err := json.Marshal(config)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(configFile.Name(), configBytes, os.ModePerm)
	Expect(err).NotTo(HaveOccurred())

	return configFile.Name()
}

func startSetup(configFilePath string) *gexec.Session {
	startCmd := exec.Command(setupPath, "--config", configFilePath)
	session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	return session
}

func startTeardown(configFilePath string) *gexec.Session {
	startCmd := exec.Command(teardownPath, "--config", configFilePath)
	session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	return session
}
