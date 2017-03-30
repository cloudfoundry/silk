package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"

	"github.com/cloudfoundry-incubator/silk/daemon/config"
	"github.com/cloudfoundry-incubator/silk/daemon/testsupport"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var (
	DEFAULT_TIMEOUT = "5s"
)

var _ = Describe("Daemon Integration", func() {
	var (
		daemonConfs  []config.Config
		testDatabase *testsupport.TestDatabase
		sessions     []*gexec.Session
	)

	BeforeEach(func() {
		dbName := fmt.Sprintf("test_database_%x", GinkgoParallelNode())
		dbConnectionInfo, err := testsupport.GetDBConnectionInfo()
		Expect(err).NotTo(HaveOccurred())
		testDatabase = dbConnectionInfo.CreateDatabase(dbName)

		conf := CreateTestConfig(testDatabase)
		daemonConfs = configureDaemons(conf, 20)
		sessions = startDaemons(daemonConfs)
	})

	AfterEach(func() {
		stopDaemons(sessions)

		if testDatabase != nil {
			testDatabase.Destroy()
		}
	})

	It("assigns a subnet to each vm and stores it in the database", func() {
		By("waiting for each daemon to acquire a subnet")
		for _, s := range sessions {
			Eventually(s.Out, "4s").Should(gbytes.Say("subnet-acquired.*subnet.*underlay ip.*"))
		}

		By("signaling all sessions to terminate")
		for _, s := range sessions {
			s.Interrupt()
		}
		By("verifying all daemons exit with status 0")
		for _, s := range sessions {
			Eventually(s, DEFAULT_TIMEOUT).Should(gexec.Exit())
		}

		By("checking that subnets do not overlap")
		subnets := map[string]int{}
		for i, s := range sessions {
			subnet, underlayIP := discoverLeaseFromLogs(s.Out.Contents())
			Expect(subnets[subnet]).To(Equal(0))
			subnets[subnet]++
			Expect(underlayIP).To(Equal(daemonConfs[i].UnderlayIP))
		}
	})

})

func CreateTestConfig(d *testsupport.TestDatabase) config.Config {
	var connectionString string
	if d.ConnInfo.Type == "mysql" {
		connectionString = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
			d.ConnInfo.Username, d.ConnInfo.Password, d.ConnInfo.Hostname, d.ConnInfo.Port, d.Name)
	} else if d.ConnInfo.Type == "postgres" {
		connectionString = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			d.ConnInfo.Username, d.ConnInfo.Password, d.ConnInfo.Hostname, d.ConnInfo.Port, d.Name)
	} else {
		connectionString = fmt.Sprintf("some unsupported db type connection string: %s\n", d.ConnInfo.Type)
	}

	return config.Config{
		SubnetRange: "10.255.0.0/16",
		SubnetMask:  24,
		Database: config.DatabaseConfig{
			Type:             d.ConnInfo.Type,
			ConnectionString: connectionString,
		},
	}
}

func discoverLeaseFromLogs(output []byte) (string, string) {
	leaseLogLineRegex := `subnet-acquired.*"subnet":"((?:[0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2})".*"underlay ip":"((?:[0-9]{1,3}\.){3}[0-9]{1,3})"`
	matches := regexp.MustCompile(leaseLogLineRegex).FindStringSubmatch(string(output))
	Expect(matches).To(HaveLen(3))
	return matches[1], matches[2]
}

func configureDaemons(template config.Config, instances int) []config.Config {
	var configs []config.Config
	for i := 0; i < instances; i++ {
		conf := template
		conf.UnderlayIP = fmt.Sprintf("10.244.4.%d", i)
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

func startDaemons(configs []config.Config) []*gexec.Session {
	var sessions []*gexec.Session
	for _, conf := range configs {
		configFilePath := writeConfigFile(conf)

		startCmd := exec.Command(daemonPath, "--config", configFilePath)
		session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		sessions = append(sessions, session)
	}
	return sessions
}

func stopDaemons(sessions []*gexec.Session) {
	for _, session := range sessions {
		session.Interrupt()
		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
	}
}
