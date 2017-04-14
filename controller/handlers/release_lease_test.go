package handlers_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	hfakes "code.cloudfoundry.org/go-db-helpers/fakes"
	"code.cloudfoundry.org/go-db-helpers/testsupport"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/handlers"
	"code.cloudfoundry.org/silk/controller/handlers/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ReleaseLease", func() {
	var (
		logger            *lagertest.TestLogger
		handler           *handlers.ReleaseLease
		resp              *httptest.ResponseRecorder
		marshaler         *hfakes.Marshaler
		unmarshaler       *hfakes.Unmarshaler
		leaseReleaser     *fakes.LeaseReleaser
		fakeErrorResponse *fakes.ErrorResponse

		expectedLease controller.Lease
		request       *http.Request
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		marshaler = &hfakes.Marshaler{}
		marshaler.MarshalStub = json.Marshal
		unmarshaler = &hfakes.Unmarshaler{}
		unmarshaler.UnmarshalStub = json.Unmarshal
		leaseReleaser = &fakes.LeaseReleaser{}
		fakeErrorResponse = &fakes.ErrorResponse{}

		handler = &handlers.ReleaseLease{
			Logger:        logger,
			Marshaler:     marshaler,
			Unmarshaler:   unmarshaler,
			LeaseReleaser: leaseReleaser,
			ErrorResponse: fakeErrorResponse,
		}
		resp = httptest.NewRecorder()

		expectedLease = controller.Lease{
			UnderlayIP:          "10.244.16.11",
			OverlaySubnet:       "10.255.17.0/24",
			OverlayHardwareAddr: "ee:ee:0a:ff:11:00",
		}
		requestBody := bytes.NewBuffer([]byte(`{ "underlay_ip": "10.244.16.11", "overlay_subnet": "10.255.17.0/24", "overlay_hardware_addr": "ee:ee:0a:ff:11:00" }`))
		var err error
		request, err = http.NewRequest("PUT", "/leases/release", requestBody)
		Expect(err).NotTo(HaveOccurred())
		request.RemoteAddr = "some-host:some-port"
	})

	AfterEach(func() {
		By("checking that the last log line is for 'done'")
		last := len(logger.Logs()) - 1
		Expect(logger.Logs()[last].LogLevel).To(Equal(lager.DEBUG))
		Expect(logger.Logs()[last].ToJSON()).To(MatchRegexp("leases-release.*done"))
	})

	It("releases a lease for subnet", func() {
		handler.ServeHTTP(resp, request)
		Expect(leaseReleaser.ReleaseSubnetLeaseCallCount()).To(Equal(1))
		Expect(leaseReleaser.ReleaseSubnetLeaseArgsForCall(0)).To(Equal(expectedLease))

		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(resp.Body.String()).To(Equal("{}"))

		Expect(logger.Logs()).To(HaveLen(2))
		Expect(logger.Logs()[0].LogLevel).To(Equal(lager.DEBUG))
		Expect(logger.Logs()[0].ToJSON()).To(MatchRegexp("leases-release.*RemoteAddr.*some-host:some-port.*URL.*/leases/release"))
	})

	Context("when there are errors reading the body bytes", func() {
		BeforeEach(func() {
			request.Body = ioutil.NopCloser(&testsupport.BadReader{})
		})

		It("logs the error and returns a 400", func() {
			handler.ServeHTTP(resp, request)

			Expect(fakeErrorResponse.BadRequestCallCount()).To(Equal(1))
			w, err, message, description := fakeErrorResponse.BadRequestArgsForCall(0)
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("banana"))
			Expect(message).To(Equal("read-body"))
			Expect(description).To(Equal("banana"))
		})
	})

	Context("when the request cannot be unmarshaled", func() {
		BeforeEach(func() {
			unmarshaler.UnmarshalReturns(errors.New("fig"))
		})

		It("returns a BadRequest error", func() {
			handler.ServeHTTP(resp, request)

			Expect(fakeErrorResponse.BadRequestCallCount()).To(Equal(1))
			w, err, message, description := fakeErrorResponse.BadRequestArgsForCall(0)
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("fig"))
			Expect(message).To(Equal("unmarshal-request"))
			Expect(description).To(Equal("fig"))
		})
	})

	Context("when the request is missing required fields", func() {
		Context("when missing the underlay ip", func() {
			BeforeEach(func() {
				requestBody := bytes.NewBuffer([]byte(`{ "overlay_subnet": "10.10.10.0/24", "overlay_hardware_addr": "ee:ee:0a:ff:11:00" }`))
				request, _ = http.NewRequest("PUT", "/leases/release", requestBody)
			})
			It("returns a BadRequest error with a useful message", func() {
				handler.ServeHTTP(resp, request)

				Expect(fakeErrorResponse.BadRequestCallCount()).To(Equal(1))
				w, err, message, description := fakeErrorResponse.BadRequestArgsForCall(0)
				Expect(w).To(Equal(resp))
				Expect(err).To(MatchError("missing required field underlay_ip"))
				Expect(message).To(Equal("validate-request"))
				Expect(description).To(Equal("missing required field underlay_ip"))
			})
		})
		Context("when missing the overlay subnet", func() {
			BeforeEach(func() {
				requestBody := bytes.NewBuffer([]byte(`{ "underlay_ip": "10.244.16.11", "overlay_hardware_addr": "ee:ee:0a:ff:11:00" }`))
				request, _ = http.NewRequest("PUT", "/leases/release", requestBody)
			})
			It("returns a BadRequest error with a useful message", func() {
				handler.ServeHTTP(resp, request)

				Expect(fakeErrorResponse.BadRequestCallCount()).To(Equal(1))
				w, err, message, description := fakeErrorResponse.BadRequestArgsForCall(0)
				Expect(w).To(Equal(resp))
				Expect(err).To(MatchError("missing required field overlay_subnet"))
				Expect(message).To(Equal("validate-request"))
				Expect(description).To(Equal("missing required field overlay_subnet"))
			})
		})
	})

	Context("when releasing a lease fails", func() {
		BeforeEach(func() {
			leaseReleaser.ReleaseSubnetLeaseReturns(errors.New("kiwi"))
		})

		It("calls the Error Response InternalServerError() handler", func() {
			handler.ServeHTTP(resp, request)

			Expect(fakeErrorResponse.InternalServerErrorCallCount()).To(Equal(1))
			w, err, message, description := fakeErrorResponse.InternalServerErrorArgsForCall(0)
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("kiwi"))
			Expect(message).To(Equal("release-subnet-lease"))
			Expect(description).To(Equal("kiwi"))
		})
	})
})
