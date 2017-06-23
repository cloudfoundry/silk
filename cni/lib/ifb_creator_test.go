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
		It("removes the ifb device in the specified namespace and device name", func() {
			Expect(ifbCreator.Teardown(containerNS, "device-name")).To(Succeed())

			Expect(containerNS.DoCallCount()).To(Equal(1))

			Expect(fakeNetlinkAdapter.LinkByNameCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkByNameArgsForCall(0)).To(Equal("device-name"))

			Expect(fakeNetlinkAdapter.AddrListCallCount()).To(Equal(1))
			link, family := fakeNetlinkAdapter.AddrListArgsForCall(0)
			Expect(link).To(Equal(containerLink))
			Expect(family).To(Equal(netlink.FAMILY_V4))

			Expect(fakeDeviceNameGenerator.GenerateForHostIFBCallCount()).To(Equal(1))
			Expect(fakeDeviceNameGenerator.GenerateForHostIFBArgsForCall(0)).To(Equal(net.IPv4(10, 255, 30, 5)))

			Expect(fakeNetlinkAdapter.LinkDelCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkDelArgsForCall(0)).To(Equal(&netlink.Ifb{
				LinkAttrs: netlink.LinkAttrs{
					Name: "ifb-device-name",
				},
			}))
		})
		Context("when getting the link fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkByNameReturns(nil, errors.New("banana"))
			})
			It("returns a sensible error", func() {
				err := ifbCreator.Teardown(containerNS, "device-name")

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("find link: banana"))
			})
		})
		Context("when listing the addresses fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.AddrListReturns(nil, errors.New("apple"))
			})
			It("returns a sensible error", func() {
				err := ifbCreator.Teardown(containerNS, "device-name")

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("list addresses: apple"))
			})
		})
		Context("when there are multiple container ip addresses", func() {
			BeforeEach(func() {
				containerAddr2 := netlink.Addr{
					IPNet: &net.IPNet{
						IP: net.IPv4(10, 255, 30, 6),
					},
				}
				fakeNetlinkAdapter.AddrListReturns([]netlink.Addr{containerAddr, containerAddr2}, nil)
				fakeDeviceNameGenerator.GenerateForHostIFBStub = func(ip net.IP) (string, error) {

					switch ip.String() {
					case "10.255.30.5":
						return "ifb-device-name", nil
					case "10.255.30.6":
						return "ifb-device-name-1", nil
					default:
						return "", nil
					}
				}

			})

			It("removes every ifb associated with every container address", func() {
				Expect(ifbCreator.Teardown(containerNS, "device-name")).To(Succeed())

				Expect(containerNS.DoCallCount()).To(Equal(1))

				Expect(fakeNetlinkAdapter.LinkByNameCallCount()).To(Equal(1))
				Expect(fakeNetlinkAdapter.LinkByNameArgsForCall(0)).To(Equal("device-name"))

				Expect(fakeNetlinkAdapter.AddrListCallCount()).To(Equal(1))
				link, family := fakeNetlinkAdapter.AddrListArgsForCall(0)
				Expect(link).To(Equal(containerLink))
				Expect(family).To(Equal(netlink.FAMILY_V4))

				Expect(fakeDeviceNameGenerator.GenerateForHostIFBCallCount()).To(Equal(2))
				Expect(fakeDeviceNameGenerator.GenerateForHostIFBArgsForCall(0)).To(Equal(net.IPv4(10, 255, 30, 5)))
				Expect(fakeDeviceNameGenerator.GenerateForHostIFBArgsForCall(1)).To(Equal(net.IPv4(10, 255, 30, 6)))

				Expect(fakeNetlinkAdapter.LinkDelCallCount()).To(Equal(2))
				Expect(fakeNetlinkAdapter.LinkDelArgsForCall(0)).To(Equal(&netlink.Ifb{
					LinkAttrs: netlink.LinkAttrs{
						Name: "ifb-device-name",
					},
				}))
				Expect(fakeNetlinkAdapter.LinkDelArgsForCall(1)).To(Equal(&netlink.Ifb{
					LinkAttrs: netlink.LinkAttrs{
						Name: "ifb-device-name-1",
					},
				}))
			})

			Context("when generating the device name for ifb fails", func() {
				BeforeEach(func() {
					fakeDeviceNameGenerator.GenerateForHostIFBStub = func(ip net.IP) (string, error) {
						switch ip.String() {
						case "10.255.30.5":
							return "", errors.New("pear")
						case "10.255.30.6":
							return "ifb-device-name-1", nil
						default:
							return "", nil
						}
					}
				})

				It("returns a sensible error", func() {
					err := ifbCreator.Teardown(containerNS, "device-name")
					Expect(err).To(HaveOccurred())

					Expect(err.Error()).To(ContainSubstring("generate ifb device name: pear"))
				})
			})
			Context("when deleting the link fails", func() {
				BeforeEach(func() {
					fakeNetlinkAdapter.LinkDelStub = func(link netlink.Link) error {
						if link.Attrs().Name == "ifb-device-name" {
							return errors.New("mango")
						}
						return nil
					}
				})
				It("returns a sensible error", func() {
					err := ifbCreator.Teardown(containerNS, "device-name")
					Expect(err).To(HaveOccurred())

					Expect(err.Error()).To(ContainSubstring("delete link: mango"))
				})
			})
		})
	})
})
