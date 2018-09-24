package database

import (
	migrate "github.com/rubenv/sql-migrate"
)

type MigrateAdapter struct {
}

func (ma *MigrateAdapter) Exec(db Db, dialect string, m migrate.MigrationSource, dir migrate.MigrationDirection) (int, error) {
	return migrate.Exec(db.RawConnection().DB, dialect, m, dir)
}
