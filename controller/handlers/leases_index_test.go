package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/go-db-helpers/fakes"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/silk/controller/handlers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LeasesIndex", func() {
	var (
		logger    *lagertest.TestLogger
		handler   *handlers.LeasesIndex
		resp      *httptest.ResponseRecorder
		marshaler *fakes.Marshaler
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		marshaler = &fakes.Marshaler{}
		marshaler.MarshalStub = json.Marshal
		handler = &handlers.LeasesIndex{
			Logger:    logger,
			Marshaler: marshaler,
		}
		resp = httptest.NewRecorder()
	})

	It("returns an empty list of leases", func() {
		expectedResponseJSON := `{ "leases": [ ] }`
		request, err := http.NewRequest("GET", "/leases", nil)
		Expect(err).NotTo(HaveOccurred())

		request.RemoteAddr = "some-host:some-port"

		handler.ServeHTTP(resp, request)
		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(resp.Body).To(MatchJSON(expectedResponseJSON))

		Expect(logger.Logs()).To(HaveLen(1))
		Expect(logger.Logs()[0].LogLevel).To(Equal(lager.DEBUG))
		Expect(logger.Logs()[0].ToJSON()).To(MatchRegexp("RemoteAddr.*some-host:some-port.*URL.*/leases"))
	})

	Context("when the response cannot be marshaled", func() {
		BeforeEach(func() {
			marshaler.MarshalStub = func(interface{}) ([]byte, error) {
				return nil, errors.New("grapes")
			}
		})

		It("calls the internal server error handler", func() {
			request, err := http.NewRequest("GET", "/leases", nil)
			Expect(err).NotTo(HaveOccurred())

			handler.ServeHTTP(resp, request)

			Expect(resp.Code).To(Equal(500))
			Expect(logger.Logs()).To(HaveLen(2))
			Expect(logger.Logs()[1].LogLevel).To(Equal(lager.ERROR))
			Expect(logger.Logs()[1].ToJSON()).To(MatchRegexp("marshal.*grapes"))
		})
	})
})
