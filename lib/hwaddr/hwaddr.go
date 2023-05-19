package hwaddr

import (
	"fmt"
	"net"
)

func GenerateHardwareAddr4(ip net.IP, prefix []byte) (net.HardwareAddr, error) {
	switch {

	case ip.To4() == nil:
		return nil, fmt.Errorf("%s is not an IPv4 address", ip)

	case len(prefix) != 2:
		return nil, fmt.Errorf("Prefix length should be 2 bytes, but received %d bytes", len(prefix))
	}

	ipByteLen := len(ip)
	return (net.HardwareAddr)(
		append(
			prefix,
			// IPs are encapsulated as 16-byte addrs even when IPv4
			// but we only care about the last 4 bytes for IPv4
			ip[ipByteLen-4:ipByteLen]...),
	), nil
}
