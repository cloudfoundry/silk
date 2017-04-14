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

var _ = Describe("RenewLease", func() {
	var (
		logger            *lagertest.TestLogger
		handler           *handlers.RenewLease
		resp              *httptest.ResponseRecorder
		unmarshaler       *hfakes.Unmarshaler
		leaseRenewer      *fakes.LeaseRenewer
		fakeErrorResponse *fakes.ErrorResponse

		expectedLease controller.Lease
		request       *http.Request
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		unmarshaler = &hfakes.Unmarshaler{}
		unmarshaler.UnmarshalStub = json.Unmarshal
		leaseRenewer = &fakes.LeaseRenewer{}
		fakeErrorResponse = &fakes.ErrorResponse{}

		handler = &handlers.RenewLease{
			Logger:        logger,
			Unmarshaler:   unmarshaler,
			LeaseRenewer:  leaseRenewer,
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
		request, err = http.NewRequest("PUT", "/leases/renew", requestBody)
		Expect(err).NotTo(HaveOccurred())
		request.RemoteAddr = "some-host:some-port"
	})

	AfterEach(func() {
		By("checking that the last log line is for 'done'")
		last := len(logger.Logs()) - 1
		Expect(logger.Logs()[last].LogLevel).To(Equal(lager.DEBUG))
		Expect(logger.Logs()[last].ToJSON()).To(MatchRegexp("leases-renew.*done"))
	})

	It("renews a lease for subnet", func() {
		handler.ServeHTTP(resp, request)
		Expect(leaseRenewer.RenewSubnetLeaseCallCount()).To(Equal(1))
		Expect(leaseRenewer.RenewSubnetLeaseArgsForCall(0)).To(Equal(expectedLease))

		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(resp.Body.String()).To(Equal("{}"))

		Expect(logger.Logs()).To(HaveLen(2))
		Expect(logger.Logs()[0].LogLevel).To(Equal(lager.DEBUG))
		Expect(logger.Logs()[0].ToJSON()).To(MatchRegexp("leases-renew.*RemoteAddr.*some-host:some-port.*URL.*/leases/renew"))
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

	Context("when renewing a lease fails due to a non-retriable error", func() {
		var terr controller.NonRetriableError
		BeforeEach(func() {
			terr = controller.NonRetriableError("kiwi")
			leaseRenewer.RenewSubnetLeaseReturns(terr)
		})

		It("calls the Error Response Conflict() handler", func() {
			handler.ServeHTTP(resp, request)

			Expect(fakeErrorResponse.ConflictCallCount()).To(Equal(1))
			w, err, message, description := fakeErrorResponse.ConflictArgsForCall(0)
			Expect(w).To(Equal(resp))
			Expect(err).To(Equal(terr))
			Expect(message).To(Equal("renew-subnet-lease"))
			Expect(description).To(Equal(terr.Error()))
		})
	})

	Context("when renewing a lease fails due to some other error", func() {
		BeforeEach(func() {
			leaseRenewer.RenewSubnetLeaseReturns(errors.New("kiwi"))
		})

		It("calls the Error Response InternalServerError() handler", func() {
			handler.ServeHTTP(resp, request)

			Expect(fakeErrorResponse.InternalServerErrorCallCount()).To(Equal(1))
			w, err, message, description := fakeErrorResponse.InternalServerErrorArgsForCall(0)
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("kiwi"))
			Expect(message).To(Equal("renew-subnet-lease"))
			Expect(description).To(Equal("kiwi"))
		})
	})
})
