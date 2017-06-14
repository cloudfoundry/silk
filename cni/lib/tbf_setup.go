package lib

import (
	"fmt"

	"code.cloudfoundry.org/silk/cni/config"

	"github.com/vishvananda/netlink"
)

type TokenBucketFilter struct {
	NetlinkAdapter netlinkAdapter
}

func (tbf *TokenBucketFilter) Setup(rateInBits, burstInBits int, cfg *config.Config) error {
	// Equivalent to
	// tc qdisc add dev cfg.Host.DeviceName root tbf
	//		rate netConf.BandwidthLimits.Rate
	//		burst netConf.BandwidthLimits.Burst
	if rateInBits <= 0 {
		return fmt.Errorf("invalid rate: %d", rateInBits)
	}
	if burstInBits <= 0 {
		return fmt.Errorf("invalid burst: %d", burstInBits)
	}
	link, err := tbf.NetlinkAdapter.LinkByName(cfg.Host.DeviceName)
	if err != nil {
		return fmt.Errorf("get host device: %s", err)
	}
	rateInBytes := rateInBits / 8
	bufferInBytes := burstInBits * 1000000000 / rateInBits / 8
	limitInBytes := rateInBytes / 10

	qdisc := &netlink.Tbf{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: link.Attrs().Index,
			Handle:    netlink.MakeHandle(1, 0),
			Parent:    netlink.HANDLE_ROOT,
		},
		Limit:  uint32(limitInBytes),
		Rate:   uint64(rateInBytes),
		Buffer: uint32(bufferInBytes),
	}
	err = tbf.NetlinkAdapter.QdiscAdd(qdisc)
	if err != nil {
		return fmt.Errorf("create qdisc: %s", err)
	}
	return nil
}
