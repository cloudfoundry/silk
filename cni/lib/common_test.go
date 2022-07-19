package lib_test

import (
	"errors"
	"net"

	"code.cloudfoundry.org/silk/cni/config"
	"code.cloudfoundry.org/silk/cni/lib"
	"code.cloudfoundry.org/silk/cni/lib/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
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

			fakeNetlinkAdapter = &fakes.NetlinkAdapter{}
			fakeLinkOperations = &fakes.LinkOperations{}
			fakeLink = &netlink.Bridge{
				LinkAttrs: netlink.LinkAttrs{
					Name:         "my-fake-bridge",
					HardwareAddr: local.Hardware,
				},
			}
			fakeNetlinkAdapter.LinkByNameReturns(fakeLink, nil)

			common = &lib.Common{
				NetlinkAdapter: fakeNetlinkAdapter,
				LinkOperations: fakeLinkOperations,
			}
			deviceName = "myDeviceName"
		})

		It("sets up a veth device", func() {
			err := common.BasicSetup(deviceName, local, peer)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeNetlinkAdapter.LinkByNameCallCount()).To(Equal(2))
			Expect(fakeNetlinkAdapter.LinkByNameArgsForCall(0)).To(Equal("myDeviceName"))

			Expect(fakeNetlinkAdapter.LinkSetHardwareAddrCallCount()).To(Equal(0))

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

		Context("when the hardware address is set to the wrong thing", func() {
			var fakeLinkWithBadHardwareAddr *netlink.Bridge
			var fakeLinkWithCorrectLocalHardwareAddr *netlink.Bridge
			var randomHardwareAddr net.HardwareAddr

			BeforeEach(func() {

				var err error
				randomHardwareAddr, err = net.ParseMAC("ff:ff:ff:ff:ff:ff")
				Expect(err).NotTo(HaveOccurred())

				fakeLinkWithBadHardwareAddr = &netlink.Bridge{
					LinkAttrs: netlink.LinkAttrs{
						Name:         "my-fake-bridge",
						HardwareAddr: randomHardwareAddr,
					},
				}

				fakeLinkWithCorrectLocalHardwareAddr = &netlink.Bridge{
					LinkAttrs: netlink.LinkAttrs{
						Name:         "my-fake-bridge",
						HardwareAddr: local.Hardware,
					},
				}
				fakeNetlinkAdapter.LinkByNameReturns(fakeLinkWithBadHardwareAddr, nil)
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

			Context("but eventually is set to the right thing", func() {
				BeforeEach(func() {
					// The 0th time this function is for getting the link so we
					// can check the hardware addr is accurate, and if not, change the hardware addr.
					// The 1st time is for checking to make sure the hardware addr is set correctly after we retried
					// The 2nd time checks again, validating that the 1st sethardwareaddr call worked
					// The 3rd time is ensuring our loop re-tries until a success is found.
					fakeNetlinkAdapter.LinkByNameReturnsOnCall(3, fakeLinkWithCorrectLocalHardwareAddr, nil)
				})

				It("retries and eventually sets the right address", func() {
					err := common.BasicSetup(deviceName, local, peer)
					Expect(err).NotTo(HaveOccurred())
					Expect(fakeNetlinkAdapter.LinkSetHardwareAddrCallCount()).To(Equal(2))
					link, hwAddr := fakeNetlinkAdapter.LinkSetHardwareAddrArgsForCall(1)
					Expect(link).To(Equal(fakeLinkWithBadHardwareAddr)) // ensure we passed in the link we want to update
					Expect(hwAddr).To(Equal(local.Hardware))
				})
			})

			Context("and it is never set to the right thing", func() {
				BeforeEach(func() {
					fakeNetlinkAdapter.LinkByNameReturns(fakeLinkWithBadHardwareAddr, nil)
				})

				It("runs out of retries and wraps and returns an error", func() {
					err := common.BasicSetup(deviceName, local, peer)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to set hardware addr"))
				})
			})
		})

		Context("when disabling IPv6 fails", func() {
			BeforeEach(func() {
				fakeLinkOperations.DisableIPv6Returns(errors.New("kiwi"))
			})
			It("ignores the error", func() {
				err := common.BasicSetup(deviceName, local, peer)
				Expect(err).NotTo(HaveOccurred())
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
