package handlers

import (
	"errors"
	"fmt"
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
		l.ErrorResponse.BadRequest(logger, w, err, fmt.Sprintf("read-body: %s", err.Error()))
		return
	}

	var payload struct {
		UnderlayIP string `json:"underlay_ip"`
	}
	err = l.Unmarshaler.Unmarshal(bodyBytes, &payload)
	if err != nil {
		l.ErrorResponse.BadRequest(logger, w, err, fmt.Sprintf("unmarshal-request: %s", err.Error()))
		return
	}

	lease, err := l.LeaseAcquirer.AcquireSubnetLease(payload.UnderlayIP)
	if err != nil {
		l.ErrorResponse.InternalServerError(logger, w, err, err.Error())
		return
	}
	if lease == nil {
		err := errors.New("No lease available")
		l.ErrorResponse.Conflict(logger, w, err, err.Error())
		return
	}

	bytes, err := l.Marshaler.Marshal(lease)
	if err != nil {
		l.ErrorResponse.InternalServerError(logger, w, err, fmt.Sprintf("marshal-response: %s", err.Error()))
		return
	}

	w.Write(bytes)
}
