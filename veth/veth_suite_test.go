package veth_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestVeth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Veth Suite")
}
