package config

import (
	"fmt"
	"net"
	"path/filepath"

	"github.com/containernetworking/cni/pkg/types"
)

type RangeSet []Range

type Range struct {
	RangeStart net.IP      `json:"rangeStart,omitempty"` // The first ip, inclusive
	RangeEnd   net.IP      `json:"rangeEnd,omitempty"`   // The last ip, inclusive
	Subnet     types.IPNet `json:"subnet"`
	Gateway    net.IP      `json:"gateway,omitempty"`
}

type IPAMConfig struct {
	*Range
	Name       string
	Type       string         `json:"type"`
	Routes     []*types.Route `json:"routes"`
	DataDir    string         `json:"dataDir"`
	ResolvConf string         `json:"resolvConf"`
	Ranges     []RangeSet     `json:"ranges"`
	IPArgs     []net.IP       `json:"-"` // Requested IPs from CNI_ARGS and args
}

type HostLocalIPAM struct {
	CNIVersion string     `json:"cniVersion"`
	Name       string     `json:"name"`
	IPAM       IPAMConfig `json:"ipam"`
}

type IPAMConfigGenerator struct{}

func (IPAMConfigGenerator) GenerateConfig(subnet, network, dataDirPath string) (*HostLocalIPAM, error) {
	subnetAsIPNet, err := types.ParseCIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet: %s", err)
	}

	return &HostLocalIPAM{
		CNIVersion: "1.0.0",
		Name:       network,
		IPAM: IPAMConfig{
			Type: "host-local",
			Ranges: []RangeSet{
				[]Range{{
					Subnet: types.IPNet(*subnetAsIPNet),
				}},
			},
			Routes: []*types.Route{},
			DataDir: filepath.Join(dataDirPath, "ipam"),
		},
	}, nil
}
