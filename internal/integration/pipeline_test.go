// SPDX-License-Identifier: Apache-2.0

// Package integration_test boots every service the way cmd/* does — real
// config from the environment, real River clients, real HTTP servers — and
// drives the whole vertical slice against a throwaway PostGIS database. It
// is the test that would catch a service that compiles but cannot start.
package integration_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ingestorsvc "github.com/jarida-io/climateshield/internal/climate/ingestor"
	"github.com/jarida-io/climateshield/internal/ledger"
	"github.com/jarida-io/climateshield/internal/notify"
	"github.com/jarida-io/climateshield/internal/notify/notifier"
	"github.com/jarida-io/climateshield/internal/platform/clock"
	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/predict"
	"github.com/jarida-io/climateshield/internal/publicapi"
	"github.com/jarida-io/climateshield/internal/registry"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/seed"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

const devKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// freePort reserves and releases a port, returning ":NNNNN".
func freePort(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().(*net.TCPAddr)
	require.NoError(t, lis.Close())
	return fmt.Sprintf(":%d", addr.Port)
}

func waitHealthy(t *testing.T, addr string) {
	t.Helper()
	url := "http://127.0.0.1" + addr + "/health"
	require.Eventually(t, func() bool {
		resp, err := http.Get(url)
		if err != nil {
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode == http.StatusOK
	}, 30*time.Second, 200*time.Millisecond, "service at %s never became healthy", addr)
}

// TestAllServicesBootAndRunThePipeline starts all six services, lets the
// periodic ingest job fire, and asserts data flows all the way through:
// observations -> risk scores -> alerts -> ledger leaves -> public API.
func TestAllServicesBootAndRunThePipeline(t *testing.T) {
	pool, dsn := testdb.PoolDSN(t)
	q := db.New(pool)
	ctx := context.Background()

	key, err := crypto.KeyFromHex(devKeyHex)
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)

	outbox := filepath.Join(t.TempDir(), "outbox.jsonl")
	goldenDir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "golden"))
	require.NoError(t, err)

	addrs := map[string]string{
		"INGESTOR_ADDR":  freePort(t),
		"PREDICTOR_ADDR": freePort(t),
		"NOTIFIER_ADDR":  freePort(t),
		"LEDGER_ADDR":    freePort(t),
		"REGISTRY_ADDR":  freePort(t),
		"PUBLICAPI_ADDR": freePort(t),
	}
	env := map[string]string{
		"DATABASE_URL":          dsn,
		"LOG_LEVEL":             "error",
		"PII_KEY_HEX":           devKeyHex,
		"CLIMATE_SOURCE":        "fixture",
		"CLIMATE_FIXTURE_DIR":   goldenDir,
		"NOTIFY_CHANNEL":        "mock",
		"MOCK_OUTBOX_PATH":      outbox,
		"LEDGER_SWEEP_INTERVAL": "5s",
		"INGEST_INTERVAL":       "1h", // RunOnStart fires the sweep immediately
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	for k, v := range addrs {
		t.Setenv(k, v)
	}

	runCtx, cancel := context.WithCancel(ctx)
	errs := make(chan error, len(addrs))
	for name, run := range map[string]func(context.Context) error{
		"ingestor":  ingestorsvc.Run,
		"predictor": predict.Run,
		"notifier":  notifier.Run,
		"ledger":    ledger.Run,
		"registry":  registry.Run,
		"publicapi": publicapi.Run,
	} {
		go func(name string, run func(context.Context) error) {
			if err := run(runCtx); err != nil {
				errs <- fmt.Errorf("%s: %w", name, err)
				return
			}
			errs <- nil
		}(name, run)
	}
	t.Cleanup(func() {
		cancel()
		for i := 0; i < len(addrs); i++ {
			select {
			case err := <-errs:
				require.NoError(t, err, "service exited with an error")
			case <-time.After(20 * time.Second):
				t.Error("a service did not shut down within 20s")
			}
		}
	})

	for _, addr := range addrs {
		waitHealthy(t, addr)
	}

	// Ingestion: 5 counties x 14 forecast days.
	require.Eventually(t, func() bool {
		n, err := q.CountObservations(ctx)
		return err == nil && n == 5*14
	}, 60*time.Second, 500*time.Millisecond, "climate observations never landed")

	// Prediction: 5 counties x 4 diseases, stamped with the rules predictor.
	require.Eventually(t, func() bool {
		rows, err := q.CurrentRisk(ctx)
		return err == nil && len(rows) == 5*4
	}, 60*time.Second, 500*time.Millisecond, "risk scores never landed")

	rows, err := q.CurrentRisk(ctx)
	require.NoError(t, err)
	levels := map[string]string{}
	for _, r := range rows {
		require.Equal(t, "rules", r.Predictor)
		require.Equal(t, predict.RulesVersion, r.PredictorVersion)
		levels[r.AreaID+"/"+r.Disease] = r.Level
	}
	// The committed demo scenario, end to end through the real services.
	require.Equal(t, "HIGH", levels["kisumu/cholera"])
	require.Equal(t, "HIGH", levels["kisumu/malaria"])
	require.Equal(t, "MEDIUM", levels["mombasa/cholera"])
	require.Equal(t, "HIGH", levels["mombasa/malaria"])
	require.Equal(t, "MEDIUM", levels["eldoret/pneumonia"])
	require.Equal(t, "LOW", levels["nairobi/cholera"])

	// Alerts: outside quiet hours the notifier records would_send rows; in
	// quiet hours it snoozes and records nothing. Both are correct — assert
	// the one that applies rather than flaking for ten hours a day.
	if notify.InQuietHours(clock.Real{}.Now()) {
		t.Log("quiet hours (21:00-07:00 EAT): alert dispatch is deferred, skipping alert assertions")
	} else {
		require.Eventually(t, func() bool {
			statuses, err := q.CountAlertsByStatus(ctx)
			if err != nil {
				return false
			}
			for _, s := range statuses {
				if s.Status == "would_send" && s.N > 0 {
					return true
				}
			}
			return false
		}, 60*time.Second, 500*time.Millisecond, "no alerts were recorded")

		statuses, err := q.CountAlertsByStatus(ctx)
		require.NoError(t, err)
		byStatus := map[string]int64{}
		for _, s := range statuses {
			byStatus[s.Status] = s.N
		}
		require.Positive(t, byStatus["would_send"])
		require.Zero(t, byStatus["sent"], "the mock channel must never record a real send")
		require.Positive(t, byStatus["skipped_consent"], "the opted-out guardian must be skipped")
	}

	// Ledger: every seeded immunization event becomes a leaf under a root.
	require.Eventually(t, func() bool {
		pending, err := q.ListEventsWithoutLeaves(ctx)
		return err == nil && len(pending) == 0
	}, 60*time.Second, 500*time.Millisecond, "ledger never swept the events")

	days, err := q.ListLeafDays(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, days)
	for _, day := range days {
		root, err := q.GetDailyRoot(ctx, day)
		require.NoError(t, err)
		leaves, err := q.LeavesForDay(ctx, day)
		require.NoError(t, err)
		hashes := make([][]byte, 0, len(leaves))
		for _, l := range leaves {
			hashes = append(hashes, l.LeafHash)
		}
		require.Equal(t, ledger.Root(hashes), root.Root)

		anchors, err := q.ListAnchorsForDay(ctx, day)
		require.NoError(t, err)
		require.NotEmpty(t, anchors, "each root must be anchored")
	}

	// Public API: serving live data over HTTP with no PII.
	resp, err := http.Get("http://127.0.0.1" + addrs["PUBLICAPI_ADDR"] + "/v1/risk/current")
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "Kisumu")
	require.NotContains(t, string(body), "+2547")

	// Metrics are exposed for the uptime obligation.
	resp, err = http.Get("http://127.0.0.1" + addrs["PUBLICAPI_ADDR"] + "/metrics")
	require.NoError(t, err)
	metricsBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, string(metricsBody), "go_goroutines")
}

// TestServicesFailFastOnBadConfig proves misconfiguration surfaces at
// startup instead of degrading silently at runtime.
func TestServicesFailFastOnBadConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://nobody:nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	t.Setenv("PII_KEY_HEX", devKeyHex)
	t.Setenv("PUBLICAPI_ADDR", freePort(t))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.Error(t, publicapi.Run(ctx), "an unreachable database must fail startup")

	// A configured-but-unusable ONNX model must also fail startup rather than
	// silently falling back to the rules engine.
	t.Setenv("ONNX_MODEL_PATH", "/models/does-not-exist.onnx")
	require.ErrorIs(t, predict.Run(ctx), predict.ErrNotImplemented)

	// An invalid PII key must fail the notifier immediately.
	t.Setenv("PII_KEY_HEX", "too-short")
	require.Error(t, notifier.Run(ctx))
}
