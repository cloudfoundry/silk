package handlers_test

import (
	"net/http"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/silk/controller/handlers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogWrap", func() {
	var (
		logger              *lagertest.TestLogger
		loggableHandlerFunc handlers.LoggableHandlerFunc
	)
	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-session")
		logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.DEBUG))
		loggableHandlerFunc = func(logger lager.Logger, w http.ResponseWriter, r *http.Request) {
			logger = logger.Session("logger-group")
			logger.Info("written-in-loggable-handler")
		}
	})
	It("creates \"request\" session and passes it to LoggableHandlerFunc", func() {
		handler := handlers.LogWrap(logger, loggableHandlerFunc)
		req, err := http.NewRequest("GET", "http://example.com", nil)
		Expect(err).NotTo(HaveOccurred())
		handler.ServeHTTP(nil, req)

		Expect(logger.Logs()).To(HaveLen(3))
		Expect(logger.Logs()[0]).To(SatisfyAll(
			LogsWith(lager.DEBUG, "test-session.request.serving"),
			HaveLogData(SatisfyAll(
				HaveKeyWithValue("session", Equal("1")),
				HaveKeyWithValue("method", Equal("GET")),
				HaveKeyWithValue("request", Equal("http://example.com")),
			)),
		))

		Expect(logger.Logs()[1]).To(SatisfyAll(
			LogsWith(lager.INFO, "test-session.request.logger-group.written-in-loggable-handler"),
			HaveLogData(HaveKeyWithValue("session", Equal("1.1"))),
		))

		Expect(logger.Logs()[2]).To(SatisfyAll(
			LogsWith(lager.DEBUG, "test-session.request.done"),
			HaveLogData(SatisfyAll(
				HaveKeyWithValue("session", Equal("1")),
				HaveKeyWithValue("method", Equal("GET")),
				HaveKeyWithValue("request", Equal("http://example.com")),
			)),
		))
	})
})
