// SPDX-License-Identifier: Apache-2.0

package climate_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/climate"
	"github.com/jarida-io/climateshield/internal/climate/chirps"
	"github.com/jarida-io/climateshield/internal/climate/era5"
	"github.com/jarida-io/climateshield/internal/climate/fixture"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

func TestUpsertForecastIsIdempotent(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	src := fixture.New(filepath.Join("..", "..", "testdata", "golden"))

	fc, err := src.FetchDaily(ctx, climate.Area{ID: "kisumu"}, 14)
	require.NoError(t, err)

	n, err := climate.UpsertForecast(ctx, q, fc)
	require.NoError(t, err)
	require.Equal(t, 14, n)

	// Same batch again: still exactly 14 rows.
	_, err = climate.UpsertForecast(ctx, q, fc)
	require.NoError(t, err)
	count, err := q.CountObservations(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 14, count)

	// A newly issued batch coexists with history rather than clobbering it.
	fc2 := fc
	fc2.IssuedAt = fc.IssuedAt.Add(6 * time.Hour)
	_, err = climate.UpsertForecast(ctx, q, fc2)
	require.NoError(t, err)
	count, err = q.CountObservations(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 28, count)

	// The latest-window query returns only the newest batch.
	window, err := q.LatestObservationWindow(ctx, "kisumu")
	require.NoError(t, err)
	require.Len(t, window, 14)
	for _, row := range window {
		require.Equal(t, fc2.IssuedAt.Unix(), row.IssuedAt.Time.Unix())
	}
}

func TestFutureSourcesAreDeclaredNotImplemented(t *testing.T) {
	_, err := chirps.New()
	require.ErrorIs(t, err, climate.ErrNotImplemented)
	_, err = era5.New()
	require.ErrorIs(t, err, climate.ErrNotImplemented)
}
