package database_test

import (
	"errors"
	"time"

	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/silk/controller/database"
	"code.cloudfoundry.org/silk/controller/database/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Migrator", func() {
	var (
		logger           *lagertest.TestLogger
		migrator         *database.Migrator
		databaseMigrator *fakes.DatabaseMigrator
	)

	Describe("TryMigrations", func() {
		BeforeEach(func() {
			databaseMigrator = &fakes.DatabaseMigrator{}
			logger = lagertest.NewTestLogger("test")
			migrator = &database.Migrator{
				DatabaseMigrator:              databaseMigrator,
				MaxMigrationAttempts:          5,
				MigrationAttemptSleepDuration: time.Nanosecond,
				Logger: logger,
			}
		})

		It("calls migrate and logs the success", func() {
			databaseMigrator.MigrateReturns(1, nil)

			err := migrator.TryMigrations()
			Expect(err).NotTo(HaveOccurred())
			Expect(logger.Logs()[0].Data["num-applied"]).To(BeEquivalentTo(1))
			Expect(logger.Logs()[0].Message).To(Equal("test.db-migration-complete"))

			Expect(databaseMigrator.MigrateCallCount()).To(Equal(1))
		})

		Context("when the database cannot be migrated within the max migration attempts", func() {
			It("returns an error", func() {
				databaseMigrator.MigrateReturns(1, errors.New("peach"))
				err := migrator.TryMigrations()

				Expect(err).To(MatchError("creating table: peach"))
				Expect(databaseMigrator.MigrateCallCount()).To(Equal(5))
			})
		})
	})

})
