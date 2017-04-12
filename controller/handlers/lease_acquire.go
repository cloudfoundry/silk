package handlers

import (
	"io/ioutil"
	"net/http"

	"code.cloudfoundry.org/go-db-helpers/marshal"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
)

//go:generate counterfeiter -o fakes/lease_acquirer.go --fake-name LeaseAcquirer . leaseAcquirer
type leaseAcquirer interface {
	AcquireSubnetLease(underlayIP string) (*controller.Lease, error)
}

type LeasesAcquire struct {
	Logger        lager.Logger
	Marshaler     marshal.Marshaler
	Unmarshaler   marshal.Unmarshaler
	LeaseAcquirer leaseAcquirer
}

func (l *LeasesAcquire) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger := l.Logger.Session("leases-acquire")
	logger.Debug("start", lager.Data{"URL": req.URL, "RemoteAddr": req.RemoteAddr})

	bodyBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Error("read-body", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var payload struct {
		UnderlayIP string `json:"underlay_ip"`
	}
	err = l.Unmarshaler.Unmarshal(bodyBytes, &payload)
	if err != nil {
		logger.Error("unmarshal-request", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	lease, err := l.LeaseAcquirer.AcquireSubnetLease(payload.UnderlayIP)
	if err != nil {
		logger.Error("acquire-subnet-lease", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	bytes, err := l.Marshaler.Marshal(lease)
	if err != nil {
		logger.Error("marshal-response", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(bytes)
}
