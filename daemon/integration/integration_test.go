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

var _ = Describe("Daemon Integration", func() {
	var (
		daemonConfs []config.Config
		sessions    []*gexec.Session
	)

	BeforeEach(func() {
		confTemplate := config.Config{
			SubnetRange: "10.255.0.0/16",
			SubnetMask:  24,
			Database:    testDatabase.DBConfig(),
		}
		daemonConfs = configureDaemons(confTemplate, 20)
		sessions = startDaemons(daemonConfs)
	})

	AfterEach(func() {
		stopDaemons(sessions)
	})
})

func configureDaemons(template config.Config, instances int) []config.Config {
	var configs []config.Config
	for i := 0; i < instances; i++ {
		conf := template
		conf.UnderlayIP = fmt.Sprintf("10.244.4.%d", i)
		configs = append(configs, conf)
	}
	return configs
}

func writeStateFile(lease state.SubnetLease) string {
	leaseFile, err := ioutil.TempFile("", "test-subnet-lease")
	Expect(err).NotTo(HaveOccurred())

	leaseBytes, err := json.Marshal(lease)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(leaseFile.Name(), leaseBytes, os.ModePerm)
	Expect(err).NotTo(HaveOccurred())

	return leaseFile.Name()
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

func startDaemon(configFilePath string) *gexec.Session {
	startCmd := exec.Command(daemonPath, "--config", configFilePath)
	session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	return session
}

func startDaemons(configs []config.Config) []*gexec.Session {
	var sessions []*gexec.Session
	for _, conf := range configs {
		sessions = append(sessions, startDaemon(writeConfigFile(conf)))
	}
	return sessions
}

func stopDaemons(sessions []*gexec.Session) {
	for _, session := range sessions {
		session.Interrupt()
		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
	}
}
