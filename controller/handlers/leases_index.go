package handlers

import (
	"net/http"

	"code.cloudfoundry.org/go-db-helpers/marshal"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
)

type LeasesIndex struct {
	Logger    lager.Logger
	Marshaler marshal.Marshaler
}

func (l *LeasesIndex) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger := l.Logger.Session("leases-index")
	logger.Debug("start", lager.Data{"URL": req.URL, "RemoteAddr": req.RemoteAddr})

	leases := []controller.Lease{}

	response := struct {
		Leases []controller.Lease `json:"leases"`
	}{leases}
	bytes, err := l.Marshaler.Marshal(response)
	if err != nil {
		logger.Error("marshal-response", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(bytes)
}
