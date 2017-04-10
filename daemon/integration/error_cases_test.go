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
			Expect(session.Err.Contents()).To(ContainSubstring("loading config file: reading file /some/bad/path"))

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
			Expect(session.Err.Contents()).To(ContainSubstring("loading config file: unmarshaling contents"))
		})
	})

	Context("when the path to the state file is bad", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			daemonConf.LocalStateFile = "/some/bad/path"
			configFilePath = writeConfigFile(daemonConf)
		})
		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("loading state file: reading file /some/bad/path"))
		})
	})

	Context("when the contents of the state file cannot be parsed", func() {
		BeforeEach(func() {
			Expect(ioutil.WriteFile(daemonConf.LocalStateFile, []byte("some-bad-contents"), os.ModePerm)).To(Succeed())
		})
		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(string(session.Err.Contents())).To(ContainSubstring("loading state file: unmarshaling contents"))
		})
	})

	Context("when the config has an unsupported type", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			daemonConf.Database.Type = "bad-type"
			configFilePath = writeConfigFile(daemonConf)
		})

		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("creating lease controller: connecting to database:"))
		})
	})

	Context("when the config has a bad connection string", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			daemonConf.Database.ConnectionString = "some-bad-connection-string"
			configFilePath = writeConfigFile(daemonConf)
		})

		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, "10s").Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("creating lease controller: connecting to database:"))
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

	Context("when the underlay ip is invalid", func() {
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

	Context("when the local state subnet is invalid", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			os.Remove(daemonConf.LocalStateFile)
			daemonLease.Subnet = "banana"
			daemonConf.LocalStateFile = writeStateFile(daemonLease)
			configFilePath = writeConfigFile(daemonConf)
		})
		It("exits with status 1", func() {
			session := startDaemon(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(string(session.Err.Contents())).To(ContainSubstring("determine vtep overlay ip: parse subnet lease: invalid CIDR address: banana"))
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
