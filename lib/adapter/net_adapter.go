package adapter

import "net"

type NetAdapter struct{}

func (*NetAdapter) Interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}
func (*NetAdapter) InterfaceAddrs(i net.Interface) ([]net.Addr, error) {
	return i.Addrs()
}

func (*NetAdapter) InterfaceByName(name string) (*net.Interface, error) {
	return net.InterfaceByName(name)
}
