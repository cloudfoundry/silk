package netinfo

import (
	"fmt"

	"code.cloudfoundry.org/silk/daemon"
)

//go:generate counterfeiter -o fakes/netinfo.go --fake-name NetInfo . netInfo
type netInfo interface {
	Get() (daemon.NetworkInfo, error)
}

type Discoverer struct {
	NetInfo netInfo
}

func (d *Discoverer) Discover(mtu int) (daemon.NetworkInfo, error) {
	info, err := d.NetInfo.Get()
	if err != nil {
		return daemon.NetworkInfo{}, fmt.Errorf("get netinfo: %s", err)
	}

	if mtu != 0 {
		info.MTU = mtu
	}

	return info, nil
}
