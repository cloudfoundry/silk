package handlers

import (
	"io/ioutil"
	"net/http"

	"code.cloudfoundry.org/go-db-helpers/marshal"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
)

//go:generate counterfeiter -o fakes/lease_renewer.go --fake-name LeaseRenewer . leaseRenewer
type leaseRenewer interface {
	RenewSubnetLease(lease controller.Lease) error
}

//go:generate counterfeiter -o fakes/error_response.go --fake-name ErrorResponse . errorResponse
type errorResponse interface {
	InternalServerError(http.ResponseWriter, error, string, string)
	BadRequest(http.ResponseWriter, error, string, string)
	Conflict(http.ResponseWriter, error, string, string)
}

type RenewLease struct {
	Logger        lager.Logger
	Unmarshaler   marshal.Unmarshaler
	LeaseRenewer  leaseRenewer
	ErrorResponse errorResponse
}

func (l *RenewLease) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger := l.Logger.Session("leases-renew")
	logger.Debug("start", lager.Data{"URL": req.URL, "RemoteAddr": req.RemoteAddr})
	defer logger.Debug("done")

	bodyBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		l.ErrorResponse.BadRequest(w, err, "read-body", err.Error())
		return
	}

	var lease controller.Lease
	err = l.Unmarshaler.Unmarshal(bodyBytes, &lease)
	if err != nil {
		l.ErrorResponse.BadRequest(w, err, "unmarshal-request", err.Error())
		return
	}

	err = l.LeaseRenewer.RenewSubnetLease(lease)
	if err != nil {
		if _, ok := err.(controller.NonRetriableError); ok {
			l.ErrorResponse.Conflict(w, err, "renew-subnet-lease", err.Error())
			return
		}

		l.ErrorResponse.InternalServerError(w, err, "renew-subnet-lease", err.Error())
		return
	}

	w.Write([]byte("{}"))
}
