package config_test

import (
	"net"

	"github.com/cloudfoundry-incubator/silk/config"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/types/current"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	var cfg *config.Config

	BeforeEach(func() {
		cfg = &config.Config{}
		cfg.Container.DeviceName = "container-device-name"
		cfg.Container.Namespace, _ = ns.NewNS()
		cfg.Container.IPAddress = net.IPNet{
			IP:   net.IP{10, 255, 30, 5},
			Mask: net.IPv4Mask(255, 255, 255, 255),
		}
		cfg.Container.HardwareAddress = net.HardwareAddr{0x01, 0x02, 0x03, 0x0A, 0xBC, 0xDE}
		cfg.Container.MTU = 1234
		cfg.Host.DeviceName = "host-device-name"
		cfg.Host.IPAddress = net.IPNet{
			IP:   net.IP{169, 254, 0, 1},
			Mask: net.IPv4Mask(255, 255, 255, 255),
		}
		cfg.Host.HardwareAddress = net.HardwareAddr{0xdd, 0xdd, 0x03, 0x0A, 0xBC, 0xDE}
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
		})
	})
})
