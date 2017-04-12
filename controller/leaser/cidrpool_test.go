package leaser_test

import (
	"net"

	"code.cloudfoundry.org/silk/controller/leaser"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cidrpool", func() {
	Describe("Size", func() {
		DescribeTable("returns the number of subnets that can be allocated",
			func(subnetRange string, subnetMask, expectedSize int) {
				cidrPool := leaser.NewCIDRPool(subnetRange, subnetMask)
				Expect(cidrPool.Size()).To(Equal(expectedSize))
			},
			Entry("when the range is /16 and mask is /24", "10.255.0.0/16", 24, 255),
			Entry("when the range is /16 and mask is /20", "10.255.0.0/16", 20, 15),
			Entry("when the range is /16 and mask is /16", "10.255.0.0/16", 16, 0),
		)
	})

	Describe("GetAvailable", func() {
		It("returns a subnet from the pool that is not taken", func() {
			subnetRange := "10.255.0.0/16"
			_, network, _ := net.ParseCIDR(subnetRange)
			cidrPool := leaser.NewCIDRPool(subnetRange, 24)

			results := map[string]int{}

			taken := []string{}
			for i := 0; i < 255; i++ {
				s, err := cidrPool.GetAvailable(taken)
				Expect(err).NotTo(HaveOccurred())
				results[s]++
				taken = append(taken, s)
			}
			Expect(len(results)).To(Equal(255))

			for result, _ := range results {
				_, subnet, err := net.ParseCIDR(result)
				Expect(err).NotTo(HaveOccurred())
				Expect(network.Contains(subnet.IP)).To(BeTrue())
				Expect(subnet.Mask).To(Equal(net.IPMask{255, 255, 255, 0}))
			}
		})

		Context("when no subnets are available", func() {
			It("returns an error", func() {
				subnetRange := "10.255.0.0/16"
				cidrPool := leaser.NewCIDRPool(subnetRange, 24)
				taken := []string{}
				for i := 0; i < 255; i++ {
					s, err := cidrPool.GetAvailable(taken)
					Expect(err).NotTo(HaveOccurred())
					taken = append(taken, s)
				}
				_, err := cidrPool.GetAvailable(taken)
				Expect(err).To(MatchError("no subnets available"))
			})
		})
	})
})
