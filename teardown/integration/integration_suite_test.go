package integration_test

import (
	"encoding/json"
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo"
	ginkgoConfig "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var binaryPaths = map[string]string{}

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Teardown Integration Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	var err error
	fmt.Fprintf(GinkgoWriter, "building binaries...")
	binaryPaths["silk-setup"], err = gexec.Build("code.cloudfoundry.org/silk/cmd/silk-setup", "-race")
	Expect(err).NotTo(HaveOccurred())
	binaryPaths["silk-teardown"], err = gexec.Build("code.cloudfoundry.org/silk/cmd/silk-teardown", "-race")
	Expect(err).NotTo(HaveOccurred())
	fmt.Fprintf(GinkgoWriter, "done")

	bytes, _ := json.Marshal(binaryPaths)
	return bytes
}, func(data []byte) {
	Expect(json.Unmarshal(data, &binaryPaths)).To(Succeed())
	rand.Seed(ginkgoConfig.GinkgoConfig.RandomSeed + int64(GinkgoParallelNode()))
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	gexec.CleanupBuildArtifacts()
})
