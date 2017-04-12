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

var _ = Describe("LeasesAcquire", func() {
	var (
		logger        *lagertest.TestLogger
		handler       *handlers.LeasesAcquire
		resp          *httptest.ResponseRecorder
		marshaler     *hfakes.Marshaler
		unmarshaler   *hfakes.Unmarshaler
		leaseAcquirer *fakes.LeaseAcquirer
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		marshaler = &hfakes.Marshaler{}
		marshaler.MarshalStub = json.Marshal
		unmarshaler = &hfakes.Unmarshaler{}
		unmarshaler.UnmarshalStub = json.Unmarshal
		leaseAcquirer = &fakes.LeaseAcquirer{}

		handler = &handlers.LeasesAcquire{
			Logger:        logger,
			Marshaler:     marshaler,
			Unmarshaler:   unmarshaler,
			LeaseAcquirer: leaseAcquirer,
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

		handler.ServeHTTP(resp, request)
		Expect(leaseAcquirer.AcquireSubnetLeaseCallCount()).To(Equal(1))
		Expect(leaseAcquirer.AcquireSubnetLeaseArgsForCall(0)).To(Equal("10.244.16.11"))

		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(resp.Body).To(MatchJSON(expectedResponseJSON))

		Expect(logger.Logs()).To(HaveLen(1))
		Expect(logger.Logs()[0].LogLevel).To(Equal(lager.DEBUG))
		Expect(logger.Logs()[0].ToJSON()).To(MatchRegexp("leases-acquire.*RemoteAddr.*some-host:some-port.*URL.*/leases/acquire"))
	})

	Context("when there are errors reading the body bytes", func() {
		var request *http.Request
		BeforeEach(func() {
			var err error
			request, err = http.NewRequest("PUT", "/leases/acquire", ioutil.NopCloser(&testsupport.BadReader{}))
			Expect(err).NotTo(HaveOccurred())
		})

		It("logs the error and returns a 400", func() {
			handler.ServeHTTP(resp, request)

			Expect(resp.Code).To(Equal(400))
			Expect(logger.Logs()).To(HaveLen(2))
			Expect(logger.Logs()[1].LogLevel).To(Equal(lager.ERROR))
			Expect(logger.Logs()[1].ToJSON()).To(MatchRegexp("leases-acquire.*read-body.*banana"))
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

			handler.ServeHTTP(resp, request)

			Expect(resp.Code).To(Equal(400))
			Expect(logger.Logs()).To(HaveLen(2))
			Expect(logger.Logs()[1].LogLevel).To(Equal(lager.ERROR))
			Expect(logger.Logs()[1].ToJSON()).To(MatchRegexp("leases-acquire.*unmarshal-request.*fig"))
		})
	})

	Context("when a lease cannot be acquired", func() {
		BeforeEach(func() {
			leaseAcquirer.AcquireSubnetLeaseReturns(nil, errors.New("kiwi"))
		})

		It("logs the error and returns a 503", func() {
			requestBody := bytes.NewBuffer([]byte(`{ "underlay_ip": "10.244.16.11" }`))
			request, err := http.NewRequest("PUT", "/leases/acquire", requestBody)
			Expect(err).NotTo(HaveOccurred())

			handler.ServeHTTP(resp, request)

			Expect(resp.Code).To(Equal(503))
			Expect(logger.Logs()).To(HaveLen(2))
			Expect(logger.Logs()[1].LogLevel).To(Equal(lager.ERROR))
			Expect(logger.Logs()[1].ToJSON()).To(MatchRegexp("leases-acquire.*acquire-subnet-lease.*kiwi"))
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

			handler.ServeHTTP(resp, request)

			Expect(resp.Code).To(Equal(500))
			Expect(logger.Logs()).To(HaveLen(2))
			Expect(logger.Logs()[1].LogLevel).To(Equal(lager.ERROR))
			Expect(logger.Logs()[1].ToJSON()).To(MatchRegexp("leases-acquire.*marshal-response.*grapes"))
		})
	})
})
