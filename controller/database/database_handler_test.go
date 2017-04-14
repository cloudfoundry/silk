package database_test

import (
	"database/sql"
	"errors"
	"fmt"
	"math/rand"

	"code.cloudfoundry.org/go-db-helpers/db"
	"code.cloudfoundry.org/go-db-helpers/testsupport"

	"code.cloudfoundry.org/silk/controller"
	"code.cloudfoundry.org/silk/controller/database"
	"code.cloudfoundry.org/silk/controller/database/fakes"
	"github.com/jmoiron/sqlx"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	migrate "github.com/rubenv/sql-migrate"
)

var _ = Describe("DatabaseHandler", func() {
	var (
		databaseHandler    *database.DatabaseHandler
		realDb             *sqlx.DB
		realMigrateAdapter *database.MigrateAdapter
		testDatabase       *testsupport.TestDatabase
		mockDb             *fakes.Db
		mockMigrateAdapter *fakes.MigrateAdapter
		lease              *controller.Lease
		lease2             *controller.Lease
	)
	BeforeEach(func() {
		mockDb = &fakes.Db{}
		mockMigrateAdapter = &fakes.MigrateAdapter{}

		dbName := fmt.Sprintf("test_db_%03d_%x", GinkgoParallelNode(), rand.Int())
		dbConnectionInfo := testsupport.GetDBConnectionInfo()
		testDatabase = dbConnectionInfo.CreateDatabase(dbName)

		var err error
		realDb, err = db.GetConnectionPool(testDatabase.DBConfig())
		Expect(err).NotTo(HaveOccurred())

		realMigrateAdapter = &database.MigrateAdapter{}

		mockDb.DriverNameReturns(realDb.DriverName())
		lease = &controller.Lease{
			UnderlayIP:          "10.244.11.22",
			OverlaySubnet:       "10.255.17.0/24",
			OverlayHardwareAddr: "ee:ee:0a:ff:11:00",
		}
		lease2 = &controller.Lease{
			UnderlayIP:          "10.244.22.33",
			OverlaySubnet:       "10.255.93.15/32",
			OverlayHardwareAddr: "ee:ee:0a:ff:5d:0f",
		}
	})

	AfterEach(func() {
		if realDb != nil {
			Expect(realDb.Close()).To(Succeed())
		}
		if testDatabase != nil {
			testDatabase.Destroy()
		}
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
						&migrate.Migration{
							Id:   "1",
							Up:   []string{"CREATE TABLE IF NOT EXISTS subnets (  id SERIAL PRIMARY KEY,  underlay_ip varchar(15),  overlay_subnet varchar(18),  overlay_hwaddr varchar(17),  UNIQUE (underlay_ip),  UNIQUE (overlay_subnet) );"},
							Down: []string{"DROP TABLE subnets"},
						},
					},
				}))
			} else {
				Expect(migrations).To(Equal(migrate.MemoryMigrationSource{
					Migrations: []*migrate.Migration{
						&migrate.Migration{
							Id:   "1",
							Up:   []string{"CREATE TABLE IF NOT EXISTS subnets (  id int NOT NULL AUTO_INCREMENT, PRIMARY KEY (id),  underlay_ip varchar(15),  overlay_subnet varchar(18),  overlay_hwaddr varchar(17),  UNIQUE (underlay_ip),  UNIQUE (overlay_subnet) );"},
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
			databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
			mockDb.ExecReturns(nil, nil)
		})

		It("adds an entry to the DB", func() {
			err := databaseHandler.AddEntry(lease)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockDb.ExecCallCount()).To(Equal(1))
			Expect(mockDb.ExecArgsForCall(0)).To(Equal("INSERT INTO subnets (underlay_ip, overlay_subnet, overlay_hwaddr) VALUES ('10.244.11.22', '10.255.17.0/24', 'ee:ee:0a:ff:11:00')"))
		})

		Context("when the database exec returns an error", func() {
			BeforeEach(func() {
				mockDb.ExecReturns(nil, errors.New("apple"))
			})
			It("returns a sensible error", func() {
				err := databaseHandler.AddEntry(lease)
				Expect(err).To(MatchError("adding entry: apple"))

				Expect(mockDb.ExecCallCount()).To(Equal(1))
				Expect(mockDb.ExecArgsForCall(0)).To(Equal("INSERT INTO subnets (underlay_ip, overlay_subnet, overlay_hwaddr) VALUES ('10.244.11.22', '10.255.17.0/24', 'ee:ee:0a:ff:11:00')"))
			})
		})
	})

	Describe("DeleteEntry", func() {
		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
			mockDb.ExecReturns(nil, nil)
		})
		It("deletes an entry from the DB", func() {
			err := databaseHandler.DeleteEntry("some-underlay")
			Expect(err).NotTo(HaveOccurred())

			Expect(mockDb.ExecCallCount()).To(Equal(1))
			Expect(mockDb.ExecArgsForCall(0)).To(Equal("DELETE FROM subnets WHERE underlay_ip = 'some-underlay'"))
		})

		Context("when the database exec returns an error", func() {
			BeforeEach(func() {
				mockDb.ExecReturns(nil, errors.New("carrot"))
			})
			It("returns a sensible error", func() {
				err := databaseHandler.DeleteEntry("some-underlay")
				Expect(err).To(MatchError("deleting entry: carrot"))

				Expect(mockDb.ExecCallCount()).To(Equal(1))
				Expect(mockDb.ExecArgsForCall(0)).To(Equal("DELETE FROM subnets WHERE underlay_ip = 'some-underlay'"))
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
			Expect(found).To(Equal(lease))
		})

		Context("when there is no entry for the underlay ip", func() {
			It("returns an error", func() {
				_, err := databaseHandler.LeaseForUnderlayIP("10.244.11.23")
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
		})
		It("all the saved subnets", func() {
			leases, err := databaseHandler.All()
			Expect(err).NotTo(HaveOccurred())

			Expect(len(leases)).To(Equal(2))
			Expect(leases).To(ConsistOf([]controller.Lease{
				{
					UnderlayIP:          "10.244.11.22",
					OverlaySubnet:       "10.255.17.0/24",
					OverlayHardwareAddr: "ee:ee:0a:ff:11:00",
				},
				{
					UnderlayIP:          "10.244.22.33",
					OverlaySubnet:       "10.255.93.15/32",
					OverlayHardwareAddr: "ee:ee:0a:ff:5d:0f",
				},
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
				rows.Close()
			})

			It("returns an error", func() {
				_, err := databaseHandler.All()
				Expect(err.Error()).To(ContainSubstring("parsing result for all subnets"))
			})
		})

	})

	Describe("Release", func() {
		var leaseToRelease controller.Lease

		BeforeEach(func() {
			databaseHandler = database.NewDatabaseHandler(realMigrateAdapter, realDb)
			_, err := databaseHandler.Migrate()
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease)
			Expect(err).NotTo(HaveOccurred())
			err = databaseHandler.AddEntry(lease2)
			Expect(err).NotTo(HaveOccurred())

			leaseToRelease = *lease

			By("checking that the leaseToRelease is present")
			leases, err := databaseHandler.All()
			Expect(err).NotTo(HaveOccurred())
			Expect(leases).To(ContainElement(leaseToRelease))
		})

		It("removes the lease", func() {
			err := databaseHandler.Release(leaseToRelease)
			Expect(err).NotTo(HaveOccurred())

			leases, err := databaseHandler.All()
			Expect(err).NotTo(HaveOccurred())
			Expect(leases).NotTo(ContainElement(leaseToRelease))
		})

		Context("when the lease does not exist", func() {
			BeforeEach(func() {
				leaseToRelease = controller.Lease{
					UnderlayIP:          "10.244.22.33",
					OverlaySubnet:       "10.255.9.0/24",
					OverlayHardwareAddr: "ee:ee:0a:ff:5d:0f",
				}
			})

			It("returns a RecordNotAffectedError", func() {
				err := databaseHandler.Release(leaseToRelease)
				Expect(err).To(Equal(database.RecordNotAffectedError))

				leases, err := databaseHandler.All()
				Expect(err).NotTo(HaveOccurred())
				Expect(leases).To(ContainElement(*lease))
				Expect(leases).To(ContainElement(*lease2))
			})
		})

		Context("when the query fails", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				mockDb.ExecReturns(nil, errors.New("strawberry"))
			})
			It("returns an error", func() {
				err := databaseHandler.Release(leaseToRelease)
				Expect(err).To(MatchError("release lease: strawberry"))
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
				err := databaseHandler.Release(leaseToRelease)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("parse result: potato"))
			})
		})

		Context("when more than one row in result", func() {
			BeforeEach(func() {
				databaseHandler = database.NewDatabaseHandler(mockMigrateAdapter, mockDb)
				badResult := &fakes.SqlResult{}
				badResult.RowsAffectedReturns(2, nil)
				mockDb.ExecReturns(badResult, nil)
			})

			It("returns a MultipleRecordsAffectedError", func() {
				err := databaseHandler.Release(leaseToRelease)
				Expect(err).To(Equal(database.MultipleRecordsAffectedError))
			})
		})
	})
})

//go:generate counterfeiter -o fakes/sqlResult.go --fake-name SqlResult . sqlResult
type sqlResult interface {
	sql.Result
}
