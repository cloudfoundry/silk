package legacy_flannel_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestLegacyFlannel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LegacyFlannel Suite")
}
