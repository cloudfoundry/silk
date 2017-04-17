package handlers

import (
	"io/ioutil"
	"net/http"

	"code.cloudfoundry.org/go-db-helpers/marshal"
	"code.cloudfoundry.org/lager"
)

//go:generate counterfeiter -o fakes/lease_releaser.go --fake-name LeaseReleaser . leaseReleaser
type leaseReleaser interface {
	ReleaseSubnetLease(underlayIP string) error
}

type ReleaseLease struct {
	Logger        lager.Logger
	Marshaler     marshal.Marshaler
	Unmarshaler   marshal.Unmarshaler
	LeaseReleaser leaseReleaser
	ErrorResponse errorResponse
}

func (l *ReleaseLease) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger := l.Logger.Session("leases-release")
	logger.Debug("start", lager.Data{"URL": req.URL, "RemoteAddr": req.RemoteAddr})
	defer logger.Debug("done")

	bodyBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		l.ErrorResponse.BadRequest(w, err, "read-body", err.Error())
		return
	}

	var payload struct {
		UnderlayIP string `json:"underlay_ip"`
	}
	err = l.Unmarshaler.Unmarshal(bodyBytes, &payload)
	if err != nil {
		l.ErrorResponse.BadRequest(w, err, "unmarshal-request", err.Error())
		return
	}

	err = l.LeaseReleaser.ReleaseSubnetLease(payload.UnderlayIP)
	if err != nil {
		l.ErrorResponse.InternalServerError(w, err, "release-subnet-lease", err.Error())
		return
	}

	w.Write([]byte(`{}`))
}
