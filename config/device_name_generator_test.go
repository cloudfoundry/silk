package config_test

import (
	"net"

	"github.com/cloudfoundry-incubator/silk/config"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DeviceNameGenerator", func() {
	Describe("GenerateForHost", func() {
		It("generates a valid Linux network device name from the given IPv4 address", func() {
			g := config.DeviceNameGenerator{}
			name, err := g.GenerateForHost(net.IP{10, 255, 30, 5})
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("s-010255030005"))
		})

		Context("when given an IPv6 address", func() {
			It("returns a meaningful error", func() {
				g := config.DeviceNameGenerator{}
				_, err := g.GenerateForHost(net.IPv6linklocalallnodes)
				Expect(err).To(MatchError("generating device name: expecting valid IPv4 address"))
			})
		})
	})
})
