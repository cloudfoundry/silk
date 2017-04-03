package config_test

import (
	"net"

	"code.cloudfoundry.org/silk/cni/config"
	"code.cloudfoundry.org/silk/cni/lib/fakes"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	var cfg *config.Config

	BeforeEach(func() {
		cfg = &config.Config{}
		cfg.Container.DeviceName = "container-device-name"
		fakeNamespace := &fakes.NetNS{}
		fakeNamespace.PathReturns("/some/namespace")
		cfg.Container.Namespace = fakeNamespace
		cfg.Container.Address.IP = net.IP{10, 255, 30, 5}
		cfg.Container.Address.Hardware = net.HardwareAddr{0x01, 0x02, 0x03, 0x0A, 0xBC, 0xDE}
		cfg.Container.MTU = 1234
		cfg.Host.DeviceName = "host-device-name"
		cfg.Host.Address.IP = net.IP{169, 254, 0, 1}
		cfg.Host.Address.Hardware = net.HardwareAddr{0xdd, 0xdd, 0x03, 0x0A, 0xBC, 0xDE}

		cfg.Container.Routes = []*types.Route{
			&types.Route{
				Dst: net.IPNet{
					IP:   net.IP{1, 1, 0, 0},
					Mask: []byte{255, 255, 255, 255},
				},
				GW: net.IP{1, 1, 0, 0},
			},
		}
	})

	AfterEach(func() {
		cfg.Container.Namespace.Close()
	})

	Describe("AsCNIResult", func() {
		It("returns a CNI v0.3.0 result that represents the config", func() {
			result := cfg.AsCNIResult()
			Expect(result.Interfaces).To(HaveLen(2))
			Expect(result.Interfaces[0]).To(Equal(&current.Interface{
				Name:    "host-device-name",
				Mac:     "dd:dd:03:0a:bc:de",
				Sandbox: "",
			}))
			Expect(result.Interfaces[1]).To(Equal(&current.Interface{
				Name:    "container-device-name",
				Mac:     "01:02:03:0a:bc:de",
				Sandbox: cfg.Container.Namespace.Path(),
			}))

			Expect(result.IPs).To(HaveLen(1))
			Expect(result.Interfaces[result.IPs[0].Interface].Name).To(Equal("container-device-name"))
			Expect(result.IPs[0].Version).To(Equal("4"))
			Expect(result.IPs[0].Address.String()).To(Equal("10.255.30.5/32"))
			Expect(result.IPs[0].Gateway.String()).To(Equal("169.254.0.1"))

			Expect(result.Routes).To(ConsistOf(cfg.Container.Routes))
		})
	})
})
