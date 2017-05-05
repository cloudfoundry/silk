package config_test

import (
	"errors"
	"net"

	"code.cloudfoundry.org/silk/cni/config"
	"code.cloudfoundry.org/silk/cni/config/fakes"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConfigCreator", func() {
	Describe("Create", func() {
		var (
			hostNS                       *fakes.NetNS
			containerNS                  *fakes.NetNS
			hostMAC                      net.HardwareAddr
			containerMAC                 net.HardwareAddr
			hostIPAddress                net.IP
			addCmdArgs                   *skel.CmdArgs
			ipamResult                   *current.Result
			fakeNamespaceAdapter         *fakes.NamespaceAdapter
			fakeHardwareAddressGenerator *fakes.HardwareAddressGenerator
			fakeDeviceNameGenerator      *fakes.DeviceNameGenerator
			configCreator                *config.ConfigCreator
		)
		BeforeEach(func() {
			hostNS = &fakes.NetNS{}
			containerNS = &fakes.NetNS{}
			addCmdArgs = &skel.CmdArgs{
				Netns:  "/some/container/namespace",
				IfName: "eth0",
			}
			ipamResult = &current.Result{
				IPs: []*current.IPConfig{
					&current.IPConfig{
						Version: "4",
						Address: net.IPNet{
							IP:   []byte{123, 124, 125, 126},
							Mask: []byte{255, 255, 255, 255},
						},
					},
				},
				Routes: []*types.Route{
					&types.Route{
						Dst: net.IPNet{
							IP:   []byte{100, 101, 102, 103},
							Mask: []byte{255, 255, 255, 255},
						},
					},
					&types.Route{
						Dst: net.IPNet{
							IP:   []byte{200, 201, 202, 203},
							Mask: []byte{255, 255, 255, 255},
						},
						GW: net.IP{10, 255, 30, 5},
					},
				},
			}
			hostIPAddress = net.IP{169, 254, 0, 1}
			fakeNamespaceAdapter = &fakes.NamespaceAdapter{}
			fakeHardwareAddressGenerator = &fakes.HardwareAddressGenerator{}
			fakeDeviceNameGenerator = &fakes.DeviceNameGenerator{}
			fakeNamespaceAdapter.GetNSReturns(containerNS, nil)
			containerMAC, _ = net.ParseMAC("ee:ee:12:34:56:78")
			hostMAC, _ = net.ParseMAC("aa:aa:12:34:56:78")
			fakeHardwareAddressGenerator.GenerateForContainerReturns(containerMAC, nil)
			fakeHardwareAddressGenerator.GenerateForHostReturns(hostMAC, nil)
			fakeDeviceNameGenerator.GenerateForHostReturns("s-010255030004", nil)
			fakeDeviceNameGenerator.GenerateTemporaryForContainerReturns("c-010255030004", nil)
			containerNS.PathReturns("/some/container/namespace")
			configCreator = &config.ConfigCreator{
				HardwareAddressGenerator: fakeHardwareAddressGenerator,
				DeviceNameGenerator:      fakeDeviceNameGenerator,
				NamespaceAdapter:         fakeNamespaceAdapter,
			}
		})

		It("creates a config with the desired container device metadata", func() {
			conf, err := configCreator.Create(hostNS, addCmdArgs, ipamResult, 1450)
			Expect(err).NotTo(HaveOccurred())

			Expect(conf.Container.DeviceName).To(Equal("eth0"))
			Expect(conf.Container.TemporaryDeviceName).To(Equal("c-010255030004"))
			Expect(conf.Container.Namespace).To(Equal(containerNS))
			Expect(conf.Container.Address.IP).To(Equal(ipamResult.IPs[0].Address.IP))
			Expect(conf.Container.Address.Hardware).To(Equal(containerMAC))
			Expect(conf.Container.Routes).To(Equal(ipamResult.Routes))
			Expect(conf.Container.MTU).To(Equal(1450))
		})

		It("creates a config with the desired host device metadata", func() {
			conf, err := configCreator.Create(hostNS, addCmdArgs, ipamResult, 1450)
			Expect(err).NotTo(HaveOccurred())

			Expect(conf.Host.DeviceName).To(Equal("s-010255030004"))
			Expect(conf.Host.Namespace).To(Equal(hostNS))
			Expect(conf.Host.Address.IP).To(Equal(hostIPAddress))
			Expect(conf.Host.Address.Hardware).To(Equal(hostMAC))
		})

		Context("when the args interface name is blank", func() {
			BeforeEach(func() {
				addCmdArgs.IfName = ""
			})
			It("returns an error", func() {
				_, err := configCreator.Create(hostNS, addCmdArgs, ipamResult, 1450)
				Expect(err).To(MatchError("IfName cannot be empty"))
			})
		})

		Context("when the args interface name is longer than 15 characters", func() {
			BeforeEach(func() {
				addCmdArgs.IfName = "1234567890123456"
			})
			It("returns an error", func() {
				_, err := configCreator.Create(hostNS, addCmdArgs, ipamResult, 1450)
				Expect(err).To(MatchError("IfName cannot be longer than 15 characters"))
			})
		})

		Context("when the container namespace cannot be found", func() {
			BeforeEach(func() {
				fakeNamespaceAdapter.GetNSReturns(nil, errors.New("banana"))
			})
			It("returns an error", func() {
				_, err := configCreator.Create(hostNS, addCmdArgs, ipamResult, 1450)
				Expect(err).To(MatchError("getting container namespace: banana"))
			})
		})

		Context("when the hardware address generator fails for the container hw address", func() {
			BeforeEach(func() {
				fakeHardwareAddressGenerator.GenerateForContainerReturns(nil, errors.New("potato"))
			})
			It("wraps and returns the error", func() {
				_, err := configCreator.Create(hostNS, addCmdArgs, ipamResult, 1450)
				Expect(err).To(MatchError("generating container veth hardware address: potato"))
			})
		})

		Context("when the hardware address generator fails for the host hw address", func() {
			BeforeEach(func() {
				fakeHardwareAddressGenerator.GenerateForHostReturns(nil, errors.New("potato"))
			})
			It("wraps and returns the error", func() {
				_, err := configCreator.Create(hostNS, addCmdArgs, ipamResult, 1450)
				Expect(err).To(MatchError("generating host veth hardware address: potato"))
			})
		})

		Context("when the device name generator fails for the host device", func() {
			BeforeEach(func() {
				fakeDeviceNameGenerator.GenerateForHostReturns("", errors.New("potato"))
			})
			It("wraps and returns the error", func() {
				_, err := configCreator.Create(hostNS, addCmdArgs, ipamResult, 1450)
				Expect(err).To(MatchError("generating host device name: potato"))
			})
		})

		Context("when the device name generator fails for the container's temporary name", func() {
			BeforeEach(func() {
				fakeDeviceNameGenerator.GenerateTemporaryForContainerReturns("", errors.New("potato"))
			})
			It("wraps and returns the error", func() {
				_, err := configCreator.Create(hostNS, addCmdArgs, ipamResult, 1450)
				Expect(err).To(MatchError("generating temporary container device name: potato"))
			})
		})

		Context("when the IPAM result has no IP addresses", func() {
			BeforeEach(func() {
				ipamResult.IPs = []*current.IPConfig{}
			})
			It("returns an error", func() {
				_, err := configCreator.Create(hostNS, addCmdArgs, ipamResult, 1450)
				Expect(err).To(MatchError("no IP address in IPAM result"))
			})
		})

	})

})
