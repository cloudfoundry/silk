package handlers

import (
	"net/http"

	"code.cloudfoundry.org/cf-networking-helpers/marshal"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
)

//go:generate counterfeiter -o fakes/lease_repository.go --fake-name LeaseRepository . leaseRepository
type leaseRepository interface {
	RoutableLeases() ([]controller.Lease, error)
}

type LeasesIndex struct {
	Marshaler       marshal.Marshaler
	LeaseRepository leaseRepository
	ErrorResponse   errorResponse
}

func (l *LeasesIndex) ServeHTTP(logger lager.Logger, w http.ResponseWriter, req *http.Request) {
	logger = logger.Session("leases-index")

	leases, err := l.LeaseRepository.RoutableLeases()
	if err != nil {
		logger.Error("failed-getting-routable-leases", err)
		l.ErrorResponse.InternalServerError(w, err, "all-routable-leases", err.Error())
		return
	}

	response := struct {
		Leases []controller.Lease `json:"leases"`
	}{leases}
	bytes, err := l.Marshaler.Marshal(response)
	if err != nil {
		logger.Error("failed-marshalling-leases", err)
		l.ErrorResponse.InternalServerError(w, err, "marshal-response", err.Error())
		return
	}

	w.Write(bytes)
}
