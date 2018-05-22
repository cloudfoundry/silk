package server_metrics

import (
	"code.cloudfoundry.org/cf-networking-helpers/metrics"
	"code.cloudfoundry.org/silk/controller"
)

//go:generate counterfeiter -o fakes/databaseHandler.go --fake-name DatabaseHandler . databaseHandler
type databaseHandler interface {
	All() ([]controller.Lease, error)
	AllActive(int) ([]controller.Lease, error)
}

//go:generate counterfeiter -o fakes/cidrPool.go --fake-name CIDRPool . cidrPool
type cidrPool interface {
	BlockPoolSize() int
}

func NewTotalLeasesSource(lister databaseHandler) metrics.MetricSource {
	return metrics.MetricSource{
		Name: "totalLeases",
		Unit: "",
		Getter: func() (float64, error) {
			allLeases, err := lister.All()
			return float64(len(allLeases)), err
		},
	}
}

func NewFreeLeasesSource(lister databaseHandler, pool cidrPool) metrics.MetricSource {
	return metrics.MetricSource{
		Name: "freeLeases",
		Unit: "",
		Getter: func() (float64, error) {
			allLeases, err := lister.All()
			size := pool.BlockPoolSize()
			return float64(size - len(allLeases)), err
		},
	}
}

func NewStaleLeasesSource(lister databaseHandler, seconds int) metrics.MetricSource {
	return metrics.MetricSource{
		Name: "staleLeases",
		Unit: "",
		Getter: func() (float64, error) {
			allLeases, err := lister.All()
			allActiveLeases, err := lister.AllActive(seconds)

			return float64(len(allLeases) - len(allActiveLeases)), err
		},
	}
}
