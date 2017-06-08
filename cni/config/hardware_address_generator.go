package config

import (
	"net"

	"github.com/containernetworking/plugins/pkg/utils/hwaddr"
)

type HardwareAddressGenerator struct{}

func (g *HardwareAddressGenerator) GenerateForContainer(containerIP net.IP) (net.HardwareAddr, error) {
	return hwaddr.GenerateHardwareAddr4(containerIP, []byte{0xee, 0xee})
}

func (g *HardwareAddressGenerator) GenerateForHost(containerIP net.IP) (net.HardwareAddr, error) {
	return hwaddr.GenerateHardwareAddr4(containerIP, []byte{0xaa, 0xaa})
}
