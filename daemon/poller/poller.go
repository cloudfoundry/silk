package poller

import (
	"fmt"
	"os"
	"time"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller"
)

type Poller struct {
	Logger       lager.Logger
	PollInterval time.Duration

	SingleCycleFunc func() error
}

func (m *Poller) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	close(ready)

	for {
		select {
		case <-signals:
			return nil
		case <-time.After(m.PollInterval):
			if err := m.SingleCycleFunc(); err != nil {
				m.Logger.Error("poll-cycle", err)
				if _, ok := err.(controller.NonRetriableError); ok {
					return fmt.Errorf("This cell must be restarted (run \"bosh restart <job>\"): %s", err)
				}
				continue
			}
		}
	}
}
