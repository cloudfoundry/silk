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
	blockPool  map[string]struct{}
	singlePool map[string]struct{}
}

func NewCIDRPool(subnetRange string, subnetMask int) *CIDRPool {
	ip, ipCIDR, err := net.ParseCIDR(subnetRange)
	if err != nil {
		panic(err)
	}
	cidrMask, _ := ipCIDR.Mask.Size()

	mathRand.Seed(getRandomSeed())

	return &CIDRPool{
		blockPool:  generateBlockPool(ip, uint(cidrMask), uint(subnetMask)),
		singlePool: generateSingleIPPool(ip),
	}
}

func (c *CIDRPool) BlockPoolSize() int {
	return len(c.blockPool)
}

func (c *CIDRPool) GetAvailableBlock(taken []string) string {
	return getAvailable(taken, c.blockPool)
}

func (c *CIDRPool) GetAvailableSingleIP(taken []string) string {
	return getAvailable(taken, c.singlePool)
}

func (c *CIDRPool) IsMember(subnet string) bool {
	_, blockOk := c.blockPool[subnet]
	_, singleOk := c.singlePool[subnet]
	return blockOk || singleOk
}

func getAvailable(taken []string, pool map[string]struct{}) string {
	available := make(map[string]struct{})
	for k, v := range pool {
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
	for subnet := range available {
		if i == n {
			return subnet
		}
		n++
	}
	return ""
}

func generateBlockPool(ipStart net.IP, cidrMask, cidrMaskBlock uint) map[string]struct{} {
	pool := make(map[string]struct{})
	fullRange := 1 << (32 - cidrMask)
	blockSize := 1 << (32 - cidrMaskBlock)
	for i := blockSize; i < fullRange; i += blockSize {
		subnet := fmt.Sprintf("%s/%d", netaddr.IPAdd(ipStart, i), cidrMaskBlock)
		pool[subnet] = struct{}{}
	}
	return pool
}

func generateSingleIPPool(ipStart net.IP) map[string]struct{} {
	pool := make(map[string]struct{})
	for i := 1; i <= 255; i++ {
		singleCIDR := fmt.Sprintf("%s/32", netaddr.IPAdd(ipStart, i))
		pool[singleCIDR] = struct{}{}
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
