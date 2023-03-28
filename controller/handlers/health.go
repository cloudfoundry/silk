package handlers

import (
	"net/http"

	"code.cloudfoundry.org/lager/v3"
)

type Health struct {
	DatabaseChecker databaseChecker
	ErrorResponse   errorResponse
}

//go:generate counterfeiter -o fakes/database_checker.go --fake-name DatabaseChecker . databaseChecker
type databaseChecker interface {
	CheckDatabase() error
}

func (h *Health) ServeHTTP(logger lager.Logger, w http.ResponseWriter, req *http.Request) {
	logger = logger.Session("health")
	err := h.DatabaseChecker.CheckDatabase()
	if err != nil {
		h.ErrorResponse.InternalServerError(logger, w, err, "check database failed")
		return
	}
}
