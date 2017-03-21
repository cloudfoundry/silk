package config

import (
	"errors"
	"fmt"
	"net"
)

type DeviceNameGenerator struct{}

func (g *DeviceNameGenerator) GenerateForHost(containerIP net.IP) (string, error) {
	i := containerIP.To4()
	if i == nil {
		return "", errors.New("generating device name: expecting valid IPv4 address")
	}
	return fmt.Sprintf("s-%03d%03d%03d%03d", i[0], i[1], i[2], i[3]), nil
}
