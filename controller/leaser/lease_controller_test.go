package leaser_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"code.cloudfoundry.org/lager"
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
		validator                *fakes.LeaseValidator
		cidrPool                 *fakes.CIDRPool
		hardwareAddressGenerator *fakes.HardwareAddressGenerator
	)
	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		databaseHandler = &fakes.DatabaseHandler{}
		cidrPool = &fakes.CIDRPool{}
		hardwareAddressGenerator = &fakes.HardwareAddressGenerator{}
		validator = &fakes.LeaseValidator{}
		leaseController = leaser.LeaseController{
			DatabaseHandler:          databaseHandler,
			HardwareAddressGenerator: hardwareAddressGenerator,
			LeaseValidator:           validator,
			Logger:                   logger,
			LeaseExpirationSeconds:   42,
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
			Context("when there are no expired leases", func() {
				It("eventually returns an error after failing to find a free subnet", func() {
					lease, err := leaseController.AcquireSubnetLease("10.244.5.6")
					Expect(err).NotTo(HaveOccurred())
					Expect(lease).To(BeNil())

					Expect(databaseHandler.AllCallCount()).To(Equal(10))
					Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))

					Expect(databaseHandler.OldestExpiredCallCount()).To(Equal(10))
					Expect(databaseHandler.OldestExpiredArgsForCall(0)).To(Equal(42))
				})
			})
			Context("when there is an expired lease", func() {
				var expiredLease *controller.Lease

				BeforeEach(func() {
					expiredLease = &controller.Lease{
						UnderlayIP:          "10.244.5.60",
						OverlaySubnet:       "10.255.76.0/24",
						OverlayHardwareAddr: "ee:ee:0a:ff:4c:00",
					}
					databaseHandler.OldestExpiredReturns(expiredLease, nil)
				})

				It("Deletes the expired lease and assigns that lease's subnet", func() {
					lease, err := leaseController.AcquireSubnetLease("10.244.5.6")
					Expect(err).NotTo(HaveOccurred())
					Expect(lease).To(Equal(&controller.Lease{
						UnderlayIP:          "10.244.5.6",
						OverlaySubnet:       expiredLease.OverlaySubnet,
						OverlayHardwareAddr: expiredLease.OverlayHardwareAddr,
					}))

					Expect(databaseHandler.AllCallCount()).To(Equal(1))
					Expect(databaseHandler.AddEntryCallCount()).To(Equal(1))
					Expect(databaseHandler.DeleteEntryCallCount()).To(Equal(1))
					Expect(databaseHandler.DeleteEntryArgsForCall(0)).To(Equal(expiredLease.UnderlayIP))

					Expect(databaseHandler.OldestExpiredCallCount()).To(Equal(1))
					Expect(databaseHandler.OldestExpiredArgsForCall(0)).To(Equal(42))
				})

				Context("when getting the oldest expired lease returns an error", func() {
					BeforeEach(func() {
						databaseHandler.OldestExpiredReturns(nil, errors.New("guava"))
					})
					It("returns an error", func() {
						_, err := leaseController.AcquireSubnetLease("10.244.5.6")
						Expect(err).To(MatchError("get oldest expired: guava"))
					})
				})

				Context("when deleting the entry errors", func() {
					BeforeEach(func() {
						databaseHandler.DeleteEntryReturns(errors.New("guava"))
					})
					It("returns an error", func() {
						_, err := leaseController.AcquireSubnetLease("10.244.5.6")
						Expect(err).To(MatchError("delete expired subnet: guava"))
					})
				})
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
			var existingLease *controller.Lease
			BeforeEach(func() {
				existingLease = &controller.Lease{
					UnderlayIP:          "10.244.5.6",
					OverlaySubnet:       "10.255.76.0/24",
					OverlayHardwareAddr: "ee:ee:0a:ff:4c:00",
				}
				databaseHandler.LeaseForUnderlayIPReturns(existingLease, nil)
				cidrPool.IsMemberReturns(true)
			})

			It("gets the previously assigned lease", func() {
				lease, err := leaseController.AcquireSubnetLease("10.244.5.6")
				Expect(err).NotTo(HaveOccurred())
				Expect(lease).To(Equal(existingLease))

				Expect(logger.Logs()[0].Message).To(Equal("test.lease-renewed"))
				loggedLease, err := json.Marshal(logger.Logs()[0].Data["lease"])
				Expect(err).NotTo(HaveOccurred())
				Expect(loggedLease).To(MatchJSON(`{"underlay_ip":"10.244.5.6","overlay_subnet":"10.255.76.0/24","overlay_hardware_addr":"ee:ee:0a:ff:4c:00"}`))

				Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))
			})
		})

		Context("when a lease has already been assigned in a different network", func() {
			var existingLease *controller.Lease
			BeforeEach(func() {
				existingLease = &controller.Lease{
					UnderlayIP:          "10.244.5.6",
					OverlaySubnet:       "10.254.76.0/24",
					OverlayHardwareAddr: "ee:ee:0a:fe:4c:00",
				}
				databaseHandler.LeaseForUnderlayIPReturns(existingLease, nil)
				cidrPool.IsMemberReturns(false)
			})

			It("deletes the previously assigned lease and assigns a new one", func() {
				lease, err := leaseController.AcquireSubnetLease("10.244.5.6")
				Expect(err).NotTo(HaveOccurred())
				Expect(lease).NotTo(Equal(existingLease))

				Expect(logger.Logs()[0].Message).To(Equal("test.lease-deleted"))
				deletedLease, err := json.Marshal(logger.Logs()[0].Data["lease"])
				Expect(err).NotTo(HaveOccurred())
				Expect(deletedLease).To(MatchJSON(`{"underlay_ip":"10.244.5.6","overlay_subnet":"10.254.76.0/24","overlay_hardware_addr":"ee:ee:0a:fe:4c:00"}`))

				Expect(databaseHandler.DeleteEntryCallCount()).To(Equal(1))
				Expect(databaseHandler.AddEntryCallCount()).To(Equal(1))
			})

			Context("when deleting the existing entry fails", func() {
				BeforeEach(func() {
					databaseHandler.DeleteEntryReturns(fmt.Errorf("peanut"))
				})
				It("returns an error", func() {
					_, err := leaseController.AcquireSubnetLease("10.244.5.6")
					Expect(err).To(MatchError("deleting lease for underlay ip 10.244.5.6: peanut"))
					Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))
				})
			})
		})

		Context("when checking for an existing lease fails", func() {
			BeforeEach(func() {
				databaseHandler.LeaseForUnderlayIPReturns(nil, fmt.Errorf("fruit"))
			})
			It("returns an error", func() {
				_, err := leaseController.AcquireSubnetLease("10.244.5.6")
				Expect(err).To(MatchError("getting lease for underlay ip: fruit"))
				Expect(databaseHandler.AddEntryCallCount()).To(Equal(0))
			})
		})
	})

	Describe("RenewSubnetLease", func() {
		var leaseToRenew controller.Lease
		var lastRenewedAt int64
		BeforeEach(func() {
			leaseToRenew = controller.Lease{
				UnderlayIP:          "10.244.11.22",
				OverlaySubnet:       "10.255.33.0/24",
				OverlayHardwareAddr: "ee:ee:0a:ff:21:00",
			}
			databaseHandler.LeaseForUnderlayIPReturns(&leaseToRenew, nil)
			lastRenewedAt = 42
			databaseHandler.LastRenewedAtForUnderlayIPReturns(lastRenewedAt, nil)
		})

		It("renews a lease and logs the success", func() {
			err := leaseController.RenewSubnetLease(leaseToRenew)
			Expect(err).NotTo(HaveOccurred())

			Expect(databaseHandler.LeaseForUnderlayIPCallCount()).To(Equal(1))
			Expect(databaseHandler.LeaseForUnderlayIPArgsForCall(0)).To(Equal("10.244.11.22"))
			Expect(databaseHandler.RenewLeaseForUnderlayIPCallCount()).To(Equal(1))
			Expect(databaseHandler.RenewLeaseForUnderlayIPArgsForCall(0)).To(Equal("10.244.11.22"))
			Expect(databaseHandler.LastRenewedAtForUnderlayIPCallCount()).To(Equal(1))

			Expect(logger.Logs()).To(HaveLen(1))
			Expect(logger.Logs()[0].Message).To(Equal("test.lease-renewed"))
			Expect(logger.Logs()[0].LogLevel).To(Equal(lager.DEBUG))
			loggedLease, err := json.Marshal(logger.Logs()[0].Data["lease"])
			Expect(err).NotTo(HaveOccurred())
			Expect(loggedLease).To(MatchJSON(`{"underlay_ip":"10.244.11.22","overlay_subnet":"10.255.33.0/24","overlay_hardware_addr":"ee:ee:0a:ff:21:00"}`))
			Expect(int64(logger.Logs()[0].Data["last_renewed_at"].(float64))).To(Equal(lastRenewedAt))
		})

		Context("when the existing lease does not equal the one we are renewing", func() {
			BeforeEach(func() {
				existingLease := &controller.Lease{
					UnderlayIP:          leaseToRenew.UnderlayIP,
					OverlaySubnet:       "10.255.77.0/24",
					OverlayHardwareAddr: leaseToRenew.OverlayHardwareAddr,
				}
				databaseHandler.LeaseForUnderlayIPReturns(existingLease, nil)
			})
			It("returns a non-retriable error", func() {
				err := leaseController.RenewSubnetLease(leaseToRenew)
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(controller.NonRetriableError("")))
				Expect(err).To(MatchError("lease mismatch"))
			})
		})

		Context("when the existing lease does not exist", func() {
			BeforeEach(func() {
				databaseHandler.LeaseForUnderlayIPReturns(nil, nil)
			})
			It("adds the entry and logs the success", func() {
				err := leaseController.RenewSubnetLease(leaseToRenew)
				Expect(err).NotTo(HaveOccurred())

				Expect(databaseHandler.LeaseForUnderlayIPCallCount()).To(Equal(1))
				Expect(databaseHandler.LeaseForUnderlayIPArgsForCall(0)).To(Equal("10.244.11.22"))

				Expect(databaseHandler.AddEntryCallCount()).To(Equal(1))
				Expect(databaseHandler.AddEntryArgsForCall(0)).To(Equal(leaseToRenew))

				Expect(databaseHandler.RenewLeaseForUnderlayIPCallCount()).To(Equal(1))
				Expect(databaseHandler.RenewLeaseForUnderlayIPArgsForCall(0)).To(Equal("10.244.11.22"))
				Expect(databaseHandler.LastRenewedAtForUnderlayIPCallCount()).To(Equal(1))

				Expect(logger.Logs()).To(HaveLen(1))
				Expect(logger.Logs()[0].Message).To(Equal("test.lease-renewed"))
				Expect(logger.Logs()[0].LogLevel).To(Equal(lager.DEBUG))
				loggedLease, err := json.Marshal(logger.Logs()[0].Data["lease"])
				Expect(err).NotTo(HaveOccurred())
				Expect(loggedLease).To(MatchJSON(`{"underlay_ip":"10.244.11.22","overlay_subnet":"10.255.33.0/24","overlay_hardware_addr":"ee:ee:0a:ff:21:00"}`))
				Expect(int64(logger.Logs()[0].Data["last_renewed_at"].(float64))).To(Equal(lastRenewedAt))
			})

			Context("when adding the entry fails", func() {
				BeforeEach(func() {
					databaseHandler.AddEntryReturns(errors.New("pineapple"))
				})
				It("returns a non-retriable error", func() {
					err := leaseController.RenewSubnetLease(leaseToRenew)
					Expect(err).To(HaveOccurred())
					Expect(err).To(BeAssignableToTypeOf(controller.NonRetriableError("")))
					Expect(err).To(MatchError("pineapple"))
				})
			})
		})

		Context("when the lease is not valid", func() {
			BeforeEach(func() {
				validator.ValidateReturns(errors.New("banana"))
			})
			It("returns a non-retriable error", func() {
				err := leaseController.RenewSubnetLease(leaseToRenew)
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(controller.NonRetriableError("")))
				Expect(err).To(MatchError("banana"))
			})
		})

		Context("when checking the lease for the underlay ip fails", func() {
			BeforeEach(func() {
				databaseHandler.LeaseForUnderlayIPReturns(nil, errors.New("banana"))
			})
			It("returns an error", func() {
				err := leaseController.RenewSubnetLease(leaseToRenew)
				Expect(err).To(MatchError("getting lease for underlay ip: banana"))
			})
		})

		Context("when renewing the lease for the underlay ip fails", func() {
			BeforeEach(func() {
				databaseHandler.RenewLeaseForUnderlayIPReturns(errors.New("banana"))
			})
			It("returns an error", func() {
				err := leaseController.RenewSubnetLease(leaseToRenew)
				Expect(err).To(MatchError("renewing lease for underlay ip: banana"))
			})
		})

		Context("when getting the last renewed at time fails", func() {
			BeforeEach(func() {
				databaseHandler.LastRenewedAtForUnderlayIPReturns(0, errors.New("banana"))
			})
			It("returns an error", func() {
				err := leaseController.RenewSubnetLease(leaseToRenew)
				Expect(err).To(MatchError("getting last renewed at: banana"))
			})
		})
	})

	Describe("ReleaseSubnetLease", func() {
		var (
			underlayIP string
		)
		BeforeEach(func() {
			underlayIP = "10.244.5.0"
		})
		It("releases the lease", func() {
			err := leaseController.ReleaseSubnetLease(underlayIP)
			Expect(err).NotTo(HaveOccurred())

			Expect(databaseHandler.DeleteEntryCallCount()).To(Equal(1))
			Expect(databaseHandler.DeleteEntryArgsForCall(0)).To(Equal(underlayIP))

			Expect(logger.Logs()).To(HaveLen(1))
			Expect(logger.Logs()[0].Data["underlay_ip"]).To(Equal("10.244.5.0"))
			Expect(logger.Logs()[0].Message).To(Equal("test.lease-released"))
		})

		Context("when the database returns RecordNotAffectedError", func() {
			BeforeEach(func() {
				databaseHandler.DeleteEntryReturns(database.RecordNotAffectedError)
			})
			It("swallows the error and logs it at DEBUG level", func() {
				err := leaseController.ReleaseSubnetLease(underlayIP)
				Expect(err).NotTo(HaveOccurred())

				Expect(logger.Logs()).To(HaveLen(1))
				Expect(logger.Logs()[0].Message).To(Equal("test.lease-not-found"))
				Expect(logger.Logs()[0].Data).To(HaveKeyWithValue("underlay_ip", "10.244.5.0"))
				Expect(logger.Logs()[0].LogLevel).To(Equal(lager.DEBUG))
			})
		})

		Context("when the database returns some other error", func() {
			BeforeEach(func() {
				databaseHandler.DeleteEntryReturns(errors.New("banana"))
			})
			It("wraps the error from the database handler", func() {
				err := leaseController.ReleaseSubnetLease(underlayIP)
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
			databaseHandler.AllActiveReturns(activeLeases, nil)
		})
		It("returns all the active subnet leases", func() {
			leases, err := leaseController.RoutableLeases()
			Expect(err).NotTo(HaveOccurred())
			Expect(databaseHandler.AllActiveCallCount()).To(Equal(1))
			Expect(databaseHandler.AllActiveArgsForCall(0)).To(Equal(42))
			Expect(leases).To(Equal(activeLeases))
		})

		Context("when getting the leases fails", func() {
			BeforeEach(func() {
				databaseHandler.AllActiveReturns(nil, errors.New("cupcake"))
			})
			It("wraps the error from the database handler", func() {
				_, err := leaseController.RoutableLeases()
				Expect(err).To(MatchError("getting all leases: cupcake"))
			})
		})
	})
})
