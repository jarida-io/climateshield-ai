// SPDX-License-Identifier: Apache-2.0

package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrateUp applies all pending migrations. Safe to run repeatedly.
func MigrateUp(dsn string) error {
	return runMigrations(dsn, func(m *migrate.Migrate) error { return m.Up() })
}

// MigrateDown rolls back ALL migrations — destructive; used by tests to prove
// the down files work.
func MigrateDown(dsn string) error {
	return runMigrations(dsn, func(m *migrate.Migrate) error { return m.Down() })
}

func runMigrations(dsn string, op func(*migrate.Migrate) error) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("store: migrations: %w", err)
	}
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("store: migrations: %w", err)
	}
	drv, err := pgxmigrate.WithInstance(sqlDB, &pgxmigrate.Config{})
	if err != nil {
		_ = sqlDB.Close()
		return fmt.Errorf("store: migrations: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", drv)
	if err != nil {
		_ = sqlDB.Close()
		return fmt.Errorf("store: migrations: %w", err)
	}
	defer func() {
		serr, derr := m.Close()
		_ = serr
		_ = derr
	}()
	if err := op(m); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("store: migrations: %w", err)
	}
	return nil
}
