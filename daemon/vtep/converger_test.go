package vtep_test

import (
	"errors"
	"net"

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
	)
	Describe("Converge", func() {
		BeforeEach(func() {
			fakeNetlink = &fakes.NetlinkAdapter{}
			_, localSubnet, _ := net.ParseCIDR("10.255.32.0/24")
			localVTEP := net.Interface{
				Index: 42,
				Name:  "silk-vtep",
			}
			converger = &vtep.Converger{
				LocalSubnet:    localSubnet,
				LocalVTEP:      localVTEP,
				NetlinkAdapter: fakeNetlink,
			}
			leases = []controller.Lease{
				controller.Lease{
					OverlaySubnet: "10.255.32.0/24",
				},
				controller.Lease{
					OverlaySubnet: "10.255.19.0/24",
				},
			}
		})

		It("adds routing rule for each remote lease", func() {
			err := converger.Converge(leases)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeNetlink.RouteAddCallCount()).To(Equal(1))
			addedRoute := fakeNetlink.RouteAddArgsForCall(0)
			destGW, destNet, _ := net.ParseCIDR("10.255.19.0/24")
			Expect(addedRoute).To(Equal(&netlink.Route{
				LinkIndex: 42,
				Scope:     netlink.SCOPE_UNIVERSE,
				Dst:       destNet,
				Gw:        destGW,
				Src:       net.ParseIP("10.255.32.0").To4(),
			}))
		})

		Context("when the lease subnet is malformed", func() {
			BeforeEach(func() {
				leases[1].OverlaySubnet = "banana"
			})
			It("breaks early and returns a meaningful error", func() {
				err := converger.Converge(leases)
				Expect(err).To(MatchError("parsing lease: invalid CIDR address: banana"))
			})
		})

		Context("when adding the route fails", func() {
			BeforeEach(func() {
				fakeNetlink.RouteAddReturns(errors.New("apricot"))
			})
			It("returns a meaningful error", func() {
				err := converger.Converge(leases)
				Expect(err).To(MatchError("adding route: apricot"))
			})
		})
	})
})
