package integration_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/silk/daemon/config"
	"github.com/cloudfoundry-incubator/silk/daemon/testsupport"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var (
	DEFAULT_TIMEOUT = "5s"
)

var _ = Describe("Daemon Integration", func() {
	var (
		conf         config.Config
		testDatabase *testsupport.TestDatabase
		sessions     []*gexec.Session
	)

	BeforeEach(func() {
		dbName := fmt.Sprintf("test_database_%x", GinkgoParallelNode())
		dbConnectionInfo, err := testsupport.GetDBConnectionInfo()
		Expect(err).NotTo(HaveOccurred())
		testDatabase = dbConnectionInfo.CreateDatabase(dbName)

		conf = CreateTestConfig(testDatabase)
		daemonConfs := configureDaemons(conf, 2)
		sessions = startDaemons(daemonConfs)
	})

	AfterEach(func() {
		stopDaemons(sessions)

		if testDatabase != nil {
			testDatabase.Destroy()
		}
	})

	It("assigns a subnet to the vm and stores it in the database", func() {
		By("all daemons exiting with status code 0")
		for _, s := range sessions {
			Consistently(s, "10s").ShouldNot(gexec.Exit())
			s.Interrupt()
			Eventually(s, DEFAULT_TIMEOUT).Should(gexec.Exit())
		}

		By("opening the database")
		db, err := sql.Open(conf.Database.Type, conf.Database.ConnectionString)
		Expect(err).NotTo(HaveOccurred())
		defer db.Close()

		By("getting the subnets from the database")
		rows, err := db.Query("SELECT subnet FROM subnets")
		Expect(err).NotTo(HaveOccurred())
		defer rows.Close()

		By("checking that each daemon got a subnet")
		numRows := 0
		for rows.Next() {
			var subnet string
			err = rows.Scan(&subnet)
			Expect(err).NotTo(HaveOccurred())
			numRows += 1
		}
		Expect(numRows).To(Equal(len(sessions)))
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
		SubnetMask:  "24",
		Database: config.DatabaseConfig{
			Type:             d.ConnInfo.Type,
			ConnectionString: connectionString,
		},
	}
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
