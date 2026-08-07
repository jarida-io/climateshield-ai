// SPDX-License-Identifier: Apache-2.0

package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

func TestMigrationsCreateSchema(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	for _, table := range []string{
		"areas", "climate_observations", "risk_scores",
		"guardians", "children", "consent_log", "immunization_events",
		"vaccine_schedule", "event_leaves", "daily_roots", "anchors", "alerts",
	} {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables
			 WHERE table_schema = 'public' AND table_name = $1)`, table).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "table %s missing", table)
	}

	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables
		 WHERE table_schema = 'sealed' AND table_name = 'child_keys')`).Scan(&exists)
	require.NoError(t, err)
	require.True(t, exists, "sealed.child_keys missing")

	var kepiRows int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM vaccine_schedule`).Scan(&kepiRows))
	require.Equal(t, 16, kepiRows, "KEPI seed rows")
}

func TestMigrationsRoundTrip(t *testing.T) {
	// Down migrations are only trustworthy if exercised: roll everything back
	// and re-apply on an isolated database.
	pool, dsn := testdb.PoolDSN(t)
	pool.Close()

	require.NoError(t, store.MigrateDown(dsn))
	require.NoError(t, store.MigrateUp(dsn))

	fresh, err := store.Connect(context.Background(), dsn)
	require.NoError(t, err)
	defer fresh.Close()

	var n int
	require.NoError(t, fresh.QueryRow(context.Background(),
		`SELECT count(*) FROM vaccine_schedule`).Scan(&n))
	require.Equal(t, 16, n)
}
