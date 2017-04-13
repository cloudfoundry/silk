package integration_test

import (
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("error cases", func() {
	var (
		configFilePath string
	)

	BeforeEach(func() {
		configFilePath = writeConfigFile(daemonConf)
	})

	AfterEach(func() {
		os.Remove(configFilePath)
	})

	Context("when the path to the config is bad", func() {
		It("exits with status 1", func() {
			session := startDaemon("/some/bad/path")
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("load config file: reading file /some/bad/path"))

			session.Interrupt()
		})
	})

	Context("when the contents of the config file cannot be unmarshaled", func() {
		BeforeEach(func() {
			Expect(ioutil.WriteFile(configFilePath, []byte("some-bad-contents"), os.ModePerm)).To(Succeed())
		})

		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("load config file: unmarshaling contents"))
		})
	})

	Context("when the port is invalid", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			daemonConf.HealthCheckPort = 0
			configFilePath = writeConfigFile(daemonConf)
		})
		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("invalid health check port: 0"))
		})
	})

	//TODO do we still want to parse this beforehand?
	PContext("when the underlay ip is invalid", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			daemonConf.UnderlayIP = "banana"
			configFilePath = writeConfigFile(daemonConf)
		})
		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(string(session.Err.Contents())).To(ContainSubstring("parse underlay ip: banana"))
		})
	})

	Context("when the controller returns a 500", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			daemonConf.UnderlayIP = "500"
			configFilePath = writeConfigFile(daemonConf)
		})
		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			//TODO better error string?
			Expect(string(session.Err.Contents())).To(ContainSubstring("acquire subnet lease:"))
		})
	})

	Context("when the controller returns a 500", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			daemonConf.UnderlayIP = "503"
			configFilePath = writeConfigFile(daemonConf)
		})
		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			//TODO better error string?
			Expect(string(session.Err.Contents())).To(ContainSubstring("acquire subnet lease:"))
		})
	})

	Context("when the controller address is wrong", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			daemonConf.ConnectivityServerURL = "https://wrong-address"
			configFilePath = writeConfigFile(daemonConf)
		})
		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			//TODO better error string?
			Expect(string(session.Err.Contents())).To(ContainSubstring("acquire subnet lease:"))
		})
	})

	Context("when a vtep device already exists", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			daemonConf.VTEPName = "vtep-name"
			configFilePath = writeConfigFile(daemonConf)
			mustSucceed("ip", "link", "add", "vtep-name", "type", "dummy")
		})
		AfterEach(func() {
			mustSucceed("ip", "link", "del", "vtep-name")
		})
		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(string(session.Err.Contents())).To(ContainSubstring("create vtep: create link: file exists"))
		})
	})
})
