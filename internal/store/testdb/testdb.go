// SPDX-License-Identifier: Apache-2.0

// Package testdb provides the shared PostGIS test harness. One container per
// test binary; migrations (app + River) run once into a template database;
// each test clones the template into an isolated database. No test ever
// touches the network beyond the local Docker daemon.
package testdb

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/seed"
)

const (
	image      = "imresamu/postgis:16-3.4"
	user       = "cs"
	password   = "cs"
	templateDB = "cs_template"
)

var (
	once     sync.Once
	baseHost string // host:port of the running container
	startErr error
	dbSeq    atomic.Int64
)

// Pool returns a pgx pool bound to a fresh database (cloned from the
// migrated, area-seeded template) that lives for the duration of the test.
func Pool(t *testing.T) *pgxpool.Pool {
	pool, _ := PoolDSN(t)
	return pool
}

// PoolDSN is Pool plus the DSN of the per-test database (for code that
// re-connects itself, e.g. migration round-trip tests).
func PoolDSN(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: requires local Docker")
	}
	once.Do(start)
	require.NoError(t, startErr, "test database container failed to start")

	ctx := context.Background()
	name := fmt.Sprintf("cs_test_%d_%d", time.Now().UnixNano(), dbSeq.Add(1))

	admin, err := pgx.Connect(ctx, dsnFor("postgres"))
	require.NoError(t, err)
	_, err = admin.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, templateDB))
	require.NoError(t, admin.Close(ctx))
	require.NoError(t, err)

	dsn := dsnFor(name)
	pool, err := store.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool, dsn
}

func start() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := tcpostgres.Run(ctx, image,
		tcpostgres.WithDatabase(templateDB),
		tcpostgres.WithUsername(user),
		tcpostgres.WithPassword(password),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(2*time.Minute),
		),
	)
	if err != nil {
		startErr = fmt.Errorf("start postgis container: %w", err)
		return
	}
	host, err := container.Host(ctx)
	if err != nil {
		startErr = err
		return
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		startErr = err
		return
	}
	baseHost = fmt.Sprintf("%s:%s", host, port.Port())

	// Prepare the template: app migrations, River tables, reference areas.
	templateDSN := dsnFor(templateDB)
	if err := store.MigrateUp(templateDSN); err != nil {
		startErr = err
		return
	}
	pool, err := store.Connect(ctx, templateDSN)
	if err != nil {
		startErr = err
		return
	}
	defer pool.Close()

	riverMig, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		startErr = err
		return
	}
	if _, err := riverMig.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		startErr = fmt.Errorf("river migrations: %w", err)
		return
	}
	if err := seed.Areas(ctx, pool); err != nil {
		startErr = fmt.Errorf("seed areas: %w", err)
		return
	}
}

func dsnFor(dbName string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", user, password, baseHost, dbName)
}
