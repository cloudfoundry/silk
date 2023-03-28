package planner_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestPlanner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Planner Suite")
}
