package lib

import (
	"database/sql"
	"fmt"

	"code.cloudfoundry.org/silk/controller"

	migrate "github.com/rubenv/sql-migrate"
)

//go:generate counterfeiter -o fakes/db.go --fake-name Db . Db
type Db interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	DriverName() string
}

//go:generate counterfeiter -o fakes/migrateAdapter.go --fake-name MigrateAdapter . migrateAdapter
type migrateAdapter interface {
	Exec(db Db, dialect string, m migrate.MigrationSource, dir migrate.MigrationDirection) (int, error)
}

type DatabaseHandler struct {
	migrator   migrateAdapter
	migrations *migrate.MemoryMigrationSource
	db         Db
}

func NewDatabaseHandler(migrator migrateAdapter, db Db) *DatabaseHandler {
	return &DatabaseHandler{
		migrator: migrator,
		migrations: &migrate.MemoryMigrationSource{
			Migrations: []*migrate.Migration{
				&migrate.Migration{
					Id:   "1",
					Up:   []string{createSubnetTable(db.DriverName())},
					Down: []string{"DROP TABLE subnets"},
				},
			},
		},
		db: db,
	}
}

func (d *DatabaseHandler) All() ([]controller.Lease, error) {
	leases := []controller.Lease{}
	rows, err := d.db.Query("SELECT underlay_ip, subnet FROM subnets")
	if err != nil {
		return nil, fmt.Errorf("selecting all subnets: %s", err)
	}
	for rows.Next() {
		var underlayIP, overlaySubnet string
		err := rows.Scan(&underlayIP, &overlaySubnet)
		if err != nil {
			return nil, fmt.Errorf("parsing result for all subnets: %s", err)
		}
		leases = append(leases, controller.Lease{
			UnderlayIP:    underlayIP,
			OverlaySubnet: overlaySubnet,
		})
	}

	return leases, nil
}

func (d *DatabaseHandler) Migrate() (int, error) {
	migrations := d.migrations
	numMigrations, err := d.migrator.Exec(d.db, d.db.DriverName(), *migrations, migrate.Up)
	if err != nil {
		return 0, fmt.Errorf("migrating: %s", err)
	}
	return numMigrations, nil
}

func (d *DatabaseHandler) AddEntry(underlayIP, subnet string) error {
	_, err := d.db.Exec(fmt.Sprintf("INSERT INTO subnets (underlay_ip, subnet) VALUES ('%s', '%s')", underlayIP, subnet))
	if err != nil {
		return fmt.Errorf("adding entry: %s", err)
	}
	return nil
}

func (d *DatabaseHandler) DeleteEntry(underlayIP string) error {
	_, err := d.db.Exec(fmt.Sprintf("DELETE FROM subnets WHERE underlay_ip = '%s'", underlayIP))
	if err != nil {
		return fmt.Errorf("deleting entry: %s", err)
	}
	return nil
}

func (d *DatabaseHandler) SubnetExists(subnet string) (bool, error) {
	var exists int
	err := d.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM subnets WHERE subnet = '%s'", subnet)).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("cannot get subnet: %s")
	}
	if exists == 1 {
		return true, nil
	} else {
		return false, nil
	}
}

func (d *DatabaseHandler) SubnetForUnderlayIP(underlayIP string) (string, error) {
	var subnet string
	result := d.db.QueryRow(fmt.Sprintf("SELECT subnet FROM subnets WHERE underlay_ip = '%s'", underlayIP))
	err := result.Scan(&subnet)
	if err != nil {
		return "", err
	}
	return subnet, nil
}

func createSubnetTable(dbType string) string {
	baseCreateTable := "CREATE TABLE IF NOT EXISTS subnets ( " +
		" %s, " +
		" underlay_ip varchar(15), " +
		" subnet varchar(18), " +
		" UNIQUE (underlay_ip), " +
		" UNIQUE (subnet) " +
		");"
	mysqlId := "id int NOT NULL AUTO_INCREMENT, PRIMARY KEY (id)"
	psqlId := "id SERIAL PRIMARY KEY"

	switch dbType {
	case "postgres":
		return fmt.Sprintf(baseCreateTable, psqlId)
	case "mysql":
		return fmt.Sprintf(baseCreateTable, mysqlId)
	}

	return ""
}
