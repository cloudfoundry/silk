package lib_test

import (
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/silk/config"
	"github.com/cloudfoundry-incubator/silk/lib"
	"github.com/cloudfoundry-incubator/silk/lib/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

var _ = Describe("VethPairCreator", func() {
	Describe("Create", func() {

		var (
			containerNS        *fakes.NetNS
			hostNS             *fakes.NetNS
			cfg                *config.Config
			creator            *lib.VethPairCreator
			fakeNetlinkAdapter *fakes.NetlinkAdapter
		)

		BeforeEach(func() {
			containerNS = &fakes.NetNS{}
			containerNS.FdReturns(42)
			hostNS = &fakes.NetNS{}
			hostNS.DoStub = lib.NetNsDoStub

			cfg = &config.Config{}
			cfg.Container.TemporaryDeviceName = "myTemporaryDeviceName"
			cfg.Container.Namespace = containerNS
			cfg.Container.MTU = 1234
			cfg.Host.DeviceName = "myDeviceName"
			cfg.Host.Namespace = hostNS

			fakeNetlinkAdapter = &fakes.NetlinkAdapter{}
			fakeNetlinkAdapter.LinkByNameReturns(&netlink.Bridge{
				LinkAttrs: netlink.LinkAttrs{
					Name: "my-fake-bridge",
				},
			}, nil)
			creator = &lib.VethPairCreator{
				NetlinkAdapter: fakeNetlinkAdapter,
			}
		})

		AfterEach(func() {
			containerNS.Close() // don't bother checking errors here
			hostNS.Close()
		})

		It("creates a correctly-named veth device in the host namespace with the correct MTU", func() {
			Expect(creator.Create(cfg)).To(Succeed())

			By("requesting to create a container veth device")
			Expect(fakeNetlinkAdapter.LinkAddCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkAddArgsForCall(0)).To(Equal(&netlink.Veth{
				LinkAttrs: netlink.LinkAttrs{
					Name:  "myDeviceName",
					Flags: net.FlagUp,
					MTU:   1234,
				},
				PeerName: "myTemporaryDeviceName",
			}))

			By("getting the newly-created container veth device")
			Expect(fakeNetlinkAdapter.LinkByNameCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkByNameArgsForCall(0)).To(Equal("myTemporaryDeviceName"))

			By("putting the container veth device into the container namespace")
			Expect(fakeNetlinkAdapter.LinkSetNsFdCallCount()).To(Equal(1))
			link, fd := fakeNetlinkAdapter.LinkSetNsFdArgsForCall(0)
			Expect(link).To(Equal(&netlink.Bridge{
				LinkAttrs: netlink.LinkAttrs{
					Name: "my-fake-bridge",
				},
			}))
			Expect(fd).To(Equal(42))
		})

		Context("when adding the link fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkAddReturns(errors.New("banana"))
				creator = &lib.VethPairCreator{
					NetlinkAdapter: fakeNetlinkAdapter,
				}
			})

			It("wraps and returns the error", func() {
				err := creator.Create(cfg)
				Expect(err).To(MatchError("creating veth pair: banana"))
			})
		})

		Context("when getting the container veth device fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkByNameReturns(nil, errors.New("banana"))
			})

			It("wraps and returns the error", func() {
				err := creator.Create(cfg)
				Expect(err).To(MatchError(
					errors.New("failed to find newly-created veth device \"myTemporaryDeviceName\": banana"),
				))
			})
		})

		Context("when moving the container-side veth into the container fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkSetNsFdReturns(errors.New("kiwi"))
			})

			It("wraps and returns the error", func() {
				err := creator.Create(cfg)
				Expect(err).To(MatchError("failed to move veth to container namespace: kiwi"))
			})
		})
	})
})
