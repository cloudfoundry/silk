package integration_test

import (
	"fmt"
	"math/rand"
	"strings"

	. "github.com/onsi/ginkgo"
	ginkgoConfig "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var setupPath string
var teardownPath string

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Teardown Integration Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	fmt.Fprintf(GinkgoWriter, "building binaries...")
	setupPath, err := gexec.Build("code.cloudfoundry.org/silk/cmd/silk-setup", "-race")
	Expect(err).NotTo(HaveOccurred())
	teardownPath, err := gexec.Build("code.cloudfoundry.org/silk/cmd/silk-teardown", "-race")
	Expect(err).NotTo(HaveOccurred())
	fmt.Fprintf(GinkgoWriter, "done")

	return []byte(setupPath + "###" + teardownPath)
}, func(data []byte) {
	paths := strings.Split(string(data), "###")
	setupPath = paths[0]
	teardownPath = paths[1]
	rand.Seed(ginkgoConfig.GinkgoConfig.RandomSeed + int64(GinkgoParallelNode()))
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	gexec.CleanupBuildArtifacts()
})
