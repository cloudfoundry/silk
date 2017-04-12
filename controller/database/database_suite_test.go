package database_test

import (
	"math/rand"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDatabase(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Database Suite")
}

var _ = BeforeSuite(func() {
	rand.Seed(config.GinkgoConfig.RandomSeed + int64(GinkgoParallelNode()))
})
