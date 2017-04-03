package adapter

import (
	"github.com/containernetworking/cni/pkg/ip"
)

type IPAdapter struct{}

func (i *IPAdapter) EnableIP4Forward() error {
	return ip.EnableIP4Forward()
}
