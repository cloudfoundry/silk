package vtep_test

import (
	"errors"
	"net"

	"code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/client/state"
	"code.cloudfoundry.org/silk/daemon/vtep"
	"code.cloudfoundry.org/silk/daemon/vtep/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConfigCreator", func() {
	Describe("Create", func() {
		var (
			creator                      *vtep.ConfigCreator
			fakeNetAdapter               *fakes.NetAdapter
			fakeHardwareAddressGenerator *fakes.HardwareAddressGenerator
			clientConf                   config.Config
			lease                        state.SubnetLease
		)
		BeforeEach(func() {
			fakeNetAdapter = &fakes.NetAdapter{}
			fakeHardwareAddressGenerator = &fakes.HardwareAddressGenerator{}
			creator = &vtep.ConfigCreator{
				NetAdapter:               fakeNetAdapter,
				HardwareAddressGenerator: fakeHardwareAddressGenerator,
			}
			clientConf = config.Config{
				UnderlayIP:         "172.255.30.2",
				SubnetRange:        "10.255.0.0/16",
				SubnetPrefixLength: 24,
				VTEPName:           "some-vtep-name",
				VNI:                99,
			}
			lease = state.SubnetLease{
				UnderlayIP: "172.255.30.20",
				Subnet:     "10.255.30.0/24",
			}

			fakeNetAdapter.InterfacesReturns([]net.Interface{net.Interface{
				Index: 42,
			}}, nil)
			fakeNetAdapter.InterfaceAddrsReturns([]net.Addr{
				&net.IPNet{
					IP:   net.IP{172, 255, 30, 2},
					Mask: net.IPMask{255, 255, 255, 255},
				},
			}, nil)

			fakeHardwareAddressGenerator.GenerateForVTEPReturns(net.HardwareAddr{0xee, 0xee, 0x0a, 0xff, 0x20, 0x00}, nil)
		})

		It("returns a Config", func() {
			conf, err := creator.Create(clientConf, lease)
			Expect(err).NotTo(HaveOccurred())
			Expect(conf.VTEPName).To(Equal("some-vtep-name"))
			Expect(conf.UnderlayInterface).To(Equal(net.Interface{Index: 42}))
			Expect(conf.UnderlayIP.String()).To(Equal("172.255.30.2"))
			Expect(conf.OverlayIP.String()).To(Equal("10.255.30.0"))
			Expect(conf.OverlayHardwareAddr).To(Equal(net.HardwareAddr{0xee, 0xee, 0x0a, 0xff, 0x20, 0x00}))
			Expect(conf.VNI).To(Equal(99))

			Expect(fakeNetAdapter.InterfacesCallCount()).To(Equal(1))

			Expect(fakeNetAdapter.InterfaceAddrsCallCount()).To(Equal(1))
			Expect(fakeNetAdapter.InterfaceAddrsArgsForCall(0)).To(Equal(net.Interface{Index: 42}))

			Expect(fakeHardwareAddressGenerator.GenerateForVTEPCallCount()).To(Equal(1))
			Expect(fakeHardwareAddressGenerator.GenerateForVTEPArgsForCall(0).String()).To(Equal("10.255.30.0"))
		})

		Context("when the vtep name is empty", func() {
			BeforeEach(func() {
				clientConf.VTEPName = ""
			})
			It("returns a sensible error", func() {
				_, err := creator.Create(clientConf, lease)
				Expect(err).To(MatchError("empty vtep name"))
			})
		})

		Context("when parsing the underlay ip returns nil", func() {
			BeforeEach(func() {
				clientConf.UnderlayIP = "some-invalid"
			})
			It("returns a sensible error", func() {
				_, err := creator.Create(clientConf, lease)
				Expect(err).To(MatchError("parse underlay ip: some-invalid"))
			})
		})

		Context("when parsing the lease subnet returns nil", func() {
			BeforeEach(func() {
				lease.Subnet = "foo"
			})
			It("returns a sensible error", func() {
				_, err := creator.Create(clientConf, lease)
				Expect(err).To(MatchError("determine vtep overlay ip: invalid CIDR address: foo"))
			})
		})

		Context("when the interface cannot be found", func() {
			BeforeEach(func() {
				fakeNetAdapter.InterfacesReturns(nil, errors.New("pomelo"))
			})
			It("returns a sensible error", func() {
				_, err := creator.Create(clientConf, lease)
				Expect(err).To(MatchError("find device from ip 172.255.30.2: find interfaces: pomelo"))
			})
		})

		Context("when the getting the addresses of the interface errors", func() {
			BeforeEach(func() {
				fakeNetAdapter.InterfaceAddrsReturns(nil, errors.New("grape"))
			})
			It("returns a sensible error", func() {
				_, err := creator.Create(clientConf, lease)
				Expect(err).To(MatchError("find device from ip 172.255.30.2: get addresses: grape"))
			})
		})

		Context("when parsing the CIDR of the interface fails", func() {
			BeforeEach(func() {
				fakeNetAdapter.InterfaceAddrsReturns([]net.Addr{
					&net.IPNet{
						IP: net.IP{173, 255, 44, 4},
					},
				}, nil)
			})
			It("returns a sensible error", func() {
				_, err := creator.Create(clientConf, lease)
				Expect(err).To(MatchError("find device from ip 172.255.30.2: parse address: invalid CIDR address: <nil>"))
			})
		})

		Context("when there are no interfaces with the given ip address", func() {
			BeforeEach(func() {
				fakeNetAdapter.InterfaceAddrsReturns([]net.Addr{
					&net.IPNet{
						IP:   net.IP{173, 255, 44, 4},
						Mask: net.IPMask{255, 255, 255, 255},
					},
				}, nil)
			})
			It("returns a sensible error", func() {
				_, err := creator.Create(clientConf, lease)
				Expect(err).To(MatchError("find device from ip 172.255.30.2: no interface with address 172.255.30.2"))
			})
		})

		Context("when generating the hardware addr fails", func() {
			BeforeEach(func() {
				fakeHardwareAddressGenerator.GenerateForVTEPReturns(nil, errors.New("pomegranate"))
			})
			It("returns a sensible error", func() {
				_, err := creator.Create(clientConf, lease)
				Expect(err).To(MatchError("generate hardware address for ip 10.255.30.0: pomegranate"))
			})
		})
	})
})
