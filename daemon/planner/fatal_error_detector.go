package planner

import (
	"time"

	"code.cloudfoundry.org/silk/controller"
)

//go:generate counterfeiter -o fakes/fatalErrorDetector.go --fake-name FatalErrorDetector . FatalErrorDetector
type FatalErrorDetector interface {
	GotSuccess()
	IsFatal(error) bool
}

type gracefulDetector struct {
	graceDuration   time.Duration
	lastSuccessTime time.Time
}

func NewGracefulDetector(gd time.Duration) FatalErrorDetector {
	return &gracefulDetector{
		graceDuration:   gd,
		lastSuccessTime: time.Now(),
	}
}

func (fed *gracefulDetector) GotSuccess() {
	fed.lastSuccessTime = time.Now()
}

func (fed *gracefulDetector) IsFatal(err error) bool {
	_, isNonRetriable := err.(controller.NonRetriableError)
	if isNonRetriable {
		return true
	}
	return time.Now().Sub(fed.lastSuccessTime) >= fed.graceDuration
}
