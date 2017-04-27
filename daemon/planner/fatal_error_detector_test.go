package planner_test

import (
	"fmt"
	"time"

	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/daemon/planner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("FatalErrorDetector", func() {
	var fed planner.FatalErrorDetector
	const graceDuration time.Duration = 10 * time.Millisecond

	BeforeEach(func() {
		fed = planner.NewGracefulDetector(graceDuration)
	})

	Describe("IsFatal", func() {
		Context("when given a non-retriable error", func() {
			It("reports it as fatal", func() {
				err := controller.NonRetriableError("guava")
				Expect(fed.IsFatal(err)).To(BeTrue())
			})
		})
		Context("when given any other type of error", func() {
			It("reports it non-fatal", func() {
				err := fmt.Errorf("banana")
				Expect(fed.IsFatal(err)).To(BeFalse())
			})
		})

		Context("when GraceDuration has passed since the last GotSuccess", func() {
			It("reports errors as fatal if GraceDuration has passed since the last New or GotSuccess", func() {
				err := fmt.Errorf("banana")

				By("checking that the error is not fatal, for a while")
				Consistently(func() bool {
					return fed.IsFatal(err)
				}, graceDuration-time.Millisecond).Should(BeFalse())

				By("waiting for the grace duration")
				time.Sleep(graceDuration)

				By("checking the error is fatal")
				Expect(fed.IsFatal(err)).To(BeTrue())

				By("reporting a success, which resets the timer")
				fed.GotSuccess()

				By("checking that the error is not fatal, for a while")
				Consistently(func() bool {
					return fed.IsFatal(err)
				}, graceDuration-time.Millisecond).Should(BeFalse())

				By("waiting for the grace duration, again")
				time.Sleep(graceDuration)

				By("checking the error is fatal, again")
				Expect(fed.IsFatal(err)).To(BeTrue())
			})
		})
	})
})
