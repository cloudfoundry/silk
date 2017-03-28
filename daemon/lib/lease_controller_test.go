package lib_test

import (
	"time"

	"code.cloudfoundry.org/lager/lagertest"

	"github.com/cloudfoundry-incubator/silk/daemon/lib"
	"github.com/cloudfoundry-incubator/silk/daemon/lib/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LeaseController", func() {
	Describe("TryMigrations", func() {
		It("calls migrate and logs the success", func() {
			logger := lagertest.NewTestLogger("test")
			databaseHandler := &fakes.DatabaseHandler{}
			databaseHandler.MigrateReturns(1, nil)

			leaseController := lib.LeaseController{
				DatabaseHandler:               databaseHandler,
				MaxMigrationAttempts:          5,
				MigrationAttemptSleepDuration: time.Second,
				Logger: logger,
			}

			err := leaseController.TryMigrations()
			Expect(err).NotTo(HaveOccurred())
			Expect(logger.Logs()[0].Data["num-applied"]).To(BeEquivalentTo(1))
			Expect(logger.Logs()[0].Message).To(Equal("test.db-migration-complete"))

			Expect(databaseHandler.MigrateCallCount()).To(Equal(1))
		})
	})
})
