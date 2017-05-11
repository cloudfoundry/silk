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
	pool map[string]struct{}
}

func NewCIDRPool(subnetRange string, subnetMask int) *CIDRPool {
	ip, ipCIDR, err := net.ParseCIDR(subnetRange)
	if err != nil {
		panic(err)
	}
	cidrMask, _ := ipCIDR.Mask.Size()

	mathRand.Seed(getRandomSeed())

	return &CIDRPool{
		pool: generatePool(ip.String(), uint(cidrMask), uint(subnetMask)),
	}
}

func (c *CIDRPool) Size() int {
	return len(c.pool)
}

func (c *CIDRPool) GetAvailable(taken []string) string {
	available := make(map[string]struct{})
	for k, v := range c.pool {
		available[k] = v
	}
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

func (c *CIDRPool) IsMember(subnet string) bool {
	_, ok := c.pool[subnet]
	return ok
}

func generatePool(ipStart string, cidrMask, cidrMaskBlock uint) map[string]struct{} {
	pool := make(map[string]struct{})
	fullRange := 1 << (32 - cidrMask)
	blockSize := 1 << (32 - cidrMaskBlock)
	var newIP net.IP
	for i := blockSize; i < fullRange; i += blockSize {
		newIP = netaddr.IPAdd(net.ParseIP(ipStart), i)
		subnet := fmt.Sprintf("%s/%d", newIP.String(), cidrMaskBlock)
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
