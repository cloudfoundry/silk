package integration_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cloudfoundry-incubator/silk/daemon/config"
	"github.com/cloudfoundry-incubator/silk/daemon/testsupport"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var DEFAULT_TIMEOUT = "5s"

var _ = Describe("Daemon Integration", func() {
	var (
		conf         config.Config
		testDatabase *testsupport.TestDatabase
		session      *gexec.Session
	)

	BeforeEach(func() {
		dbName := fmt.Sprintf("test_database_%x", GinkgoParallelNode())
		dbConnectionInfo, err := testsupport.GetDBConnectionInfo()
		Expect(err).NotTo(HaveOccurred())
		testDatabase = dbConnectionInfo.CreateDatabase(dbName)
		conf = CreateTestConfig(testDatabase)

		config, err := json.Marshal(conf)
		Expect(err).NotTo(HaveOccurred())
		tempDir, err := ioutil.TempDir("", "")
		Expect(err).NotTo(HaveOccurred())
		configFilePath := filepath.Join(tempDir, "config")

		err = ioutil.WriteFile(configFilePath, config, os.ModePerm)
		Expect(err).NotTo(HaveOccurred())

		startCmd := exec.Command(daemonPath, "--config", configFilePath)
		session, err = gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if session != nil {
			session.Interrupt()
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
		}

		if testDatabase != nil {
			testDatabase.Destroy()
		}
	})

	It("starts and stops normally", func() {
		Eventually(session.Out.Contents, "5s").Should(ContainSubstring("connected to db"))

		Consistently(session, "10s").ShouldNot(gexec.Exit())
		session.Interrupt()
		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
	})

	It("runs the SQL migrations", func() {
		Eventually(session.Out.Contents, "5s").Should(ContainSubstring("db migration complete"))

		db, err := sql.Open(conf.Database.Type, conf.Database.ConnectionString)
		Expect(err).NotTo(HaveOccurred())

		rows, err := db.Query("SELECT * FROM subnets")
		Expect(err).NotTo(HaveOccurred())
		Expect(rows.Next()).To(BeFalse())

		err = db.Close()
		Expect(err).NotTo(HaveOccurred())
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
		Database: config.DatabaseConfig{
			Type:             d.ConnInfo.Type,
			ConnectionString: connectionString,
		},
	}
}
