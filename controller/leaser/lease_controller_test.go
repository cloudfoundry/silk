package leaser_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"code.cloudfoundry.org/lager/lagertest"

	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/leaser"
	"code.cloudfoundry.org/silk/controller/leaser/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LeaseController", func() {
	var (
		logger                   *lagertest.TestLogger
		databaseHandler          *fakes.DatabaseHandler
		leaseController          leaser.LeaseController
		cidrPool                 *fakes.CIDRPool
		hardwareAddressGenerator *fakes.HardwareAddressGenerator
	)
	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		databaseHandler = &fakes.DatabaseHandler{}
		cidrPool = &fakes.CIDRPool{}
		hardwareAddressGenerator = &fakes.HardwareAddressGenerator{}
		leaseController = leaser.LeaseController{
			DatabaseHandler:          databaseHandler,
			HardwareAddressGenerator: hardwareAddressGenerator,
			Logger: logger,
		}
		hardwareAddressGenerator.GenerateForVTEPReturns(
			net.HardwareAddr{0xee, 0xee, 0x0a, 0xff, 0x4c, 0x00}, nil,
		)
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
			databaseHandler.AllReturns([]controller.Lease{
				{UnderlayIP: "10.244.11.22", OverlaySubnet: "10.255.33.0/24"},
				{UnderlayIP: "10.244.22.33", OverlaySubnet: "10.255.44.0/24"},
			}, nil)
			cidrPool.GetAvailableReturns("10.255.76.0/24", nil)
		})

		It("acquires a lease and logs the success", func() {
			lease, err := leaseController.AcquireSubnetLease("10.244.5.6")
			Expect(err).NotTo(HaveOccurred())
			Expect(lease.UnderlayIP).To(Equal("10.244.5.6"))
			Expect(lease.OverlaySubnet).To(Equal("10.255.76.0/24"))
			Expect(lease.OverlayHardwareAddr).To(Equal("ee:ee:0a:ff:4c:00"))
			Expect(logger.Logs()[0].Message).To(Equal("test.lease-acquired"))

			loggedLease, err := json.Marshal(logger.Logs()[0].Data["lease"])
			Expect(err).NotTo(HaveOccurred())
			Expect(loggedLease).To(MatchJSON(`{"underlay_ip":"10.244.5.6","overlay_subnet":"10.255.76.0/24","overlay_hardware_addr":"ee:ee:0a:ff:4c:00"}`))

			Expect(databaseHandler.AllCallCount()).To(Equal(1))
			Expect(cidrPool.GetAvailableCallCount()).To(Equal(1))
			Expect(cidrPool.GetAvailableArgsForCall(0)).To(Equal([]string{"10.255.33.0/24", "10.255.44.0/24"}))
			Expect(databaseHandler.AddEntryCallCount()).To(Equal(1))

			savedLease := databaseHandler.AddEntryArgsForCall(0)
			Expect(savedLease.UnderlayIP).To(Equal("10.244.5.6"))
			Expect(savedLease.OverlaySubnet).To(Equal("10.255.76.0/24"))
			Expect(savedLease.OverlayHardwareAddr).To(Equal("ee:ee:0a:ff:4c:00"))
		})

		Context("when getting all taken subnets returns an error", func() {
			It("returns an error", func() {
				databaseHandler.AllReturns(nil, errors.New("guava"))

				_, err := leaseController.AcquireSubnetLease("10.244.5.6")
				Expect(err).To(MatchError("getting all subnets: guava"))

				Expect(databaseHandler.AllCallCount()).To(Equal(10))
				Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))
			})
		})

		Context("when no subnets are free but getting all subnets does not error", func() {
			BeforeEach(func() {
				cidrPool.GetAvailableReturns("", errors.New("pineapple"))
			})
			It("eventually returns an error after failing to find a free subnet", func() {
				_, err := leaseController.AcquireSubnetLease("10.244.5.6")
				Expect(err).To(MatchError("get available subnet: pineapple"))

				Expect(databaseHandler.AllCallCount()).To(Equal(10))
				Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))
			})
		})

		Context("when the subnet is an invalid CIDR", func() {
			BeforeEach(func() {
				cidrPool.GetAvailableReturns("foo", nil)
			})
			It("eventually returns an error after failing to find a free subnet", func() {
				_, err := leaseController.AcquireSubnetLease("10.244.5.6")
				Expect(err).To(MatchError("parse subnet: invalid CIDR address: foo"))

				Expect(databaseHandler.AllCallCount()).To(Equal(10))
				Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))
			})
		})

		Context("when generating the hardware address fails", func() {
			BeforeEach(func() {
				hardwareAddressGenerator.GenerateForVTEPReturns(nil, errors.New("guava"))
			})
			It("eventually returns an error after failing to find a free subnet", func() {
				_, err := leaseController.AcquireSubnetLease("10.244.5.6")
				Expect(err).To(MatchError("generate hardware address: guava"))

				Expect(databaseHandler.AllCallCount()).To(Equal(10))
				Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))
			})
		})

		Context("when adding the lease entry fails", func() {
			It("returns an error", func() {
				databaseHandler.AddEntryReturns(errors.New("guava"))

				_, err := leaseController.AcquireSubnetLease("10.244.5.6")
				Expect(err).To(MatchError("adding lease entry: guava"))

				Expect(databaseHandler.AddEntryCallCount()).To(Equal(10))
			})
		})

		Context("when a lease has already been assigned", func() {
			BeforeEach(func() {
				existingLease := &controller.Lease{
					UnderlayIP:          "10.244.5.6",
					OverlaySubnet:       "10.255.76.0/24",
					OverlayHardwareAddr: "ee:ee:0a:ff:4c:00",
				}
				databaseHandler.LeaseForUnderlayIPReturns(existingLease, nil)
			})

			It("gets the previously assigned lease", func() {
				_, err := leaseController.AcquireSubnetLease("10.244.5.6")
				Expect(err).NotTo(HaveOccurred())
				Expect(logger.Logs()[0].Message).To(Equal("test.lease-renewed"))
				loggedLease, err := json.Marshal(logger.Logs()[0].Data["lease"])
				Expect(err).NotTo(HaveOccurred())
				Expect(loggedLease).To(MatchJSON(`{"underlay_ip":"10.244.5.6","overlay_subnet":"10.255.76.0/24","overlay_hardware_addr":"ee:ee:0a:ff:4c:00"}`))

				Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))
			})
		})

		Context("when checking for an existing lease fails", func() {
			BeforeEach(func() {
				databaseHandler.LeaseForUnderlayIPReturns(nil, fmt.Errorf("fruit"))
			})
			It("ignores the error and tries to get a new lease", func() {
				cidrPool.GetAvailableReturns("10.255.76.0/24", nil)

				_, err := leaseController.AcquireSubnetLease("10.244.5.6")
				Expect(err).NotTo(HaveOccurred())

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

	Describe("RoutableLeases", func() {
		activeLeases := []controller.Lease{
			{
				UnderlayIP:    "10.244.5.9",
				OverlaySubnet: "10.255.16.0/24",
			},
			{
				UnderlayIP:    "10.244.22.33",
				OverlaySubnet: "10.255.75.0/32",
			},
		}
		BeforeEach(func() {
			databaseHandler.AllReturns(activeLeases, nil)
		})
		It("returns all the subnet leases", func() {
			leases, err := leaseController.RoutableLeases()
			Expect(err).NotTo(HaveOccurred())
			Expect(databaseHandler.AllCallCount()).To(Equal(1))
			Expect(leases).To(Equal(activeLeases))
		})

		Context("when getting the leases fails", func() {
			BeforeEach(func() {
				databaseHandler.AllReturns(nil, errors.New("cupcake"))
			})
			It("wraps the error from the database handler", func() {
				_, err := leaseController.RoutableLeases()
				Expect(err).To(MatchError("getting all leases: cupcake"))
			})
		})
	})
})
