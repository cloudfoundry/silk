package database

import (
	"database/sql"
	"errors"
	"fmt"

	"code.cloudfoundry.org/silk/controller"
	migrate "github.com/rubenv/sql-migrate"
)

const postgresTimeNow = "EXTRACT(EPOCH FROM now())::numeric::integer"
const mysqlTimeNow = "UNIX_TIMESTAMP()"
const MySQL = "mysql"
const Postgres = "postgres"

var RecordNotAffectedError = errors.New("record not affected")

//go:generate counterfeiter -o fakes/db.go --fake-name Db . Db
type Db interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Rebind(query string) string
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
				{
					Id:   "1",
					Up:   []string{createSubnetTable(db.DriverName())},
					Down: []string{"DROP TABLE subnets"},
				},
			},
		},
		db: db,
	}
}

func (d *DatabaseHandler) CheckDatabase() error {
	var result int
	return d.db.QueryRow("SELECT 1").Scan(&result)
}

func (d *DatabaseHandler) All() ([]controller.Lease, error) {
	rows, err := d.db.Query("SELECT underlay_ip, overlay_subnet, overlay_hwaddr FROM subnets")
	if err != nil {
		return nil, fmt.Errorf("selecting all subnets: %s", err)
	}
	defer rows.Close() // untested
	leases, err := rowsToLeases(rows)
	if err != nil {
		return nil, fmt.Errorf("selecting all subnets: %s", err)
	}

	return leases, nil
}

func (d *DatabaseHandler) AllSingleIPSubnets() ([]controller.Lease, error) {
	rows, err := d.db.Query("SELECT underlay_ip, overlay_subnet, overlay_hwaddr FROM subnets WHERE overlay_subnet LIKE '%/32'")
	if err != nil {
		return nil, fmt.Errorf("selecting all single ip subnets: %s", err)
	}
	defer rows.Close() // untested
	leases, err := rowsToLeases(rows)
	if err != nil {
		return nil, fmt.Errorf("selecting all single ip subnets: %s", err)
	}

	return leases, nil
}

func (d *DatabaseHandler) AllBlockSubnets() ([]controller.Lease, error) {
	rows, err := d.db.Query("SELECT underlay_ip, overlay_subnet, overlay_hwaddr FROM subnets WHERE overlay_subnet NOT LIKE '%/32'")
	if err != nil {
		return nil, fmt.Errorf("selecting all block subnets: %s", err)
	}
	defer rows.Close() // untested
	leases, err := rowsToLeases(rows)
	if err != nil {
		return nil, fmt.Errorf("selecting all block subnets: %s", err)
	}

	return leases, nil
}

func (d *DatabaseHandler) AllActive(duration int) ([]controller.Lease, error) {
	timestamp, err := timestampForDriver(d.db.DriverName())
	if err != nil {
		return nil, err
	}
	rows, err := d.db.Query(fmt.Sprintf("SELECT underlay_ip, overlay_subnet, overlay_hwaddr FROM subnets WHERE last_renewed_at + %d > %s", duration, timestamp))
	if err != nil {
		return nil, fmt.Errorf("selecting all active subnets: %s", err)
	}
	defer rows.Close() // untested
	leases, err := rowsToLeases(rows)
	if err != nil {
		return nil, fmt.Errorf("selecting all active subnets: %s", err)
	}

	return leases, nil
}

func (d *DatabaseHandler) OldestExpiredBlockSubnet(expirationTime int) (*controller.Lease, error) {
	timestamp, err := timestampForDriver(d.db.DriverName())
	if err != nil {
		return nil, err
	}

	var underlayIP, overlaySubnet, overlayHWAddr string
	result := d.db.QueryRow(fmt.Sprintf("SELECT underlay_ip, overlay_subnet, overlay_hwaddr FROM subnets WHERE overlay_subnet NOT LIKE '%%/32' AND last_renewed_at + %d <= %s ORDER BY last_renewed_at ASC LIMIT 1", expirationTime, timestamp))
	err = result.Scan(&underlayIP, &overlaySubnet, &overlayHWAddr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan result: %s", err)
	}
	return &controller.Lease{
		UnderlayIP:          underlayIP,
		OverlaySubnet:       overlaySubnet,
		OverlayHardwareAddr: overlayHWAddr,
	}, nil
}

