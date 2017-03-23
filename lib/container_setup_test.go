package lib_test

import (
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/silk/config"
	"github.com/cloudfoundry-incubator/silk/lib"
	"github.com/cloudfoundry-incubator/silk/lib/fakes"
	"github.com/containernetworking/cni/pkg/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

var _ = Describe("Container Setup", func() {

	var (
		containerNS        *fakes.NetNS
		cfg                *config.Config
		fakeLinkOperations *fakes.LinkOperations
		fakeCommon         *fakes.Common
		containerSetup     *lib.Container
		containerAddr      config.DualAddress
		hostAddr           config.DualAddress
	)

	BeforeEach(func() {
		fakeLinkOperations = &fakes.LinkOperations{}
		fakeCommon = &fakes.Common{}
		containerNS = &fakes.NetNS{}
		containerNS.DoStub = lib.NetNsDoStub

		containerAddr = config.DualAddress{IP: net.IP{10, 255, 30, 4}}
		hostAddr = config.DualAddress{IP: net.IP{169, 254, 0, 1}}

		cfg = &config.Config{}
		cfg.Container.DeviceName = "eth0"
		cfg.Container.Namespace = containerNS
		cfg.Container.TemporaryDeviceName = "someTemporaryDeviceName"
		cfg.Container.Address = containerAddr
		cfg.Host.Address = hostAddr
		cfg.Container.Routes = []types.Route{
			types.Route{
				Dst: net.IPNet{
					IP:   []byte{50, 51, 52, 53},
					Mask: []byte{255, 255, 255, 255},
				},
			},
			types.Route{
				Dst: net.IPNet{
					IP:   []byte{150, 151, 152, 153},
					Mask: []byte{255, 255, 255, 255},
				},
				GW: net.IP{10, 150, 25, 2},
			},
			types.Route{
				Dst: net.IPNet{
					IP:   []byte{250, 251, 252, 0},
					Mask: []byte{255, 255, 255, 0},
				},
				GW: net.IP{10, 250, 25, 2},
			},
		}

		containerSetup = &lib.Container{
			Common:         fakeCommon,
			LinkOperations: fakeLinkOperations,
		}
	})

	Describe("Setup", func() {
		It("renames the device and calls basic setup in the container namespace", func() {
			err := containerSetup.Setup(cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeLinkOperations.RenameLinkCallCount()).To(Equal(1))
			oldName, newName := fakeLinkOperations.RenameLinkArgsForCall(0)
			Expect(oldName).To(Equal("someTemporaryDeviceName"))
			Expect(newName).To(Equal("eth0"))

			Expect(fakeCommon.BasicSetupCallCount()).To(Equal(1))
			device, local, peer := fakeCommon.BasicSetupArgsForCall(0)
			Expect(device).To(Equal("eth0"))
			Expect(local).To(Equal(containerAddr))
			Expect(peer).To(Equal(hostAddr))

			By("Adding all the routes")
			Expect(fakeLinkOperations.RouteAddCallCount()).To(Equal(3))
			Expect(fakeLinkOperations.RouteAddArgsForCall(0)).To(Equal(netlink.Route{
				Src: containerAddr.IP,
				Dst: &net.IPNet{
					IP:   []byte{50, 51, 52, 53},
					Mask: []byte{255, 255, 255, 255},
				},
			}))
			Expect(fakeLinkOperations.RouteAddArgsForCall(1)).To(Equal(netlink.Route{
				Src: containerAddr.IP,
				Dst: &net.IPNet{
					IP:   []byte{150, 151, 152, 153},
					Mask: []byte{255, 255, 255, 255},
				},
				Gw: net.IP{10, 150, 25, 2},
			}))
			Expect(fakeLinkOperations.RouteAddArgsForCall(2)).To(Equal(netlink.Route{
				Src: containerAddr.IP,
				Dst: &net.IPNet{
					IP:   []byte{250, 251, 252, 0},
					Mask: []byte{255, 255, 255, 0},
				},
				Gw: net.IP{10, 250, 25, 2},
			}))
		})

		Context("when renaming the link fails", func() {
			BeforeEach(func() {
				fakeLinkOperations.RenameLinkReturns(errors.New("asparagus"))
			})
			It("returns a meaningul error", func() {
				err := containerSetup.Setup(cfg)
				Expect(err).To(MatchError("renaming link in container: asparagus"))
			})
		})

		Context("when the basic device setup fails", func() {
			BeforeEach(func() {
				fakeCommon.BasicSetupReturns(errors.New("lettuce"))
			})
			It("returns a meaningul error", func() {
				err := containerSetup.Setup(cfg)
				Expect(err).To(MatchError("setting up device in container: lettuce"))
			})
		})

		Context("when adding one of the routes fails", func() {
			BeforeEach(func() {
				fakeLinkOperations.RouteAddStub = func(route netlink.Route) error {
					if route.Gw.String() == "10.150.25.2" {
						return errors.New("pickle")
					}
					return nil
				}
			})
			It("returns a meaningul error", func() {
				err := containerSetup.Setup(cfg)
				Expect(err).To(MatchError("adding route in container: pickle"))

				Expect(fakeLinkOperations.RouteAddCallCount()).To(Equal(2))
			})
		})
	})

	Describe("Teardown", func() {
		It("deletes the link in the specified namespace and device name", func() {
			err := containerSetup.Teardown(containerNS, "eth0")
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeLinkOperations.DeleteLinkByNameCallCount()).To(Equal(1))
			Expect(fakeLinkOperations.DeleteLinkByNameArgsForCall(0)).To(Equal("eth0"))
		})

		Context("when deleting the link fails", func() {
			BeforeEach(func() {
				fakeLinkOperations.DeleteLinkByNameReturns(errors.New("eggplant"))
			})
			It("returns a meaningul error", func() {
				err := containerSetup.Teardown(containerNS, "eth0")
				Expect(err).To(MatchError("deleting link: eggplant"))
			})
		})
	})
})
