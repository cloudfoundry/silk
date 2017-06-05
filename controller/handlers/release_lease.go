package handlers

import (
	"io/ioutil"
	"net/http"

	"code.cloudfoundry.org/cf-networking-helpers/marshal"
	"code.cloudfoundry.org/lager"
)

//go:generate counterfeiter -o fakes/lease_releaser.go --fake-name LeaseReleaser . leaseReleaser
type leaseReleaser interface {
	ReleaseSubnetLease(underlayIP string) error
}

type ReleaseLease struct {
	Marshaler     marshal.Marshaler
	Unmarshaler   marshal.Unmarshaler
	LeaseReleaser leaseReleaser
	ErrorResponse errorResponse
}

func (l *ReleaseLease) ServeHTTP(logger lager.Logger, w http.ResponseWriter, req *http.Request) {
	logger = logger.Session("leases-release")

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

	err = l.LeaseReleaser.ReleaseSubnetLease(payload.UnderlayIP)
	if err != nil {
		logger.Error("failed-releasing-lease", err)
		l.ErrorResponse.InternalServerError(w, err, "release-subnet-lease", err.Error())
		return
	}

	w.Write([]byte(`{}`))
}
