package lib

import (
	"github.com/containernetworking/cni/pkg/utils/sysctl"
)

//go:generate counterfeiter -o fakes/sysctlAdapter.go --fake-name SysctlAdapter . sysctlAdapter
type sysctlAdapter interface {
	Sysctl(name string, params ...string) (string, error)
}

type SysctlAdapter struct{}

func (*SysctlAdapter) Sysctl(name string, params ...string) (string, error) {
	return sysctl.Sysctl(name, params...)
}
