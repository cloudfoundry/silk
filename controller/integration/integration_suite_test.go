package integration_test

import (
	"fmt"
	"math/rand"
	"net/http"

	. "github.com/onsi/ginkgo"
	ginkgoConfig "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

const DEFAULT_TIMEOUT = "5s"

var controllerBinaryPath string

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Integration Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	controllerBinaryPath, err := gexec.Build(
		"code.cloudfoundry.org/silk/cmd/silk-controller",
		"-race",
	)
	Expect(err).NotTo(HaveOccurred())
	return []byte(controllerBinaryPath)
}, func(data []byte) {
	controllerBinaryPath = string(data)
	rand.Seed(ginkgoConfig.GinkgoConfig.RandomSeed + int64(GinkgoParallelNode()))
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	gexec.CleanupBuildArtifacts()
})

func VerifyHTTPConnection(baseURL string) error {
	resp, err := http.Get(baseURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected server to respond %d but got %d", http.StatusOK, resp.StatusCode)
	}
	return nil
}
