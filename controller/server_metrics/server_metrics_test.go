package server_metrics_test

import (
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/server_metrics"
	"code.cloudfoundry.org/silk/controller/server_metrics/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServerMetrics", func() {
	var allLeases []controller.Lease
	var fakeDatabaseHandler *fakes.DatabaseHandler
	var fakeCIDRPool *fakes.CIDRPool

	BeforeEach(func() {
		allLeases = []controller.Lease{
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
		}
		fakeDatabaseHandler = &fakes.DatabaseHandler{}
		fakeDatabaseHandler.AllReturns(allLeases, nil)
		fakeDatabaseHandler.AllActiveReturns([]controller.Lease{allLeases[0]}, nil)

		fakeCIDRPool = &fakes.CIDRPool{}
		fakeCIDRPool.SizeReturns(100)
	})

	Describe("totalLeases", func() {
		It("returns the total number of leases in the datastore", func() {
			source := server_metrics.NewTotalLeasesSource(fakeDatabaseHandler)

			Expect(source.Name).To(Equal("totalLeases"))
			Expect(source.Unit).To(Equal(""))

			value, err := source.Getter()
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeDatabaseHandler.AllCallCount()).To(Equal(1))
			Expect(value).To(Equal(2.0))
		})
	})

	Describe("freeLeases", func() {
		It("returns the total number of free leases", func() {
			source := server_metrics.NewFreeLeasesSource(fakeDatabaseHandler, fakeCIDRPool)

			Expect(source.Name).To(Equal("freeLeases"))
			Expect(source.Unit).To(Equal(""))

			value, err := source.Getter()
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeDatabaseHandler.AllCallCount()).To(Equal(1))
			Expect(fakeCIDRPool.SizeCallCount()).To(Equal(1))
			Expect(value).To(Equal(98.0))
		})
	})

	Describe("staleLeases", func() {
		It("returns the total number of stale leases in the datastore", func() {
			source := server_metrics.NewStaleLeasesSource(fakeDatabaseHandler, 5)

			Expect(source.Name).To(Equal("staleLeases"))
			Expect(source.Unit).To(Equal(""))

			value, err := source.Getter()
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeDatabaseHandler.AllCallCount()).To(Equal(1))
			Expect(fakeDatabaseHandler.AllActiveCallCount()).To(Equal(1))
			Expect(fakeDatabaseHandler.AllActiveArgsForCall(0)).To(Equal(5))
			Expect(value).To(Equal(1.0))
		})
	})

})
