package planner_test

import (
	"errors"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/silk/controller"
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
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		controllerClient = &fakes.ControllerClient{}
		converger = &fakes.Converger{}
		vxlanPlanner = &planner.VXLANPlanner{
			Logger:           logger,
			ControllerClient: controllerClient,
			Converger:        converger,
			Lease: controller.Lease{
				UnderlayIP:          "172.244.17.0",
				OverlaySubnet:       "10.244.17.0/24",
				OverlayHardwareAddr: "ee:ee:0a:f4:11:00",
			},
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
		})

		It("calls the converger to get all routable leases", func() {
			err := vxlanPlanner.DoCycle()
			Expect(err).NotTo(HaveOccurred())

			Expect(converger.ConvergeCallCount()).To(Equal(1))
			Expect(converger.ConvergeArgsForCall(0)).To(Equal(leases))

			By("checking that the leases were logged at debug level")
			Expect(logger.Logs()).To(ContainElement(SatisfyAll(
				LogsWith(lager.DEBUG, "test.get-routable-leases"),
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

		Context("when the renewing the subnet lease fails", func() {
			BeforeEach(func() {
				controllerClient.RenewSubnetLeaseReturns(errors.New("guava"))
			})
			It("returns the error", func() {
				err := vxlanPlanner.DoCycle()
				Expect(err).To(MatchError("renew lease: guava"))
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
