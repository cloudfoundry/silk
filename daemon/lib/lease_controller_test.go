package lib_test

import (
	"errors"
	"time"

	"code.cloudfoundry.org/lager/lagertest"

	"github.com/cloudfoundry-incubator/silk/daemon/lib"
	"github.com/cloudfoundry-incubator/silk/daemon/lib/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LeaseController", func() {
	Describe("TryMigrations", func() {
		var (
			logger          *lagertest.TestLogger
			databaseHandler *fakes.DatabaseHandler
			leaseController lib.LeaseController
		)

		BeforeEach(func() {
			logger = lagertest.NewTestLogger("test")
			databaseHandler = &fakes.DatabaseHandler{}

			leaseController = lib.LeaseController{
				DatabaseHandler:               databaseHandler,
				MaxMigrationAttempts:          5,
				MigrationAttemptSleepDuration: time.Nanosecond,
				Logger: logger,
			}
		})

		It("calls migrate and logs the success", func() {
			databaseHandler.MigrateReturns(1, nil)

			err := leaseController.TryMigrations()
			Expect(err).NotTo(HaveOccurred())
			Expect(logger.Logs()[0].Data["num-applied"]).To(BeEquivalentTo(1))
			Expect(logger.Logs()[0].Message).To(Equal("test.db-migration-complete"))

			Expect(databaseHandler.MigrateCallCount()).To(Equal(1))
		})

		Context("when the database cannot be migrated within the max migration attempts", func() {
			It("returns an error", func() {
				databaseHandler.MigrateReturns(1, errors.New("peach"))
				err := leaseController.TryMigrations()

				Expect(err).To(MatchError("creating table: peach"))
				Expect(databaseHandler.MigrateCallCount()).To(Equal(5))
			})
		})
	})

	Describe("AcquireSubnetLease", func() {
		var (
			logger          *lagertest.TestLogger
			databaseHandler *fakes.DatabaseHandler
			leaseController lib.LeaseController
			cidrPool        *fakes.CIDRPool
		)

		BeforeEach(func() {
			logger = lagertest.NewTestLogger("test")
			databaseHandler = &fakes.DatabaseHandler{}
			cidrPool = &fakes.CIDRPool{}

			leaseController = lib.LeaseController{
				DatabaseHandler:            databaseHandler,
				AcquireSubnetLeaseAttempts: 10,
				CIDRPool:                   cidrPool,
				UnderlayIP:                 "10.244.5.6",
				Logger:                     logger,
			}
		})

		It("acquires a lease and logs the success", func() {
			databaseHandler.SubnetExistsReturns(false, nil)
			cidrPool.GetRandomReturns("10.255.76.0/24")

			_, err := leaseController.AcquireSubnetLease()
			Expect(err).NotTo(HaveOccurred())
			Expect(logger.Logs()[0].Data["subnet"]).To(Equal("10.255.76.0/24"))
			Expect(logger.Logs()[0].Data["underlay ip"]).To(Equal("10.244.5.6"))
			Expect(logger.Logs()[0].Message).To(Equal("test.subnet-acquired"))

			Expect(databaseHandler.AddEntryCallCount()).To(Equal(1))
		})

		Context("when checking if a subnet exists fails", func() {
			It("returns an error", func() {
				databaseHandler.SubnetExistsReturns(false, errors.New("guava"))

				_, err := leaseController.AcquireSubnetLease()
				Expect(err).To(MatchError("checking if subnet is available: guava"))

				Expect(databaseHandler.SubnetExistsCallCount()).To(Equal(10))
				Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))
			})
		})

		Context("when adding the lease entry fails", func() {
			It("returns an error", func() {
				databaseHandler.AddEntryReturns(errors.New("guava"))

				_, err := leaseController.AcquireSubnetLease()
				Expect(err).To(MatchError("adding lease entry: guava"))

				Expect(databaseHandler.AddEntryCallCount()).To(Equal(10))
			})
		})
	})
})
