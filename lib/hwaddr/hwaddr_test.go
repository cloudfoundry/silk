package hwaddr_test

import (
	"fmt"
	"net"

	"code.cloudfoundry.org/silk/lib/hwaddr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hwaddr", func() {

	Describe("GenerateHardwareAddr4", func() {
		validPrefix := []byte{0xaa, 0xbb}
		ipV4Addr := net.ParseIP("192.168.1.1")
		ipV6Addr := net.ParseIP("2001:db8::68")
		Context("when the provided IP isn't ipv4", func() {
			It("returns an error", func() {
				_, err := hwaddr.GenerateHardwareAddr4(ipV6Addr, validPrefix)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(fmt.Errorf("2001:db8::68 is not an IPv4 address")))
			})
		})
		DescribeTable("when the provided prefix isn't 2 bytes", func(prefix []byte) {
			_, err := hwaddr.GenerateHardwareAddr4(ipV4Addr, prefix)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Errorf("Prefix length should be 2 bytes, but received %d bytes", len(prefix))))
		},
			Entry("empty prefix", []byte{}),
			Entry("< 8 bytes", []byte{0xaa}),
			Entry("> 8 bytes", []byte{0xaa, 0xbb, 0xcc}),
		)
		It("returns a MAC addr with the given prefix, based on the provided IP", func() {
			addr, err := hwaddr.GenerateHardwareAddr4(ipV4Addr, validPrefix)
			Expect(err).ToNot(HaveOccurred())
			// IP variables are []byte types, with len 16. IPv4 addrs only use the last 4 bytes for addr info
			Expect(addr.String()).To(Equal(fmt.Sprintf("aa:bb:%02x:%02x:%02x:%02x", ipV4Addr[12], ipV4Addr[13], ipV4Addr[14], ipV4Addr[15])))
		})
	})
})
