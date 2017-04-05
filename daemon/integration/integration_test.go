package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"regexp"

	"code.cloudfoundry.org/go-db-helpers/testsupport"
	"code.cloudfoundry.org/silk/daemon/config"

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

		By("gathering all the subnet info")
		subnetCounts := map[string]int{}
		for i, s := range sessions {
			subnet, underlayIP := discoverLeaseFromLogs(s.Out.Contents())
			Expect(subnetCounts[subnet]).To(Equal(0))
			subnetCounts[subnet]++
			Expect(underlayIP).To(Equal(daemonConfs[i].UnderlayIP))
		}

		By("checking that the subnets do not collide")
		for _, count := range subnetCounts {
			Expect(count).To(Equal(1))
		}
	})
})

func discoverLeaseFromLogs(output []byte) (string, string) {
	leaseLogLineRegex := `subnet-.*"subnet":"((?:[0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2})".*"underlay ip":"((?:[0-9]{1,3}\.){3}[0-9]{1,3})"`
	matches := regexp.MustCompile(leaseLogLineRegex).FindStringSubmatch(string(output))
	Expect(matches).To(HaveLen(3))

	subnetString := matches[1]
	_, subnet, err := net.ParseCIDR(subnetString)
	Expect(err).NotTo(HaveOccurred())
	Expect(subnet.String()).To(Equal(subnetString))

	ipString := matches[2]
	ip := net.ParseIP(ipString)
	Expect(ip).NotTo(BeNil())
	Expect(ip.String()).To(Equal(ipString))

	return subnetString, ipString
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
