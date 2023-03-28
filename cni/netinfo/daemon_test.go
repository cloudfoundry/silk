package netinfo_test

import (
	"errors"

	helperfakes "code.cloudfoundry.org/cf-networking-helpers/fakes"
	"code.cloudfoundry.org/silk/cni/netinfo"
	"code.cloudfoundry.org/silk/daemon"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Daemon Netinfo", func() {
	var (
		daemonNetInfo  *netinfo.Daemon
		fakeJsonClient *helperfakes.JSONClient
	)
	BeforeEach(func() {
		fakeJsonClient = &helperfakes.JSONClient{}
		daemonNetInfo = &netinfo.Daemon{
			JSONClient: fakeJsonClient,
		}

		fakeJsonClient.DoStub = func(method, route string, reqData, respData interface{}, token string) error {
			netInfo := respData.(*daemon.NetworkInfo)
			netInfo.MTU = 543
			netInfo.OverlaySubnet = "10.240.0.0/12"
			return nil
		}
	})
	It("gets the daemon netinfo with the http client", func() {
		info, err := daemonNetInfo.Get()
		Expect(err).NotTo(HaveOccurred())
		Expect(info).To(Equal(daemon.NetworkInfo{
			MTU:           543,
			OverlaySubnet: "10.240.0.0/12",
		}))

		Expect(fakeJsonClient.DoCallCount()).To(Equal(1))
		method, route, reqData, respData, _ := fakeJsonClient.DoArgsForCall(0)
		Expect(method).To(Equal("GET"))
		Expect(route).To(Equal("/"))
		Expect(reqData).To(BeNil())
		Expect(respData).To(BeAssignableToTypeOf(&daemon.NetworkInfo{}))
	})
	Context("when the json client errors", func() {
		BeforeEach(func() {
			fakeJsonClient.DoReturns(errors.New("banana"))
		})
		It("returns the error", func() {
			_, err := daemonNetInfo.Get()
			Expect(err).To(MatchError("json client do: banana"))
		})
	})
})
