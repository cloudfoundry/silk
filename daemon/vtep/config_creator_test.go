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
			creator              *vtep.ConfigCreator
			fakeNetAdapter       *fakes.NetAdapter
			clientConf           clientConfig.Config
			lease                controller.Lease
			underlayInterface1IP net.IP
			underlayInterface2IP net.IP
			loopbackInterfaceIP  net.IP
			underlayInterface1   net.Interface
			underlayInterface2   net.Interface
			loopbackInterface    net.Interface
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

			underlayInterface1 = net.Interface{Index: 42, Name: "eth1"}
			underlayInterface2 = net.Interface{Index: 43, Name: "eth2"}
			loopbackInterface = net.Interface{Index: 93, Name: "lo"}

			fakeNetAdapter.InterfacesReturns([]net.Interface{
				underlayInterface2, underlayInterface1, loopbackInterface,
			}, nil)

			underlayInterface1IP = net.IP{172, 255, 30, 2}
			underlayInterface2IP = net.IP{172, 255, 30, 3}
			loopbackInterfaceIP = net.IP{127, 0, 0, 1}

			fakeNetAdapter.InterfaceAddrsStub = func(requested net.Interface) ([]net.Addr, error) {
				switch requested.Index {
				case underlayInterface1.Index:
					return []net.Addr{
						&net.IPNet{
							IP:   underlayInterface1IP,
							Mask: net.IPMask{255, 255, 255, 255},
						},
					}, nil
				case underlayInterface2.Index:
					return []net.Addr{
						&net.IPNet{
							IP:   underlayInterface2IP,
							Mask: net.IPMask{255, 255, 255, 255},
						},
					}, nil
				case loopbackInterface.Index:
					return []net.Addr{
						&net.IPNet{
							IP:   loopbackInterfaceIP,
							Mask: net.IPMask{255, 255, 255, 255},
						},
					}, nil
				default:
					return nil, errors.New("interface not found")
				}
			}
		})

		It("returns a Config", func() {
			conf, err := creator.Create(clientConf, lease)
			Expect(err).NotTo(HaveOccurred())
			Expect(conf.VTEPName).To(Equal("some-vtep-name"))
			Expect(conf.UnderlayInterface).To(Equal(net.Interface{Index: 42, Name: "eth1"}))
			Expect(conf.UnderlayIP.String()).To(Equal("172.255.30.2"))
			Expect(conf.OverlayIP.String()).To(Equal("10.255.30.0"))
			Expect(conf.OverlayHardwareAddr).To(Equal(net.HardwareAddr{0xee, 0xee, 0x0a, 0xff, 0x1e, 0x00}))
			Expect(conf.VNI).To(Equal(99))
			Expect(conf.OverlayNetworkPrefixLength).To(Equal(16))
			Expect(conf.VTEPPort).To(Equal(12225))

			Expect(fakeNetAdapter.InterfacesCallCount()).To(Equal(1))
			Expect(fakeNetAdapter.InterfaceAddrsCallCount()).To(Equal(2))
			Expect(fakeNetAdapter.InterfaceAddrsArgsForCall(0)).To(Equal(net.Interface{Index: 43, Name: "eth2"}))
			Expect(fakeNetAdapter.InterfaceAddrsArgsForCall(1)).To(Equal(net.Interface{Index: 42, Name: "eth1"}))
			Expect(fakeNetAdapter.InterfaceByNameCallCount()).To(Equal(0))
		})

		Context("when CustomUnderlayInterfaceName is set", func() {
			BeforeEach(func() {
				clientConf.UnderlayIPs = []string{underlayInterface1IP.String(), underlayInterface2IP.String()}
				fakeNetAdapter.InterfaceByNameStub = func(name string) (*net.Interface, error) {
					switch name {
					case "eth1":
						return &underlayInterface1, nil
					case "eth2":
						return &underlayInterface2, nil
					case "lo":
						return &loopbackInterface, nil
					default:
						return nil, errors.New("interface not found")
					}
				}
			})
			It("uses the underlay interface name in the config", func() {
				clientConf.CustomUnderlayInterfaceName = "eth1"

				conf, err := creator.Create(clientConf, lease)
				Expect(err).NotTo(HaveOccurred())
				Expect(conf.UnderlayInterface).To(Equal(net.Interface{Index: 42, Name: "eth1"}))

				Expect(fakeNetAdapter.InterfaceByNameCallCount()).To(Equal(1))
				Expect(fakeNetAdapter.InterfaceByNameArgsForCall(0)).To(Equal("eth1"))
			})
			Context("when the CustomUnderlayInterfaceName does not exist", func() {
				It("returns an error", func() {
					clientConf.CustomUnderlayInterfaceName = "foo"

					_, err := creator.Create(clientConf, lease)

					Expect(err).To(MatchError("find device from name foo: interface not found"))
				})
			})
			Context("when the CustomUnderlayInterfaceName refers to an iface that is not one of the ifaces that have one of the underlay ips", func() {
				It("returns an error", func() {
					clientConf.CustomUnderlayInterfaceName = "lo"

					_, err := creator.Create(clientConf, lease)

					Expect(err).To(MatchError("requested custom underlay interface name 'lo' refers to a non underlay device. valid choices are [eth1, eth2]"))
				})
			})
			Context("when one of the underlay IPs is invalid.", func(){
				It("returns an error", func(){
					clientConf.CustomUnderlayInterfaceName = "eth1"
					clientConf.UnderlayIPs = []string{"banana", underlayInterface2IP.String()}
					_, err := creator.Create(clientConf, lease)

					Expect(err).To(MatchError("parse underlay ip: banana"))
				})
			})
			Context("when one of the underlay IPs is unknown", func(){
				It("returns an error", func(){
					clientConf.CustomUnderlayInterfaceName = "eth1"
					clientConf.UnderlayIPs = []string{"192.168.0.0", underlayInterface2IP.String()}
					_, err := creator.Create(clientConf, lease)

					Expect(err).To(MatchError("find device from ip 192.168.0.0: no interface with address 192.168.0.0"))
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
