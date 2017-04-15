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
		configFilePath = writeConfigFile(clientConf)
	})

	AfterEach(func() {
		os.Remove(configFilePath)
	})

	Context("when the path to the config is bad", func() {
		It("exits with status 1", func() {
			session := startTeardown("/some/bad/path")
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(string(session.Err.Contents())).To(ContainSubstring("load config file: reading file /some/bad/path"))

			session.Interrupt()
		})
	})

	Context("when the contents of the config file cannot be unmarshaled", func() {
		BeforeEach(func() {
			Expect(ioutil.WriteFile(configFilePath, []byte("some-bad-contents"), os.ModePerm)).To(Succeed())
		})

		It("exits with status 1", func() {
			session := startTeardown(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("load config file: unmarshaling contents"))
		})
	})

	Context("when the tls config is invalid", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			clientConf.ServerCACertFile = ""
			configFilePath = writeConfigFile(clientConf)
		})
		It("exits with status 1", func() {
			session := startTeardown(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
			Expect(session.Err.Contents()).To(ContainSubstring("create tls config:"))
		})
	})

	// Context("when the controller address is not reachable", func() {
	// 	BeforeEach(func() {
	// 		stopServer(fakeServer)
	// 	})
	// 	It("exits with status 1", func() {
	// 		session := startTeardown(configFilePath)
	// 		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
	// 		Expect(string(session.Err.Contents())).To(MatchRegexp(`.*release subnet lease:.*dial tcp.*`))
	// 	})
	// })

	// Context("when the port is invalid", func() {
	// 	BeforeEach(func() {
	// 		os.Remove(configFilePath)
	// 		daemonConf.HealthCheckPort = 0
	// 		configFilePath = writeConfigFile(daemonConf)
	// 	})
	// 	It("exits with status 1", func() {
	// 		session := startDaemon(configFilePath)
	// 		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
	// 		Expect(session.Err.Contents()).To(ContainSubstring("invalid health check port: 0"))
	// 	})
	// })
	//
	// Context("when the controller is reachable and returns a 500", func() {
	// 	BeforeEach(func() {
	// 		stopServer(fakeServer)
	//
	// 		tlsConfig, err := mutualtls.NewServerTLSConfig(paths.ServerCertFile, paths.ServerKeyFile, paths.ClientCACertFile)
	// 		Expect(err).NotTo(HaveOccurred())
	//
	// 		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			w.WriteHeader(http.StatusInternalServerError)
	// 			return
	// 		})
	//
	// 		someServer := http_server.NewTLSServer(serverListenAddr, testHandler, tlsConfig)
	//
	// 		members := grouper.Members{{
	// 			Name:   "http_server",
	// 			Runner: someServer,
	// 		}}
	// 		group := grouper.NewOrdered(os.Interrupt, members)
	// 		fakeServer = ifrit.Invoke(sigmon.New(group))
	//
	// 		Eventually(fakeServer.Ready()).Should(BeClosed())
	//
	// 	})
	// 	It("exits with status 1", func() {
	// 		session := startDaemon(configFilePath)
	// 		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
	// 		Expect(string(session.Err.Contents())).To(ContainSubstring("acquire subnet lease: 500"))
	// 	})
	// })
	//
	// Context("when the controller address is not reachable", func() {
	// 	BeforeEach(func() {
	// 		stopServer(fakeServer)
	// 	})
	// 	It("exits with status 1", func() {
	// 		session := startDaemon(configFilePath)
	// 		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
	// 		Expect(string(session.Err.Contents())).To(MatchRegexp(`.*acquire subnet lease:.*dial tcp.*`))
	// 	})
	// })
	//
	// Context("when a vtep device already exists", func() {
	// 	BeforeEach(func() {
	// 		os.Remove(configFilePath)
	// 		daemonConf.VTEPName = "vtep-name"
	// 		configFilePath = writeConfigFile(daemonConf)
	// 		mustSucceed("ip", "link", "add", "vtep-name", "type", "dummy")
	// 	})
	// 	AfterEach(func() {
	// 		mustSucceed("ip", "link", "del", "vtep-name")
	// 	})
	// 	It("exits with status 1", func() {
	// 		session := startDaemon(configFilePath)
	// 		Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(1))
	// 		Expect(string(session.Err.Contents())).To(ContainSubstring("create vtep: create link: file exists"))
	// 	})
	// })
})
