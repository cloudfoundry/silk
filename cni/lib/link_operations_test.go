package lib_test

import (
	"errors"
	"net"

	"code.cloudfoundry.org/lager/v3/lagertest"

	"code.cloudfoundry.org/silk/cni/lib"
	"code.cloudfoundry.org/silk/cni/lib/fakes"
	"github.com/containernetworking/cni/pkg/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

var _ = Describe("Link Operations", func() {

	var (
		fakeSysctlAdapter  *fakes.SysctlAdapter
		fakeNetlinkAdapter *fakes.NetlinkAdapter
		linkOperations     *lib.LinkOperations
		fakeLink           netlink.Link
		ipAddr             net.IP
		peerIP             net.IP
		hwAddr             net.HardwareAddr
		routes             []*types.Route
		logger             *lagertest.TestLogger
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		fakeSysctlAdapter = &fakes.SysctlAdapter{}
		fakeNetlinkAdapter = &fakes.NetlinkAdapter{}
		linkOperations = &lib.LinkOperations{
			SysctlAdapter:  fakeSysctlAdapter,
			NetlinkAdapter: fakeNetlinkAdapter,
			Logger:         logger,
		}
		fakeLink = &netlink.Bridge{
			LinkAttrs: netlink.LinkAttrs{
				Name:  "my-fake-bridge",
				Index: 42,
			},
		}

		var err error
		ipAddr = net.IP{10, 255, 30, 4}
		peerIP = net.IP{169, 254, 0, 1}
		hwAddr, err = net.ParseMAC("aa:aa:12:34:56:78")
		Expect(err).NotTo(HaveOccurred())

		routes = []*types.Route{
			&types.Route{
				Dst: net.IPNet{
					IP:   []byte{200, 201, 202, 203},
					Mask: []byte{255, 255, 255, 255},
				},
				GW: net.IP{10, 255, 30, 2},
			},
			&types.Route{
				Dst: net.IPNet{
					IP:   []byte{100, 101, 102, 103},
					Mask: []byte{255, 255, 255, 255},
				},
				GW: net.IP{10, 255, 30, 1},
			},
			&types.Route{
				Dst: net.IPNet{
					IP:   []byte{0, 1, 2, 3},
					Mask: []byte{255, 255, 255, 255},
				},
				GW: net.IP{10, 255, 30, 0},
			},
		}
	})

	Describe("Disable IPv6", func() {
		It("calls the sysctl adapter to disable IPv6", func() {
			err := linkOperations.DisableIPv6("someDevice")
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeSysctlAdapter.SysctlCallCount()).To(Equal(1))
			name, params := fakeSysctlAdapter.SysctlArgsForCall(0)
			Expect(name).To(Equal("net.ipv6.conf.someDevice.disable_ipv6"))
			Expect(len(params)).To(Equal(1))
			Expect(params[0]).To(Equal("1"))
		})

		Context("when the sysctl command fails", func() {
			BeforeEach(func() {
				fakeSysctlAdapter.SysctlReturns("", errors.New("cuttlefish"))
			})
			It("returns a meaningful error", func() {
				err := linkOperations.DisableIPv6("someDevice")
				Expect(err).To(MatchError("sysctl for someDevice: cuttlefish"))
			})
		})
	})

	Describe("EnableReversePathFiltering", func() {
		It("calls the sysctl adapter to set rp_filtering to strict mode", func() {
			err := linkOperations.EnableReversePathFiltering("someDevice")
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeSysctlAdapter.SysctlCallCount()).To(Equal(1))
			name, params := fakeSysctlAdapter.SysctlArgsForCall(0)
			Expect(name).To(Equal("net.ipv4.conf.someDevice.rp_filter"))
			Expect(len(params)).To(Equal(1))
			Expect(params[0]).To(Equal("1"))
		})

		Context("when the sysctl command fails", func() {
			BeforeEach(func() {
				fakeSysctlAdapter.SysctlReturns("", errors.New("cuttlefish"))
			})
			It("returns a meaningful error", func() {
				err := linkOperations.EnableReversePathFiltering("someDevice")
				Expect(err).To(MatchError("sysctl for someDevice: cuttlefish"))
			})
		})
	})

	Describe("EnableIPv4Forwarding", func() {
		It("calls the sysctl adapter to enable IPv4 forwarding", func() {
			err := linkOperations.EnableIPv4Forwarding()
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeSysctlAdapter.SysctlCallCount()).To(Equal(1))
			name, params := fakeSysctlAdapter.SysctlArgsForCall(0)
			Expect(name).To(Equal("net.ipv4.ip_forward"))
			Expect(len(params)).To(Equal(1))
			Expect(params[0]).To(Equal("1"))
		})

		Context("when the sysctl command fails", func() {
			BeforeEach(func() {
				fakeSysctlAdapter.SysctlReturns("", errors.New("cuttlefish"))
			})
			It("returns a meaningful error", func() {
				err := linkOperations.EnableIPv4Forwarding()
				Expect(err).To(MatchError("enabling IPv4 forwarding: cuttlefish"))
			})
		})
	})

	Describe("StaticNeighborNoARP", func() {
		It("calls the netlink adapter to disable ARP", func() {
			err := linkOperations.StaticNeighborNoARP(fakeLink, ipAddr, hwAddr)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeNetlinkAdapter.LinkSetARPOffCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkSetARPOffArgsForCall(0)).To(Equal(fakeLink))
		})

		It("calls the netlink adapter to install a permanent neighbor rule", func() {
			err := linkOperations.StaticNeighborNoARP(fakeLink, ipAddr, hwAddr)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeNetlinkAdapter.NeighAddPermanentIPv4CallCount()).To(Equal(1))
			index, destIP, destHardwareAddr := fakeNetlinkAdapter.NeighAddPermanentIPv4ArgsForCall(0)
			Expect(index).To(Equal(42))
			Expect(destIP).To(Equal(ipAddr))
			Expect(destHardwareAddr).To(Equal(hwAddr))
		})

		Context("when disabling ARP fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkSetARPOffReturns(errors.New("shrimp"))
			})
			It("returns a meaningul error", func() {
				err := linkOperations.StaticNeighborNoARP(fakeLink, ipAddr, hwAddr)
				Expect(err).To(MatchError("set ARP off: shrimp"))
			})
		})

		Context("when installing the neighbor rule fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.NeighAddPermanentIPv4Returns(errors.New("crab"))
			})
			It("returns a meaningul error", func() {
				err := linkOperations.StaticNeighborNoARP(fakeLink, ipAddr, hwAddr)
				Expect(err).To(MatchError("neigh add: crab"))
			})
		})
	})

	Describe("SetPointToPointAddress", func() {
		var (
			parsedAddr *netlink.Addr
			ptpAddr    *netlink.Addr
		)
		BeforeEach(func() {
			parsedAddr = &netlink.Addr{}
			ptpAddr = &netlink.Addr{Peer: &net.IPNet{
				IP:   peerIP,
				Mask: []byte{255, 255, 255, 255},
			}}
			fakeNetlinkAdapter.ParseAddrReturns(parsedAddr, nil)
		})
		It("sets the peer IP address on the link", func() {
			err := linkOperations.SetPointToPointAddress(fakeLink, ipAddr, peerIP)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeNetlinkAdapter.ParseAddrCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.ParseAddrArgsForCall(0)).To(Equal("10.255.30.4/32"))

			Expect(fakeNetlinkAdapter.AddrAddScopeLinkCallCount()).To(Equal(1))
			link, addr := fakeNetlinkAdapter.AddrAddScopeLinkArgsForCall(0)
			Expect(link).To(Equal(fakeLink))
			Expect(addr).To(Equal(ptpAddr))
		})

		Context("when parsing the IP address fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.ParseAddrReturns(nil, errors.New("lobster"))
			})
			It("returns a meaningul error", func() {
				err := linkOperations.SetPointToPointAddress(fakeLink, ipAddr, peerIP)
				Expect(err).To(MatchError("parsing address 10.255.30.4/32: lobster"))
			})
		})

		Context("when setting the point to point address fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.AddrAddScopeLinkReturns(errors.New("oyster"))
			})
			It("returns a meaningul error", func() {
				err := linkOperations.SetPointToPointAddress(fakeLink, ipAddr, peerIP)
				Expect(err).To(MatchError("adding IP address 10.255.30.4/32: oyster"))
			})
		})
	})

	Describe("RenameLink", func() {
		BeforeEach(func() {
			fakeNetlinkAdapter.LinkByNameReturns(fakeLink, nil)
		})
		It("finds the link with the old name and renames it to the new name", func() {
			err := linkOperations.RenameLink("old", "new")
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeNetlinkAdapter.LinkByNameCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkByNameArgsForCall(0)).To(Equal("old"))

			Expect(fakeNetlinkAdapter.LinkSetNameCallCount()).To(Equal(1))
			link, new := fakeNetlinkAdapter.LinkSetNameArgsForCall(0)
			Expect(link).To(Equal(fakeLink))
			Expect(new).To(Equal("new"))
		})

		Context("when finding the link fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkByNameReturns(nil, errors.New("uni"))
			})
			It("returns a meaningful error", func() {
				err := linkOperations.RenameLink("old", "new")
				Expect(err).To(MatchError("failed to find link \"old\": uni"))
			})
		})

		Context("when setting the link name fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkSetNameReturns(errors.New("starfish"))
			})
			It("returns a meaningful error", func() {
				err := linkOperations.RenameLink("old", "new")
				Expect(err).To(MatchError("set link name: starfish"))
			})
		})
	})

	Describe("DeleteLinkByName", func() {
		BeforeEach(func() {
			fakeNetlinkAdapter.LinkByNameReturns(fakeLink, nil)
		})
		It("finds the link by name and deletes it", func() {
			err := linkOperations.DeleteLinkByName("someName")
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeNetlinkAdapter.LinkByNameCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkByNameArgsForCall(0)).To(Equal("someName"))

			Expect(fakeNetlinkAdapter.LinkDelCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkDelArgsForCall(0)).To(Equal(fakeLink))
		})

		Context("when finding the link fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkByNameReturns(nil, errors.New("some error returned by LinkByName"))
			})
			It("swallows the error and return success", func() {
				err := linkOperations.DeleteLinkByName("someName")
				Expect(err).NotTo(HaveOccurred())
			})

			It("logs an informational message with the failure", func() {
				linkOperations.DeleteLinkByName("someName")

				Expect(logger.Logs()).To(HaveLen(1))
				Expect(logger.Logs()[0].Data).To(HaveKeyWithValue("deviceName", "someName"))
				Expect(logger.Logs()[0].Data).To(HaveKeyWithValue("message", "some error returned by LinkByName"))
			})
		})

		Context("when deleting the link fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkDelReturns(errors.New("starfish"))
			})
			It("returns the error", func() {
				err := linkOperations.DeleteLinkByName("someName")
				Expect(err).To(MatchError("starfish"))
			})
		})
	})

	Describe("RouteAddAll", func() {
		BeforeEach(func() {
			fakeNetlinkAdapter.RouteAddReturns(nil)
		})
		It("adds all routes", func() {
			err := linkOperations.RouteAddAll(routes, ipAddr)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeNetlinkAdapter.RouteAddCallCount()).To(Equal(3))
			Expect(fakeNetlinkAdapter.RouteAddArgsForCall(0)).To(Equal(&netlink.Route{
				Src: ipAddr,
				Dst: &net.IPNet{
					IP:   []byte{200, 201, 202, 203},
					Mask: []byte{255, 255, 255, 255},
				},
				Gw: net.IP{10, 255, 30, 2},
			}))
			Expect(fakeNetlinkAdapter.RouteAddArgsForCall(1)).To(Equal(&netlink.Route{
				Src: ipAddr,
				Dst: &net.IPNet{
					IP:   []byte{100, 101, 102, 103},
					Mask: []byte{255, 255, 255, 255},
				},
				Gw: net.IP{10, 255, 30, 1},
			}))
			Expect(fakeNetlinkAdapter.RouteAddArgsForCall(2)).To(Equal(&netlink.Route{
				Src: ipAddr,
				Dst: &net.IPNet{
					IP:   []byte{0, 1, 2, 3},
					Mask: []byte{255, 255, 255, 255},
				},
				Gw: net.IP{10, 255, 30, 0},
			}))
		})

		Context("when adding one of the routes fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.RouteAddStub = func(route *netlink.Route) error {
					if route.Gw.String() == "10.255.30.1" {
						return errors.New("pickle")
					}
					return nil
				}
			})
			It("returns a meaningful error", func() {
				err := linkOperations.RouteAddAll(routes, ipAddr)
				Expect(err).To(MatchError("adding route: pickle"))

				Expect(fakeNetlinkAdapter.RouteAddCallCount()).To(Equal(2))
			})
		})
	})
})
