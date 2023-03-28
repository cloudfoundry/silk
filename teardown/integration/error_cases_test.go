package integration_test

import (
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/silk/testsupport"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("error cases", func() {
	var configFilePath string

	BeforeEach(func() {
		configFilePath = writeConfigFile(clientConf)
	})

	AfterEach(func() {
		os.Remove(configFilePath)
	})

	Context("when the path to the config is bad", func() {
		It("exits with non-zero status code", func() {
			session := runTeardown("/some/bad/path")
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
			Expect(session.ExitCode).NotTo(Equal(0))
			Expect(string(session.Err.Contents())).To(ContainSubstring("load config file: reading file /some/bad/path"))
		})
	})

	Context("when the contents of the config file cannot be unmarshaled", func() {
		BeforeEach(func() {
			Expect(ioutil.WriteFile(configFilePath, []byte("some-bad-contents"), os.ModePerm)).To(Succeed())
		})

		It("exits with non-zero status code", func() {
			session := runTeardown(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
			Expect(session.ExitCode).NotTo(Equal(0))
			Expect(session.Err.Contents()).To(ContainSubstring("load config file: unmarshaling contents"))
		})
	})

	Context("when the tls config is invalid", func() {
		BeforeEach(func() {
			os.Remove(configFilePath)
			clientConf.ServerCACertFile = "/dev/null"
			configFilePath = writeConfigFile(clientConf)
		})

		It("exits with non-zero status code", func() {
			session := runTeardown(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
			Expect(session.ExitCode).NotTo(Equal(0))
			Expect(session.Err.Contents()).To(ContainSubstring("create tls config:"))
		})
	})

	Context("when the controller address is not reachable", func() {
		BeforeEach(func() {
			fakeServer.Stop()
		})

		It("logs the error and exits with non-zero status code, but still deletes the VTEP", func() {
			session := runTeardown(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
			Expect(session.ExitCode).NotTo(Equal(0))
			Expect(string(session.Err.Contents())).To(MatchRegexp(`.*release.*dial tcp.*`))

			_, _, _, err := vtepFactory.GetVTEPState(clientConf.VTEPName)
			Expect(err).To(MatchError("find link: Link not found"))
		})
	})

	Context("when the controller is reachable but returns a 500", func() {
		BeforeEach(func() {
			fakeServer.SetHandler("/leases/release", &testsupport.FakeHandler{
				ResponseCode: 500,
				ResponseBody: map[string]string{"error": "potato"},
			})
		})

		It("logs the error and exits with non-zero status", func() {
			session := runTeardown(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
			Expect(session.ExitCode).NotTo(Equal(0))
			Expect(string(session.Err.Contents())).To(ContainSubstring("release subnet lease: http status 500: potato"))
		})
	})

	Context("when the vtep does not exist", func() {
		BeforeEach(func() {
			removeVTEP()
		})

		It("exits with non-zero status code", func() {
			session := runTeardown(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
			Expect(session.ExitCode).NotTo(Equal(0))
			Expect(string(session.Err.Contents())).To(MatchRegexp("delete vtep: find link.*Link not found"))
		})
	})

	Context("when the controller is unavailable and the vtep is missing", func() {
		BeforeEach(func() {
			removeVTEP()
			fakeServer.Stop()
		})

		It("logs both errors", func() {
			session := runTeardown(configFilePath)
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
			Expect(session.ExitCode).NotTo(Equal(0))
			Expect(string(session.Err.Contents())).To(MatchRegexp("release subnet lease.*dial tcp"))
			Expect(string(session.Err.Contents())).To(MatchRegexp("delete vtep: find link.*Link not found"))
		})
	})
})
