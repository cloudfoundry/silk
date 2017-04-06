package lib_test

import (
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/lager/lagertest"

	"code.cloudfoundry.org/silk/daemon/lib"
	"code.cloudfoundry.org/silk/daemon/lib/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LeaseController", func() {
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
			DatabaseHandler: databaseHandler,
			Logger:          logger,
		}
	})
	Describe("TryMigrations", func() {
		BeforeEach(func() {
			leaseController.MaxMigrationAttempts = 5
			leaseController.MigrationAttemptSleepDuration = time.Nanosecond
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
		BeforeEach(func() {
			leaseController.AcquireSubnetLeaseAttempts = 10
			leaseController.CIDRPool = cidrPool
			leaseController.UnderlayIP = "10.244.5.6"
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

		Context("when checking if a subnet exists returns an error", func() {
			It("returns an error", func() {
				databaseHandler.SubnetExistsReturns(false, errors.New("guava"))

				_, err := leaseController.AcquireSubnetLease()
				Expect(err).To(MatchError("checking if subnet is available: guava"))

				Expect(databaseHandler.SubnetExistsCallCount()).To(Equal(10))
				Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))
			})
		})

		Context("when no subnets are free but checking if a subnet exists does not error", func() {
			BeforeEach(func() {
				databaseHandler.SubnetExistsReturns(true, nil)
			})
			It("eventually returns an error after failing to find a free subnet", func() {
				_, err := leaseController.AcquireSubnetLease()
				Expect(err).To(MatchError("unable to find a free subnet after 10 attempts"))

				Expect(databaseHandler.SubnetExistsCallCount()).To(Equal(100))
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

		Context("when a lease has already been assigned", func() {
			BeforeEach(func() {
				databaseHandler.SubnetForUnderlayIPReturns("10.255.76.0/24", nil)
			})

			It("gets the previously assigned lease", func() {
				_, err := leaseController.AcquireSubnetLease()
				Expect(err).NotTo(HaveOccurred())
				Expect(logger.Logs()[0].Data["subnet"]).To(Equal("10.255.76.0/24"))
				Expect(logger.Logs()[0].Data["underlay ip"]).To(Equal("10.244.5.6"))
				Expect(logger.Logs()[0].Message).To(Equal("test.subnet-renewed"))

				Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))
			})
		})

		Context("when checking for an existing lease fails", func() {
			BeforeEach(func() {
				databaseHandler.SubnetForUnderlayIPReturns("", fmt.Errorf("fruit"))
				databaseHandler.SubnetExistsReturns(false, nil)
			})
			It("ignores the error and tries to get a new lease", func() {
				cidrPool.GetRandomReturns("10.255.76.0/24")

				_, err := leaseController.AcquireSubnetLease()
				Expect(err).NotTo(HaveOccurred())
				Expect(logger.Logs()[0].Data["subnet"]).To(Equal("10.255.76.0/24"))
				Expect(logger.Logs()[0].Data["underlay ip"]).To(Equal("10.244.5.6"))
				Expect(logger.Logs()[0].Message).To(Equal("test.subnet-acquired"))

				Expect(databaseHandler.AddEntryCallCount()).To(Equal(1))
			})
		})
	})

	Describe("ReleaseSubnetLease", func() {
		BeforeEach(func() {
			leaseController.UnderlayIP = "10.244.5.6"
			databaseHandler.DeleteEntryReturns(nil)
		})
		It("deletes the lease", func() {
			err := leaseController.ReleaseSubnetLease()
			Expect(err).NotTo(HaveOccurred())

			Expect(databaseHandler.DeleteEntryCallCount()).To(Equal(1))
			Expect(databaseHandler.DeleteEntryArgsForCall(0)).To(Equal("10.244.5.6"))

			Expect(logger.Logs()[0].Data["underlay ip"]).To(Equal("10.244.5.6"))
			Expect(logger.Logs()[0].Message).To(Equal("test.subnet-released"))
		})
		Context("when the delete fails", func() {
			BeforeEach(func() {
				databaseHandler.DeleteEntryReturns(errors.New("banana"))
			})
			It("wraps the error from the database handler", func() {
				err := leaseController.ReleaseSubnetLease()
				Expect(err).To(MatchError("releasing lease: banana"))
			})
		})
	})
})
