package lib_test

import (
	"errors"
	"net"

	"code.cloudfoundry.org/silk/cni/lib/fakes"

	"code.cloudfoundry.org/silk/cni/config"

	"code.cloudfoundry.org/silk/cni/lib"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

var _ = Describe("IfbCreator", func() {
	var (
		containerNS             *fakes.NetNS
		containerLink           netlink.Link
		fakeNetlinkAdapter      *fakes.NetlinkAdapter
		fakeDeviceNameGenerator *fakes.DeviceNameGenerator
		ifbCreator              *lib.IFBCreator
		cfg                     *config.Config
		containerAddr           netlink.Addr
	)
	BeforeEach(func() {
		fakeNetlinkAdapter = &fakes.NetlinkAdapter{}
		containerLink = &netlink.Bridge{
			LinkAttrs: netlink.LinkAttrs{
				Index: 60,
				Name:  "my-fake-container-device",
			},
		}
		containerAddr = netlink.Addr{
			IPNet: &net.IPNet{
				IP: net.IPv4(10, 255, 30, 5),
			},
		}
		fakeNetlinkAdapter.LinkByNameReturns(containerLink, nil)
		fakeNetlinkAdapter.AddrListReturns([]netlink.Addr{containerAddr}, nil)

		fakeDeviceNameGenerator = &fakes.DeviceNameGenerator{}
		fakeDeviceNameGenerator.GenerateForHostIFBReturns("ifb-device-name", nil)

		ifbCreator = &lib.IFBCreator{
			NetlinkAdapter:      fakeNetlinkAdapter,
			DeviceNameGenerator: fakeDeviceNameGenerator,
		}

		containerNS = &fakes.NetNS{}
		containerNS.DoStub = lib.NetNsDoStub

		cfg = &config.Config{}
		cfg.Container.MTU = 1234
		cfg.IFB.DeviceName = "myIfbDeviceName"

	})

	Describe("Create", func() {
		It("creates an IFB device", func() {
			Expect(ifbCreator.Create(cfg)).To(Succeed())

			Expect(fakeNetlinkAdapter.LinkAddCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkAddArgsForCall(0)).To(Equal(&netlink.Ifb{
				LinkAttrs: netlink.LinkAttrs{
					Name:  cfg.IFB.DeviceName,
					Flags: net.FlagUp,
					MTU:   1234,
				},
			}))
		})

		Context("when adding a link fails", func() {

			BeforeEach(func() {
				fakeNetlinkAdapter.LinkAddReturns(errors.New("banana"))
			})

			It("should return a sensible error", func() {
				err := ifbCreator.Create(cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("adding link: banana"))
			})
		})
	})

	Describe("Teardown", func() {
		It("removes the ifb device given the container handle", func() {
			Expect(ifbCreator.Teardown("10.255.30.5")).To(Succeed())

			Expect(fakeDeviceNameGenerator.GenerateForHostIFBCallCount()).To(Equal(1))
			Expect(fakeDeviceNameGenerator.GenerateForHostIFBArgsForCall(0)).To(Equal(net.IPv4(10, 255, 30, 5)))

			Expect(fakeNetlinkAdapter.LinkDelCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkDelArgsForCall(0)).To(Equal(&netlink.Ifb{
				LinkAttrs: netlink.LinkAttrs{
					Name: "ifb-device-name",
				},
			}))
		})

		Context("when generating the device name for ifb fails", func() {
			BeforeEach(func() {
				fakeDeviceNameGenerator.GenerateForHostIFBReturns("", errors.New("pear"))
			})

			It("returns a sensible error", func() {
				err := ifbCreator.Teardown("some-container-handle")
				Expect(err).To(HaveOccurred())

				Expect(err.Error()).To(ContainSubstring("generate ifb device name: pear"))
			})
		})

		Context("when deleting the link fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkDelReturns(errors.New("mango"))
			})
			It("returns a sensible error", func() {
				err := ifbCreator.Teardown("some-container-handle")
				Expect(err).To(HaveOccurred())

				Expect(err.Error()).To(ContainSubstring("delete link: mango"))
			})
		})
	})
})
