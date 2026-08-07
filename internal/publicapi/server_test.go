// SPDX-License-Identifier: Apache-2.0

package publicapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/jarida-io/climateshield/internal/climate"
	"github.com/jarida-io/climateshield/internal/climate/fixture"
	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/publicapi"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

// statRow is the test-side view of one county's stats.
type statRow struct {
	registered *int64
	suppressed bool
}

func parseStats(t *testing.T, body string) map[string]statRow {
	t.Helper()
	var msg climateshieldv1.GetStatsResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))
	out := map[string]statRow{}
	for _, s := range msg.GetStats() {
		out[s.GetArea()] = statRow{registered: s.ChildrenRegistered, suppressed: s.GetChildrenRegisteredSuppressed()}
	}
	return out
}

func seedRiskScores(t *testing.T, q *db.Queries) {
	t.Helper()
	ctx := context.Background()
	for _, rs := range []struct {
		area, disease, level string
		value                float64
	}{
		{"kisumu", "cholera", "HIGH", 74},
		{"kisumu", "malaria", "HIGH", 74},
		{"mombasa", "malaria", "HIGH", 41},
		{"eldoret", "pneumonia", "MEDIUM", 17.2},
		{"nairobi", "cholera", "LOW", 18},
	} {
		_, err := q.UpsertRiskScore(ctx, db.UpsertRiskScoreParams{
			AreaID: rs.area, Disease: rs.disease, Level: rs.level,
			Driver: "peak_rainfall_mm_14d", DriverValue: rs.value,
			ForecastDate: pgtype.Date{Time: time.Now(), Valid: true},
			WindowDays:   14, Predictor: "rules", PredictorVersion: "1.0.0",
		})
		require.NoError(t, err)
	}
}

func newTestServer(t *testing.T) (*httptest.Server, *db.Queries) {
	t.Helper()
	pool := testdb.Pool(t)
	q := db.New(pool)
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info"))
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)
	return ts, q
}

func TestCurrentRiskFormats(t *testing.T) {
	ts, q := newTestServer(t)
	seedRiskScores(t, q)

	// JSON: canonical protojson of the proto contract.
	status, body := get(t, ts, "/v1/risk/current")
	require.Equal(t, http.StatusOK, status)
	var msg climateshieldv1.GetCurrentRiskResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))
	require.Len(t, msg.GetScores(), 5)

	// CSV: header + one row per score.
	_, csvBody := get(t, ts, "/v1/risk/current?format=csv")
	lines := strings.Split(strings.TrimSpace(csvBody), "\n")
	require.Len(t, lines, 6)
	require.True(t, strings.HasPrefix(lines[0], "area,disease,level,forecast_date"))
	require.Contains(t, csvBody, "Kisumu,cholera,HIGH")

	// GeoJSON: FeatureCollection with Point coordinates (lon, lat).
	_, geoBody := get(t, ts, "/v1/risk/current?format=geojson")
	var fc map[string]any
	require.NoError(t, json.Unmarshal([]byte(geoBody), &fc))
	require.Equal(t, "FeatureCollection", fc["type"])
	features := fc["features"].([]any)
	require.Len(t, features, 5)
	first := features[0].(map[string]any)
	require.Equal(t, "Feature", first["type"])
	require.Equal(t, "Point", first["geometry"].(map[string]any)["type"])

	// Unknown format is a client error, not a 500.
	status, _ = get(t, ts, "/v1/risk/current?format=xml")
	require.Equal(t, http.StatusBadRequest, status)

	// GeoJSON is undefined for stats (no geometry in the message).
	status, _ = get(t, ts, "/v1/stats?format=geojson")
	require.Equal(t, http.StatusBadRequest, status)
}

func TestRiskHistoryFilters(t *testing.T) {
	ts, q := newTestServer(t)
	seedRiskScores(t, q)

	_, body := get(t, ts, "/v1/risk/history?area=Kisumu")
	var msg climateshieldv1.GetRiskHistoryResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))
	require.Len(t, msg.GetScores(), 2)
	for _, s := range msg.GetScores() {
		require.Equal(t, "Kisumu", s.GetArea())
	}

	_, body = get(t, ts, "/v1/risk/history?disease=cholera")
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))
	require.Len(t, msg.GetScores(), 2)

	status, _ := get(t, ts, "/v1/risk/history?from=not-a-date")
	require.Equal(t, http.StatusBadRequest, status)

	status, _ = get(t, ts, "/v1/risk/history?limit=banana")
	require.Equal(t, http.StatusBadRequest, status)
}

func TestNeverFiveHundred_StaleCacheAndColdStart(t *testing.T) {
	pool, _ := testdb.PoolDSN(t)
	q := db.New(pool)
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info"))
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	seedRiskScores(t, q)

	// Warm the cache with a good response.
	resp, err := http.Get(ts.URL + "/v1/risk/current")
	require.NoError(t, err)
	warmBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Empty(t, resp.Header.Get("X-Data-Stale"))

	// Kill the database out from under the server.
	pool.Close()

	resp, err = http.Get(ts.URL + "/v1/risk/current")
	require.NoError(t, err)
	staleBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "reads must never 500")
	require.Equal(t, "true", resp.Header.Get("X-Data-Stale"))
	require.JSONEq(t, string(warmBody), string(staleBody), "stale response must be the last good one")

	// Cold start: an endpoint never served successfully still returns 200 +
	// stale header with an empty valid payload.
	resp, err = http.Get(ts.URL + "/v1/stats")
	require.NoError(t, err)
	coldBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "true", resp.Header.Get("X-Data-Stale"))
	var stats climateshieldv1.GetStatsResponse
	require.NoError(t, protojson.Unmarshal(coldBody, &stats))
	require.Empty(t, stats.GetStats())
}

func TestConnectAndRESTServeSameData(t *testing.T) {
	ts, q := newTestServer(t)
	seedRiskScores(t, q)

	_, restBody := get(t, ts, "/v1/risk/current")
	var restMsg climateshieldv1.GetCurrentRiskResponse
	require.NoError(t, protojson.Unmarshal([]byte(restBody), &restMsg))

	resp, err := http.Post(ts.URL+"/climateshield.v1.PublicService/GetCurrentRisk",
		"application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	connectBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var connectMsg climateshieldv1.GetCurrentRiskResponse
	require.NoError(t, protojson.Unmarshal(connectBody, &connectMsg))

	require.Equal(t, len(restMsg.GetScores()), len(connectMsg.GetScores()))
	require.Equal(t, restMsg.GetScores()[0].GetArea(), connectMsg.GetScores()[0].GetArea())
	require.Equal(t, restMsg.GetScores()[0].GetLevel(), connectMsg.GetScores()[0].GetLevel())
}

// ingestFixtureWindow loads one county's committed 14-day golden window so
// evidence tests have real climate rows behind them.
func ingestFixtureWindow(t *testing.T, q *db.Queries, areaID string) {
	t.Helper()
	src := fixture.New(filepath.Join("..", "..", "testdata", "golden"))
	fc, err := src.FetchDaily(context.Background(), climate.Area{ID: areaID}, 14)
	require.NoError(t, err)
	_, err = climate.UpsertForecast(context.Background(), q, fc)
	require.NoError(t, err)
}
