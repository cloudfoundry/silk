package database

import (
	"database/sql"
	"fmt"

	"github.com/cloudfoundry-incubator/silk/daemon/config"
	"github.com/rubenv/sql-migrate"
)

type DatabaseHandler struct {
	migrations *migrate.MemoryMigrationSource
	db         *sql.DB
	dbType     string
}

func NewDatabaseHandler(databaseConfig config.DatabaseConfig) (*DatabaseHandler, error) {
	db, err := sql.Open(databaseConfig.Type, databaseConfig.ConnectionString)
	if err != nil {
		panic(err)
	}

	migrations := &migrate.MemoryMigrationSource{
		Migrations: []*migrate.Migration{
			&migrate.Migration{
				Id:   "1",
				Up:   []string{createSubnetTable(databaseConfig.Type)},
				Down: []string{"DROP TABLE subnets"},
			},
		},
	}

	return &DatabaseHandler{
		migrations: migrations,
		db:         db,
		dbType:     databaseConfig.Type,
	}, nil
}

func (d *DatabaseHandler) Migrate() (int, error) {
	return migrate.Exec(d.db, d.dbType, d.migrations, migrate.Up)
}

func (d *DatabaseHandler) AddEntry(underlayIP, subnet string) error {
	_, err := d.db.Exec(fmt.Sprintf("INSERT INTO subnets (underlay_ip, subnet) VALUES ('%s', '%s')", underlayIP, subnet))
	return err
}

func (d *DatabaseHandler) EntryExists(entry, value string) (bool, error) {
	var exists bool
	err := d.db.QueryRow(fmt.Sprintf("SELECT IF(COUNT(*),'true','false') FROM subnets WHERE %s = '%s'", entry, value)).Scan(&exists)
	return exists, err
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
