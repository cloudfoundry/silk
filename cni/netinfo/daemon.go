package netinfo

import (
	"fmt"

	"code.cloudfoundry.org/cf-networking-helpers/json_client"
	"code.cloudfoundry.org/silk/daemon"
)

type Daemon struct {
	JSONClient json_client.JsonClient
}

func (d *Daemon) Get() (daemon.NetworkInfo, error) {
	info := &daemon.NetworkInfo{}
	err := d.JSONClient.Do("GET", "/", nil, info, "")
	if err != nil {
		return daemon.NetworkInfo{}, fmt.Errorf("json client do: %s", err)
	}

	return *info, nil
}
