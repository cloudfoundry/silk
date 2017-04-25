package leaser

import (
	cryptoRand "crypto/rand"
	"fmt"
	"math"
	"math/big"
	mathRand "math/rand"
	"net"

	"github.com/ziutek/utils/netaddr"
)

type CIDRPool struct {
	ipStart       string
	cidrMask      uint
	cidrMaskBlock uint
}

func NewCIDRPool(subnetRange string, subnetMask int) *CIDRPool {
	ip, ipCIDR, err := net.ParseCIDR(subnetRange)
	if err != nil {
		panic(err)
	}
	cidrMask, _ := ipCIDR.Mask.Size()

	mathRand.Seed(getRandomSeed())

	return &CIDRPool{
		ipStart:       ip.String(),
		cidrMask:      uint(cidrMask),
		cidrMaskBlock: uint(subnetMask),
	}
}

func (c *CIDRPool) Size() int {
	return len(c.generateCIDRPool())
}

func (c *CIDRPool) GetAvailable(taken []string) string {
	available := c.generateCIDRPool()
	for _, subnet := range taken {
		delete(available, subnet)
	}
	if len(available) == 0 {
		return ""
	}
	i := mathRand.Intn(len(available))
	n := 0
	for subnet, _ := range available {
		if i == n {
			return subnet
		}
		n++
	}
	return ""
}

func (c *CIDRPool) generateCIDRPool() map[string]struct{} {
	pool := make(map[string]struct{})
	fullRange := 1 << (32 - c.cidrMask)
	blockSize := 1 << (32 - c.cidrMaskBlock)
	var newIP net.IP
	for i := blockSize; i < fullRange; i += blockSize {
		newIP = netaddr.IPAdd(net.ParseIP(c.ipStart), i)
		subnet := fmt.Sprintf("%s/%d", newIP.String(), c.cidrMaskBlock)
		pool[subnet] = struct{}{}
	}
	return pool
}

func getRandomSeed() int64 {
	num, err := cryptoRand.Int(cryptoRand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		panic("generating random seed: " + err.Error())
	}
	return num.Int64()
}
