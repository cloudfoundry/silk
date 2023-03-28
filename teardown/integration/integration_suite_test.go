package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"

	"code.cloudfoundry.org/cf-networking-helpers/testsupport"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var (
	paths testPaths
)

type testPaths struct {
	CertDir          string
	ServerCACertFile string
	ClientCACertFile string
	ServerCertFile   string
	ServerKeyFile    string
	ClientCertFile   string
	ClientKeyFile    string
	TeardownBin      string
}

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Teardown Integration Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	var err error
	paths.CertDir, err = ioutil.TempDir("", "silk-certs")
	Expect(err).NotTo(HaveOccurred())

	certWriter, err := testsupport.NewCertWriter(paths.CertDir)
	Expect(err).NotTo(HaveOccurred())

	paths.ServerCACertFile, err = certWriter.WriteCA("server-ca")
	Expect(err).NotTo(HaveOccurred())
	paths.ServerCertFile, paths.ServerKeyFile, err = certWriter.WriteAndSign("server", "server-ca")
	Expect(err).NotTo(HaveOccurred())

	paths.ClientCACertFile, err = certWriter.WriteCA("client-ca")
	Expect(err).NotTo(HaveOccurred())
	paths.ClientCertFile, paths.ClientKeyFile, err = certWriter.WriteAndSign("client", "client-ca")
	Expect(err).NotTo(HaveOccurred())

	fmt.Fprintf(GinkgoWriter, "building binary...")
	paths.TeardownBin, err = gexec.Build("code.cloudfoundry.org/silk/cmd/silk-teardown", "-race", "-buildvcs=false")
	fmt.Fprintf(GinkgoWriter, "done")
	Expect(err).NotTo(HaveOccurred())

	data, err := json.Marshal(paths)
	Expect(err).NotTo(HaveOccurred())

	return data
}, func(data []byte) {
	Expect(json.Unmarshal(data, &paths)).To(Succeed())

	rand.Seed(GinkgoRandomSeed() + int64(GinkgoParallelProcess()))
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	gexec.CleanupBuildArtifacts()
	os.Remove(paths.CertDir)
})
