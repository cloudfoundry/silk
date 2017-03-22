package config

import (
	"errors"
	"fmt"
	"net"

	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types/current"
)

//go:generate counterfeiter -o fakes/hardwareAddressGenerator.go --fake-name HardwareAddressGenerator . hardwareAddressGenerator
type hardwareAddressGenerator interface {
	GenerateForContainer(containerIP net.IP) (net.HardwareAddr, error)
	GenerateForHost(containerIP net.IP) (net.HardwareAddr, error)
}

//go:generate counterfeiter -o fakes/deviceNameGenerator.go --fake-name DeviceNameGenerator . deviceNameGenerator
type deviceNameGenerator interface {
	GenerateForHost(containerIP net.IP) (string, error)
	GenerateTemporaryForContainer(containerIP net.IP) (string, error)
}

//go:generate counterfeiter -o fakes/namespaceAdapter.go --fake-name NamespaceAdapter . namespaceAdapter
type namespaceAdapter interface {
	GetNS(string) (ns.NetNS, error)
	GetCurrentNS() (ns.NetNS, error)
}

type ConfigCreator struct {
	HardwareAddressGenerator hardwareAddressGenerator
	DeviceNameGenerator      deviceNameGenerator
	NamespaceAdapter         namespaceAdapter
}

func (c *ConfigCreator) Create(hostNS netNS, addCmdArgs *skel.CmdArgs, ipamResult *current.Result) (*Config, error) {
	var conf Config
	var err error
	if addCmdArgs.IfName == "" {
		return nil, errors.New("IfName cannot be empty")
	}
	if len(addCmdArgs.IfName) > 15 {
		return nil, errors.New("IfName cannot be longer than 15 characters")
	}
	conf.Container.DeviceName = addCmdArgs.IfName
	conf.Container.Namespace, err = c.NamespaceAdapter.GetNS(addCmdArgs.Netns)
	if err != nil {
		return nil, fmt.Errorf("getting container namespace: %s", err)
	}
	if len(ipamResult.IPs) == 0 {
		return nil, errors.New("no IP address in IPAM result")
	}
	conf.Container.IPAddress = ipamResult.IPs[0].Address
	conf.Container.TemporaryDeviceName, err = c.DeviceNameGenerator.GenerateTemporaryForContainer(conf.Container.IPAddress.IP)
	if err != nil {
		return nil, fmt.Errorf("generating temporary container device name: %s", err)
	}
	conf.Container.HardwareAddress, err = c.HardwareAddressGenerator.GenerateForContainer(conf.Container.IPAddress.IP)
	if err != nil {
		return nil, fmt.Errorf("generating container veth hardware address: %s", err)
	}
	conf.Container.MTU = 1500
	conf.Host.DeviceName, err = c.DeviceNameGenerator.GenerateForHost(conf.Container.IPAddress.IP)
	if err != nil {
		return nil, fmt.Errorf("generating host device name: %s", err)
	}
	conf.Host.Namespace = hostNS
	conf.Host.IPAddress = net.IPNet{
		IP:   []byte{169, 254, 0, 1},
		Mask: []byte{255, 255, 255, 255},
	}
	conf.Host.HardwareAddress, err = c.HardwareAddressGenerator.GenerateForHost(conf.Container.IPAddress.IP)
	if err != nil {
		return nil, fmt.Errorf("generating host veth hardware address: %s", err)
	}

	return &conf, nil
}
