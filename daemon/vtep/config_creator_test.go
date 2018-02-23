package vtep_test

import (
	"errors"
	"net"

	clientConfig "code.cloudfoundry.org/silk/client/config"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/daemon/vtep"
	"code.cloudfoundry.org/silk/daemon/vtep/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConfigCreator", func() {
	Describe("Create", func() {
		var (
			creator        *vtep.ConfigCreator
			fakeNetAdapter *fakes.NetAdapter
			clientConf     clientConfig.Config
			lease          controller.Lease
		)
		BeforeEach(func() {
			fakeNetAdapter = &fakes.NetAdapter{}
			creator = &vtep.ConfigCreator{
				NetAdapter: fakeNetAdapter,
			}
			clientConf = clientConfig.Config{
				UnderlayIP:         "172.255.30.2",
				SubnetPrefixLength: 24,
				VTEPName:           "some-vtep-name",
				VNI:                99,
				OverlayNetwork:     "10.255.0.0/16",
				VTEPPort:           12225,
			}
			lease = controller.Lease{
				UnderlayIP:          "172.255.30.02",
				OverlaySubnet:       "10.255.30.0/24",
				OverlayHardwareAddr: "ee:ee:0a:ff:1e:00",
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
		})

		It("returns a Config", func() {
			conf, err := creator.Create(clientConf, lease)
			Expect(err).NotTo(HaveOccurred())
			Expect(conf.VTEPName).To(Equal("some-vtep-name"))
			Expect(conf.UnderlayInterface).To(Equal(net.Interface{Index: 42}))
			Expect(conf.UnderlayIP.String()).To(Equal("172.255.30.2"))
			Expect(conf.OverlayIP.String()).To(Equal("10.255.30.0"))
			Expect(conf.OverlayHardwareAddr).To(Equal(net.HardwareAddr{0xee, 0xee, 0x0a, 0xff, 0x1e, 0x00}))
			Expect(conf.VNI).To(Equal(99))
			Expect(conf.OverlayNetworkPrefixLength).To(Equal(16))
			Expect(conf.VTEPPort).To(Equal(12225))

			Expect(fakeNetAdapter.InterfacesCallCount()).To(Equal(1))
			Expect(fakeNetAdapter.InterfaceAddrsCallCount()).To(Equal(1))
			Expect(fakeNetAdapter.InterfaceAddrsArgsForCall(0)).To(Equal(net.Interface{Index: 42}))
			Expect(fakeNetAdapter.InterfaceByNameCallCount()).To(Equal(0))
		})

		Context("when CustomUnderlayInterfaceName is set", func() {
			BeforeEach(func() {
				clientConf.CustomUnderlayInterfaceName = "eth1"
				fakeNetAdapter.InterfaceByNameReturns(&net.Interface{
					Index: 38,
				}, nil)
			})
			It("uses the underlay interface name in the config", func() {
				conf, err := creator.Create(clientConf, lease)
				Expect(err).NotTo(HaveOccurred())
				Expect(conf.UnderlayInterface).To(Equal(net.Interface{Index: 38}))

				Expect(fakeNetAdapter.InterfacesCallCount()).To(Equal(0))
				Expect(fakeNetAdapter.InterfaceByNameCallCount()).To(Equal(1))
				Expect(fakeNetAdapter.InterfaceByNameArgsForCall(0)).To(Equal("eth1"))
			})
			Context("when the CustomUnderlayInterfaceName does not exist", func() {
				BeforeEach(func() {
					fakeNetAdapter.InterfaceByNameReturns(nil, errors.New("banana"))
				})
				It("returns an error", func() {
					_, err := creator.Create(clientConf, lease)
					Expect(err).To(MatchError("find device from name eth1: banana"))
				})
			})
		})

		Context("when the overlay network prefix length is greater than or equal to the subnet prefix length", func() {
			BeforeEach(func() {
				clientConf.OverlayNetwork = "10.255.0.0/30"
			})
			It("returns an error", func() {
				_, err := creator.Create(clientConf, lease)
				Expect(err).To(MatchError("overlay prefix 30 must be smaller than subnet prefix 24"))
			})
		})

		Context("when the overlay network is not set", func() {
			BeforeEach(func() {
				clientConf.OverlayNetwork = ""
			})
			It("returns an error", func() {
				_, err := creator.Create(clientConf, lease)
				Expect(err).To(MatchError("determine overlay network: invalid CIDR address: "))
			})
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

		Context("when the vtep port is less than 1", func() {
			BeforeEach(func() {
				clientConf.VTEPPort = 0
			})

			It("returns a sensible error", func() {
				_, err := creator.Create(clientConf, lease)
				Expect(err).To(MatchError("vtep port must be greater than 0"))
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
				lease.OverlaySubnet = "foo"
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

		Context("when parsing the hardware addr fails", func() {
			BeforeEach(func() {
				lease.OverlayHardwareAddr = "foo"
			})
			It("returns a sensible error", func() {
				_, err := creator.Create(clientConf, lease)
				Expect(err).To(MatchError(ContainSubstring("parsing hardware address:")))
			})
		})
	})
})
