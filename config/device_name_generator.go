package config

import (
	"errors"
	"fmt"
	"net"
)

type DeviceNameGenerator struct{}

func (g *DeviceNameGenerator) generate(prefix string, containerIP net.IP) (string, error) {
	i := containerIP.To4()
	if i == nil {
		return "", errors.New("generating device name: expecting valid IPv4 address")
	}
	return fmt.Sprintf("%s-%03d%03d%03d%03d", prefix, i[0], i[1], i[2], i[3]), nil
}

func (g *DeviceNameGenerator) GenerateForHost(containerIP net.IP) (string, error) {
	return g.generate("s", containerIP)
}

func (g *DeviceNameGenerator) GenerateTemporaryForContainer(containerIP net.IP) (string, error) {
	return g.generate("c", containerIP)
}
