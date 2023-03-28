package poller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestPoller(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Poller Suite")
}
