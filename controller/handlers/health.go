package handlers

import "net/http"

type Health struct {
	DatabaseChecker databaseChecker
	ErrorResponse   errorResponse
}

//go:generate counterfeiter -o fakes/database_checker.go --fake-name DatabaseChecker . databaseChecker
type databaseChecker interface {
	CheckDatabase() error
}

func (h *Health) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	err := h.DatabaseChecker.CheckDatabase()
	if err != nil {
		h.ErrorResponse.InternalServerError(w, err, "health", "check database failed")
		return
	}
}
