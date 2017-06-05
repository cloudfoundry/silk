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
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
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

		request *http.Request
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
			Marshaler:     marshaler,
			Unmarshaler:   unmarshaler,
			LeaseReleaser: leaseReleaser,
			ErrorResponse: fakeErrorResponse,
		}
		resp = httptest.NewRecorder()

		requestBody := bytes.NewBuffer([]byte(`{ "underlay_ip": "10.244.16.11" }`))
		var err error
		request, err = http.NewRequest("PUT", "/leases/release", requestBody)
		Expect(err).NotTo(HaveOccurred())
		request.RemoteAddr = "some-host:some-port"
	})

	It("releases a lease for subnet", func() {
		handler.ServeHTTP(logger, resp, request)
		Expect(leaseReleaser.ReleaseSubnetLeaseCallCount()).To(Equal(1))
		Expect(leaseReleaser.ReleaseSubnetLeaseArgsForCall(0)).To(Equal("10.244.16.11"))

		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(resp.Body.String()).To(MatchJSON(`{}`))
	})

	Context("when there are errors reading the body bytes", func() {
		BeforeEach(func() {
			request.Body = ioutil.NopCloser(&testsupport.BadReader{})
		})

		It("logs the error and returns a 400", func() {
			handler.ServeHTTP(logger, resp, request)

			Expect(fakeErrorResponse.BadRequestCallCount()).To(Equal(1))
			w, err, message, description := fakeErrorResponse.BadRequestArgsForCall(0)
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("banana"))
			Expect(message).To(Equal("read-body"))
			Expect(description).To(Equal("banana"))

			By("logging the error")
			Expect(logger.Logs()).To(HaveLen(1))
			Expect(logger.Logs()[0]).To(SatisfyAll(
				LogsWith(lager.ERROR, "test.leases-release.failed-reading-request-body"),
				HaveLogData(SatisfyAll(
					HaveLen(2),
					HaveKeyWithValue("error", "banana"),
					HaveKeyWithValue("session", "1"),
				)),
			))
		})
	})

	Context("when the request cannot be unmarshaled", func() {
		BeforeEach(func() {
			unmarshaler.UnmarshalReturns(errors.New("fig"))
		})

		It("returns a BadRequest error", func() {
			handler.ServeHTTP(logger, resp, request)

			Expect(fakeErrorResponse.BadRequestCallCount()).To(Equal(1))
			w, err, message, description := fakeErrorResponse.BadRequestArgsForCall(0)
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("fig"))
			Expect(message).To(Equal("unmarshal-request"))
			Expect(description).To(Equal("fig"))

			By("logging the error")
			Expect(logger.Logs()).To(HaveLen(1))
			Expect(logger.Logs()[0]).To(SatisfyAll(
				LogsWith(lager.ERROR, "test.leases-release.failed-unmarshalling-payload"),
				HaveLogData(SatisfyAll(
					HaveLen(2),
					HaveKeyWithValue("error", "fig"),
					HaveKeyWithValue("session", "1"),
				)),
			))
		})
	})

	Context("when releasing a lease fails", func() {
		BeforeEach(func() {
			leaseReleaser.ReleaseSubnetLeaseReturns(errors.New("kiwi"))
		})

		It("calls the Error Response InternalServerError() handler", func() {
			handler.ServeHTTP(logger, resp, request)

			Expect(fakeErrorResponse.InternalServerErrorCallCount()).To(Equal(1))
			w, err, message, description := fakeErrorResponse.InternalServerErrorArgsForCall(0)
			Expect(w).To(Equal(resp))
			Expect(err).To(MatchError("kiwi"))
			Expect(message).To(Equal("release-subnet-lease"))
			Expect(description).To(Equal("kiwi"))

			By("logging the error")
			Expect(logger.Logs()).To(HaveLen(1))
			Expect(logger.Logs()[0]).To(SatisfyAll(
				LogsWith(lager.ERROR, "test.leases-release.failed-releasing-lease"),
				HaveLogData(SatisfyAll(
					HaveLen(2),
					HaveKeyWithValue("error", "kiwi"),
					HaveKeyWithValue("session", "1"),
				)),
			))
		})
	})

})
