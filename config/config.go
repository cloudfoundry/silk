package config

import (
	"net"

	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
)

//go:generate counterfeiter -o fakes/netNS.go --fake-name NetNS . netNS
type netNS interface {
	ns.NetNS
}

type Config struct {
	Container struct {
		DeviceName          string
		TemporaryDeviceName string
		Namespace           netNS
		IPAddress           net.IPNet
		HardwareAddress     net.HardwareAddr
		MTU                 int
	}
	Host struct {
		DeviceName      string
		Namespace       netNS
		IPAddress       net.IPNet
		HardwareAddress net.HardwareAddr
	}
}

func (c *Config) AsCNIResult() *current.Result {
	return &current.Result{
		Interfaces: []*current.Interface{
			&current.Interface{
				Name:    c.Host.DeviceName,
				Mac:     c.Host.HardwareAddress.String(),
				Sandbox: "",
			},
			&current.Interface{
				Name:    c.Container.DeviceName,
				Mac:     c.Container.HardwareAddress.String(),
				Sandbox: c.Container.Namespace.Path(),
			},
		},
		IPs: []*current.IPConfig{
			&current.IPConfig{
				Version:   "4",
				Interface: 1,
				Address:   c.Container.IPAddress,
				Gateway:   c.Host.IPAddress.IP,
			},
		},
		Routes: []*types.Route{
			&types.Route{
				Dst: net.IPNet{
					IP:   net.IPv4zero,
					Mask: net.IPv4Mask(0, 0, 0, 0),
				},
				GW: c.Host.IPAddress.IP,
			},
		},
		DNS: types.DNS{},
	}
}
