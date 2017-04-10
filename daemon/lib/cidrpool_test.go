package lib_test

import (
	"net"

	"code.cloudfoundry.org/silk/daemon/lib"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cidrpool", func() {
	Describe("Size", func() {
		DescribeTable("returns the number of subnets that can be allocated",
			func(subnetRange string, subnetMask, expectedSize int) {
				cidrPool := lib.NewCIDRPool(subnetRange, subnetMask)
				Expect(cidrPool.Size()).To(Equal(expectedSize))
			},
			Entry("when the range is /16 and mask is /24", "10.255.0.0/16", 24, 255),
			Entry("when the range is /16 and mask is /20", "10.255.0.0/16", 20, 15),
			Entry("when the range is /16 and mask is /16", "10.255.0.0/16", 16, 0),
		)
	})

	Describe("GetRandom", func() {
		It("returns a random subnet from the pool", func() {
			subnetRange := "10.255.0.0/16"
			_, network, _ := net.ParseCIDR(subnetRange)
			cidrPool := lib.NewCIDRPool(subnetRange, 24)

			results := map[string]int{}

			for i := 0; i < 20; i++ {
				s := cidrPool.GetRandom()
				results[s]++
			}

			for result, count := range results {
				_, subnet, err := net.ParseCIDR(result)
				Expect(err).NotTo(HaveOccurred())
				Expect(network.Contains(subnet.IP)).To(BeTrue())
				Expect(subnet.Mask).To(Equal(net.IPMask{255, 255, 255, 0}))
				Expect(count).To(BeNumerically("<", 4))
			}
		})
	})
})
