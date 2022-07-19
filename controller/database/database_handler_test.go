package database_test

import (
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/cf-networking-helpers/db"
	"code.cloudfoundry.org/cf-networking-helpers/testsupport"
	"code.cloudfoundry.org/lager/lagertest"

	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/database"
	"code.cloudfoundry.org/silk/controller/database/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	migrate "github.com/rubenv/sql-migrate"
)

var _ = Describe("DatabaseHandler", func() {
	var (
		databaseHandler    *database.DatabaseHandler
		realDb             *db.ConnWrapper
		realMigrateAdapter *database.MigrateAdapter
		dbConfig           db.Config
		mockDb             *fakes.Db
		mockMigrateAdapter *fakes.MigrateAdapter
		lease              controller.Lease
		lease2             controller.Lease
		singleIPLease      controller.Lease
		singleIPLease2     controller.Lease
	)
	BeforeEach(func() {
		mockDb = &fakes.Db{}
		mockMigrateAdapter = &fakes.MigrateAdapter{}

		dbConfig = testsupport.GetDBConfig()
		dbConfig.DatabaseName = fmt.Sprintf("test_db_%03d_%x", GinkgoParallelProcess(), rand.Int())
		testsupport.CreateDatabase(dbConfig)

		var err error
		realDb, err = db.NewConnectionPool(
			dbConfig,
			200,
			200,
			5*time.Minute,
			"controller",
			"database-handler",
			lagertest.NewTestLogger("test"),
		)
		Expect(err).NotTo(HaveOccurred())

		realMigrateAdapter = &database.MigrateAdapter{}

		mockDb.DriverNameReturns(realDb.DriverName())

		lease = controller.Lease{
			UnderlayIP:          "10.244.11.22",
			OverlaySubnet:       "10.255.17.0/24",
			OverlayHardwareAddr: "ee:ee:0a:ff:11:00",
		}
		lease2 = controller.Lease{
			UnderlayIP:          "10.244.22.33",
			OverlaySubnet:       "10.255.93.0/24",
			OverlayHardwareAddr: "ee:ee:0a:ff:5d:0f",
		}
		singleIPLease = controller.Lease{
			UnderlayIP:          "10.244.11.26",
			OverlaySubnet:       "10.255.0.12/32",
			OverlayHardwareAddr: "ee:ee:0a:ff:11:11",
		}
		singleIPLease2 = controller.Lease{
			UnderlayIP:          "10.244.11.28",
			OverlaySubnet:       "10.255.0.19/32",
			OverlayHardwareAddr: "ee:ee:0a:ff:11:12",
		}
	})

	AfterEach(func() {
		if realDb != nil {
			Expect(realDb.Close()).To(Succeed())
		}
		testsupport.RemoveDatabase(dbConfig)
	})

	Describe("Migrate", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
			mockMigrateAdapter.ExecReturns(43, nil)
		})
		It("calls the migrate adapter", func() {
			By("returning the results from the migrator")
			numMigrations, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
			Expect(numMigrations).To(Equal(43))

			By("calling the migrator")
			Expect(mockMigrateAdapter.ExecCallCount()).To(Equal(1))
			db, dbType, migrations, dir := mockMigrateAdapter.ExecArgsForCall(0)
			Expect(db).To(Equal(mockDb))
			Expect(dbType).To(Equal(realDb.DriverName()))
			if dbType == "postgres" {
				Expect(migrations).To(Equal(migrate.MemoryMigrationSource{
					Migrations: []*migrate.Migration{
						{
							Id:   "1",
							Up:   []string{"CREATE TABLE IF NOT EXISTS subnets (id SERIAL PRIMARY KEY, underlay_ip varchar(15) NOT NULL, overlay_subnet varchar(18) NOT NULL, overlay_hwaddr varchar(17) NOT NULL, last_renewed_at bigint NOT NULL, UNIQUE (underlay_ip), UNIQUE (overlay_subnet), UNIQUE (overlay_hwaddr));"},
							Down: []string{"DROP TABLE subnets"},
						},
					},
				}))
			} else {
				Expect(migrations).To(Equal(migrate.MemoryMigrationSource{
					Migrations: []*migrate.Migration{
						{
							Id:   "1",
							Up:   []string{"CREATE TABLE IF NOT EXISTS subnets (id int NOT NULL AUTO_INCREMENT, PRIMARY KEY (id), underlay_ip varchar(15) NOT NULL, overlay_subnet varchar(18) NOT NULL, overlay_hwaddr varchar(17) NOT NULL, last_renewed_at bigint NOT NULL, UNIQUE (underlay_ip), UNIQUE (overlay_subnet), UNIQUE (overlay_hwaddr));"},
							Down: []string{"DROP TABLE subnets"},
						},
					},
				}))
			}
			Expect(dir).To(Equal(migrate.Up))
		})
		Context("when the migrator fails", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockMigrateAdapter.ExecReturns(0, errors.New("guava"))
			})
			It("returns the error", func() {
				_, err := databaseHandler.Migrate()
				Expect(err).To(MatchError("migrating: guava"))
			})
		})
	})

	Describe("AddEntry", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("adds an entry to the DB", func() {
			err := databaseHandler.AddEntry(lease)
			Expect(err).NotTo(HaveOccurred())

			leases, err := databaseHandler.All()
			Expect(err).NotTo(HaveOccurred())
			Expect(leases).To(ContainElement(lease))
		})

		Context("when the database type is postgres", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.RebindReturns("INSERT INTO subnets (underlay_ip, overlay_subnet, overlay_hwaddr, last_renewed_at) VALUES ($1, $2, $3, EXTRACT(EPOCH FROM now())::numeric::integer)")
				mockDb.DriverNameReturns("postgres")
			})
			It("adds an entry to the DB", func() {
				err := databaseHandler.AddEntry(lease)
				Expect(err).NotTo(HaveOccurred())

				Expect(mockDb.ExecCallCount()).To(Equal(1))
				query, args := mockDb.ExecArgsForCall(0)
				Expect(mockDb.RebindArgsForCall(0)).To(Equal("INSERT INTO subnets (underlay_ip, overlay_subnet, overlay_hwaddr, last_renewed_at) VALUES (?, ?, ?, EXTRACT(EPOCH FROM now())::numeric::integer)"))
				Expect(query).To(Equal("INSERT INTO subnets (underlay_ip, overlay_subnet, overlay_hwaddr, last_renewed_at) VALUES ($1, $2, $3, EXTRACT(EPOCH FROM now())::numeric::integer)"))
				Expect(args).To(Equal([]interface{}{"10.244.11.22", "10.255.17.0/24", "ee:ee:0a:ff:11:00"}))
			})
		})

		Context("when the database type is mysql", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.DriverNameReturns("mysql")
				mockDb.RebindReturns("INSERT INTO subnets (underlay_ip, overlay_subnet, overlay_hwaddr, last_renewed_at) VALUES (?, ?, ?, UNIX_TIMESTAMP())")
			})
			It("adds an entry to the DB", func() {
				err := databaseHandler.AddEntry(lease)
				Expect(err).NotTo(HaveOccurred())

				Expect(mockDb.ExecCallCount()).To(Equal(1))
				query, args := mockDb.ExecArgsForCall(0)
				Expect(mockDb.RebindArgsForCall(0)).To(Equal("INSERT INTO subnets (underlay_ip, overlay_subnet, overlay_hwaddr, last_renewed_at) VALUES (?, ?, ?, UNIX_TIMESTAMP())"))
				Expect(query).To(Equal("INSERT INTO subnets (underlay_ip, overlay_subnet, overlay_hwaddr, last_renewed_at) VALUES (?, ?, ?, UNIX_TIMESTAMP())"))
				Expect(args).To(Equal([]interface{}{"10.244.11.22", "10.255.17.0/24", "ee:ee:0a:ff:11:00"}))
			})
		})

		Context("when the database type is not supported", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.DriverNameReturns("foo")
			})
			It("returns an error", func() {
				err := databaseHandler.RenewLeaseForUnderlayIP("1.2.3.4")
				Expect(err).To(MatchError("database type foo is not supported"))
			})
		})

		Context("when the database exec returns an error", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.ExecReturns(nil, errors.New("apple"))
			})
			It("returns a sensible error", func() {
				err := databaseHandler.AddEntry(lease)
				Expect(err).To(MatchError("adding entry: apple"))
			})
		})

	})

	Describe("DeleteEntry", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease)
			Expect(err).NotTo(HaveOccurred())

			By("checking that the lease is present")
			leases, err := databaseHandler.All()
			Expect(err).NotTo(HaveOccurred())
			Expect(leases).To(ContainElement(lease))
		})

		Context("when the database exec returns some other error", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.ExecReturns(nil, errors.New("carrot"))
				mockDb.RebindReturns("DELETE FROM subnets WHERE underlay_ip = $1")
				mockDb.DriverNameReturns("postgres")

			})
			It("returns a sensible error", func() {
				err := databaseHandler.DeleteEntry("some-underlay")
				Expect(err).To(MatchError("deleting entry: carrot"))

				Expect(mockDb.ExecCallCount()).To(Equal(1))

				query, args := mockDb.ExecArgsForCall(0)
				Expect(mockDb.RebindArgsForCall(0)).To(Equal("DELETE FROM subnets WHERE underlay_ip = ?"))
				Expect(query).To(Equal("DELETE FROM subnets WHERE underlay_ip = $1"))
				Expect(args).To(Equal([]interface{}{"some-underlay"}))
			})
		})

		Context("when the parsing the result fails", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				badResult := &fakes.SqlResult{}
				badResult.RowsAffectedReturns(0, errors.New("potato"))
				mockDb.ExecReturns(badResult, nil)
			})

			It("returns an error", func() {
				err := databaseHandler.DeleteEntry("10.244.11.22")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("parse result: potato"))
			})
		})

		It("deletes an entry from the DB", func() {
			err := databaseHandler.DeleteEntry("10.244.11.22")
			Expect(err).NotTo(HaveOccurred())

			By("checking that the lease is not present")
			leases, err := databaseHandler.All()
			Expect(err).NotTo(HaveOccurred())
			Expect(leases).NotTo(ContainElement(lease))
		})

		Context("when no entry exists", func() {
			It("returns a RecordNotAffectedError", func() {
				err := databaseHandler.DeleteEntry("8.8.8.8")
				Expect(err).To(Equal(database.RecordNotAffectedError))
			})
		})
	})

	Describe("LeaseForUnderlayIP", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease)
			Expect(err).NotTo(HaveOccurred())
		})
		It("returns the subnet for the given underlay IP", func() {
			found, err := databaseHandler.LeaseForUnderlayIP("10.244.11.22")
			Expect(err).NotTo(HaveOccurred())
			Expect(*found).To(Equal(lease))
		})

		Context("when there is no entry for the underlay ip", func() {
			It("returns nil", func() {
				entry, err := databaseHandler.LeaseForUnderlayIP("10.244.11.23")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry).To(BeNil())
			})
		})
	})

	Describe("RenewLeaseForUnderlayIP", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
		})

		Context("when the database is postgres", func() {
			BeforeEach(func() {
				mockDb.DriverNameReturns("postgres")
				mockDb.RebindReturns("UPDATE subnets SET last_renewed_at = EXTRACT(EPOCH FROM now())::numeric::integer WHERE underlay_ip = $1")
			})
			It("updates the last renewed at time", func() {
				err := databaseHandler.RenewLeaseForUnderlayIP("1.2.3.4")
				Expect(err).NotTo(HaveOccurred())

				Expect(mockDb.ExecCallCount()).To(Equal(1))
				query, args := mockDb.ExecArgsForCall(0)

				Expect(mockDb.RebindArgsForCall(0)).To(Equal("UPDATE subnets SET last_renewed_at = EXTRACT(EPOCH FROM now())::numeric::integer WHERE underlay_ip = ?"))
				Expect(query).To(Equal("UPDATE subnets SET last_renewed_at = EXTRACT(EPOCH FROM now())::numeric::integer WHERE underlay_ip = $1"))
				Expect(args).To(ContainElement("1.2.3.4"))
			})
		})

		Context("when the database is mysql", func() {
			BeforeEach(func() {
				mockDb.DriverNameReturns("mysql")
				mockDb.RebindReturns("UPDATE subnets SET last_renewed_at = UNIX_TIMESTAMP() WHERE underlay_ip = ?")
			})
			It("updates the last renewed at time", func() {
				err := databaseHandler.RenewLeaseForUnderlayIP("1.2.3.4")
				Expect(err).NotTo(HaveOccurred())

				query, args := mockDb.ExecArgsForCall(0)
				Expect(mockDb.RebindArgsForCall(0)).To(Equal("UPDATE subnets SET last_renewed_at = UNIX_TIMESTAMP() WHERE underlay_ip = ?"))
				Expect(query).To(Equal("UPDATE subnets SET last_renewed_at = UNIX_TIMESTAMP() WHERE underlay_ip = ?"))
				Expect(args).To(ContainElement("1.2.3.4"))
			})
		})

		Context("when the database type is not supported", func() {
			BeforeEach(func() {
				mockDb.DriverNameReturns("foo")
			})
			It("returns an error", func() {
				err := databaseHandler.RenewLeaseForUnderlayIP("1.2.3.4")
				Expect(err).To(MatchError("database type foo is not supported"))
			})
		})

		Context("when the database exec returns an error", func() {
			BeforeEach(func() {
				mockDb.ExecReturns(nil, errors.New("apple"))
			})
			It("returns a sensible error", func() {
				err := databaseHandler.RenewLeaseForUnderlayIP("1.2.3.4")
				Expect(err).To(MatchError("renewing lease: apple"))
			})
		})
	})

	Describe("LastRenewedAtForUnderlayIP", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease)
			Expect(err).NotTo(HaveOccurred())
		})
		It("selects the last_renewed_at time for the lease", func() {
			lastRenewedAt, err := databaseHandler.LastRenewedAtForUnderlayIP("10.244.11.22")
			Expect(err).NotTo(HaveOccurred())
			Expect(lastRenewedAt).To(BeNumerically(">", 0))
		})
		It("gets updated when the lease is renewed", func() {
			createdAt, err := databaseHandler.LastRenewedAtForUnderlayIP("10.244.11.22")
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(1 * time.Second)
			err = databaseHandler.RenewLeaseForUnderlayIP("10.244.11.22")
			Expect(err).NotTo(HaveOccurred())
			updatedAt, err := databaseHandler.LastRenewedAtForUnderlayIP("10.244.11.22")
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedAt).To(BeNumerically(">", createdAt))
		})

		Context("when there is no entry for the underlay ip", func() {
			It("returns an error", func() {
				_, err := databaseHandler.LastRenewedAtForUnderlayIP("10.244.11.23")
				Expect(err).To(MatchError("sql: no rows in result set"))
			})
		})
	})

	Describe("All", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease)
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease2)
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(singleIPLease)
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(singleIPLease2)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns all the saved subnets", func() {
			leases, err := databaseHandler.All()
			Expect(err).NotTo(HaveOccurred())

			Expect(len(leases)).To(Equal(4))
			Expect(leases).To(ConsistOf([]controller.Lease{
				lease,
				lease2,
				singleIPLease,
				singleIPLease2,
			}))
		})

		Context("when the query fails", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.QueryReturns(nil, errors.New("strawberry"))
			})
			It("returns an error", func() {
				_, err := databaseHandler.All()
				Expect(err).To(MatchError("selecting all subnets: strawberry"))
			})
		})

		Context("when the parsing the result fails", func() {
			var rows *sql.Rows
			BeforeEach(func() {
				var err error
				rows, err = realDb.Query("SELECT 1")
				Expect(err).NotTo(HaveOccurred())

				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.QueryReturns(rows, nil)
			})

			AfterEach(func() {
				Expect(rows.Close()).To(Succeed())
			})

			It("returns an error", func() {
				_, err := databaseHandler.All()
				Expect(err.Error()).To(ContainSubstring("selecting all subnets: parsing result"))
			})
		})
	})

	Describe("AllBlockSubnets", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease)
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease2)
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(singleIPLease)
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(singleIPLease2)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns all the saved subnets", func() {
			leases, err := databaseHandler.AllBlockSubnets()
			Expect(err).NotTo(HaveOccurred())

			Expect(len(leases)).To(Equal(2))
			Expect(leases).To(ConsistOf([]controller.Lease{
				lease,
				lease2,
			}))
		})

		Context("when the query fails", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.QueryReturns(nil, errors.New("strawberry"))
			})
			It("returns an error", func() {
				_, err := databaseHandler.AllBlockSubnets()
				Expect(err).To(MatchError("selecting all block subnets: strawberry"))
			})
		})

		Context("when the parsing the result fails", func() {
			var rows *sql.Rows
			BeforeEach(func() {
				var err error
				rows, err = realDb.Query("SELECT 1")
				Expect(err).NotTo(HaveOccurred())

				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.QueryReturns(rows, nil)
			})

			AfterEach(func() {
				Expect(rows.Close()).To(Succeed())
			})

			It("returns an error", func() {
				_, err := databaseHandler.AllBlockSubnets()
				Expect(err.Error()).To(ContainSubstring("selecting all block subnets: parsing result"))
			})
		})
	})

	Describe("AllSingleIPSubnets", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease)
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(singleIPLease)
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(singleIPLease2)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns all singleIP subnets", func() {
			leases, err := databaseHandler.AllSingleIPSubnets()
			Expect(err).NotTo(HaveOccurred())

			Expect(leases).To(HaveLen(2))
			Expect(leases).To(ConsistOf([]controller.Lease{
				singleIPLease,
				singleIPLease2,
			}))
		})

		Context("when the query fails", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.QueryReturns(nil, errors.New("strawberry"))
			})

			It("returns an error", func() {
				_, err := databaseHandler.AllSingleIPSubnets()
				Expect(err).To(MatchError("selecting all single ip subnets: strawberry"))
			})
		})

		Context("when the parsing the result fails", func() {
			var rows *sql.Rows
			BeforeEach(func() {
				var err error
				rows, err = realDb.Query("SELECT 1")
				Expect(err).NotTo(HaveOccurred())

				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.QueryReturns(rows, nil)
			})

			AfterEach(func() {
				Expect(rows.Close()).To(Succeed())
			})

			It("returns an error", func() {
				_, err := databaseHandler.AllSingleIPSubnets()
				Expect(err.Error()).To(ContainSubstring("selecting all single ip subnets: parsing result"))
			})
		})
	})

	Describe("AllActive", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease)
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease2)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the leases which have been renewed within the expiration time", func() {
			leases, err := databaseHandler.AllActive(1000)
			Expect(err).NotTo(HaveOccurred())

			Expect(leases).To(HaveLen(2))
			Expect(leases).To(ConsistOf([]controller.Lease{
				lease,
				lease2,
			}))

			leases, err = databaseHandler.AllActive(0)
			Expect(err).NotTo(HaveOccurred())

			Expect(leases).To(HaveLen(0))
		})

		Context("when the db driver name is not supported", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.DriverNameReturns("foo")
			})
			It("should return an error", func() {
				_, err := databaseHandler.AllActive(1000)
				Expect(err).To(MatchError("database type foo is not supported"))
			})
		})

		Context("when the query fails", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.QueryReturns(nil, errors.New("strawberry"))
			})
			It("returns an error", func() {
				_, err := databaseHandler.AllActive(100)
				Expect(err).To(MatchError("selecting all active subnets: strawberry"))
			})
		})

		Context("when the parsing the result fails", func() {
			var rows *sql.Rows
			BeforeEach(func() {
				var err error
				rows, err = realDb.Query("SELECT 1")
				Expect(err).NotTo(HaveOccurred())

				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.QueryReturns(rows, nil)
			})

			AfterEach(func() {
				Expect(rows.Close()).To(Succeed())
			})

			It("returns an error", func() {
				_, err := databaseHandler.AllActive(100)
				Expect(err.Error()).To(ContainSubstring("selecting all active subnets: parsing result"))
			})
		})
	})

	Describe("CheckDatabase", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("checks the database", func() {
			err := databaseHandler.CheckDatabase()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the connection to the database is closed", func() {
			BeforeEach(func() {
				if realDb != nil {
					Expect(realDb.Close()).To(Succeed())
				}
			})

			It("returns an error", func() {
				err := databaseHandler.CheckDatabase()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("OldestExpiredBlockSubnet", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(singleIPLease)
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease)
			Expect(err).NotTo(HaveOccurred())
		})

		It("gets the oldest lease that is expired", func() {
			expiredLease, err := databaseHandler.OldestExpiredBlockSubnet(0)
			Expect(err).NotTo(HaveOccurred())

			Expect(expiredLease).To(Equal(&lease))
		})

		Context("when the database type is not supported", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.DriverNameReturns("foo")
			})
			It("returns an error", func() {
				_, err := databaseHandler.OldestExpiredBlockSubnet(23)
				Expect(err).To(MatchError("database type foo is not supported"))
			})
		})

		Context("when no lease is expired", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
				_, err := databaseHandler.Migrate()
				Expect(err).NotTo(HaveOccurred())
			})
			It("returns nil and does not error", func() {
				lease, err := databaseHandler.OldestExpiredBlockSubnet(23)
				Expect(err).NotTo(HaveOccurred())
				Expect(lease).To(BeNil())
			})
		})

		Context("when parsing the result fails", func() {
			var result *sql.Row
			BeforeEach(func() {
				result = realDb.QueryRow("SELECT 1")

				mockDb.QueryRowReturns(result)
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
			})
			It("returns an error", func() {
				_, err := databaseHandler.OldestExpiredBlockSubnet(23)
				Expect(err).To(MatchError(ContainSubstring("scan result:")))
			})
		})
	})

	Describe("OldestExipredSingleIP", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease)
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(singleIPLease)
			Expect(err).NotTo(HaveOccurred())
		})

		It("gets the oldest lease that is expired", func() {
			expiredLease, err := databaseHandler.OldestExpiredSingleIP(0)
			Expect(err).NotTo(HaveOccurred())

			Expect(expiredLease).To(Equal(&singleIPLease))
		})

		Context("when the database type is not supported", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.DriverNameReturns("foo")
			})
			It("returns an error", func() {
				_, err := databaseHandler.OldestExpiredSingleIP(23)
				Expect(err).To(MatchError("database type foo is not supported"))
			})
		})

		Context("when no lease is expired", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
				_, err := databaseHandler.Migrate()
				Expect(err).NotTo(HaveOccurred())
			})
			It("returns nil and does not error", func() {
				lease, err := databaseHandler.OldestExpiredSingleIP(23)
				Expect(err).NotTo(HaveOccurred())
				Expect(lease).To(BeNil())
			})
		})

		Context("when parsing the result fails", func() {
			var result *sql.Row
			BeforeEach(func() {
				result = realDb.QueryRow("SELECT 1")

				mockDb.QueryRowReturns(result)
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
			})
			It("returns an error", func() {
				_, err := databaseHandler.OldestExpiredSingleIP(23)
				Expect(err).To(MatchError(ContainSubstring("scan result:")))
			})
		})
	})

	Describe("concurrent add and delete requests", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
		})
		It("remains consistent", func() {
			nLeases := 1000
			leases := []interface{}{}
			for i := 0; i < nLeases; i++ {
				leases = append(leases, controller.Lease{
					UnderlayIP:          fmt.Sprintf("underlay-%d", i),
					OverlaySubnet:       fmt.Sprintf("subnet-%d", i),
					OverlayHardwareAddr: fmt.Sprintf("hardware-%x", i),
				})
			}
			parallelRunner := &testsupport.ParallelRunner{
				NumWorkers: 4,
			}
			toDelete := make(chan (interface{}), nLeases)
			go func() {
				parallelRunner.RunOnSlice(leases, func(lease interface{}) {
					l := lease.(controller.Lease)
					Expect(databaseHandler.AddEntry(l)).To(Succeed())
					toDelete <- l
				})
				close(toDelete)
			}()

			var nDeleted int32
			parallelRunner.RunOnChannel(toDelete, func(lease interface{}) {
				l := lease.(controller.Lease)
				Expect(databaseHandler.DeleteEntry(l.UnderlayIP)).To(Succeed())
				atomic.AddInt32(&nDeleted, 1)
			})

			Expect(nDeleted).To(Equal(int32(nLeases)))

			allLeases, err := databaseHandler.All()
			Expect(err).NotTo(HaveOccurred())

			Expect(allLeases).To(BeEmpty())
		})
	})
})
