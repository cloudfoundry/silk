package poller

import (
	"fmt"
	"os"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/silk/daemon"
)

type Poller struct {
	Logger       lager.Logger
	PollInterval time.Duration

	SingleCycleFunc func() error
}

func (m *Poller) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	close(ready)

	if err := m.runFunction(); err != nil {
		return err
	}

	for {
		select {
		case <-signals:
			return nil
		case <-time.After(m.PollInterval):
			if err := m.runFunction(); err != nil {
				return err
			}
		}
	}
}

func (m *Poller) runFunction() error {
	if err := m.SingleCycleFunc(); err != nil {
		m.Logger.Error("poll-cycle", err)
		if _, ok := err.(daemon.FatalError); ok {
			return fmt.Errorf("This cell must be restarted (run \"bosh restart <job>\"): %s", err)
		}
	}
	return nil
}
