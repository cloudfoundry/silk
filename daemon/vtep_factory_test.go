package daemon_test

import (
	"errors"
	"net"

	"code.cloudfoundry.org/silk/daemon"
	"code.cloudfoundry.org/silk/daemon/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

var _ = Describe("VtepFactory", func() {
	var (
		fakeNetlinkAdapter           *fakes.NetlinkAdapter
		fakeHardwareAddressGenerator *fakes.HardwareAddressGenerator
		vtepFactory                  *daemon.VTEPFactory
		deviceName                   string
		underlayInterface            net.Interface
		underlayIP                   net.IP
		overlayIP                    net.IP
		overlayHardwareAddr          net.HardwareAddr
	)
	Describe("CreateVTEP", func() {
		BeforeEach(func() {
			fakeNetlinkAdapter = &fakes.NetlinkAdapter{}
			fakeHardwareAddressGenerator = &fakes.HardwareAddressGenerator{}
			vtepFactory = &daemon.VTEPFactory{
				NetlinkAdapter:           fakeNetlinkAdapter,
				HardwareAddressGenerator: fakeHardwareAddressGenerator,
			}
			deviceName = "some-device"
			underlayInterface = net.Interface{
				Index:        4,
				MTU:          1450,
				Name:         "eth4",
				HardwareAddr: net.HardwareAddr{0xbb, 0xbb, 0x00, 0x00, 0x12, 0x34},
				Flags:        net.FlagUp | net.FlagMulticast,
			}
			underlayIP = net.IP{172, 255, 0, 0}
			overlayIP = net.IP{10, 255, 32, 0}

			overlayHardwareAddr = net.HardwareAddr{0xee, 0xee, 0x0a, 0xff, 0x20, 0x00}

			fakeHardwareAddressGenerator.GenerateForVTEPReturns(overlayHardwareAddr, nil)
		})

		It("creates the link", func() {
			err := vtepFactory.CreateVTEP(deviceName, underlayInterface, underlayIP, overlayIP)
			Expect(err).NotTo(HaveOccurred())

			expectedLink := &netlink.Vxlan{
				LinkAttrs: netlink.LinkAttrs{
					Name: deviceName,
				},
				VxlanId:      42,
				SrcAddr:      underlayIP,
				GBP:          true,
				Port:         4789,
				VtepDevIndex: underlayInterface.Index,
			}

			Expect(fakeHardwareAddressGenerator.GenerateForVTEPCallCount()).To(Equal(1))
			Expect(fakeHardwareAddressGenerator.GenerateForVTEPArgsForCall(0)).To(Equal(overlayIP))

			Expect(fakeNetlinkAdapter.LinkAddCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkAddArgsForCall(0)).To(Equal(expectedLink))

			Expect(fakeNetlinkAdapter.LinkSetUpCallCount()).To(Equal(1))
			Expect(fakeNetlinkAdapter.LinkSetUpArgsForCall(0)).To(Equal(expectedLink))

			Expect(fakeNetlinkAdapter.LinkSetHardwareAddrCallCount()).To(Equal(1))
			link, hardwareAddr := fakeNetlinkAdapter.LinkSetHardwareAddrArgsForCall(0)
			Expect(link).To(Equal(expectedLink))
			Expect(hardwareAddr).To(Equal(overlayHardwareAddr))
		})

		Context("when generating the vtep hardware address fails", func() {
			BeforeEach(func() {
				fakeHardwareAddressGenerator.GenerateForVTEPReturns(nil, errors.New("potato"))
			})
			It("wraps and returns the error", func() {
				err := vtepFactory.CreateVTEP(deviceName, underlayInterface, underlayIP, overlayIP)
				Expect(err).To(MatchError("generate vtep hardware address: potato"))
			})
		})

		Context("when adding the link fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkAddReturns(errors.New("potato"))
			})
			It("wraps and returns the error", func() {
				err := vtepFactory.CreateVTEP(deviceName, underlayInterface, underlayIP, overlayIP)
				Expect(err).To(MatchError("create link: potato"))
			})
		})

		Context("when setting the link up fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkSetUpReturns(errors.New("potato"))
			})
			It("wraps and returns the error", func() {
				err := vtepFactory.CreateVTEP(deviceName, underlayInterface, underlayIP, overlayIP)
				Expect(err).To(MatchError("up link: potato"))
			})
		})

		Context("when setting the hardware address fails", func() {
			BeforeEach(func() {
				fakeNetlinkAdapter.LinkSetHardwareAddrReturns(errors.New("potato"))
			})
			It("wraps and returns the error", func() {
				err := vtepFactory.CreateVTEP(deviceName, underlayInterface, underlayIP, overlayIP)
				Expect(err).To(MatchError("set hardware addr: potato"))
			})
		})
	})
})
