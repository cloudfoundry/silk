package leaser_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestLeaser(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Leaser Suite")
}
