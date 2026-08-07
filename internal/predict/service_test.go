// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/climate"
	"github.com/jarida-io/climateshield/internal/climate/fixture"
	"github.com/jarida-io/climateshield/internal/jobs"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

func ingestFixture(t *testing.T, q *db.Queries, areaID string) {
	t.Helper()
	src := fixture.New(filepath.Join("..", "..", "testdata", "golden"))
	fc, err := src.FetchDaily(context.Background(), climate.Area{ID: areaID}, 14)
	require.NoError(t, err)
	_, err = climate.UpsertForecast(context.Background(), q, fc)
	require.NoError(t, err)
}

func TestScoreAreaPersistsScoresAndEnqueuesAlerts(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	ingestFixture(t, q, "kisumu")

	var enqueued []jobs.AlertDispatchArgs
	capture := func(_ context.Context, args river.JobArgs, queue string) error {
		require.Equal(t, jobs.QueueNotify, queue)
		enqueued = append(enqueued, args.(jobs.AlertDispatchArgs))
		return nil
	}

	n, err := scoreArea(ctx, q, NewRulesPredictor(), "kisumu", capture, logging.New(io.Discard, "info"))
	require.NoError(t, err)
	require.Equal(t, 2, n, "Kisumu demo scenario: cholera HIGH + malaria HIGH")

	rows, err := q.CurrentRisk(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 4, "one score per disease")
	byDisease := map[string]string{}
	for _, r := range rows {
		require.Equal(t, "rules", r.Predictor)
		require.Equal(t, RulesVersion, r.PredictorVersion)
		require.Equal(t, "kisumu", r.AreaID)
		byDisease[r.Disease] = r.Level
	}
	require.Equal(t, "HIGH", byDisease["cholera"])
	require.Equal(t, "HIGH", byDisease["malaria"])
	require.Equal(t, "LOW", byDisease["pneumonia"])
	require.Equal(t, "LOW", byDisease["meningitis"])

	require.Len(t, enqueued, 2)
	for _, a := range enqueued {
		require.Equal(t, "kisumu", a.AreaID)
		require.Contains(t, []string{"cholera", "malaria"}, a.Disease)
		require.Equal(t, "HIGH", a.Level)
		require.NotZero(t, a.RiskScoreID)
	}
}

func TestScoreAreaMediumAlsoAlerts(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	ingestFixture(t, q, "eldoret")

	var enqueued []jobs.AlertDispatchArgs
	capture := func(_ context.Context, args river.JobArgs, _ string) error {
		enqueued = append(enqueued, args.(jobs.AlertDispatchArgs))
		return nil
	}

	n, err := scoreArea(ctx, q, NewRulesPredictor(), "eldoret", capture, logging.New(io.Discard, "info"))
	require.NoError(t, err)
	require.Equal(t, 1, n, "Eldoret demo scenario: pneumonia MEDIUM")
	require.Equal(t, "pneumonia", enqueued[0].Disease)
	require.Equal(t, "MEDIUM", enqueued[0].Level)
}

func TestScoreAreaSkipsWithoutObservations(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)

	n, err := scoreArea(context.Background(), q, NewRulesPredictor(), "nairobi",
		func(context.Context, river.JobArgs, string) error {
			t.Fatal("nothing should be enqueued")
			return nil
		}, logging.New(io.Discard, "info"))
	require.NoError(t, err)
	require.Zero(t, n)

	rows, err := q.CurrentRisk(context.Background())
	require.NoError(t, err)
	require.Empty(t, rows, "no scores may be fabricated without data")
}

func TestScoreAreaRescoringUpserts(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	ingestFixture(t, q, "kisumu")

	noop := func(context.Context, river.JobArgs, string) error { return nil }
	log := logging.New(io.Discard, "info")

	_, err := scoreArea(ctx, q, NewRulesPredictor(), "kisumu", noop, log)
	require.NoError(t, err)
	_, err = scoreArea(ctx, q, NewRulesPredictor(), "kisumu", noop, log)
	require.NoError(t, err)

	rows, err := q.CurrentRisk(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 4, "rescoring the same window must upsert, not duplicate")
}
