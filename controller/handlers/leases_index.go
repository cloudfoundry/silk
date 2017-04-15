package handlers

import (
	"net/http"

	"code.cloudfoundry.org/go-db-helpers/marshal"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
)

//go:generate counterfeiter -o fakes/lease_repository.go --fake-name LeaseRepository . leaseRepository
type leaseRepository interface {
	RoutableLeases() ([]controller.Lease, error)
}

type LeasesIndex struct {
	Logger          lager.Logger
	Marshaler       marshal.Marshaler
	LeaseRepository leaseRepository
	ErrorResponse   errorResponse
}

func (l *LeasesIndex) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger := l.Logger.Session("leases-index")
	logger.Debug("start", lager.Data{"URL": req.URL, "RemoteAddr": req.RemoteAddr})

	leases, err := l.LeaseRepository.RoutableLeases()
	if err != nil {
		l.ErrorResponse.InternalServerError(w, err, "all-routable-leases", err.Error())
		return
	}

	response := struct {
		Leases []controller.Lease `json:"leases"`
	}{leases}
	bytes, err := l.Marshaler.Marshal(response)
	if err != nil {
		l.ErrorResponse.InternalServerError(w, err, "marshal-response", err.Error())
		return
	}

	w.Write(bytes)
}
