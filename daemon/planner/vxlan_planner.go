package planner

import (
	"fmt"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
)

//go:generate counterfeiter -o fakes/controller_client.go --fake-name ControllerClient . controllerClient
type controllerClient interface {
	GetRoutableLeases() ([]controller.Lease, error)
}

type VXLANPlanner struct {
	Logger           lager.Logger
	ControllerClient controllerClient
}

func (v *VXLANPlanner) DoCycle() error {
	leases, err := v.ControllerClient.GetRoutableLeases()
	if err != nil {
		return fmt.Errorf("get routable leases: %s", err)
	}

	v.Logger.Debug("get-routable-leases", lager.Data{"leases": leases})
	return nil
}
