package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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
		daemonLeases []state.SubnetLease
		daemonConfs  []config.Config
		sessions     []*gexec.Session
		client       *http.Client
	)

	BeforeEach(func() {
		confTemplate := config.Config{
			SubnetRange: "10.255.0.0/16",
			SubnetMask:  24,
			Database:    testDatabase.DBConfig(),
		}
		daemonLeases = configureLeases(20)
		daemonConfs = configureDaemons(confTemplate, 20)
		client = http.DefaultClient
		sessions = startDaemons(daemonConfs, daemonLeases)

		By("waiting until all sessions are healthy before tests")
		for i, _ := range sessions {
			callHealthcheck := func() (int, error) {
				resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", 4000+i))
				if resp == nil {
					return -1, err
				}
				return resp.StatusCode, err
			}
			Eventually(callHealthcheck, "5s").Should(Equal(http.StatusOK))
		}
	})

	AfterEach(func() {
		stopDaemons(sessions)
	})

	It("responds on its health check endpoint", func() {
		for i, lease := range daemonLeases {
			By("responding with a status code ok")
			resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", 4000+i))
			Expect(err).NotTo(HaveOccurred())
			responseBytes, err := ioutil.ReadAll(resp.Body)

			By("responding with its current state")
			leaseBytes, err := json.Marshal(lease)
			Expect(err).NotTo(HaveOccurred())
			Expect(responseBytes).To(Equal(leaseBytes))
		}
	})
})

func configureLeases(instances int) []state.SubnetLease {
	var leases []state.SubnetLease
	for i := 0; i < instances; i++ {
		lease := state.SubnetLease{
			Subnet:     fmt.Sprintf("10.255.%d.0/24", i),
			UnderlayIP: fmt.Sprintf("10.244.0.%d", i),
		}
		leases = append(leases, lease)
	}
	return leases
}

func configureDaemons(template config.Config, instances int) []config.Config {
	var configs []config.Config
	for i := 0; i < instances; i++ {
		conf := template
		conf.UnderlayIP = fmt.Sprintf("10.244.4.%d", i)
		conf.HealthCheckPort = uint16(4000 + i)
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

func startDaemons(configs []config.Config, leases []state.SubnetLease) []*gexec.Session {
	var sessions []*gexec.Session
	for i, conf := range configs {
		conf.LocalStateFile = writeStateFile(leases[i])
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
