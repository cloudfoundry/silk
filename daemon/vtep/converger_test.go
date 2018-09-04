package vtep_test

import (
	"errors"
	"net"
	"syscall"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/daemon/vtep"
	"code.cloudfoundry.org/silk/daemon/vtep/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

var _ = Describe("Converger", func() {
	var (
		fakeNetlink *fakes.NetlinkAdapter
		converger   *vtep.Converger
		leases      []controller.Lease
		overlayNet  *net.IPNet
		logger      *lagertest.TestLogger
		localMac    net.HardwareAddr
		remoteMac   net.HardwareAddr
	)

	Describe("Converge", func() {
		BeforeEach(func() {
			fakeNetlink = &fakes.NetlinkAdapter{}
			_, localSubnet, _ := net.ParseCIDR("10.255.32.0/24")
			_, overlayNet, _ = net.ParseCIDR("10.255.0.0/16")
			logger = lagertest.NewTestLogger("test")
			localVTEP := net.Interface{
				Index: 42,
				Name:  "silk-vtep",
			}
			converger = &vtep.Converger{
				OverlayNetwork:    overlayNet,
				LocalSubnet:       localSubnet,
				LocalVTEP:         localVTEP,
				NetlinkAdapter:    fakeNetlink,
				Logger:            logger,
				UnderlayAddresses: make(map[string]net.IP),
			}
			localMac, _ = net.ParseMAC("ee:ee:aa:bb:cc:dd")
			remoteMac, _ = net.ParseMAC("ee:ee:aa:aa:aa:ff")
			leases = []controller.Lease{
				controller.Lease{
					UnderlayIP:          "10.10.0.4",
					OverlaySubnet:       "10.255.32.0/24",
					OverlayHardwareAddr: localMac.String(),
				},
				controller.Lease{
					UnderlayIP:          "10.10.0.5",
					OverlaySubnet:       "10.255.19.0/24",
					OverlayHardwareAddr: remoteMac.String(),
				},
			}
		})

		It("adds routing rule for each remote lease", func() {
			err := converger.Converge(leases)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeNetlink.RouteReplaceCallCount()).To(Equal(1))
			addedRoute := fakeNetlink.RouteReplaceArgsForCall(0)
			destGW, destNet, _ := net.ParseCIDR("10.255.19.0/24")
			Expect(addedRoute).To(Equal(&netlink.Route{
				LinkIndex: 42,
				Scope:     netlink.SCOPE_UNIVERSE,
				Dst:       destNet,
				Gw:        destGW,
				Src:       net.ParseIP("10.255.32.0").To4(),
			}))
		})

		It("adds an ARP and FDB rule for each remote lease", func() {
			err := converger.Converge(leases)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeNetlink.NeighSetCallCount()).To(Equal(2))
			neighs := []*netlink.Neigh{
				fakeNetlink.NeighSetArgsForCall(0),
				fakeNetlink.NeighSetArgsForCall(1),
			}
			Expect(neighs).To(ConsistOf(
				&netlink.Neigh{
					LinkIndex:    42,
					State:        netlink.NUD_PERMANENT,
					Type:         syscall.RTN_UNICAST,
					IP:           net.ParseIP("10.255.19.0"),
					HardwareAddr: remoteMac,
				},
				&netlink.Neigh{
					LinkIndex:    42,
					State:        netlink.NUD_PERMANENT,
					Family:       syscall.AF_BRIDGE,
					Flags:        netlink.NTF_SELF,
					IP:           net.ParseIP("10.10.0.5"),
					HardwareAddr: remoteMac,
				},
			))
		})

		It("does not log anything about non-routable leases", func() {
			err := converger.Converge(leases)
			Expect(err).NotTo(HaveOccurred())

			/*
				Commenting out because of bug with netlink library and xenial stemcell
				Bug tracker number: #160007813
				This results in log line: "neighsForDeletion count: 0"
				When the bug is fixed, revert this commit
			*/

			// Expect(logger.Logs()).To(HaveLen(0))

			Expect(logger.Logs()).To(HaveLen(1))
			Expect(logger.Logs()[0].LogLevel).To(Equal(lager.DEBUG))
			Expect(logger.Logs()[0].ToJSON()).To(MatchRegexp(`"neighsForDeletion count":0`))

		})

		Context("when the a remote lease is removed", func() {
			var (
				deletedNeighs []netlink.Neigh
				oldDestMac    net.HardwareAddr
			)
			BeforeEach(func() {
				destGW, destNet, _ := net.ParseCIDR("10.255.19.0/24")

				oldDestMac, _ = net.ParseMAC("ee:ee:0a:ff:14:00")
				oldDestGW, oldDestNet, _ := net.ParseCIDR("10.255.20.0/24")

				fakeNetlink.FDBListReturns([]netlink.Neigh{
					netlink.Neigh{
						LinkIndex:    42,
						State:        netlink.NUD_PERMANENT,
						Family:       syscall.AF_BRIDGE,
						Flags:        netlink.NTF_SELF,
						IP:           net.ParseIP("10.10.0.5"),
						HardwareAddr: remoteMac,
					},
					netlink.Neigh{
						LinkIndex:    42,
						State:        netlink.NUD_PERMANENT,
						Family:       syscall.AF_BRIDGE,
						Flags:        netlink.NTF_SELF,
						IP:           net.ParseIP("10.10.0.6"),
						HardwareAddr: oldDestMac,
					},
				}, nil)

				fakeNetlink.ARPListReturns([]netlink.Neigh{
					netlink.Neigh{
						LinkIndex:    42,
						State:        netlink.NUD_PERMANENT,
						Type:         syscall.RTN_UNICAST,
						IP:           net.ParseIP("10.255.19.0"),
						HardwareAddr: remoteMac,
					},
					netlink.Neigh{
						LinkIndex:    42,
						State:        netlink.NUD_PERMANENT,
						Type:         syscall.RTN_UNICAST,
						IP:           net.ParseIP("10.255.20.0"),
						HardwareAddr: oldDestMac,
					},
				}, nil)

				fakeNetlink.RouteListReturns([]netlink.Route{
					netlink.Route{
						LinkIndex: 42,
						Scope:     netlink.SCOPE_UNIVERSE,
						Dst:       destNet,
						Gw:        destGW,
						Src:       net.ParseIP("10.255.32.0").To4(),
					},
					netlink.Route{
						LinkIndex: 42,
						Scope:     netlink.SCOPE_UNIVERSE,
						Dst:       overlayNet,
						Gw:        nil,
						Src:       net.ParseIP("10.255.32.0").To4(),
					},
					netlink.Route{
						LinkIndex: 42,
						Scope:     netlink.SCOPE_UNIVERSE,
						Dst:       oldDestNet,
						Gw:        oldDestGW,
						Src:       net.ParseIP("10.255.48.0").To4(),
					},
				}, nil)

				deletedNeighs = []netlink.Neigh{}
				fakeNetlink.NeighDelStub = func(neigh *netlink.Neigh) error {
					deletedNeighs = append(deletedNeighs, *neigh)
					return nil
				}
			})

			It("deletes the routing, ARP, and FDB rules related to the removed lease", func() {
				err := converger.Converge(leases)
				Expect(err).NotTo(HaveOccurred())

				By("checking that the route is deleted")
				Expect(fakeNetlink.RouteDelCallCount()).To(Equal(1))
				deletedRoute := fakeNetlink.RouteDelArgsForCall(0)
				destGW, destNet, _ := net.ParseCIDR("10.255.20.0/24")
				Expect(deletedRoute).To(Equal(&netlink.Route{
					LinkIndex: 42,
					Scope:     netlink.SCOPE_UNIVERSE,
					Dst:       destNet,
					Gw:        destGW,
					Src:       net.ParseIP("10.255.48.0").To4(),
				}))

				By("checking that the ARP and FDB rules is deleted")
				Expect(fakeNetlink.NeighDelCallCount()).To(Equal(2))

				Expect(deletedNeighs).To(ConsistOf(
					// ARP
					netlink.Neigh{
						LinkIndex:    42,
						State:        netlink.NUD_PERMANENT,
						Type:         syscall.RTN_UNICAST,
						IP:           net.ParseIP("10.255.20.0"),
						HardwareAddr: oldDestMac,
					},
					// FDB
					netlink.Neigh{
						LinkIndex:    42,
						State:        netlink.NUD_PERMANENT,
						Family:       syscall.AF_BRIDGE,
						Flags:        netlink.NTF_SELF,
						IP:           net.ParseIP("10.10.0.6"),
						HardwareAddr: oldDestMac,
					},
				))
			})

		})

		Context("when there are other routing rules", func() {
			BeforeEach(func() {
				fakeNetlink.RouteListReturns([]netlink.Route{
					netlink.Route{
						LinkIndex: 42,
						Scope:     netlink.SCOPE_UNIVERSE,
						Dst:       overlayNet,
						Gw:        nil,
						Src:       net.ParseIP("10.255.32.0").To4(),
					},
				}, nil)
			})

			It("does not delete rules it did not create", func() {
				err := converger.Converge(leases)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeNetlink.RouteDelCallCount()).To(Equal(0))
			})
		})

		Context("when the link cannot be found", func() {
			BeforeEach(func() {
				fakeNetlink.LinkByIndexReturns(nil, errors.New("passionfruit"))
			})

			It("breaks early and returns a meaningful error", func() {
				err := converger.Converge(leases)
				Expect(err).To(MatchError("link by index: passionfruit"))
			})
		})

		Context("when previous routes cannot be found", func() {
			BeforeEach(func() {
				fakeNetlink.RouteListReturns(nil, errors.New("peach"))
			})

			It("breaks early and returns a meaningful error", func() {
				err := converger.Converge(leases)
				Expect(err).To(MatchError("list routes: peach"))
			})
		})

		Context("when previous fdb entries cannot be found", func() {
			BeforeEach(func() {
				fakeNetlink.FDBListReturns(nil, errors.New("kiwi"))
			})

			It("breaks early and returns a meaningful error", func() {
				err := converger.Converge(leases)
				Expect(err).To(MatchError("list fdb: kiwi"))
			})
		})

		Context("when previous arp entries cannot be found", func() {
			BeforeEach(func() {
				fakeNetlink.ARPListReturns(nil, errors.New("lychee"))
			})

			It("breaks early and returns a meaningful error", func() {
				err := converger.Converge(leases)
				Expect(err).To(MatchError("list arp: lychee"))
			})
		})

		Context("when the lease subnet is malformed", func() {
			BeforeEach(func() {
				leases[1].OverlaySubnet = "banana"
			})
			It("breaks early and returns a meaningful error", func() {
				err := converger.Converge(leases)
				Expect(err).To(MatchError("parse lease: invalid CIDR address: banana"))
			})
		})

		Context("when the underlay IP is malformed", func() {
			BeforeEach(func() {
				leases[1].UnderlayIP = "kumquat"
			})
			It("breaks early and returns a meaningful error", func() {
				err := converger.Converge(leases)
				Expect(err).To(MatchError("invalid underlay ip: kumquat"))
			})
		})

		Context("when adding the route fails", func() {
			BeforeEach(func() {
				fakeNetlink.RouteReplaceReturns(errors.New("apricot"))
			})
			It("returns a meaningful error", func() {
				err := converger.Converge(leases)
				Expect(err).To(MatchError("add route: apricot"))
			})
		})

		Context("when adding a neigh fails", func() {
			BeforeEach(func() {
				fakeNetlink.NeighSetReturns(errors.New("pear"))
			})
			It("returns a meaningful error", func() {
				err := converger.Converge(leases)
				Expect(err).To(MatchError("set neigh: pear"))
			})
		})

		Context("when deleting the route fails", func() {
			BeforeEach(func() {
				fakeNetlink.RouteDelReturns(errors.New("durian"))

				destGW, destNet, _ := net.ParseCIDR("10.255.19.0/24")
				fakeNetlink.RouteListReturns([]netlink.Route{
					netlink.Route{
						LinkIndex: 42,
						Scope:     netlink.SCOPE_UNIVERSE,
						Dst:       destNet,
						Gw:        destGW,
						Src:       net.ParseIP("10.255.32.0").To4(),
					},
				}, nil)
			})
			It("returns a meaningful error", func() {
				err := converger.Converge([]controller.Lease{})
				Expect(err).To(MatchError("del route: durian"))
			})
		})

		Context("when deleting a neigh fails", func() {
			BeforeEach(func() {
				fakeNetlink.NeighDelReturns(errors.New("mango"))

				fakeNetlink.ARPListReturns([]netlink.Neigh{
					netlink.Neigh{
						LinkIndex:    42,
						State:        netlink.NUD_PERMANENT,
						Type:         syscall.RTN_UNICAST,
						IP:           net.ParseIP("10.255.19.0"),
						HardwareAddr: remoteMac,
					},
				}, nil)
			})
			It("returns a meaningful error", func() {
				err := converger.Converge([]controller.Lease{})
				Expect(err).To(MatchError("del neigh with ip/hwaddr 10.255.19.0 ee:ee:aa:aa:aa:ff: mango"))
			})
		})

		Context("when there are remote leases that are not in the overlay network", func() {
			BeforeEach(func() {
				leases = []controller.Lease{
					{ // local, skipped
						UnderlayIP:          "10.10.0.2",
						OverlaySubnet:       "10.255.32.0/24",
						OverlayHardwareAddr: "aa:aa:00:00:00:00",
					},
					{ // not in overlay, skipped
						UnderlayIP:          "10.10.0.3",
						OverlaySubnet:       "10.254.11.0/24",
						OverlayHardwareAddr: "aa:aa:00:00:00:01",
					},
					{ // not in overlay, skipped
						UnderlayIP:          "10.10.0.4",
						OverlaySubnet:       "10.254.12.0/24",
						OverlayHardwareAddr: "aa:aa:00:00:00:02",
					},
					{ // in overlay
						UnderlayIP:          "10.10.0.5",
						OverlaySubnet:       "10.255.19.0/24",
						OverlayHardwareAddr: "aa:aa:00:00:00:03",
					},
				}
			})

			It("does not touch them and adds only the leases in the overlay network", func() {
				err := converger.Converge(leases)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeNetlink.RouteReplaceCallCount()).To(Equal(1))
				addedRoute := fakeNetlink.RouteReplaceArgsForCall(0)
				Expect(addedRoute.Dst.IP).To(Equal(net.ParseIP("10.255.19.0").To4()))
				Expect(fakeNetlink.NeighSetCallCount()).To(Equal(2))
				addedARP := fakeNetlink.NeighSetArgsForCall(0)
				Expect(addedARP.IP).To(Equal(net.ParseIP("10.255.19.0")))
				addedFDB := fakeNetlink.NeighSetArgsForCall(1)
				Expect(addedFDB.IP).To(Equal(net.ParseIP("10.10.0.5")))

				/*
					Commenting out because of bug with netlink library and xenial stemcell
					Bug tracker number: #160007813
					This results in log line: "neighsForDeletion count: 0"
					When the bug is fixed, revert this commit
				*/

				//Expect(logger.Logs()).To(HaveLen(1))
				//Expect(logger.Logs()[0].LogLevel).To(Equal(lager.INFO))
				//Expect(logger.Logs()[0].ToJSON()).To(MatchRegexp("converger.*non-routable-lease-count.*2"))

				Expect(logger.Logs()).To(HaveLen(2))
				Expect(logger.Logs()[1].LogLevel).To(Equal(lager.INFO))
				Expect(logger.Logs()[1].ToJSON()).To(MatchRegexp("converger.*non-routable-lease-count.*2"))
			})
		})

		Context("when a lease has an invalid MAC", func() {
			BeforeEach(func() {
				leases = []controller.Lease{
					{
						UnderlayIP:          "10.10.0.5",
						OverlaySubnet:       "10.255.19.0/24",
						OverlayHardwareAddr: "banana",
					},
				}
			})
			It("returns an error", func() {
				err := converger.Converge(leases)
				Expect(err).To(MatchError("invalid hardware addr: banana"))
			})
		})
	})
})
