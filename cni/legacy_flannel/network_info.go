package legacy_flannel

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"
)

const (
	flannelSubnetRegex = `FLANNEL_SUBNET=((?:[0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2})`
	flannelMTURegex    = `FLANNEL_MTU=([0-9]{1,5})`
)

type NetworkInfo struct {
	Subnet string
	MTU    int
}

func DiscoverNetworkInfo(filePath string, mtu int) (NetworkInfo, error) {
	fileContents, err := ioutil.ReadFile(filePath)
	if err != nil {
		return NetworkInfo{}, err
	}

	subnetMatches := regexp.MustCompile(flannelSubnetRegex).FindStringSubmatch(string(fileContents))
	if len(subnetMatches) < 2 {
		return NetworkInfo{}, fmt.Errorf("unable to parse flannel subnet file")
	}

	if mtu == 0 {
		mtuMatches := regexp.MustCompile(flannelMTURegex).FindStringSubmatch(string(fileContents))
		if len(mtuMatches) < 2 {
			return NetworkInfo{}, fmt.Errorf("unable to parse MTU from subnet file")
		}

		mtu, err = strconv.Atoi(mtuMatches[1])
		if err != nil {
			return NetworkInfo{}, err // untested, should be impossible given regex
		}
	}

	return NetworkInfo{
		Subnet: subnetMatches[1],
		MTU:    mtu,
	}, nil
}
