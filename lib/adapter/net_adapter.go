package adapter

import "net"

type NetAdapter struct{}

func (*NetAdapter) Interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}
func (*NetAdapter) InterfaceAddrs(i net.Interface) ([]net.Addr, error) {
	return i.Addrs()
}
