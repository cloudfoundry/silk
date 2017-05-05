package netinfo

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"

	"code.cloudfoundry.org/silk/daemon"
)

const (
	flannelSubnetRegex = `FLANNEL_SUBNET=((?:[0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2})`
	flannelMTURegex    = `FLANNEL_MTU=([0-9]{1,5})`
)

type Flannel struct {
	SubnetFilePath string
}

func (f *Flannel) Get() (daemon.NetworkInfo, error) {
	fileContents, err := ioutil.ReadFile(f.SubnetFilePath)
	if err != nil {
		return daemon.NetworkInfo{}, err
	}

	subnetMatches := regexp.MustCompile(flannelSubnetRegex).FindStringSubmatch(string(fileContents))
	if len(subnetMatches) < 2 {
		return daemon.NetworkInfo{}, fmt.Errorf("unable to parse flannel subnet file")
	}

	mtuMatches := regexp.MustCompile(flannelMTURegex).FindStringSubmatch(string(fileContents))
	if len(mtuMatches) < 2 {
		return daemon.NetworkInfo{}, fmt.Errorf("unable to parse MTU from subnet file")
	}

	mtu, err := strconv.Atoi(mtuMatches[1])
	if err != nil {
		return daemon.NetworkInfo{}, err // untested, should be impossible given regex
	}

	return daemon.NetworkInfo{
		OverlaySubnet: subnetMatches[1],
		MTU:           uint(mtu), // mtu should always be non-negative due to regex match
	}, nil
}
