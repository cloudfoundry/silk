package lib

import (
	cryptoRand "crypto/rand"
	"fmt"
	"math"
	"math/big"
	mathRand "math/rand"
	"net"
	"strconv"

	"github.com/ziutek/utils/netaddr"
)

type CIDRPool struct {
	ipStart       string
	cidrMask      uint
	cidrMaskBlock uint
	pool          []string
}

func NewCIDRPool(subnetRange, subnetMask string) *CIDRPool {
	ip, ipCIDR, err := net.ParseCIDR(subnetRange)
	if err != nil {
		panic(err)
	}
	cidrMask, _ := ipCIDR.Mask.Size()
	cidrMaskBlock, err := strconv.Atoi(subnetMask)
	if err != nil {
		panic(err)
	}

	mathRand.Seed(getRandomSeed())

	return &CIDRPool{
		ipStart:       ip.String(),
		cidrMask:      uint(cidrMask),
		cidrMaskBlock: uint(cidrMaskBlock),
		pool:          generateCIDRPool(ip.String(), uint(cidrMask), uint(cidrMaskBlock)),
	}
}

func (c *CIDRPool) Size() int {
	return len(c.pool)
}

func (c *CIDRPool) GetRandom() string {
	i := mathRand.Intn(c.Size())
	return c.pool[i]
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

func getRandomSeed() int64 {
	num, err := cryptoRand.Int(cryptoRand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		panic("generating random seed: " + err.Error())
	}
	return num.Int64()
}
