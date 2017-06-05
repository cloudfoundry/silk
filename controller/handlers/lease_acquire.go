package handlers

import (
	"errors"
	"io/ioutil"
	"net/http"

	"code.cloudfoundry.org/cf-networking-helpers/marshal"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
)

//go:generate counterfeiter -o fakes/lease_acquirer.go --fake-name LeaseAcquirer . leaseAcquirer
type leaseAcquirer interface {
	AcquireSubnetLease(underlayIP string) (*controller.Lease, error)
}

type LeasesAcquire struct {
	Marshaler     marshal.Marshaler
	Unmarshaler   marshal.Unmarshaler
	LeaseAcquirer leaseAcquirer
	ErrorResponse errorResponse
}

func (l *LeasesAcquire) ServeHTTP(logger lager.Logger, w http.ResponseWriter, req *http.Request) {
	logger = logger.Session("leases-acquire")

	bodyBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Error("failed-reading-request-body", err)
		l.ErrorResponse.BadRequest(w, err, "read-body", err.Error())
		return
	}

	var payload struct {
		UnderlayIP string `json:"underlay_ip"`
	}
	err = l.Unmarshaler.Unmarshal(bodyBytes, &payload)
	if err != nil {
		logger.Error("failed-unmarshalling-payload", err)
		l.ErrorResponse.BadRequest(w, err, "unmarshal-request", err.Error())
		return
	}

	lease, err := l.LeaseAcquirer.AcquireSubnetLease(payload.UnderlayIP)
	if err != nil {
		logger.Error("failed-acquiring-lease", err)
		l.ErrorResponse.InternalServerError(w, err, "acquire-subnet-lease", err.Error())
		return
	}
	if lease == nil {
		err := errors.New("No lease available")
		logger.Error("failed-finding-available-lease", err)
		l.ErrorResponse.Conflict(w, err, "acquire-subnet-lease", err.Error())
		return
	}

	bytes, err := l.Marshaler.Marshal(lease)
	if err != nil {
		logger.Error("failed-marshalling-lease", err)
		l.ErrorResponse.InternalServerError(w, err, "marshal-response", err.Error())
		return
	}

	w.Write(bytes)
}
