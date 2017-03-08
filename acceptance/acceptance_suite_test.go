package acceptance_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"path"

	. "github.com/onsi/ginkgo"
	ginkgoConfig "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

func TestAcceptance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Acceptance Suite")
}

var paths testPaths

type testPaths struct {
	PathToPlugin string
	CNIPath      string
}

var _ = SynchronizedBeforeSuite(func() []byte {
	var err error
	pathToSilkCNI, err := gexec.Build("github.com/cloudfoundry-incubator/silk", "-race")
	Expect(err).NotTo(HaveOccurred())

	pathToIPAM, err := gexec.Build("github.com/cloudfoundry-incubator/silk/vendor/github.com/containernetworking/cni/plugins/ipam/host-local", "-race")
	Expect(err).NotTo(HaveOccurred())

	paths = testPaths{
		PathToPlugin: pathToSilkCNI,
		CNIPath:      fmt.Sprintf("%s:%s", path.Dir(pathToSilkCNI), path.Dir(pathToIPAM)),
	}

	bytes, err := json.Marshal(paths)
	Expect(err).NotTo(HaveOccurred())
	return bytes
}, func(data []byte) {
	Expect(json.Unmarshal(data, &paths)).To(Succeed())

	rand.Seed(ginkgoConfig.GinkgoConfig.RandomSeed + int64(GinkgoParallelNode()))
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	gexec.CleanupBuildArtifacts()
})
