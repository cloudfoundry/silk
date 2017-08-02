package controller_test

import (
	"encoding/json"
	"errors"
	"net/http"

	"code.cloudfoundry.org/cf-networking-helpers/fakes"
	"code.cloudfoundry.org/cf-networking-helpers/json_client"
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

	Describe("GetActiveLeases", func() {
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
			leases, err := client.GetActiveLeases()
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
				_, err := client.GetActiveLeases()
				Expect(err).To(MatchError("banana"))
			})
		})
	})

	Describe("AcquireSubnetLease", func() {
		BeforeEach(func() {
			jsonClient.DoStub = func(method, route string, reqData, respData interface{}, token string) error {
				respBytes := []byte(`
				{
					"underlay_ip": "10.0.3.1",
					"overlay_subnet": "10.255.90.0/24"
				}`)
				json.Unmarshal(respBytes, respData)
				return nil
			}
		})

		It("does all the right things", func() {
			lease, err := client.AcquireSubnetLease("10.0.3.1")
			Expect(err).NotTo(HaveOccurred())

			Expect(jsonClient.DoCallCount()).To(Equal(1))
			method, route, reqData, _, token := jsonClient.DoArgsForCall(0)
			Expect(method).To(Equal("PUT"))
			Expect(route).To(Equal("/leases/acquire"))
			Expect(reqData).To(Equal(controller.AcquireLeaseRequest{UnderlayIP: "10.0.3.1"}))
			Expect(token).To(BeEmpty())

			Expect(lease).To(Equal(controller.Lease{
				UnderlayIP:    "10.0.3.1",
				OverlaySubnet: "10.255.90.0/24",
			},
			))
		})

		Context("when the json client fails", func() {
			BeforeEach(func() {
				jsonClient.DoReturns(errors.New("carrot"))
			})
			It("returns the error", func() {
				_, err := client.AcquireSubnetLease("10.0.3.1")
				Expect(err).To(MatchError("carrot"))
			})
		})
	})

	Describe("RenewSubnetLease", func() {
		var lease controller.Lease
		BeforeEach(func() {
			lease = controller.Lease{
				UnderlayIP:          "10.0.3.1",
				OverlaySubnet:       "10.255.90.0/24",
				OverlayHardwareAddr: "ee:ee:0a:ff:5a:00",
			}
		})

		It("calls the controller to renew the subnet lease", func() {
			err := client.RenewSubnetLease(lease)
			Expect(err).NotTo(HaveOccurred())

			Expect(jsonClient.DoCallCount()).To(Equal(1))
			method, route, reqData, _, token := jsonClient.DoArgsForCall(0)
			Expect(method).To(Equal("PUT"))
			Expect(route).To(Equal("/leases/renew"))
			Expect(reqData).To(Equal(lease))
			Expect(token).To(BeEmpty())
		})

		Context("when the json client fails due to a HTTP 409 Conflict", func() {
			BeforeEach(func() {
				jsonClient.DoReturns(&json_client.HttpResponseCodeError{
					StatusCode: http.StatusConflict,
					Message:    "banana",
				})
			})

			It("returns a non-retriable error", func() {
				err := client.RenewSubnetLease(lease)
				Expect(err).NotTo(BeNil())
				typedErr, ok := err.(controller.NonRetriableError)
				Expect(ok).To(BeTrue())
				Expect(typedErr.Error()).To(Equal("non-retriable: banana"))
			})
		})

		Context("when the json client returns any other error", func() {
			BeforeEach(func() {
				jsonClient.DoReturns(errors.New("no you're a teapot"))
			})

			It("returns the error", func() {
				err := client.RenewSubnetLease(lease)
				Expect(err).To(MatchError("no you're a teapot"))
			})
		})
	})

	Describe("ReleaseSubnetLease", func() {
		BeforeEach(func() {
			jsonClient.DoStub = func(method, route string, reqData, respData interface{}, token string) error {
				respBytes := []byte(`
				{
					"underlay_ip": "10.0.3.1",
					"overlay_subnet": "10.255.90.0/24",
					"overlay_hardware_addr": "ee:ee:0a:ff:5a:00"
				}`)
				json.Unmarshal(respBytes, respData)
				return nil
			}
		})
		It("calls the controller to release the subnet lease", func() {
			err := client.ReleaseSubnetLease("10.0.3.1")
			Expect(err).NotTo(HaveOccurred())

			Expect(jsonClient.DoCallCount()).To(Equal(1))
			method, route, reqData, response, token := jsonClient.DoArgsForCall(0)
			Expect(method).To(Equal("PUT"))
			Expect(route).To(Equal("/leases/release"))
			Expect(reqData).To(Equal(controller.ReleaseLeaseRequest{UnderlayIP: "10.0.3.1"}))
			Expect(response).To(BeNil())
			Expect(token).To(BeEmpty())
		})

		Context("when the json client returns an error", func() {
			BeforeEach(func() {
				jsonClient.DoReturns(errors.New("no you're a teapot"))
			})

			It("returns the error", func() {
				err := client.ReleaseSubnetLease("10.0.3.1")
				Expect(err).To(MatchError("no you're a teapot"))
			})
		})
	})
})
