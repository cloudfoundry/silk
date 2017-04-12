package database

import (
	"fmt"
	"time"

	"code.cloudfoundry.org/lager"
)

//go:generate counterfeiter -o fakes/database_migrator.go --fake-name DatabaseMigrator . databaseMigrator
type databaseMigrator interface {
	Migrate() (int, error)
}

type Migrator struct {
	DatabaseMigrator              databaseMigrator
	MaxMigrationAttempts          int
	MigrationAttemptSleepDuration time.Duration
	Logger                        lager.Logger
}

func (m *Migrator) TryMigrations() error {
	nErrors := 0
	var err error
	for nErrors < m.MaxMigrationAttempts {
		var n int
		n, err = m.DatabaseMigrator.Migrate()
		if err == nil {
			m.Logger.Info("db-migration-complete", lager.Data{"num-applied": n})
			return nil
		}

		nErrors++
		time.Sleep(m.MigrationAttemptSleepDuration)
	}

	return fmt.Errorf("creating table: %s", err)
}
