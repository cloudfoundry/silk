package controller_test

import (
	"encoding/json"
	"errors"

	"code.cloudfoundry.org/go-db-helpers/fakes"
	"code.cloudfoundry.org/silk/controller"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client", func() {
	var (
		client     *controller.Client
		jsonClient *fakes.JSONClient
	)

	BeforeEach(func() {
		jsonClient = &fakes.JSONClient{}
		client = &controller.Client{
			JsonClient: jsonClient,
		}
	})

	Describe("GetRoutableLeases", func() {
		BeforeEach(func() {
			jsonClient.DoStub = func(method, route string, reqData, respData interface{}, token string) error {
				respBytes := []byte(`
				{
					"leases": [
						{ "underlay_ip": "10.0.3.1", "overlay_subnet": "10.255.90.0/24" },
						{ "underlay_ip": "10.0.5.9", "overlay_subnet": "10.253.30.0/24" },
						{ "underlay_ip": "10.0.0.8", "overlay_subnet": "10.255.255.55/32" }
					]
				}`)
				json.Unmarshal(respBytes, respData)
				return nil
			}
		})

		It("does all the right things", func() {
			leases, err := client.GetRoutableLeases()
			Expect(err).NotTo(HaveOccurred())

			Expect(jsonClient.DoCallCount()).To(Equal(1))
			method, route, reqData, _, token := jsonClient.DoArgsForCall(0)
			Expect(method).To(Equal("GET"))
			Expect(route).To(Equal("/leases"))
			Expect(reqData).To(BeNil())
			Expect(token).To(BeEmpty())

			Expect(leases).To(Equal([]controller.Lease{
				{
					UnderlayIP:    "10.0.3.1",
					OverlaySubnet: "10.255.90.0/24",
				},
				{
					UnderlayIP:    "10.0.5.9",
					OverlaySubnet: "10.253.30.0/24",
				},
				{
					UnderlayIP:    "10.0.0.8",
					OverlaySubnet: "10.255.255.55/32",
				},
			},
			))
		})

		Context("when the json client fails", func() {
			BeforeEach(func() {
				jsonClient.DoReturns(errors.New("banana"))
			})
			It("returns the error", func() {
				_, err := client.GetRoutableLeases()
				Expect(err).To(MatchError("banana"))
			})
		})
	})
})
