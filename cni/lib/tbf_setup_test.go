package lib

import (
	"errors"

	"code.cloudfoundry.org/silk/cni/config"
	"code.cloudfoundry.org/silk/cni/lib/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

var _ = Describe("TokenBucketFilter Setup", func() {

	var (
		cfg                *config.Config
		fakeNetlinkAdapter *fakes.NetlinkAdapter
		tbf                TokenBucketFilter
		fakeLink           netlink.Link
	)

	BeforeEach(func() {
		fakeLink = &netlink.Bridge{
			LinkAttrs: netlink.LinkAttrs{
				Name: "my-fake-bridge",
			},
		}
		fakeNetlinkAdapter = &fakes.NetlinkAdapter{}
		fakeNetlinkAdapter.LinkByNameReturns(fakeLink, nil)
		tbf = TokenBucketFilter{
			NetlinkAdapter: fakeNetlinkAdapter,
		}
		cfg = &config.Config{}
		cfg.Host.DeviceName = "host-device"
	})

	It("creates a qdisc tbf", func() {
		Expect(tbf.Setup(1400, 1400, cfg)).To(Succeed())

		Expect(fakeNetlinkAdapter.LinkByNameCallCount()).To(Equal(1))
		Expect(fakeNetlinkAdapter.LinkByNameArgsForCall(0)).To(Equal("host-device"))

		Expect(fakeNetlinkAdapter.QdiscAddCallCount()).To(Equal(1))
		Expect(fakeNetlinkAdapter.QdiscAddArgsForCall(0)).To(Equal(&netlink.Tbf{
			QdiscAttrs: netlink.QdiscAttrs{
				LinkIndex: fakeLink.Attrs().Index,
				Handle:    65536,
				Parent:    netlink.HANDLE_ROOT,
			},
			Rate:   uint64(175),
			Limit:  uint32(17),
			Buffer: uint32(125000000),
		}))

	})

	Context("when getting the link for the host fails", func() {
		BeforeEach(func() {
			fakeNetlinkAdapter.LinkByNameReturns(nil, errors.New("banana"))
		})
		It("returns a sensible error", func() {
			err := tbf.Setup(1400, 1400, cfg)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("get host device: banana"))
		})
	})

	Context("when creating the qdisc fails", func() {
		BeforeEach(func() {
			fakeNetlinkAdapter.QdiscAddReturns(errors.New("banana"))
		})
		It("returns a sensible error", func() {
			err := tbf.Setup(1400, 1400, cfg)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("create qdisc: banana"))
		})
	})

	Context("when the burst is invalid", func() {
		BeforeEach(func() {
			tbf = TokenBucketFilter{
				NetlinkAdapter: fakeNetlinkAdapter,
			}
		})
		It("returns a sensible error", func() {
			err := tbf.Setup(1400, 0, cfg)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("invalid burst: 0"))
		})
	})

	Context("when the burst is invalid", func() {
		BeforeEach(func() {
			tbf = TokenBucketFilter{
				NetlinkAdapter: fakeNetlinkAdapter,
			}
		})
		It("returns a sensible error", func() {
			err := tbf.Setup(0, 1400, cfg)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("invalid rate: 0"))
		})
	})
})
