package leaser_test

import (
	"code.cloudfoundry.org/silk/controller/leaser"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"net"
)

var _ = Describe("CIDRPool", func() {
	Describe("BlockPoolSize", func() {
		DescribeTable("returns the number of subnets that can be allocated",
			func(subnetRange string, subnetMask, expectedSize int) {
				cidrPool := leaser.NewCIDRPool(subnetRange, subnetMask)
				Expect(cidrPool.BlockPoolSize()).To(Equal(expectedSize))
			},
			Entry("when the range is /16 and mask is /24", "10.255.0.0/16", 24, 255),
			Entry("when the range is /16 and mask is /20", "10.255.0.0/16", 20, 15),
			Entry("when the range is /16 and mask is /16", "10.255.0.0/16", 16, 0),
		)

		DescribeTable("produces valid subnets within the correct range",
			func(overlayCIDR string, subnetMask int) {
				_, networkWithFirstIp, _ := net.ParseCIDR(overlayCIDR)
				cidrPool := leaser.NewCIDRPool(overlayCIDR, subnetMask)

				for blockDividedCIDR, _ := range cidrPool.GetBlockPool() {
					_, ipNet, _ := net.ParseCIDR(blockDividedCIDR)
					Expect(networkWithFirstIp.Contains(ipNet.IP)).Should(BeTrue())
				}
			},
			Entry("when ip is in the start of the range", "10.240.0.0/12", 24),
			Entry("when ip is in the middle of the range", "10.255.0.0/12", 24),
			Entry("when ip is in the end of the range", "10.255.255.255/12", 24),
		)
	})

	Describe("SingleIPPoolSize", func() {
		DescribeTable("returns the number of subnets that can be allocated",
			func(subnetRange string, subnetMask, expectedSize int) {
				cidrPool := leaser.NewCIDRPool(subnetRange, subnetMask)
				Expect(cidrPool.SingleIPPoolSize()).To(Equal(expectedSize))
			},
			Entry("when the range is /16 and mask is /25", "10.255.0.0/16", 25, 127),
			Entry("when the range is /16 and mask is /26", "10.255.0.0/16", 26, 63),
			Entry("when the range is /16 and mask is /27", "10.255.0.0/16", 27, 31),
		)
	})

	Describe("GetAvailableBlock", func() {
		It("returns a subnet from the pool that is not taken", func() {
			subnetRange := "10.255.0.0/16"
			_, network, _ := net.ParseCIDR(subnetRange)
			cidrPool := leaser.NewCIDRPool(subnetRange, 24)

			results := map[string]int{}

			var taken []string
			for i := 0; i < 255; i++ {
				s := cidrPool.GetAvailableBlock(taken)
				results[s]++
				taken = append(taken, s)
			}
			Expect(len(results)).To(Equal(255))

			for result := range results {
				_, subnet, err := net.ParseCIDR(result)
				Expect(err).NotTo(HaveOccurred())
				Expect(network.Contains(subnet.IP)).To(BeTrue())
				Expect(subnet.Mask).To(Equal(net.IPMask{255, 255, 255, 0}))
				// first subnet from range is never allocated
				Expect(subnet.IP.To4()).NotTo(Equal(network.IP.To4()))
			}
		})

		Context("when no subnets are available", func() {
			It("returns an empty string", func() {
				subnetRange := "10.255.0.0/16"
				cidrPool := leaser.NewCIDRPool(subnetRange, 24)
				var taken []string
				for i := 0; i < 255; i++ {
					s := cidrPool.GetAvailableBlock(taken)
					taken = append(taken, s)
				}
				s := cidrPool.GetAvailableBlock(taken)
				Expect(s).To(Equal(""))
			})
		})
	})

	Describe("GetAvailableSingleIP", func() {
		It("returns a single ip that is not taken", func() {
			subnetRange := "10.255.0.0/16"
			_, network, _ := net.ParseCIDR(subnetRange)
			cidrPool := leaser.NewCIDRPool(subnetRange, 24)

			results := map[string]int{}

			var taken []string
			for i := 0; i < 255; i++ {
				s := cidrPool.GetAvailableSingleIP(taken)
				results[s]++
				taken = append(taken, s)
			}
			Expect(len(results)).To(Equal(255))

			for result := range results {
				_, subnet, err := net.ParseCIDR(result)
				Expect(err).NotTo(HaveOccurred())
				Expect(network.Contains(subnet.IP)).To(BeTrue())
				Expect(subnet.Mask).To(Equal(net.IPMask{255, 255, 255, 255}))
				// 10.255.0.0 should never be allocated? Maybe?
				Expect(subnet.IP.To4()).NotTo(Equal(network.IP.To4()))
			}
		})

		Context("when the subnet mask is 29", func() {
			It("returns ips containing only .1-.7", func() {
				subnetRange := "10.255.0.0/16"
				_, network, _ := net.ParseCIDR(subnetRange)
				cidrPool := leaser.NewCIDRPool(subnetRange, 29)

				results := map[string]int{}

				var taken []string
				for i := 0; i < 7; i++ {
					s := cidrPool.GetAvailableSingleIP(taken)
					results[s]++
					taken = append(taken, s)
				}
				Expect(len(results)).To(Equal(7))

				Expect(results).To(Equal(map[string]int{
					"10.255.0.1/32": 1,
					"10.255.0.2/32": 1,
					"10.255.0.3/32": 1,
					"10.255.0.4/32": 1,
					"10.255.0.5/32": 1,
					"10.255.0.6/32": 1,
					"10.255.0.7/32": 1,
				}))

				for result := range results {
					_, subnet, err := net.ParseCIDR(result)
					Expect(err).NotTo(HaveOccurred())
					Expect(network.Contains(subnet.IP)).To(BeTrue())
					Expect(subnet.Mask).To(Equal(net.IPMask{255, 255, 255, 255}))
					// 10.255.0.0 should never be allocated? Maybe?
					Expect(subnet.IP.To4()).NotTo(Equal(network.IP.To4()))
				}
			})
		})

		Context("when no subnets are available", func() {
			It("returns an empty string", func() {
				subnetRange := "10.255.0.0/16"
				cidrPool := leaser.NewCIDRPool(subnetRange, 24)
				var taken []string
				for i := 0; i < 255; i++ {
					s := cidrPool.GetAvailableSingleIP(taken)
					taken = append(taken, s)
				}
				s := cidrPool.GetAvailableSingleIP(taken)
				Expect(s).To(Equal(""))
			})
		})
	})

	Describe("IsMember", func() {
		var cidrPool *leaser.CIDRPool
		BeforeEach(func() {
			subnetRange := "10.255.0.0/16"
			cidrPool = leaser.NewCIDRPool(subnetRange, 24)
		})

		Context("when the subnet is in the block pool and not in the single pool", func() {
			It("returns true", func() {
				Expect(cidrPool.IsMember("10.255.30.0/24")).To(BeTrue())
			})
		})

		Context("when the subnet is in the single pool and not in the block pool", func() {
			It("returns true", func() {
				Expect(cidrPool.IsMember("10.255.0.5/32")).To(BeTrue())
			})
		})

		Context("when the subnet start is not a match for an entry", func() {
			It("returns false", func() {
				Expect(cidrPool.IsMember("10.255.30.10/24")).To(BeFalse())
			})
		})

		Context("when the subnet size is not a match", func() {
			It("returns false", func() {
				Expect(cidrPool.IsMember("10.255.30.0/20")).To(BeFalse())
			})
		})
	})
})
