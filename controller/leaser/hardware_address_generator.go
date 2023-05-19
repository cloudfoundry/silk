package leaser

import (
	"net"

	"code.cloudfoundry.org/silk/lib/hwaddr"
)

type HardwareAddressGenerator struct{}

func (g *HardwareAddressGenerator) GenerateForVTEP(containerIP net.IP) (net.HardwareAddr, error) {
	return hwaddr.GenerateHardwareAddr4(containerIP, []byte{0xee, 0xee})
}
