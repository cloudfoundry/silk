package lib_test

import (
	"errors"
	"net"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"

	"github.com/cloudfoundry-incubator/silk/config"
	"github.com/cloudfoundry-incubator/silk/lib"
	"github.com/cloudfoundry-incubator/silk/lib/fakes"
)

var _ = Describe("Common", func() {
	Describe("BasicSetup", func() {
		var (
			fakeNetlinkAdapter *fakes.NetlinkAdapter
			fakeLinkOperations *fakes.LinkOperations
			fakeLink           netlink.Link
			deviceName         string
			local              config.DualAddress
			peer               config.DualAddress
			common             *lib.Common
		)
		BeforeEach(func() {
			fakeNetlinkAdapter = &fakes.NetlinkAdapter{}
			fakeLinkOperations = &fakes.LinkOperations{}
			fakeLink = &netlink.Bridge{
				LinkAttrs: netlink.LinkAttrs{
					Name: "my-fake-bridge",
				},
			}
			fakeNetlinkAdapter.LinkByNameReturns(fakeLink, nil)

			common = &lib.Common{
				NetlinkAdapter: fakeNetlinkAdapter,
				LinkOperations: fakeLinkOperations,
			}
			deviceName = "myDeviceName"
			localMAC, err := net.ParseMAC("aa:aa:12:34:56:78")
			Expect(err).NotTo(HaveOccurred())
			peerMAC, err := net.ParseMAC("ee:ee:12:34:56:78")
			Expect(err).NotTo(HaveOccurred())
			local = config.DualAddress{
				IP:       net.IP{10, 255, 30, 4},
				Hardware: localMAC,
			}
			peer = config.DualAddress{
				IP:       net.IP{169, 254, 0, 1},
				Hardware: peerMAC,
			}
		})

		It("sets up a veth device", func() {
			err := common.BasicSetup(deviceName, local, peer)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeNetlinkAdapter.LinkByNameCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkByNameArgsForCall(0)).To(Equal("myDeviceName"))

			Expect(fakeNetlinkAdapter.LinkSetHardwareAddrCallCount()).To(Equal(1))
			link, hwAddr := fakeNetlinkAdapter.LinkSetHardwareAddrArgsForCall(0)
			Expect(link).To(Equal(fakeLink))
			Expect(hwAddr).To(Equal(local.Hardware))

			Expect(fakeLinkOperations.DisableIPv6CallCount()).To(Equal(1))
			Expect(fakeLinkOperations.DisableIPv6ArgsForCall(0)).To(Equal("myDeviceName"))

			Expect(fakeLinkOperations.StaticNeighborNoARPCallCount()).To(Equal(1))
			link, peerIP, peerHardwareAddr := fakeLinkOperations.StaticNeighborNoARPArgsForCall(0)
			Expect(link).To(Equal(fakeLink))
			Expect(peerIP).To(Equal(peer.IP))
			Expect(peerHardwareAddr).To(Equal(peer.Hardware))

			Expect(fakeLinkOperations.SetPointToPointAddressCallCount()).To(Equal(1))
			link, localIP, peerIP := fakeLinkOperations.SetPointToPointAddressArgsForCall(0)
			Expect(link).To(Equal(fakeLink))
			Expect(localIP).To(Equal(local.IP))
			Expect(peerIP).To(Equal(peer.IP))

			Expect(fakeLinkOperations.EnableReversePathFilteringCallCount()).To(Equal(1))
			Expect(fakeLinkOperations.EnableReversePathFilteringArgsForCall(0)).To(Equal("myDeviceName"))

			Expect(fakeNetlinkAdapter.LinkSetUpCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkSetUpArgsForCall(0)).To(Equal(fakeLink))
		})

		Context("when the link cannot be found", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkByNameReturns(nil, errors.New("strawberry"))
			})
			It("wraps and returns the error", func() {
				err := common.BasicSetup(deviceName, local, peer)
				Expect(err).To(Equal(errors.New("failed to find link \"myDeviceName\": strawberry")))

			})
		})

		Context("when setting the hardware address fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkSetHardwareAddrReturns(errors.New("apple"))
			})
			It("wraps and returns the error", func() {
				err := common.BasicSetup(deviceName, local, peer)
				Expect(err).To(Equal(errors.New("setting hardware address: apple")))
			})
		})

		Context("when disabling IPv6 fails", func() {
			BeforeEach(func() {
				fakeLinkOperations.DisableIPv6Returns(errors.New("kiwi"))
			})
			It("wraps and returns the error", func() {
				err := common.BasicSetup(deviceName, local, peer)
				Expect(err).To(Equal(errors.New("disable IPv6: kiwi")))
			})
		})

		Context("when replacing ARP with permanent neighbor rule fails", func() {
			BeforeEach(func() {
				fakeLinkOperations.StaticNeighborNoARPReturns(errors.New("raspberry"))
			})
			It("wraps and returns the error", func() {
				err := common.BasicSetup(deviceName, local, peer)
				Expect(err).To(Equal(errors.New("replace ARP with permanent neighbor rule: raspberry")))
			})
		})

		Context("when setting the point to point address fails", func() {
			BeforeEach(func() {
				fakeLinkOperations.SetPointToPointAddressReturns(errors.New("dragonfruit"))
			})
			It("wraps and returns the error", func() {
				err := common.BasicSetup(deviceName, local, peer)
				Expect(err).To(Equal(errors.New("setting point to point address: dragonfruit")))
			})
		})

		Context("when enabling reverse path filtering fails", func() {
			BeforeEach(func() {
				fakeLinkOperations.EnableReversePathFilteringReturns(errors.New("pomegranate"))
			})
			It("wraps and returns the error", func() {
				err := common.BasicSetup(deviceName, local, peer)
				Expect(err).To(Equal(errors.New("enable reverse path filtering: pomegranate")))
			})
		})

		Context("when setting link up fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkSetUpReturns(errors.New("cantaloupe"))
			})
			It("wraps and returns the error", func() {
				err := common.BasicSetup(deviceName, local, peer)
				Expect(err).To(Equal(errors.New("setting link myDeviceName up: cantaloupe")))
			})
		})

	})
})
