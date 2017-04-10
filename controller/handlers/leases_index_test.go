package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	hfakes "code.cloudfoundry.org/go-db-helpers/fakes"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/handlers"
	"code.cloudfoundry.org/silk/controller/handlers/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LeasesIndex", func() {
	var (
		logger          *lagertest.TestLogger
		handler         *handlers.LeasesIndex
		leaseRepository *fakes.LeaseRepository
		resp            *httptest.ResponseRecorder
		marshaler       *hfakes.Marshaler
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		marshaler = &hfakes.Marshaler{}
		marshaler.MarshalStub = json.Marshal
		leaseRepository = &fakes.LeaseRepository{}
		handler = &handlers.LeasesIndex{
			Logger:          logger,
			Marshaler:       marshaler,
			LeaseRepository: leaseRepository,
		}
		resp = httptest.NewRecorder()
		leaseRepository.RoutableLeasesReturns([]controller.Lease{
			{
				UnderlayIP:    "10.244.5.9",
				OverlaySubnet: "10.255.16.0/24",
			},
			{
				UnderlayIP:    "10.244.22.33",
				OverlaySubnet: "10.255.75.0/32",
			},
		}, nil)
	})

	It("returns the routable leases", func() {
		expectedResponseJSON := `{ "leases": [
		  { "underlay_ip": "10.244.5.9", "overlay_subnet": "10.255.16.0/24" },
		  { "underlay_ip": "10.244.22.33", "overlay_subnet": "10.255.75.0/32" }
		] }`
		request, err := http.NewRequest("GET", "/leases", nil)
		Expect(err).NotTo(HaveOccurred())

		request.RemoteAddr = "some-host:some-port"

		handler.ServeHTTP(resp, request)
		Expect(leaseRepository.RoutableLeasesCallCount()).To(Equal(1))
		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(resp.Body).To(MatchJSON(expectedResponseJSON))

		Expect(logger.Logs()).To(HaveLen(1))
		Expect(logger.Logs()[0].LogLevel).To(Equal(lager.DEBUG))
		Expect(logger.Logs()[0].ToJSON()).To(MatchRegexp("RemoteAddr.*some-host:some-port.*URL.*/leases"))
	})

	Context("when getting the routable leases fails", func() {
		BeforeEach(func() {
			leaseRepository.RoutableLeasesReturns(nil, errors.New("butter"))
		})

		It("calls the internal server error handler", func() {
			request, err := http.NewRequest("GET", "/leases", nil)
			Expect(err).NotTo(HaveOccurred())

			handler.ServeHTTP(resp, request)

			Expect(resp.Code).To(Equal(500))
			Expect(logger.Logs()).To(HaveLen(2))
			Expect(logger.Logs()[1].LogLevel).To(Equal(lager.ERROR))
			Expect(logger.Logs()[1].ToJSON()).To(MatchRegexp("all-routable-leases.*butter"))
		})
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
