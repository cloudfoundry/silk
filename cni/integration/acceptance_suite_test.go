package integration_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"path"

	"github.com/containernetworking/plugins/pkg/ns"
	. "github.com/onsi/ginkgo"
	ginkgoConfig "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CNI Integration Suite")
}

var (
	paths  testPaths
	hostNS ns.NetNS
)

type testPaths struct {
	PathToPlugin     string
	CNIPath          string
	PathToFakeDaemon string
}

var _ = SynchronizedBeforeSuite(func() []byte {
	var err error

	hostNS, err = ns.GetCurrentNS()
	Expect(err).NotTo(HaveOccurred())

	pathToSilkCNI, err := gexec.Build("code.cloudfoundry.org/silk/cmd/silk-cni", `-ldflags="-extldflags=-Wl,--allow-multiple-definition"`, "-race")
	Expect(err).NotTo(HaveOccurred())

	pathToIPAM, err := gexec.Build("code.cloudfoundry.org/silk/vendor/github.com/containernetworking/plugins/plugins/ipam/host-local", "-race")
	Expect(err).NotTo(HaveOccurred())

	pathToFakeDaemon, err := gexec.Build("code.cloudfoundry.org/silk/cni/integration/fake_daemon", "-race")
	Expect(err).NotTo(HaveOccurred())

	paths = testPaths{
		PathToPlugin:     pathToSilkCNI,
		CNIPath:          fmt.Sprintf("%s:%s", path.Dir(pathToSilkCNI), path.Dir(pathToIPAM)),
		PathToFakeDaemon: pathToFakeDaemon,
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
