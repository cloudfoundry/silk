package lib_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestLib(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Daemon Lib Suite")
}
