package lib_test

import (
	"errors"
	"fmt"
	"math/rand"

	"code.cloudfoundry.org/go-db-helpers/db"
	"code.cloudfoundry.org/go-db-helpers/testsupport"

	"code.cloudfoundry.org/silk/daemon/lib"
	"code.cloudfoundry.org/silk/daemon/lib/fakes"
	"github.com/jmoiron/sqlx"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	migrate "github.com/rubenv/sql-migrate"
)

var _ = Describe("DatabaseHandler", func() {
	var (
		databaseHandler    *lib.DatabaseHandler
		realDb             *sqlx.DB
		realMigrateAdapter *lib.MigrateAdapter
		testDatabase       *testsupport.TestDatabase
		mockDb             *fakes.Db
		mockMigrateAdapter *fakes.MigrateAdapter
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

		realMigrateAdapter = &lib.MigrateAdapter{}

		mockDb.DriverNameReturns(realDb.DriverName())
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
			databaseHandler = lib.NewDatabaseHandler(mockMigrateAdapter, mockDb)
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
							Up:   []string{"CREATE TABLE IF NOT EXISTS subnets (  id SERIAL PRIMARY KEY,  underlay_ip varchar(15),  subnet varchar(18),  UNIQUE (underlay_ip),  UNIQUE (subnet) );"},
							Down: []string{"DROP TABLE subnets"},
						},
					},
				}))
			} else {
				Expect(migrations).To(Equal(migrate.MemoryMigrationSource{
					Migrations: []*migrate.Migration{
						&migrate.Migration{
							Id:   "1",
							Up:   []string{"CREATE TABLE IF NOT EXISTS subnets (  id int NOT NULL AUTO_INCREMENT, PRIMARY KEY (id),  underlay_ip varchar(15),  subnet varchar(18),  UNIQUE (underlay_ip),  UNIQUE (subnet) );"},
							Down: []string{"DROP TABLE subnets"},
						},
					},
				}))
			}
			Expect(dir).To(Equal(migrate.Up))
		})
		Context("when the migrator fails", func() {
			BeforeEach(func() {
				databaseHandler = lib.NewDatabaseHandler(mockMigrateAdapter, mockDb)
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
			databaseHandler = lib.NewDatabaseHandler(mockMigrateAdapter, mockDb)
			mockDb.ExecReturns(nil, nil)
		})

		It("adds an entry to the DB", func() {
			err := databaseHandler.AddEntry("some-underlay", "some-subnet")
			Expect(err).NotTo(HaveOccurred())

			Expect(mockDb.ExecCallCount()).To(Equal(1))
			Expect(mockDb.ExecArgsForCall(0)).To(Equal("INSERT INTO subnets (underlay_ip, subnet) VALUES ('some-underlay', 'some-subnet')"))
		})

		Context("when the database exec returns an error", func() {
			BeforeEach(func() {
				mockDb.ExecReturns(nil, errors.New("apple"))
			})
			It("returns a sensible error", func() {
				err := databaseHandler.AddEntry("some-underlay", "some-subnet")
				Expect(err).To(MatchError("adding entry: apple"))

				Expect(mockDb.ExecCallCount()).To(Equal(1))
				Expect(mockDb.ExecArgsForCall(0)).To(Equal("INSERT INTO subnets (underlay_ip, subnet) VALUES ('some-underlay', 'some-subnet')"))
			})
		})
	})

	Describe("DeleteEntry", func() {
		BeforeEach(func() {
			databaseHandler = lib.NewDatabaseHandler(mockMigrateAdapter, mockDb)
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

	Describe("SubnetExists", func() {
		BeforeEach(func() {
			databaseHandler = lib.NewDatabaseHandler(realMigrateAdapter, realDb)
		})

		Context("when subnets table has not been created", func() {
			It("returns false and an error", func() {
				_, err := databaseHandler.SubnetExists("any-subnet")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cannot get subnet"))
			})
		})

		Context("when subnets table has been created", func() {
			BeforeEach(func() {
				_, err := databaseHandler.Migrate()
				Expect(err).NotTo(HaveOccurred())
			})
			Context("when no leases exist", func() {
				It("returns false but no error", func() {
					exists, err := databaseHandler.SubnetExists("any-subnet")
					Expect(err).NotTo(HaveOccurred())
					Expect(exists).To(BeFalse())
				})
			})
			Context("when the lease exists", func() {
				BeforeEach(func() {
					databaseHandler.AddEntry("some-underlay", "some-subnet")
				})
				It("returns true and no error", func() {
					exists, err := databaseHandler.SubnetExists("some-subnet")
					Expect(err).NotTo(HaveOccurred())
					Expect(exists).To(BeTrue())
				})
			})
		})
	})
})
