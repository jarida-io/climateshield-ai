// SPDX-License-Identifier: Apache-2.0

package ingestor

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/climate/fixture"
	"github.com/jarida-io/climateshield/internal/jobs"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

func TestRunIngestSweepIngestsAllAreasAndEnqueuesPredict(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	src := fixture.New(filepath.Join("..", "..", "..", "testdata", "golden"))

	type enq struct {
		kind  string
		queue string
	}
	var enqueued []enq
	capture := func(_ context.Context, args river.JobArgs, queue string) error {
		enqueued = append(enqueued, enq{kind: args.Kind(), queue: queue})
		return nil
	}

	err := runIngestSweep(ctx, q, src, 14, capture, logging.New(io.Discard, "info"))
	require.NoError(t, err)

	count, err := q.CountObservations(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 5*14, count, "5 areas x 14 days")

	require.Len(t, enqueued, 5, "one risk_predict job per area")
	for _, e := range enqueued {
		require.Equal(t, "risk_predict", e.kind)
		require.Equal(t, jobs.QueuePredict, e.queue)
	}

	// A second sweep of the same fixtures upserts, never duplicates.
	err = runIngestSweep(ctx, q, src, 14, capture, logging.New(io.Discard, "info"))
	require.NoError(t, err)
	count, err = q.CountObservations(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 5*14, count)
}

func TestBuildSource(t *testing.T) {
	cfg := ServiceConfig{OpenMeteoBaseURL: "https://api.open-meteo.com", FixtureDir: "x"}

	s, err := buildSource("openmeteo", cfg)
	require.NoError(t, err)
	require.NotNil(t, s)

	s, err = buildSource("fixture", cfg)
	require.NoError(t, err)
	require.NotNil(t, s)

	_, err = buildSource("kenya-met", cfg)
	require.ErrorContains(t, err, "unknown source")
}
