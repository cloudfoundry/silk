package database

import (
	"database/sql"
	"fmt"

	"github.com/cloudfoundry-incubator/silk/daemon/config"
	"github.com/rubenv/sql-migrate"
)

type Migrator struct {
	migrations *migrate.MemoryMigrationSource
	db         *sql.DB
	dbType     string
}

func NewMigrator(databaseConfig config.DatabaseConfig) (*Migrator, error) {
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

	return &Migrator{
		migrations: migrations,
		db:         db,
		dbType:     databaseConfig.Type,
	}, nil
}

func (m *Migrator) Migrate() (int, error) {
	return migrate.Exec(m.db, m.dbType, m.migrations, migrate.Up)
}

func createSubnetTable(dbType string) string {
	baseCreateTable := "CREATE TABLE subnets ( " +
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
