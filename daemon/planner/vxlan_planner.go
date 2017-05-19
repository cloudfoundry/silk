package planner

import (
	"fmt"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/daemon"
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

//go:generate counterfeiter -o fakes/metricSender.go --fake-name MetricSender . metricSender
type metricSender interface {
	SendValue(name string, value float64, units string)
}

type VXLANPlanner struct {
	Logger           lager.Logger
	ControllerClient controllerClient
	Converger        converger
	Lease            controller.Lease
	ErrorDetector    FatalErrorDetector
	MetricSender     metricSender
}

func (v *VXLANPlanner) DoCycle() error {
	err := v.ControllerClient.RenewSubnetLease(v.Lease)
	if err != nil {
		if v.ErrorDetector.IsFatal(err) {
			return daemon.FatalError(fmt.Sprintf("renew lease: %s", err))
		}
		return fmt.Errorf("renew lease: %s", err)
	}
	v.ErrorDetector.GotSuccess()
	v.Logger.Debug("renew-lease", lager.Data{"lease": v.Lease})

	leases, err := v.ControllerClient.GetRoutableLeases()
	if err != nil {
		return fmt.Errorf("get routable leases: %s", err)
	}

	v.MetricSender.SendValue("numberLeases", float64(len(leases)), "")

	err = v.Converger.Converge(leases)
	if err != nil {
		return fmt.Errorf("converge leases: %s", err)
	}

	v.Logger.Debug("converge-leases", lager.Data{"leases": leases})
	return nil
}
