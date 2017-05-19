package planner_test

import (
	"errors"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/daemon"
	"code.cloudfoundry.org/silk/daemon/planner"
	"code.cloudfoundry.org/silk/daemon/planner/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var LogsWith = func(level lager.LogLevel, msg string) types.GomegaMatcher {
	return And(
		WithTransform(func(log lager.LogFormat) string {
			return log.Message
		}, Equal(msg)),
		WithTransform(func(log lager.LogFormat) lager.LogLevel {
			return log.LogLevel
		}, Equal(level)),
	)
}

var HaveLogData = func(nextMatcher types.GomegaMatcher) types.GomegaMatcher {
	return WithTransform(func(log lager.LogFormat) lager.Data {
		return log.Data
	}, nextMatcher)
}

var _ = Describe("VxlanPlanner", func() {
	var (
		logger           *lagertest.TestLogger
		vxlanPlanner     *planner.VXLANPlanner
		controllerClient *fakes.ControllerClient
		converger        *fakes.Converger
		errorDetector    *fakes.FatalErrorDetector
		metricSender     *fakes.MetricSender
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		controllerClient = &fakes.ControllerClient{}
		converger = &fakes.Converger{}
		metricSender = &fakes.MetricSender{}
		errorDetector = &fakes.FatalErrorDetector{}
		vxlanPlanner = &planner.VXLANPlanner{
			Logger:           logger,
			ControllerClient: controllerClient,
			Converger:        converger,
			Lease: controller.Lease{
				UnderlayIP:          "172.244.17.0",
				OverlaySubnet:       "10.244.17.0/24",
				OverlayHardwareAddr: "ee:ee:0a:f4:11:00",
			},
			ErrorDetector: errorDetector,
			MetricSender:  metricSender,
		}
	})

	Describe("DoCycle", func() {
		var leases []controller.Lease

		BeforeEach(func() {
			leases = []controller.Lease{controller.Lease{
				UnderlayIP:          "172.244.15.0",
				OverlaySubnet:       "10.244.15.0/24",
				OverlayHardwareAddr: "ee:ee:0a:f4:0f:00",
			}, controller.Lease{
				UnderlayIP:          "172.244.16.0",
				OverlaySubnet:       "10.244.16.0/24",
				OverlayHardwareAddr: "ee:ee:0a:f4:10:00",
			}}
			controllerClient.GetRoutableLeasesReturns(leases, nil)
		})

		It("calls the controller to renew its lease", func() {
			err := vxlanPlanner.DoCycle()
			Expect(err).NotTo(HaveOccurred())

			Expect(controllerClient.RenewSubnetLeaseCallCount()).To(Equal(1))
			Expect(controllerClient.RenewSubnetLeaseArgsForCall(0)).To(Equal(controller.Lease{
				UnderlayIP:          "172.244.17.0",
				OverlaySubnet:       "10.244.17.0/24",
				OverlayHardwareAddr: "ee:ee:0a:f4:11:00",
			}))

			By("checking that the lease was logged at debug level")
			Expect(logger.Logs()).To(ContainElement(SatisfyAll(
				LogsWith(lager.DEBUG, "test.renew-lease"),
				HaveLogData(HaveKeyWithValue("lease",
					SatisfyAll(
						HaveKeyWithValue("underlay_ip", "172.244.17.0"),
						HaveKeyWithValue("overlay_subnet", "10.244.17.0/24"),
						HaveKeyWithValue("overlay_hardware_addr", "ee:ee:0a:f4:11:00"),
					),
				)),
			)))

			By("informing the error detector of the successful renewal")
			Expect(errorDetector.GotSuccessCallCount()).To(Equal(1))
		})

		It("emits a metric with the number of leases received", func() {
			err := vxlanPlanner.DoCycle()
			Expect(err).NotTo(HaveOccurred())

			Expect(metricSender.SendValueCallCount()).To(Equal(1))
			name, value, unit := metricSender.SendValueArgsForCall(0)
			Expect(name).To(Equal("numberLeases"))
			Expect(value).To(BeEquivalentTo(2))
			Expect(unit).To(Equal(""))

			leases = append(leases, controller.Lease{
				UnderlayIP:          "172.244.17.0",
				OverlaySubnet:       "10.244.17.0/24",
				OverlayHardwareAddr: "ee:ee:0a:f6:10:00",
			})
			controllerClient.GetRoutableLeasesReturns(leases, nil)

			err = vxlanPlanner.DoCycle()
			name, value, unit = metricSender.SendValueArgsForCall(1)
			Expect(name).To(Equal("numberLeases"))
			Expect(value).To(BeEquivalentTo(3))
			Expect(unit).To(Equal(""))

		})

		It("passes the received leases to the converger to update the networking stack", func() {
			err := vxlanPlanner.DoCycle()
			Expect(err).NotTo(HaveOccurred())

			Expect(converger.ConvergeCallCount()).To(Equal(1))
			Expect(converger.ConvergeArgsForCall(0)).To(Equal(leases))

			By("checking that the leases were logged at debug level")
			Expect(logger.Logs()).To(ContainElement(SatisfyAll(
				LogsWith(lager.DEBUG, "test.converge-leases"),
				HaveLogData(HaveKeyWithValue("leases", ConsistOf(
					SatisfyAll(
						HaveKeyWithValue("underlay_ip", "172.244.15.0"),
						HaveKeyWithValue("overlay_subnet", "10.244.15.0/24"),
						HaveKeyWithValue("overlay_hardware_addr", "ee:ee:0a:f4:0f:00"),
					),
					SatisfyAll(
						HaveKeyWithValue("underlay_ip", "172.244.16.0"),
						HaveKeyWithValue("overlay_subnet", "10.244.16.0/24"),
						HaveKeyWithValue("overlay_hardware_addr", "ee:ee:0a:f4:10:00"),
					),
				))),
			)))
		})

		Context("when renewing the subnet lease fails from an error", func() {
			Context("when the error is detected as non-fatal", func() {
				BeforeEach(func() {
					controllerClient.RenewSubnetLeaseReturns(errors.New("guava"))
					errorDetector.IsFatalReturns(false)
				})
				It("returns the error as non-fatal", func() {
					err := vxlanPlanner.DoCycle()
					Expect(err).To(MatchError("renew lease: guava"))
					_, ok := err.(daemon.FatalError)
					Expect(ok).NotTo(BeTrue())

					Expect(errorDetector.IsFatalCallCount()).To(Equal(1))
					Expect(errorDetector.IsFatalArgsForCall(0)).To(Equal(errors.New("guava")))
					Expect(errorDetector.GotSuccessCallCount()).To(Equal(0))
				})
			})

			Context("when the error is detected as fatal", func() {
				BeforeEach(func() {
					controllerClient.RenewSubnetLeaseReturns(errors.New("guava"))
					errorDetector.IsFatalReturns(true)
				})
				It("returns the error as a fatal error", func() {
					err := vxlanPlanner.DoCycle()
					Expect(err).To(MatchError("fatal: renew lease: guava"))
					_, ok := err.(daemon.FatalError)
					Expect(ok).To(BeTrue())

					Expect(errorDetector.IsFatalCallCount()).To(Equal(1))
					Expect(errorDetector.IsFatalArgsForCall(0)).To(Equal(errors.New("guava")))
					Expect(errorDetector.GotSuccessCallCount()).To(Equal(0))
				})
			})
		})

		Context("when getting the routable releases fails", func() {
			BeforeEach(func() {
				controllerClient.GetRoutableLeasesReturns(nil, errors.New("guava"))
			})
			It("returns the error", func() {
				err := vxlanPlanner.DoCycle()
				Expect(err).To(MatchError("get routable leases: guava"))
			})
		})

		Context("when the converger fails", func() {
			BeforeEach(func() {
				converger.ConvergeReturns(errors.New("banana"))
			})
			It("returns an error", func() {
				err := vxlanPlanner.DoCycle()
				Expect(err).To(MatchError("converge leases: banana"))
			})
		})
	})
})
