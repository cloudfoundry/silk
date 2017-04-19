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
)

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

		It("calls the converger with the leases", func() {
			err := vxlanPlanner.DoCycle()
			Expect(err).NotTo(HaveOccurred())

			Expect(converger.ConvergeCallCount()).To(Equal(1))
			Expect(converger.ConvergeArgsForCall(0)).To(Equal(leases))
		})

		It("calls the controller and logs the leases", func() {
			err := vxlanPlanner.DoCycle()
			Expect(err).NotTo(HaveOccurred())

			Expect(logger.Logs()).To(HaveLen(1))

			By("checking that the leases were logged at debug level")
			Expect(logger.Logs()[0].Message).To(Equal("test.get-routable-leases"))
			Expect(logger.Logs()[0].LogLevel).To(Equal(lager.DEBUG))
			Expect(logger.Logs()[0].Data).To(HaveKey("leases"))
			returnedLeases, ok := logger.Logs()[0].Data["leases"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(returnedLeases).To(HaveLen(2))
			Expect(returnedLeases[0]).To(HaveKeyWithValue("underlay_ip", "172.244.15.0"))
			Expect(returnedLeases[0]).To(HaveKeyWithValue("overlay_subnet", "10.244.15.0/24"))
			Expect(returnedLeases[0]).To(HaveKeyWithValue("overlay_hardware_addr", "ee:ee:0a:f4:0f:00"))
			Expect(returnedLeases[1]).To(HaveKeyWithValue("underlay_ip", "172.244.16.0"))
			Expect(returnedLeases[1]).To(HaveKeyWithValue("overlay_subnet", "10.244.16.0/24"))
			Expect(returnedLeases[1]).To(HaveKeyWithValue("overlay_hardware_addr", "ee:ee:0a:f4:10:00"))
		})

		Context("when the controller is unreachable", func() {
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
