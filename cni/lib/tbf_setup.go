package lib

import (
	"fmt"
	"syscall"

	"code.cloudfoundry.org/silk/cni/config"

	"github.com/vishvananda/netlink"
)

type TokenBucketFilter struct {
	NetlinkAdapter netlinkAdapter
}

func (tbf *TokenBucketFilter) setup(rateInBits, burstInBits, linkIndex int) error {
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
	rateInBytes := rateInBits / 8
	bufferInBytes := burstInBits * 1000000000 / rateInBits / 8
	limitInBytes := rateInBytes / 10

	qdisc := &netlink.Tbf{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: linkIndex,
			Handle:    netlink.MakeHandle(1, 0),
			Parent:    netlink.HANDLE_ROOT,
		},
		Limit:  uint32(limitInBytes),
		Rate:   uint64(rateInBytes),
		Buffer: uint32(bufferInBytes),
	}
	err := tbf.NetlinkAdapter.QdiscAdd(qdisc)
	if err != nil {
		return fmt.Errorf("create qdisc: %s", err)
	}
	return nil
}

func (tbf *TokenBucketFilter) Setup(rateInBits, burstInBits int, cfg *config.Config) error {
	hostDevice, err := tbf.NetlinkAdapter.LinkByName(cfg.Host.DeviceName)
	if err != nil {
		return fmt.Errorf("get host device: %s", err)
	}
	return tbf.setup(rateInBits, burstInBits, hostDevice.Attrs().Index)
}

func (tbf *TokenBucketFilter) OutboundSetup(rateInBits, burstInBits int, cfg *config.Config) error {
	ifbDevice, err := tbf.NetlinkAdapter.LinkByName(cfg.IFB.DeviceName)
	if err != nil {
		return fmt.Errorf("get ifb device: %s", err)
	}
	hostDevice, err := tbf.NetlinkAdapter.LinkByName(cfg.Host.DeviceName)
	if err != nil {
		return fmt.Errorf("get host device: %s", err)
	}

	// add qdisc ingress on host device
	ingress := &netlink.Ingress{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: hostDevice.Attrs().Index,
			Handle:    netlink.MakeHandle(0xffff, 0), // ffff:
			Parent:    netlink.HANDLE_INGRESS,
		},
	}

	err = tbf.NetlinkAdapter.QdiscAdd(ingress)
	if err != nil {
		return fmt.Errorf("create ingress qdisc: %s", err)
	}

	// add filter on host device to mirror traffic to ifb device
	filter := &netlink.U32{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: hostDevice.Attrs().Index,
			Parent:    ingress.QdiscAttrs.Handle,
			Priority:  1,
			Protocol:  syscall.ETH_P_ALL,
		},
		ClassId:    netlink.MakeHandle(1, 1),
		RedirIndex: ifbDevice.Attrs().Index,
		Actions: []netlink.Action{
			&netlink.MirredAction{
				ActionAttrs:  netlink.ActionAttrs{},
				MirredAction: netlink.TCA_EGRESS_REDIR,
				Ifindex:      ifbDevice.Attrs().Index,
			},
		},
	}
	err = tbf.NetlinkAdapter.FilterAdd(filter)
	if err != nil {
		return fmt.Errorf("add filter: %s", err)
	}

	// throttle traffic on ifb device
	err = tbf.setup(rateInBits, burstInBits, ifbDevice.Attrs().Index)
	if err != nil {
		return fmt.Errorf("create ifb qdisc: %s", err)
	}
	return nil
}
