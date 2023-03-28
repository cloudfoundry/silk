package handlers_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	hfakes "code.cloudfoundry.org/cf-networking-helpers/fakes"
	"code.cloudfoundry.org/cf-networking-helpers/testsupport"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/handlers"
	"code.cloudfoundry.org/silk/controller/handlers/fakes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LeasesAcquire", func() {
	var (
		logger            *lagertest.TestLogger
		expectedLogger    lager.Logger
		handler           *handlers.LeasesAcquire
		resp              *httptest.ResponseRecorder
		marshaler         *hfakes.Marshaler
		unmarshaler       *hfakes.Unmarshaler
		leaseAcquirer     *fakes.LeaseAcquirer
		fakeErrorResponse *fakes.ErrorResponse
	)

	BeforeEach(func() {
		expectedLogger = lager.NewLogger("test").Session("leases-acquire")

		testSink := lagertest.NewTestSink()
		expectedLogger.RegisterSink(testSink)
		expectedLogger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.DEBUG))

		logger = lagertest.NewTestLogger("test")
		marshaler = &hfakes.Marshaler{}
		marshaler.MarshalStub = json.Marshal
		unmarshaler = &hfakes.Unmarshaler{}
		unmarshaler.UnmarshalStub = json.Unmarshal
		leaseAcquirer = &fakes.LeaseAcquirer{}
		fakeErrorResponse = &fakes.ErrorResponse{}

		handler = &handlers.LeasesAcquire{
			Marshaler:     marshaler,
			Unmarshaler:   unmarshaler,
			LeaseAcquirer: leaseAcquirer,
			ErrorResponse: fakeErrorResponse,
		}
		resp = httptest.NewRecorder()

		lease := &controller.Lease{
			UnderlayIP:          "10.244.16.11",
			OverlaySubnet:       "10.255.17.0/24",
			OverlayHardwareAddr: "ee:ee:0a:ff:11:00",
		}
		leaseAcquirer.AcquireSubnetLeaseReturns(lease, nil)
	})

	It("acquires a lease for subnet", func() {
		expectedResponseJSON := `{ "underlay_ip": "10.244.16.11", "overlay_subnet": "10.255.17.0/24", "overlay_hardware_addr": "ee:ee:0a:ff:11:00" }`
		requestBody := bytes.NewBuffer([]byte(`{ "underlay_ip": "10.244.16.11" }`))
		request, err := http.NewRequest("PUT", "/leases/acquire", requestBody)
		Expect(err).NotTo(HaveOccurred())

		request.RemoteAddr = "some-host:some-port"

		handler.ServeHTTP(logger, resp, request)
		Expect(leaseAcquirer.AcquireSubnetLeaseCallCount()).To(Equal(1))
		underlayIP, singleOverlayIP := leaseAcquirer.AcquireSubnetLeaseArgsForCall(0)
		Expect(underlayIP).To(Equal("10.244.16.11"))
		Expect(singleOverlayIP).To(Equal(false))

		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(resp.Body).To(MatchJSON(expectedResponseJSON))
	})

	It("acquires a lease for a single overlay IP", func() {
		lease := &controller.Lease{
			UnderlayIP:          "10.244.0.12",
			OverlaySubnet:       "10.255.0.17/32",
			OverlayHardwareAddr: "ee:ee:0a:fb:00:11",
		}
		leaseAcquirer.AcquireSubnetLeaseReturns(lease, nil)

		expectedResponseJSON := `{ "underlay_ip": "10.244.0.12", "overlay_subnet": "10.255.0.17/32", "overlay_hardware_addr": "ee:ee:0a:fb:00:11" }`
		requestBody := bytes.NewBuffer([]byte(`{ "underlay_ip": "10.244.0.12", "single_overlay_ip": true }`))
		request, err := http.NewRequest("PUT", "/leases/acquire", requestBody)
		Expect(err).NotTo(HaveOccurred())

		request.RemoteAddr = "remote-host:remote-port"

		handler.ServeHTTP(logger, resp, request)
		Expect(leaseAcquirer.AcquireSubnetLeaseCallCount()).To(Equal(1))
		underlayIP, singleOverlayIP := leaseAcquirer.AcquireSubnetLeaseArgsForCall(0)
		Expect(underlayIP).To(Equal("10.244.0.12"))
		Expect(singleOverlayIP).To(Equal(true))

		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(resp.Body).To(MatchJSON(expectedResponseJSON))
	})

	Context("when there are errors reading the body bytes", func() {
		var request *http.Request
		BeforeEach(func() {
			var err error
			request, err = http.NewRequest("PUT", "/leases/acquire", ioutil.NopCloser(&testsupport.BadReader{}))
			Expect(err).NotTo(HaveOccurred())
		})

		It("calls the BadRequest error handler", func() {
			handler.ServeHTTP(logger, resp, request)

			Expect(fakeErrorResponse.BadRequestCallCount()).To(Equal(1))
			l, w, err, description := fakeErrorResponse.BadRequestArgsForCall(0)
			Expect(l).To(Equal(expectedLogger))
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("banana"))
			Expect(description).To(Equal("read-body: banana"))
		})
	})

	Context("when the request cannot be unmarshaled", func() {
		BeforeEach(func() {
			unmarshaler.UnmarshalReturns(errors.New("fig"))
		})

		It("logs the error and returns a 400", func() {
			requestBody := bytes.NewBuffer([]byte(`{ "underlay_ip": "10.244.16.11" }`))
			request, err := http.NewRequest("PUT", "/leases/acquire", requestBody)
			Expect(err).NotTo(HaveOccurred())

			handler.ServeHTTP(logger, resp, request)

			Expect(fakeErrorResponse.BadRequestCallCount()).To(Equal(1))
			l, w, err, description := fakeErrorResponse.BadRequestArgsForCall(0)
			Expect(l).To(Equal(expectedLogger))
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("fig"))
			Expect(description).To(Equal("unmarshal-request: fig"))
		})
	})

	Context("when acquiring a lease fails", func() {
		BeforeEach(func() {
			leaseAcquirer.AcquireSubnetLeaseReturns(nil, errors.New("kiwi"))
		})

		It("logs the error and returns a 500", func() {
			requestBody := bytes.NewBuffer([]byte(`{ "underlay_ip": "10.244.16.11" }`))
			request, err := http.NewRequest("PUT", "/leases/acquire", requestBody)
			Expect(err).NotTo(HaveOccurred())

			handler.ServeHTTP(logger, resp, request)

			Expect(fakeErrorResponse.InternalServerErrorCallCount()).To(Equal(1))
			l, w, err, description := fakeErrorResponse.InternalServerErrorArgsForCall(0)
			Expect(l).To(Equal(expectedLogger))
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("kiwi"))
			Expect(description).To(Equal("kiwi"))
		})
	})

	Context("when no leases are available", func() {
		BeforeEach(func() {
			leaseAcquirer.AcquireSubnetLeaseReturns(nil, nil)
		})

		It("logs the error and returns a 503", func() {
			requestBody := bytes.NewBuffer([]byte(`{ "underlay_ip": "10.244.16.11" }`))
			request, err := http.NewRequest("PUT", "/leases/acquire", requestBody)
			Expect(err).NotTo(HaveOccurred())

			handler.ServeHTTP(logger, resp, request)

			Expect(fakeErrorResponse.ConflictCallCount()).To(Equal(1))
			l, w, err, description := fakeErrorResponse.ConflictArgsForCall(0)
			Expect(l).To(Equal(expectedLogger))
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("no lease available"))
			Expect(description).To(Equal("no lease available"))
		})
	})

	Context("when the response cannot be marshaled", func() {
		BeforeEach(func() {
			marshaler.MarshalStub = func(interface{}) ([]byte, error) {
				return nil, errors.New("grapes")
			}
		})

		It("logs the error and returns a 500", func() {
			requestBody := bytes.NewBuffer([]byte(`{ "underlay_ip": "10.244.16.11" }`))
			request, err := http.NewRequest("PUT", "/leases/acquire", requestBody)
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
