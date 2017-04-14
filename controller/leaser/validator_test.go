package leaser_test

import (
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/leaser"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validator", func() {
	var (
		validator *leaser.LeaseValidator
		lease     controller.Lease
	)

	BeforeEach(func() {
		lease = controller.Lease{
			UnderlayIP:          "1.2.3.4",
			OverlaySubnet:       "5.4.3.2/24",
			OverlayHardwareAddr: "aa:bb:cc:dd:ee:ff",
		}
		validator = &leaser.LeaseValidator{}
	})

	It("checks that the lease is valid", func() {
		err := validator.Validate(lease)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when the underlay ip is not a valid ip", func() {
		BeforeEach(func() {
			lease.UnderlayIP = "not-an-ip"
		})
		It("returns an error", func() {
			err := validator.Validate(lease)
			Expect(err).To(MatchError("invalid underlay ip: not-an-ip"))
		})
	})

	Context("when the overlay subnet is invalid", func() {
		BeforeEach(func() {
			lease.OverlaySubnet = "not-a-subnet"
		})
		It("returns an error", func() {
			err := validator.Validate(lease)
			Expect(err).To(MatchError("invalid CIDR address: not-a-subnet"))
		})
	})

	Context("when the hardware addr is invalid is invalid", func() {
		BeforeEach(func() {
			lease.OverlayHardwareAddr = "not-a-mac"
		})
		It("returns an error", func() {
			err := validator.Validate(lease)
			Expect(err).To(MatchError(ContainSubstring("invalid MAC address")))
		})
	})
})
