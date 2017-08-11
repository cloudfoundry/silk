package handlers

import (
	"net/http"

	"code.cloudfoundry.org/lager"
)

type LoggableHandlerFunc func(logger lager.Logger, w http.ResponseWriter, r *http.Request)

func LogWrap(logger lager.Logger, loggableHandlerFunc LoggableHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestLogger := logger.Session("request", lager.Data{
			"method":  r.Method,
			"request": r.URL.String(),
		})
		requestLogger.Debug("serving")
		defer requestLogger.Debug("done")

		loggableHandlerFunc(requestLogger, w, r)
	}
}
