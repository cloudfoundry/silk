package lib

import (
	"fmt"
	"syscall"

	"code.cloudfoundry.org/silk/cni/config"

	"github.com/vishvananda/netlink"
)

type Bandwidth struct {
	NetlinkAdapter netlinkAdapter
}

const latencyInMillis = 25

func (b *Bandwidth) createTBF(rateInBits, burstInBits, linkIndex int) error {
	// Equivalent to
	// tc qdisc add dev link root tbf
	//		rate netConf.BandwidthLimits.Rate
	//		burst netConf.BandwidthLimits.Burst
	if rateInBits <= 0 {
		return fmt.Errorf("invalid rate: %d", rateInBits)
	}
	if burstInBits <= 0 {
		return fmt.Errorf("invalid burst: %d", burstInBits)
	}
	rateInBytes := rateInBits / 8
	bufferInBytes := b.buffer(uint64(rateInBytes), uint32(burstInBits))
	latency := b.latencyInUsec(latencyInMillis)
	limitInBytes := b.limit(uint64(rateInBytes), latency, uint32(bufferInBytes))

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
	err := b.NetlinkAdapter.QdiscAdd(qdisc)
	if err != nil {
		return fmt.Errorf("create qdisc: %s", err)
	}
	return nil
}

func (b *Bandwidth) InboundSetup(rateInBits, burstInBits int, cfg *config.Config) error {
	hostDevice, err := b.NetlinkAdapter.LinkByName(cfg.Host.DeviceName)
	if err != nil {
		return fmt.Errorf("get host device: %s", err)
	}
	return b.createTBF(rateInBits, burstInBits, hostDevice.Attrs().Index)
}

func (b *Bandwidth) OutboundSetup(rateInBits, burstInBits int, cfg *config.Config) error {
	ifbDevice, err := b.NetlinkAdapter.LinkByName(cfg.IFB.DeviceName)
	if err != nil {
		return fmt.Errorf("get ifb device: %s", err)
	}
	hostDevice, err := b.NetlinkAdapter.LinkByName(cfg.Host.DeviceName)
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

	err = b.NetlinkAdapter.QdiscAdd(ingress)
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
	err = b.NetlinkAdapter.FilterAdd(filter)
	if err != nil {
		return fmt.Errorf("add filter: %s", err)
	}

	// throttle traffic on ifb device
	err = b.createTBF(rateInBits, burstInBits, ifbDevice.Attrs().Index)
	if err != nil {
		return fmt.Errorf("create ifb qdisc: %s", err)
	}
	return nil
}

func (b *Bandwidth) tick2Time(tick uint32) uint32 {
	return uint32(float64(tick) / float64(b.NetlinkAdapter.TickInUsec()))
}

func (b *Bandwidth) time2Tick(time uint32) uint32 {
	return uint32(float64(time) * float64(b.NetlinkAdapter.TickInUsec()))
}

func (b *Bandwidth) buffer(rate uint64, burst uint32) uint32 {
	// do reverse of netlink.burst calculation
	return b.time2Tick(uint32(float64(burst) * float64(netlink.TIME_UNITS_PER_SEC) / float64(rate)))
}

func (b *Bandwidth) limit(rate uint64, latency float64, buffer uint32) uint32 {
	// do reverse of netlink.latency calculation
	return uint32(float64(rate) / float64(netlink.TIME_UNITS_PER_SEC) * (latency + float64(b.tick2Time(buffer))))
}

func (b *Bandwidth) latencyInUsec(latencyInMillis float64) float64 {
	return float64(netlink.TIME_UNITS_PER_SEC) * (latencyInMillis / 1000.0)
}
