package handlers

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"code.cloudfoundry.org/go-db-helpers/marshal"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
)

//go:generate counterfeiter -o fakes/lease_releaser.go --fake-name LeaseReleaser . leaseReleaser
type leaseReleaser interface {
	ReleaseSubnetLease(lease controller.Lease) error
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

	var lease controller.Lease
	err = l.Unmarshaler.Unmarshal(bodyBytes, &lease)
	if err != nil {
		l.ErrorResponse.BadRequest(w, err, "unmarshal-request", err.Error())
		return
	}

	if lease.UnderlayIP == "" {
		err := fmt.Errorf("missing required field underlay_ip")
		l.ErrorResponse.BadRequest(w, err, "validate-request", err.Error())
		return
	}

	if lease.OverlaySubnet == "" {
		err := fmt.Errorf("missing required field overlay_subnet")
		l.ErrorResponse.BadRequest(w, err, "validate-request", err.Error())
		return
	}

	err = l.LeaseReleaser.ReleaseSubnetLease(lease)
	if err != nil {
		l.ErrorResponse.InternalServerError(w, err, "release-subnet-lease", err.Error())
		return
	}

	w.Write([]byte("{}"))
}
