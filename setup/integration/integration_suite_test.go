package integration_test

import (
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo"
	ginkgoConfig "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var setupPath string

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Setup Integration Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	fmt.Fprintf(GinkgoWriter, "building binary...")
	setupPath, err := gexec.Build("code.cloudfoundry.org/silk/cmd/silk-setup", "-race")
	fmt.Fprintf(GinkgoWriter, "done")
	Expect(err).NotTo(HaveOccurred())

	return []byte(setupPath)
}, func(data []byte) {
	setupPath = string(data)
	rand.Seed(ginkgoConfig.GinkgoConfig.RandomSeed + int64(GinkgoParallelNode()))
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	gexec.CleanupBuildArtifacts()
})
