// SPDX-License-Identifier: Apache-2.0

package publicapi_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/gen/climateshield/v1/climateshieldv1connect"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/publicapi"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

func TestConnectRiskHistoryFiltersAndValidation(t *testing.T) {
	pool := testdb.Pool(t)
	seedRiskScores(t, db.New(pool))
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info"))
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	client := climateshieldv1connect.NewPublicServiceClient(http.DefaultClient, ts.URL)
	ctx := context.Background()

	all, err := client.GetRiskHistory(ctx, connect.NewRequest(&climateshieldv1.GetRiskHistoryRequest{}))
	require.NoError(t, err)
	require.Len(t, all.Msg.GetScores(), 5)

	// The disease enum maps onto the stored lowercase value.
	cholera, err := client.GetRiskHistory(ctx, connect.NewRequest(&climateshieldv1.GetRiskHistoryRequest{
		Disease: climateshieldv1.Disease_DISEASE_CHOLERA,
	}))
	require.NoError(t, err)
	require.Len(t, cholera.Msg.GetScores(), 2)
	for _, s := range cholera.Msg.GetScores() {
		require.Equal(t, climateshieldv1.Disease_DISEASE_CHOLERA, s.GetDisease())
	}

	limited, err := client.GetRiskHistory(ctx, connect.NewRequest(&climateshieldv1.GetRiskHistoryRequest{Limit: 2}))
	require.NoError(t, err)
	require.Len(t, limited.Msg.GetScores(), 2)

	// A malformed date is a client error, not a stale response.
	_, err = client.GetRiskHistory(ctx, connect.NewRequest(&climateshieldv1.GetRiskHistoryRequest{
		FromDate: "07-08-2026",
	}))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestConnectNeverFailsWhenBackendIsDown(t *testing.T) {
	pool, _ := testdb.PoolDSN(t)
	seedRiskScores(t, db.New(pool))
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info"))
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	client := climateshieldv1connect.NewPublicServiceClient(http.DefaultClient, ts.URL)
	ctx := context.Background()

	// Warm all three methods so each has a last-good response.
	warm, err := client.GetCurrentRisk(ctx, connect.NewRequest(&climateshieldv1.GetCurrentRiskRequest{}))
	require.NoError(t, err)
	require.NotEmpty(t, warm.Msg.GetScores())
	_, err = client.GetRiskHistory(ctx, connect.NewRequest(&climateshieldv1.GetRiskHistoryRequest{}))
	require.NoError(t, err)
	_, err = client.GetStats(ctx, connect.NewRequest(&climateshieldv1.GetStatsRequest{}))
	require.NoError(t, err)

	pool.Close()

	// Every RPC still succeeds, flagged stale, serving the last good data.
	current, err := client.GetCurrentRisk(ctx, connect.NewRequest(&climateshieldv1.GetCurrentRiskRequest{}))
	require.NoError(t, err, "Connect reads must not fail when the database is down")
	require.Equal(t, "true", current.Header().Get("X-Data-Stale"))
	require.Len(t, current.Msg.GetScores(), len(warm.Msg.GetScores()))

	history, err := client.GetRiskHistory(ctx, connect.NewRequest(&climateshieldv1.GetRiskHistoryRequest{}))
	require.NoError(t, err)
	require.Equal(t, "true", history.Header().Get("X-Data-Stale"))

	stats, err := client.GetStats(ctx, connect.NewRequest(&climateshieldv1.GetStatsRequest{}))
	require.NoError(t, err)
	require.Equal(t, "true", stats.Header().Get("X-Data-Stale"))
}

// The availability promise covers the evidence surfaces too: a reviewer
// clicking through the dashboard during a database outage sees stale data
// marked as stale, not a stack of broken panels.
func TestConnectEvidenceSurfacesSurviveBackendLoss(t *testing.T) {
	pool, _ := testdb.PoolDSN(t)
	seedRiskScores(t, db.New(pool))
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info")).
		WithDeployment("climatology", "1.0.0", "mock", "6h")
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	client := climateshieldv1connect.NewPublicServiceClient(http.DefaultClient, ts.URL)
	ctx := context.Background()

	// Warm every evidence method.
	_, err := client.GetModelInfo(ctx, connect.NewRequest(&climateshieldv1.GetModelInfoRequest{}))
	require.NoError(t, err)
	_, err = client.GetClimateSeries(ctx, connect.NewRequest(&climateshieldv1.GetClimateSeriesRequest{}))
	require.NoError(t, err)
	_, err = client.GetLedgerSummary(ctx, connect.NewRequest(&climateshieldv1.GetLedgerSummaryRequest{}))
	require.NoError(t, err)
	_, err = client.GetAlertSummary(ctx, connect.NewRequest(&climateshieldv1.GetAlertSummaryRequest{}))
	require.NoError(t, err)
	_, err = client.GetPipelineStatus(ctx, connect.NewRequest(&climateshieldv1.GetPipelineStatusRequest{}))
	require.NoError(t, err)

	pool.Close()

	// Model info needs no database at all, so it keeps working outright.
	model, err := client.GetModelInfo(ctx, connect.NewRequest(&climateshieldv1.GetModelInfoRequest{}))
	require.NoError(t, err)
	require.Equal(t, "climatology", model.Msg.GetActivePredictor())

	// The database-backed ones degrade to stale rather than failing.
	climate, err := client.GetClimateSeries(ctx, connect.NewRequest(&climateshieldv1.GetClimateSeriesRequest{}))
	require.NoError(t, err)
	require.Equal(t, "true", climate.Header().Get("X-Data-Stale"))

	ledger, err := client.GetLedgerSummary(ctx, connect.NewRequest(&climateshieldv1.GetLedgerSummaryRequest{}))
	require.NoError(t, err)
	require.Equal(t, "true", ledger.Header().Get("X-Data-Stale"))

	alerts, err := client.GetAlertSummary(ctx, connect.NewRequest(&climateshieldv1.GetAlertSummaryRequest{}))
	require.NoError(t, err)
	require.Equal(t, "true", alerts.Header().Get("X-Data-Stale"))

	pipeline, err := client.GetPipelineStatus(ctx, connect.NewRequest(&climateshieldv1.GetPipelineStatusRequest{}))
	require.NoError(t, err)
	require.Equal(t, "true", pipeline.Header().Get("X-Data-Stale"))
}

func TestConnectStaleColdStart(t *testing.T) {
	// Never served successfully and the database is gone: still no error,
	// just an empty payload marked stale.
	pool, _ := testdb.PoolDSN(t)
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info"))
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)
	pool.Close()

	client := climateshieldv1connect.NewPublicServiceClient(http.DefaultClient, ts.URL)
	resp, err := client.GetStats(context.Background(), connect.NewRequest(&climateshieldv1.GetStatsRequest{}))
	require.NoError(t, err)
	require.Equal(t, "true", resp.Header().Get("X-Data-Stale"))
	require.Empty(t, resp.Msg.GetStats())
}
