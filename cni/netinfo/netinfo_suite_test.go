package netinfo_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestLegacyFlannel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CNI Netinfo Suite")
}
