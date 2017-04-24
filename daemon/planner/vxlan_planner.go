package planner

import (
	"fmt"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
)

//go:generate counterfeiter -o fakes/controller_client.go --fake-name ControllerClient . controllerClient
type controllerClient interface {
	GetRoutableLeases() ([]controller.Lease, error)
	RenewSubnetLease(controller.Lease) error
}

//go:generate counterfeiter -o fakes/converger.go --fake-name Converger . converger
type converger interface {
	Converge([]controller.Lease) error
}

type VXLANPlanner struct {
	Logger           lager.Logger
	ControllerClient controllerClient
	Converger        converger
	Lease            controller.Lease
}

func (v *VXLANPlanner) DoCycle() error {
	err := v.ControllerClient.RenewSubnetLease(v.Lease)
	if err != nil {
		if _, ok := err.(controller.NonRetriableError); ok {
			return controller.NonRetriableError(fmt.Sprintf("non-retriable renew lease: %s", err))
		}
		return fmt.Errorf("renew lease: %s", err)
	}
	v.Logger.Debug("renew-lease", lager.Data{"lease": v.Lease})

	leases, err := v.ControllerClient.GetRoutableLeases()
	if err != nil {
		return fmt.Errorf("get routable leases: %s", err)
	}

	err = v.Converger.Converge(leases)
	if err != nil {
		return fmt.Errorf("converge leases: %s", err)
	}

	v.Logger.Debug("get-routable-leases", lager.Data{"leases": leases})
	return nil
}
