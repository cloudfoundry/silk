package integration_test

import (
	"math/rand"

	"code.cloudfoundry.org/cf-networking-helpers/testsupport"

	. "github.com/onsi/ginkgo"
	ginkgoConfig "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/types"

	"testing"
)

var controllerBinaryPath string

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Integration Suite")
}

var HaveName = func(name string) types.GomegaMatcher {
	return WithTransform(func(ev testsupport.Event) string {
		return ev.Name
	}, Equal(name))
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
