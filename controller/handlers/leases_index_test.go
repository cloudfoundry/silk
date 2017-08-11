package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	hfakes "code.cloudfoundry.org/cf-networking-helpers/fakes"
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
		logger            *lagertest.TestLogger
		expectedLogger    lager.Logger
		handler           *handlers.LeasesIndex
		leaseRepository   *fakes.LeaseRepository
		resp              *httptest.ResponseRecorder
		marshaler         *hfakes.Marshaler
		fakeErrorResponse *fakes.ErrorResponse
	)

	BeforeEach(func() {
		expectedLogger = lager.NewLogger("test").Session("leases-index")

		testSink := lagertest.NewTestSink()
		expectedLogger.RegisterSink(testSink)
		expectedLogger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.DEBUG))

		logger = lagertest.NewTestLogger("test")
		marshaler = &hfakes.Marshaler{}
		marshaler.MarshalStub = json.Marshal
		leaseRepository = &fakes.LeaseRepository{}
		fakeErrorResponse = &fakes.ErrorResponse{}
		handler = &handlers.LeasesIndex{
			Marshaler:       marshaler,
			LeaseRepository: leaseRepository,
			ErrorResponse:   fakeErrorResponse,
		}
		resp = httptest.NewRecorder()
		leaseRepository.RoutableLeasesReturns([]controller.Lease{
			{
				UnderlayIP:          "10.244.5.9",
				OverlaySubnet:       "10.255.16.0/24",
				OverlayHardwareAddr: "ee:ee:0a:ff:10:00",
			},
			{
				UnderlayIP:          "10.244.22.33",
				OverlaySubnet:       "10.255.75.0/32",
				OverlayHardwareAddr: "ee:ee:0a:ff:4b:00",
			},
		}, nil)
	})

	It("returns the routable leases", func() {
		expectedResponseJSON := `{ "leases": [
		{ "underlay_ip": "10.244.5.9", "overlay_subnet": "10.255.16.0/24", "overlay_hardware_addr": "ee:ee:0a:ff:10:00" },
		  { "underlay_ip": "10.244.22.33", "overlay_subnet": "10.255.75.0/32", "overlay_hardware_addr": "ee:ee:0a:ff:4b:00" }
		] }`
		request, err := http.NewRequest("GET", "/leases", nil)
		Expect(err).NotTo(HaveOccurred())

		request.RemoteAddr = "some-host:some-port"

		handler.ServeHTTP(logger, resp, request)
		Expect(leaseRepository.RoutableLeasesCallCount()).To(Equal(1))
		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(resp.Body).To(MatchJSON(expectedResponseJSON))
	})

	Context("when getting the routable leases fails", func() {
		BeforeEach(func() {
			leaseRepository.RoutableLeasesReturns(nil, errors.New("butter"))
		})

		It("calls the internal server error handler", func() {
			request, err := http.NewRequest("GET", "/leases", nil)
			Expect(err).NotTo(HaveOccurred())

			handler.ServeHTTP(logger, resp, request)

			Expect(fakeErrorResponse.InternalServerErrorCallCount()).To(Equal(1))
			l, w, err, description := fakeErrorResponse.InternalServerErrorArgsForCall(0)
			Expect(l).To(Equal(expectedLogger))
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("butter"))
			Expect(description).To(Equal("all-routable-leases: butter"))
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

			handler.ServeHTTP(logger, resp, request)

			Expect(fakeErrorResponse.InternalServerErrorCallCount()).To(Equal(1))
			l, w, err, description := fakeErrorResponse.InternalServerErrorArgsForCall(0)
			Expect(l).To(Equal(expectedLogger))
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("grapes"))
			Expect(description).To(Equal("marshal-response: grapes"))
		})
	})
})
