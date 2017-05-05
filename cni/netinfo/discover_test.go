package netinfo_test

import (
	"errors"

	"code.cloudfoundry.org/silk/cni/netinfo"
	"code.cloudfoundry.org/silk/cni/netinfo/fakes"
	"code.cloudfoundry.org/silk/daemon"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Discover", func() {
	var (
		discoverer  *netinfo.Discoverer
		fakeNetInfo *fakes.NetInfo
	)

	BeforeEach(func() {
		fakeNetInfo = &fakes.NetInfo{}
		discoverer = &netinfo.Discoverer{
			NetInfo: fakeNetInfo,
		}

		fakeNetInfo.GetReturns(daemon.NetworkInfo{
			OverlaySubnet: "1.2.3.4/23",
			MTU:           4321,
		}, nil)
	})

	Context("when it is called with zero MTU", func() {
		It("gets the netinfo and sets the MTU based on the netinfo", func() {
			info, err := discoverer.Discover(0)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeNetInfo.GetCallCount()).To(Equal(1))
			Expect(info).To(Equal(daemon.NetworkInfo{
				OverlaySubnet: "1.2.3.4/23",
				MTU:           4321,
			}))
		})
	})

	Context("when it is called with MTU greater than 0", func() {
		It("overrides the MTU from the netinfo", func() {
			info, err := discoverer.Discover(42)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeNetInfo.GetCallCount()).To(Equal(1))
			Expect(info).To(Equal(daemon.NetworkInfo{
				OverlaySubnet: "1.2.3.4/23",
				MTU:           42,
			}))
		})
	})

	Context("when getting the netinfo fails", func() {
		BeforeEach(func() {
			fakeNetInfo.GetReturns(daemon.NetworkInfo{}, errors.New("banana"))
		})
		It("returns an error", func() {
			_, err := discoverer.Discover(42)
			Expect(err).To(MatchError("get netinfo: banana"))
		})
	})
})
