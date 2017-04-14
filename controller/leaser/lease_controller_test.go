package leaser_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"code.cloudfoundry.org/lager/lagertest"

	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/database"
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

	Describe("AcquireSubnetLease", func() {
		BeforeEach(func() {
			leaseController.AcquireSubnetLeaseAttempts = 10
			leaseController.CIDRPool = cidrPool
			databaseHandler.AllReturns([]controller.Lease{
				{UnderlayIP: "10.244.11.22", OverlaySubnet: "10.255.33.0/24"},
				{UnderlayIP: "10.244.22.33", OverlaySubnet: "10.255.44.0/24"},
			}, nil)
			cidrPool.GetAvailableReturns("10.255.76.0/24")
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

		Context("when no subnets are free", func() {
			BeforeEach(func() {
				cidrPool.GetAvailableReturns("")
			})
			It("eventually returns an error after failing to find a free subnet", func() {
				lease, err := leaseController.AcquireSubnetLease("10.244.5.6")
				Expect(err).NotTo(HaveOccurred())
				Expect(lease).To(BeNil())

				Expect(databaseHandler.AllCallCount()).To(Equal(10))
				Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))
			})
		})

		Context("when the underlay ip is not an IPv4 addr", func() {
			It("returns an error", func() {
				_, err := leaseController.AcquireSubnetLease("banana")
				Expect(err).To(MatchError("invalid ipv4 address: banana"))
			})
		})

		Context("when the subnet is an invalid CIDR", func() {
			BeforeEach(func() {
				cidrPool.GetAvailableReturns("foo")
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
				cidrPool.GetAvailableReturns("10.255.76.0/24")

				_, err := leaseController.AcquireSubnetLease("10.244.5.6")
				Expect(err).NotTo(HaveOccurred())

				Expect(databaseHandler.AddEntryCallCount()).To(Equal(1))
			})
		})
	})

	Describe("ReleaseSubnetLease", func() {
		var (
			lease controller.Lease
		)
		BeforeEach(func() {
			lease = controller.Lease{
				UnderlayIP:          "10.244.5.0",
				OverlaySubnet:       "10.255.7.0/24",
				OverlayHardwareAddr: "ee:ee:0a:ff:07:00",
			}

			databaseHandler.ReleaseReturns(nil)
		})
		It("releases the lease", func() {
			err := leaseController.ReleaseSubnetLease(lease)
			Expect(err).NotTo(HaveOccurred())

			Expect(databaseHandler.ReleaseCallCount()).To(Equal(1))
			Expect(databaseHandler.ReleaseArgsForCall(0)).To(Equal(lease))

			Expect(logger.Logs()).To(HaveLen(1))
			Expect(logger.Logs()[0].Data["lease"]).To(HaveKeyWithValue("underlay_ip", "10.244.5.0"))
			Expect(logger.Logs()[0].Data["lease"]).To(HaveKeyWithValue("overlay_subnet", "10.255.7.0/24"))
			Expect(logger.Logs()[0].Data["lease"]).To(HaveKeyWithValue("overlay_hardware_addr", "ee:ee:0a:ff:07:00"))
			Expect(logger.Logs()[0].Message).To(Equal("test.lease-released"))
		})

		Context("when the database returns RecordNotAffectedError", func() {
			BeforeEach(func() {
				databaseHandler.ReleaseReturns(database.RecordNotAffectedError)
			})
			It("logs but does not error", func() {
				err := leaseController.ReleaseSubnetLease(lease)
				Expect(err).NotTo(HaveOccurred())

				Expect(logger.Logs()).To(HaveLen(1))
				Expect(logger.Logs()[0].Data["lease"]).To(HaveKeyWithValue("underlay_ip", "10.244.5.0"))
				Expect(logger.Logs()[0].Data["lease"]).To(HaveKeyWithValue("overlay_subnet", "10.255.7.0/24"))
				Expect(logger.Logs()[0].Data["lease"]).To(HaveKeyWithValue("overlay_hardware_addr", "ee:ee:0a:ff:07:00"))
				Expect(logger.Logs()[0].Message).To(Equal("test.lease-not-found"))
			})
		})

		Context("when the database returns MultipleRecordsAffectedError", func() {
			BeforeEach(func() {
				databaseHandler.ReleaseReturns(database.MultipleRecordsAffectedError)
			})
			It("logs but does not error", func() {
				err := leaseController.ReleaseSubnetLease(lease)
				Expect(err).NotTo(HaveOccurred())

				Expect(logger.Logs()).To(HaveLen(1))
				Expect(logger.Logs()[0].Data["lease"]).To(HaveKeyWithValue("underlay_ip", "10.244.5.0"))
				Expect(logger.Logs()[0].Data["lease"]).To(HaveKeyWithValue("overlay_subnet", "10.255.7.0/24"))
				Expect(logger.Logs()[0].Data["lease"]).To(HaveKeyWithValue("overlay_hardware_addr", "ee:ee:0a:ff:07:00"))
				Expect(logger.Logs()[0].Message).To(Equal("test.multiple-leases-deleted"))
			})
		})

		Context("when the database returns some other error", func() {
			BeforeEach(func() {
				databaseHandler.ReleaseReturns(errors.New("banana"))
			})
			It("wraps the error from the database handler", func() {
				err := leaseController.ReleaseSubnetLease(lease)
				Expect(err).To(MatchError("release lease: banana"))
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
