package veth2_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestVeth2(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Veth2 Suite")
}
