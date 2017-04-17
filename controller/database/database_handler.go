package database

import (
	"database/sql"
	"errors"
	"fmt"

	"code.cloudfoundry.org/silk/controller"

	migrate "github.com/rubenv/sql-migrate"
)

var RecordNotAffectedError = errors.New("record not affected")
var MultipleRecordsAffectedError = errors.New("multiple records affected")

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
	rows, err := d.db.Query("SELECT underlay_ip, overlay_subnet, overlay_hwaddr FROM subnets")
	if err != nil {
		return nil, fmt.Errorf("selecting all subnets: %s", err)
	}
	for rows.Next() {
		var underlayIP, overlaySubnet, overlayHWAddr string
		err := rows.Scan(&underlayIP, &overlaySubnet, &overlayHWAddr)
		if err != nil {
			return nil, fmt.Errorf("parsing result for all subnets: %s", err)
		}
		leases = append(leases, controller.Lease{
			UnderlayIP:          underlayIP,
			OverlaySubnet:       overlaySubnet,
			OverlayHardwareAddr: overlayHWAddr,
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

func (d *DatabaseHandler) AddEntry(lease controller.Lease) error {
	var query string
	switch d.db.DriverName() {
	case "mysql":
		query = fmt.Sprintf("INSERT INTO subnets (underlay_ip, overlay_subnet, overlay_hwaddr, last_renewed_at) VALUES ('%s', '%s', '%s', UNIX_TIMESTAMP())", lease.UnderlayIP, lease.OverlaySubnet, lease.OverlayHardwareAddr)
	case "postgres":
		query = fmt.Sprintf("INSERT INTO subnets (underlay_ip, overlay_subnet, overlay_hwaddr, last_renewed_at) VALUES ('%s', '%s', '%s', EXTRACT(EPOCH FROM now())::numeric::integer)", lease.UnderlayIP, lease.OverlaySubnet, lease.OverlayHardwareAddr)
	default:
		return fmt.Errorf("database type %s is not supported", d.db.DriverName())
	}
	_, err := d.db.Exec(query)
	if err != nil {
		return fmt.Errorf("adding entry: %s", err)
	}
	return nil
}

func (d *DatabaseHandler) DeleteEntry(underlayIP string) error {
	result, err := d.db.Exec(fmt.Sprintf("DELETE FROM subnets WHERE underlay_ip = '%s'", underlayIP))
	if err != nil {
		return fmt.Errorf("deleting entry: %s", err)
	}
	nRows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("parse result: %s", err)
	}
	if nRows == 0 {
		return RecordNotAffectedError
	}
	if nRows > 1 {
		return MultipleRecordsAffectedError
	}
	return nil
}

func (d *DatabaseHandler) LeaseForUnderlayIP(underlayIP string) (*controller.Lease, error) {
	var overlaySubnet, overlayHWAddr string
	result := d.db.QueryRow(fmt.Sprintf("SELECT overlay_subnet, overlay_hwaddr FROM subnets WHERE underlay_ip = '%s'", underlayIP))
	err := result.Scan(&overlaySubnet, &overlayHWAddr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &controller.Lease{
		UnderlayIP:          underlayIP,
		OverlaySubnet:       overlaySubnet,
		OverlayHardwareAddr: overlayHWAddr,
	}, nil
}

func (d *DatabaseHandler) RenewLeaseForUnderlayIP(underlayIP string) error {
	var query string
	switch d.db.DriverName() {
	case "mysql":
		query = fmt.Sprintf("UPDATE subnets SET last_renewed_at = UNIX_TIMESTAMP() WHERE underlay_ip = '%s'", underlayIP)
	case "postgres":
		query = fmt.Sprintf("UPDATE subnets SET last_renewed_at = EXTRACT(EPOCH FROM now())::numeric::integer WHERE underlay_ip = '%s'", underlayIP)
	default:
		return fmt.Errorf("database type %s is not supported", d.db.DriverName())
	}
	_, err := d.db.Exec(query)
	if err != nil {
		return fmt.Errorf("renewing lease: %s", err)
	}
	return nil
}

func (d *DatabaseHandler) LastRenewedAtForUnderlayIP(underlayIP string) (int64, error) {
	var lastRenewedAt int64
	result := d.db.QueryRow(fmt.Sprintf("SELECT last_renewed_at FROM subnets WHERE underlay_ip = '%s'", underlayIP))
	err := result.Scan(&lastRenewedAt)
	if err != nil {
		return 0, err
	}
	return lastRenewedAt, nil
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
	baseCreateTable := "CREATE TABLE IF NOT EXISTS subnets (" +
		"%s" +
		", underlay_ip varchar(15) NOT NULL" +
		", overlay_subnet varchar(18) NOT NULL" +
		", overlay_hwaddr varchar(17) NOT NULL" +
		", last_renewed_at bigint NOT NULL" +
		", UNIQUE (underlay_ip)" +
		", UNIQUE (overlay_subnet)" +
		", UNIQUE (overlay_hwaddr)" +
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
