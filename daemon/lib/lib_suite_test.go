package lib_test

import (
	"math/rand"

	. "github.com/onsi/ginkgo"
	ginkgoConfig "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"

	"testing"
)

func TestLib(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Daemon Lib Suite")
}

var _ = BeforeSuite(func() {
	rand.Seed(ginkgoConfig.GinkgoConfig.RandomSeed + int64(GinkgoParallelNode()))
})
