package handlers_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	"code.cloudfoundry.org/silk/controller/handlers"
	"code.cloudfoundry.org/silk/controller/handlers/fakes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Health handler", func() {
	var (
		expectedLogger      lager.Logger
		logger              *lagertest.TestLogger
		handler             *handlers.Health
		request             *http.Request
		fakeDatabaseChecker *fakes.DatabaseChecker
		fakeErrorResponse   *fakes.ErrorResponse
		resp                *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		expectedLogger = lager.NewLogger("test").Session("health")

		testSink := lagertest.NewTestSink()
		expectedLogger.RegisterSink(testSink)
		expectedLogger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.DEBUG))
		logger = lagertest.NewTestLogger("test")

		var err error
		request, err = http.NewRequest("GET", "/health", nil)
		Expect(err).NotTo(HaveOccurred())

		fakeDatabaseChecker = &fakes.DatabaseChecker{}
		fakeErrorResponse = &fakes.ErrorResponse{}

		handler = &handlers.Health{
			DatabaseChecker: fakeDatabaseChecker,
			ErrorResponse:   fakeErrorResponse,
		}
		resp = httptest.NewRecorder()
	})

	It("checks the database is up and returns a 200", func() {
		handler.ServeHTTP(logger, resp, request)
		Expect(fakeDatabaseChecker.CheckDatabaseCallCount()).To(Equal(1))
		Expect(resp.Code).To(Equal(http.StatusOK))
	})

	Context("when the database returns an error", func() {
		BeforeEach(func() {
			fakeDatabaseChecker.CheckDatabaseReturns(errors.New("pineapple"))
		})

		It("calls the internal server error handler", func() {
			handler.ServeHTTP(logger, resp, request)
			Expect(fakeDatabaseChecker.CheckDatabaseCallCount()).To(Equal(1))
			Expect(fakeErrorResponse.InternalServerErrorCallCount()).To(Equal(1))

			l, w, err, description := fakeErrorResponse.InternalServerErrorArgsForCall(0)
			Expect(l).To(Equal(expectedLogger))
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("pineapple"))
			Expect(description).To(Equal("check database failed"))
		})
	})
})
