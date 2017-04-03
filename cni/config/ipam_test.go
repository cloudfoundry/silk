package config_test

import (
	"net"

	"github.com/cloudfoundry-incubator/silk/cni/config"
	"github.com/containernetworking/cni/pkg/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Ipam config generation", func() {
	It("returns IPAM config object", func() {

		generator := config.IPAMConfigGenerator{}
		ipamConfig := generator.GenerateConfig("10.255.30.0/24", "some-network-name", "/some/data/dir")

		Expect(ipamConfig).To(Equal(
			&config.HostLocalIPAM{
				CNIVersion: "0.3.0",
				Name:       "some-network-name",
				IPAM: config.IPAM{
					Type:    "host-local",
					Subnet:  "10.255.30.0/24",
					Gateway: "169.254.0.1",
					Routes: []*types.Route{
						&types.Route{
							Dst: net.IPNet{
								IP:   net.IPv4zero,
								Mask: net.CIDRMask(0, 32),
							},
							GW: net.ParseIP("169.254.0.1"),
						},
					},
					DataDir: "/some/data/dir/ipam",
				},
			}))
	})

})
