package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"

	"code.cloudfoundry.org/go-db-helpers/testsupport"

	. "github.com/onsi/ginkgo"
	ginkgoConfig "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var (
	certDir string
	paths   testPaths
)

type testPaths struct {
	ServerCACertFile string
	ClientCACertFile string
	ServerCertFile   string
	ServerKeyFile    string
	ClientCertFile   string
	ClientKeyFile    string
	DaemonBin        string
}

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Daemon Integration Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	var err error
	certDir, err = ioutil.TempDir("", "silk-certs")
	Expect(err).NotTo(HaveOccurred())

	// TODO
	// Stop doing this!
	// Change CertWriter to use certstrap as a library instead
	certstrapBin := fmt.Sprintf("/%s/certstrap", certDir)
	cmd := exec.Command("go", "build", "-o", certstrapBin, ".")
	fmt.Println(os.Getwd())
	cmd.Dir = "../../vendor/github.com/square/certstrap"
	Expect(cmd.Run()).NotTo(HaveOccurred())

	certWriter := &testsupport.CertWriter{
		BinPath:  certstrapBin,
		CertPath: certDir,
	}

	paths.ServerCACertFile, err = certWriter.WriteCA("server-ca")
	Expect(err).NotTo(HaveOccurred())
	paths.ServerCertFile, paths.ServerKeyFile, err = certWriter.WriteAndSignForServer("server", "server-ca")
	Expect(err).NotTo(HaveOccurred())

	paths.ClientCACertFile, err = certWriter.WriteCA("client-ca")
	Expect(err).NotTo(HaveOccurred())
	paths.ClientCertFile, paths.ClientKeyFile, err = certWriter.WriteAndSignForClient("client", "client-ca")
	Expect(err).NotTo(HaveOccurred())

	fmt.Fprintf(GinkgoWriter, "building binary...")
	paths.DaemonBin, err = gexec.Build("code.cloudfoundry.org/silk/cmd/silk-daemon", "-race")
	fmt.Fprintf(GinkgoWriter, "done")
	Expect(err).NotTo(HaveOccurred())

	data, err := json.Marshal(paths)
	Expect(err).NotTo(HaveOccurred())

	return data
}, func(data []byte) {
	Expect(json.Unmarshal(data, &paths)).To(Succeed())

	rand.Seed(ginkgoConfig.GinkgoConfig.RandomSeed + int64(GinkgoParallelNode()))
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	gexec.CleanupBuildArtifacts()
	os.Remove(certDir)
})
