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

var _ = Describe("Setup Integration", func() {
	var (
		setupConfs []config.Config
		sessions   []*gexec.Session
	)

	BeforeEach(func() {
		confTemplate := config.Config{
			SubnetRange: "10.255.0.0/16",
			SubnetMask:  24,
			Database:    testDatabase.DBConfig(),
		}
		setupConfs = configureSetups(confTemplate, 20)
	})

	AfterEach(func() {
		// delete local state file path
		for _, conf := range setupConfs {
			os.Remove(conf.LocalStateFile)
		}
	})

	It("assigns a subnet to each vm and stores it in the database", func() {
		sessions = startSetups(setupConfs)
		By("waiting for each setup to acquire a subnet")
		for _, s := range sessions {
			Eventually(s.Out, "4s").Should(gbytes.Say("lease-acquired.*underlay_ip.*overlay_subnet"))
		}

		By("verifying all setups exit with status 0")
		for _, s := range sessions {
			Eventually(s, DEFAULT_TIMEOUT).Should(gexec.Exit())
		}

		By("gathering all the subnet info")
		subnetCounts := map[string]int{}
		for _, conf := range setupConfs {
			silkState := readStateFile(conf.LocalStateFile)
			subnet, underlayIP := silkState.Subnet, silkState.UnderlayIP
			Expect(subnetCounts[subnet]).To(Equal(0))
			subnetCounts[subnet]++
			Expect(underlayIP).To(Equal(conf.UnderlayIP))
		}

		By("checking that the subnets do not collide")
		for _, count := range subnetCounts {
			Expect(count).To(Equal(1))
		}
	})

	It("releases its old lease before acquiring a new one", func() {
		conf := setupConfs[0]
		configFilePath := writeConfigFile(conf)
		session := startSetup(configFilePath)

		By("verifying the setup exits with status 0")
		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())

		By("verifying the state file is updated")
		oldState := readStateFile(conf.LocalStateFile)
		Eventually(session.Out, "4s").Should(gbytes.Say(fmt.Sprintf("lease-acquired.*overlay_subnet.*%s.*", oldState.Subnet)))

		By("calling setup again")
		session = startSetup(configFilePath)

		By("checking that the setup released its lease")
		Eventually(session.Out, "4s").Should(gbytes.Say(fmt.Sprintf("subnet-released.*underlay ip.*")))

		By("verifying the setup exits with status 0")
		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())

		By("verifying the state file is updated")
		newState := readStateFile(conf.LocalStateFile)
		Eventually(session.Out, "4s").Should(gbytes.Say(fmt.Sprintf("lease-acquired.*overlay_subnet.*%s.*", newState.Subnet)))
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

func configureSetups(template config.Config, instances int) []config.Config {
	var configs []config.Config
	for i := 0; i < instances; i++ {
		conf := template
		conf.UnderlayIP = fmt.Sprintf("10.244.4.%d", i)

		stateFilePath, err := ioutil.TempFile("", "")
		Expect(err).NotTo(HaveOccurred())
		conf.LocalStateFile = stateFilePath.Name()

		configs = append(configs, conf)
	}
	return configs
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

func startSetups(configs []config.Config) []*gexec.Session {
	var sessions []*gexec.Session
	for _, conf := range configs {
		sessions = append(sessions, startSetup(writeConfigFile(conf)))
	}
	return sessions
}
