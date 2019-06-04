package poller_test

import (
	"errors"
	"os"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/silk/daemon"
	"code.cloudfoundry.org/silk/daemon/poller"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Poller", func() {
	Describe("Run", func() {
		var (
			logger  *lagertest.TestLogger
			p       *poller.Poller
			signals chan os.Signal
			ready   chan struct{}

			cycleCount uint64
			retChan    chan error
		)

		BeforeEach(func() {
			signals = make(chan os.Signal)
			ready = make(chan struct{})

			cycleCount = 0
			retChan = make(chan error)

			logger = lagertest.NewTestLogger("test")

			p = &poller.Poller{
				Logger:       logger,
				PollInterval: 1 * time.Second,

				SingleCycleFunc: func() error {
					atomic.AddUint64(&cycleCount, 1)
					return nil
				},
			}
		})

		Context("when running", func() {

			It("calls the single cycle func on start", func() {
				go func() {
					retChan <- p.Run(signals, ready)
				}()

				Eventually(ready).Should(BeClosed())
				Expect(atomic.LoadUint64(&cycleCount)).To(Equal(uint64(1)))

				signals <- os.Interrupt
				Eventually(retChan).Should(Receive(nil))
			})

			It("calls the single cycle func after the poll interval", func() {
				go func() {
					retChan <- p.Run(signals, ready)
				}()

				Eventually(ready).Should(BeClosed())
				Eventually(func() uint64 {
					return atomic.LoadUint64(&cycleCount)
				}).Should(BeNumerically(">", 1))

				Consistently(retChan).ShouldNot(Receive())

				signals <- os.Interrupt
				Eventually(retChan).Should(Receive(nil))
			})

		})

		Context("when the cycle func fails with a non-fatal error", func() {
			BeforeEach(func() {
				p.SingleCycleFunc = func() error { return errors.New("banana") }
			})

			It("logs the error and continues", func() {
				go func() {
					retChan <- p.Run(signals, ready)
				}()

				Eventually(ready).Should(BeClosed())

				Eventually(logger).Should(gbytes.Say("poll-cycle.*banana"))

				Consistently(retChan).ShouldNot(Receive())

				signals <- os.Interrupt
				Eventually(retChan).Should(Receive(nil))
			})
		})

		Context("when the cycle func fails with a fatal error", func() {
			BeforeEach(func() {
				p.SingleCycleFunc = func() error {
					return daemon.FatalError("banana")
				}
			})

			It("logs the error and exits", func() {
				go func() {
					retChan <- p.Run(signals, ready)
				}()

				Eventually(ready).Should(BeClosed())
				Eventually(logger).Should(gbytes.Say("poll-cycle.*banana"))
				Eventually(retChan).Should(Receive(MatchError(
					"This cell must be restarted (run \"bosh restart <job>\"): fatal: banana",
				)))
			})
		})
	})
})
