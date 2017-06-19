package lib

import (
	"errors"
	"syscall"

	"code.cloudfoundry.org/silk/cni/config"
	"code.cloudfoundry.org/silk/cni/lib/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

var _ = Describe("TokenBucketFilter", func() {
	// TODO rename this from TokenBucketFilter. maybe make two different structs for outbound and inbound

	var (
		cfg                *config.Config
		fakeNetlinkAdapter *fakes.NetlinkAdapter
		tbf                TokenBucketFilter
		fakeHostDevice     netlink.Link
		fakeIFBDevice      netlink.Link
	)

	BeforeEach(func() {
		fakeHostDevice = &netlink.Bridge{
			LinkAttrs: netlink.LinkAttrs{
				Index: 42,
				Name:  "my-fake-host-device",
			},
		}
		fakeIFBDevice = &netlink.Bridge{
			LinkAttrs: netlink.LinkAttrs{
				Index: 43,
				Name:  "my-fake-host-device",
			},
		}
		fakeNetlinkAdapter = &fakes.NetlinkAdapter{}
		fakeNetlinkAdapter.LinkByNameStub = func(name string) (netlink.Link, error) {
			if name == "host-device" {
				return fakeHostDevice, nil
			} else if name == "ifb-device" {
				return fakeIFBDevice, nil
			}
			return &netlink.Bridge{}, errors.New("invalid")
		}
		tbf = TokenBucketFilter{
			NetlinkAdapter: fakeNetlinkAdapter,
		}
		cfg = &config.Config{}
		cfg.Host.DeviceName = "host-device"
		cfg.IFB.DeviceName = "ifb-device"
	})

	Describe("Setup", func() {
		It("creates a qdisc tbf", func() {
			Expect(tbf.Setup(1400, 1400, cfg)).To(Succeed())

			Expect(fakeNetlinkAdapter.LinkByNameCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkByNameArgsForCall(0)).To(Equal("host-device"))

			Expect(fakeNetlinkAdapter.QdiscAddCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.QdiscAddArgsForCall(0)).To(Equal(&netlink.Tbf{
				QdiscAttrs: netlink.QdiscAttrs{
					LinkIndex: fakeHostDevice.Attrs().Index,
					Handle:    netlink.MakeHandle(1, 0),
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

		Context("when the rate is invalid", func() {
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

	Describe("OutboundSetup", func() {
		It("attaches the ifb device to the host interface then creates a qdisc tbf", func() {
			Expect(tbf.OutboundSetup(1400, 1400, cfg)).To(Succeed())

			Expect(fakeNetlinkAdapter.LinkByNameCallCount()).To(Equal(2))
			Expect(fakeNetlinkAdapter.LinkByNameArgsForCall(0)).To(Equal("ifb-device"))
			Expect(fakeNetlinkAdapter.LinkByNameArgsForCall(1)).To(Equal("host-device"))

			Expect(fakeNetlinkAdapter.QdiscAddCallCount()).To(Equal(2))
			Expect(fakeNetlinkAdapter.QdiscAddArgsForCall(0)).To(Equal(&netlink.Ingress{
				QdiscAttrs: netlink.QdiscAttrs{
					LinkIndex: fakeHostDevice.Attrs().Index,
					Handle:    netlink.MakeHandle(0xffff, 0),
					Parent:    netlink.HANDLE_INGRESS,
				},
			}))
			Expect(fakeNetlinkAdapter.QdiscAddArgsForCall(1)).To(Equal(&netlink.Tbf{
				QdiscAttrs: netlink.QdiscAttrs{
					LinkIndex: fakeIFBDevice.Attrs().Index,
					Handle:    netlink.MakeHandle(1, 0),
					Parent:    netlink.HANDLE_ROOT,
				},
				Rate:   uint64(175),
				Limit:  uint32(17),
				Buffer: uint32(125000000),
			}))

			Expect(fakeNetlinkAdapter.FilterAddCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.FilterAddArgsForCall(0)).To(Equal(&netlink.U32{
				FilterAttrs: netlink.FilterAttrs{
					LinkIndex: fakeHostDevice.Attrs().Index,
					Handle:    0,
					Parent:    netlink.MakeHandle(0xffff, 0),
					Priority:  1,
					Protocol:  syscall.ETH_P_ALL,
				},
				ClassId:    netlink.MakeHandle(1, 1),
				RedirIndex: 43,
				Actions: []netlink.Action{
					&netlink.MirredAction{
						ActionAttrs:  netlink.ActionAttrs{},
						MirredAction: netlink.TCA_EGRESS_REDIR,
						Ifindex:      fakeIFBDevice.Attrs().Index,
					},
				},
			}))
		})

		Context("when getting the link for the ifb fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkByNameStub = func(name string) (netlink.Link, error) {
					if name == "host-device" {
						return fakeHostDevice, nil
					} else if name == "ifb-device" {
						return &netlink.Bridge{}, errors.New("banana")
					}
					return &netlink.Bridge{}, errors.New("invalid")
				}
			})
			It("returns a sensible error", func() {
				err := tbf.OutboundSetup(1400, 1400, cfg)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("get ifb device: banana"))
			})
		})

		Context("when getting the link for the host fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkByNameStub = func(name string) (netlink.Link, error) {
					if name == "host-device" {
						return &netlink.Bridge{}, errors.New("banana")
					} else if name == "ifb-device" {
						return fakeIFBDevice, nil
					}
					return &netlink.Bridge{}, errors.New("invalid")
				}
			})
			It("returns a sensible error", func() {
				err := tbf.OutboundSetup(1400, 1400, cfg)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("get host device: banana"))
			})
		})

		Context("when creating the ingress qdisc fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.QdiscAddStub = func(qdisc netlink.Qdisc) error {
					if _, ok := qdisc.(*netlink.Ingress); ok {
						return errors.New("banana")
					}
					return nil
				}
			})
			It("returns a sensible error", func() {
				err := tbf.OutboundSetup(1400, 1400, cfg)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("create ingress qdisc: banana"))
			})
		})

		Context("when adding a filter to the host device fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.FilterAddReturns(errors.New("filter-fail"))
			})

			It("returns a sensible error", func() {
				err := tbf.OutboundSetup(1400, 1400, cfg)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("add filter: filter-fail"))
			})
		})

		Context("when the burst is invalid", func() {
			It("returns a sensible error", func() {
				err := tbf.OutboundSetup(1400, 0, cfg)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("create ifb qdisc: invalid burst: 0"))
			})
		})

		Context("when the rate is invalid", func() {
			It("returns a sensible error", func() {
				err := tbf.OutboundSetup(0, 1400, cfg)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("create ifb qdisc: invalid rate: 0"))
			})
		})

		Context("when creating the tbf qdisc fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.QdiscAddStub = func(qdisc netlink.Qdisc) error {
					if _, ok := qdisc.(*netlink.Tbf); ok {
						return errors.New("banana")
					}
					return nil
				}
			})
			It("returns a sensible error", func() {
				err := tbf.OutboundSetup(1400, 1400, cfg)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("create ifb qdisc: create qdisc: banana"))
			})
		})
	})
})
