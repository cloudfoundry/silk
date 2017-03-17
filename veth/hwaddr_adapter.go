package veth

import (
	"net"

	"github.com/containernetworking/cni/pkg/utils/hwaddr"
)

//go:generate counterfeiter -o fakes/hwAddrAdapter.go --fake-name HWAddrAdapter . hwAddrAdapter
type hwAddrAdapter interface {
	GenerateHardwareAddr4(net.IP, []byte) (net.HardwareAddr, error)
}

type HWAddrAdapter struct{}

func (*HWAddrAdapter) GenerateHardwareAddr4(ipAddr net.IP, prefix []byte) (net.HardwareAddr, error) {
	return hwaddr.GenerateHardwareAddr4(ipAddr, prefix)
}