func (d *DatabaseHandler) OldestExpiredSingleIP(expirationTime int) (*controller.Lease, error) {
	timestamp, err := timestampForDriver(d.db.DriverName())
	if err != nil {
		return nil, err
	}

	var underlayIP, overlaySubnet, overlayHWAddr string
	result := d.db.QueryRow(fmt.Sprintf("SELECT underlay_ip, overlay_subnet, overlay_hwaddr FROM subnets WHERE overlay_subnet LIKE '%%/32' AND last_renewed_at + %d <= %s ORDER BY last_renewed_at ASC LIMIT 1", expirationTime, timestamp))
	err = result.Scan(&underlayIP, &overlaySubnet, &overlayHWAddr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan result: %s", err)
	}
	return &controller.Lease{
		UnderlayIP:          underlayIP,
		OverlaySubnet:       overlaySubnet,
		OverlayHardwareAddr: overlayHWAddr,
	}, nil
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
	timestamp, err := timestampForDriver(d.db.DriverName())
	if err != nil {
		return err
	}

	_, err = d.db.Exec(d.db.Rebind(fmt.Sprintf("INSERT INTO subnets (underlay_ip, overlay_subnet, overlay_hwaddr, last_renewed_at) VALUES (?, ?, ?, %s)", timestamp)), lease.UnderlayIP, lease.OverlaySubnet, lease.OverlayHardwareAddr)
	if err != nil {
		return fmt.Errorf("adding entry: %s", err)
	}
	return nil
}

func (d *DatabaseHandler) DeleteEntry(underlayIP string) error {
	deleteRows, err := d.db.Exec(d.db.Rebind("DELETE FROM subnets WHERE underlay_ip = ?"), underlayIP)

	if err != nil {
		return fmt.Errorf("deleting entry: %s", err)
	}

	rowsAffected, err := deleteRows.RowsAffected()
	if err != nil {
		return fmt.Errorf("parse result: %s", err)
	}

	if rowsAffected == 0 {
		return RecordNotAffectedError
	}

	return nil
}

func (d *DatabaseHandler) LeaseForUnderlayIP(underlayIP string) (*controller.Lease, error) {
	var overlaySubnet, overlayHWAddr string
	result := d.db.QueryRow(d.db.Rebind("SELECT overlay_subnet, overlay_hwaddr FROM subnets WHERE underlay_ip = ?"), underlayIP)
	err := result.Scan(&overlaySubnet, &overlayHWAddr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err // test me
	}
	return &controller.Lease{
		UnderlayIP:          underlayIP,
		OverlaySubnet:       overlaySubnet,
		OverlayHardwareAddr: overlayHWAddr,
	}, nil
}

func (d *DatabaseHandler) RenewLeaseForUnderlayIP(underlayIP string) error {
	timestamp, err := timestampForDriver(d.db.DriverName())
	if err != nil {
		return err
	}

	_, err = d.db.Exec(d.db.Rebind(fmt.Sprintf("UPDATE subnets SET last_renewed_at = %s WHERE underlay_ip = ?", timestamp)), underlayIP)
	if err != nil {
		return fmt.Errorf("renewing lease: %s", err)
	}
	return nil
}

func (d *DatabaseHandler) LastRenewedAtForUnderlayIP(underlayIP string) (int64, error) {
	var lastRenewedAt int64
	result := d.db.QueryRow(d.db.Rebind("SELECT last_renewed_at FROM subnets WHERE underlay_ip = ?"), underlayIP)
	err := result.Scan(&lastRenewedAt)
	if err != nil {
		return 0, err
	}
	return lastRenewedAt, nil
}

func (d *DatabaseHandler) SubnetForUnderlayIP(underlayIP string) (string, error) {
	var subnet string
	result := d.db.QueryRow(d.db.Rebind("SELECT subnet FROM subnets WHERE underlay_ip = ?"), underlayIP)
	err := result.Scan(&subnet)
	if err != nil {
		return "", err
	}
	return subnet, nil
}

func rowsToLeases(rows *sql.Rows) ([]controller.Lease, error) {
	leases := []controller.Lease{}
	for rows.Next() {
		var underlayIP, overlaySubnet, overlayHWAddr string
		err := rows.Scan(&underlayIP, &overlaySubnet, &overlayHWAddr)
		if err != nil {
			return nil, fmt.Errorf("parsing result: %s", err)
		}
		leases = append(leases, controller.Lease{
			UnderlayIP:          underlayIP,
			OverlaySubnet:       overlaySubnet,
			OverlayHardwareAddr: overlayHWAddr,
		})
	}
	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("getting next row: %s", err) // untested
	}

	return leases, nil
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
	case Postgres:
		return fmt.Sprintf(baseCreateTable, psqlId)
	case MySQL:
		return fmt.Sprintf(baseCreateTable, mysqlId)
	}

	return ""
}

func timestampForDriver(driverName string) (string, error) {
	switch driverName {
	case MySQL:
		return mysqlTimeNow, nil
	case Postgres:
		return postgresTimeNow, nil
	default:
		return "", fmt.Errorf("database type %s is not supported", driverName)
	}
}
