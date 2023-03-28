package config_test

import (
	"code.cloudfoundry.org/silk/cni/config"
	"github.com/containernetworking/cni/pkg/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Ipam config generation", func() {
	It("returns IPAM config object", func() {

		generator := config.IPAMConfigGenerator{}
		ipamConfig, err := generator.GenerateConfig("10.255.30.0/24", "some-network-name", "/some/data/dir")
		Expect(err).NotTo(HaveOccurred())

		subnetAsIPNet, err := types.ParseCIDR("10.255.30.0/24")
		Expect(err).NotTo(HaveOccurred())

		Expect(ipamConfig).To(Equal(
			&config.HostLocalIPAM{
				CNIVersion: "0.3.1",
				Name:       "some-network-name",
				IPAM: config.IPAMConfig{
					Type: "host-local",
					Ranges: []config.RangeSet{
						[]config.Range{
							{
								Subnet: types.IPNet(*subnetAsIPNet),
							},
						}},
					Routes:  []*types.Route{},
					DataDir: "/some/data/dir/ipam",
				},
			}))
	})
	Context("when the subnet is invalid", func() {
		It("returns an error", func() {
			generator := config.IPAMConfigGenerator{}
			_, err := generator.GenerateConfig("10.255.30.0/33", "some-network-name", "/some/data/dir")
			Expect(err).To(MatchError("invalid subnet: invalid CIDR address: 10.255.30.0/33"))
		})
	})
})
