package lib

import (
	"fmt"
	"net"

	"github.com/ziutek/utils/netaddr"
)

type CIDRPool struct {
	ipStart       string
	cidrMask      uint
	cidrMaskBlock uint
	pool          []string
}

func NewCIDRPool(ipStart string, cidrMask, cidrMaskBlock uint) CIDRPool {
	return CIDRPool{
		ipStart:       ipStart,
		cidrMask:      cidrMask,
		cidrMaskBlock: cidrMaskBlock,
		pool:          generateCIDRPool(ipStart, cidrMask, cidrMaskBlock),
	}
}

func (c *CIDRPool) Get(index int) (string, error) {
	if index < 0 {
		return "", fmt.Errorf("invalid index: %d", index)
	}
	if len(c.pool) <= index {
		return "", fmt.Errorf("cannot get cidr of index %d when pool size is size of %d", index, len(c.pool))
	}

	return c.pool[index], nil
}

func generateCIDRPool(ipStart string, cidrMask, cidrMaskBlock uint) []string {
	pool := []string{}
	fullRange := 1 << (32 - cidrMask)
	blockSize := 1 << (32 - cidrMaskBlock)
	var newIP net.IP
	for i := 2 * blockSize; i < fullRange-blockSize; i += blockSize {
		newIP = netaddr.IPAdd(net.ParseIP(ipStart), i)
		pool = append(pool, fmt.Sprintf("%s/%d", newIP.String(), cidrMaskBlock))
	}
	return pool
}
