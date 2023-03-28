package database_test

import (
	"math/rand"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDatabase(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Database Suite")
}

var _ = BeforeSuite(func() {
	rand.Seed(GinkgoRandomSeed() + int64(GinkgoParallelProcess()))
})
