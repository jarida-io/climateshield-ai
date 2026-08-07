// SPDX-License-Identifier: Apache-2.0

package notifier

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/jobs"
	"github.com/jarida-io/climateshield/internal/notify"
	"github.com/jarida-io/climateshield/internal/notify/mock"
	"github.com/jarida-io/climateshield/internal/platform/clock"
	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/seed"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

func noonEAT() time.Time { return time.Date(2026, 8, 7, 12, 0, 0, 0, clock.EAT) }

// setupDispatch seeds the demo population and one HIGH cholera risk score
// for Kisumu, returning everything dispatchAlert needs.
func setupDispatch(t *testing.T) (q *db.Queries, key crypto.Key, args jobs.AlertDispatchArgs, outbox string) {
	t.Helper()
	pool := testdb.Pool(t)
	ctx := context.Background()
	q = db.New(pool)

	var err error
	key, err = crypto.NewRandomKey()
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, noonEAT())
	require.NoError(t, err)

	scoreID, err := q.UpsertRiskScore(ctx, db.UpsertRiskScoreParams{
		AreaID: "kisumu", Disease: "cholera", Level: "HIGH",
		Driver: "peak_rainfall_mm_14d", DriverValue: 74,
		ForecastDate: pgtype.Date{Time: noonEAT(), Valid: true},
		WindowDays:   14, Predictor: "rules", PredictorVersion: "1.0.0",
	})
	require.NoError(t, err)

	args = jobs.AlertDispatchArgs{RiskScoreID: scoreID, AreaID: "kisumu", Disease: "cholera", Level: "HIGH"}
	outbox = filepath.Join(t.TempDir(), "outbox.jsonl")
	return q, key, args, outbox
}

func TestDispatchAlertMockHonestOutput(t *testing.T) {
	q, key, args, outbox := setupDispatch(t)
	ctx := context.Background()

	var logBuf bytes.Buffer
	log := logging.New(&logBuf, "info")

	out, snooze, err := dispatchAlert(ctx, q, mock.New(outbox), mock.ChannelName,
		key, clock.Fixed{T: noonEAT()}, args, log)
	require.NoError(t, err)
	require.Zero(t, snooze)

	// Kisumu: 12 children, one guardian (2 children) opted out.
	require.Equal(t, 10, out.WouldSend)
	require.Equal(t, 2, out.SkippedConsent)
	require.Zero(t, out.Sent, "the mock channel must NEVER count as sent")

	// The log line uses the mandated honest phrasing.
	require.Contains(t, logBuf.String(), "[mock] would send 10 alerts")
	require.Contains(t, logBuf.String(), "no SMS was sent")

	// Statuses recorded truthfully: would_send + skipped_consent, never sent.
	counts := map[string]int64{}
	rows, err := q.CountAlertsByStatus(ctx)
	require.NoError(t, err)
	for _, r := range rows {
		counts[r.Status] = r.N
	}
	require.EqualValues(t, 10, counts["would_send"])
	require.EqualValues(t, 2, counts["skipped_consent"])
	require.Zero(t, counts["sent"])

	// The outbox exists and contains 10 JSONL lines.
	raw, err := os.ReadFile(outbox)
	require.NoError(t, err)
	require.Len(t, bytes.Split(bytes.TrimSpace(raw), []byte("\n")), 10)
	require.Contains(t, string(raw), "NOT sent")
}

func TestDispatchAlertIsIdempotentPerRiskScore(t *testing.T) {
	q, key, args, outbox := setupDispatch(t)
	ctx := context.Background()
	log := logging.New(bytes.NewBuffer(nil), "info")
	clk := clock.Fixed{T: noonEAT()}

	first, _, err := dispatchAlert(ctx, q, mock.New(outbox), mock.ChannelName, key, clk, args, log)
	require.NoError(t, err)
	require.Equal(t, 10, first.WouldSend)

	second, _, err := dispatchAlert(ctx, q, mock.New(outbox), mock.ChannelName, key, clk, args, log)
	require.NoError(t, err)
	require.Zero(t, second.WouldSend, "retried job must not re-alert anyone")
	require.Equal(t, 12, second.Deduplicated)
}

func TestDispatchAlertSnoozesInQuietHours(t *testing.T) {
	q, key, args, outbox := setupDispatch(t)
	ctx := context.Background()

	tenPM := time.Date(2026, 8, 7, 22, 0, 0, 0, clock.EAT)
	out, snooze, err := dispatchAlert(ctx, q, mock.New(outbox), mock.ChannelName,
		key, clock.Fixed{T: tenPM}, args, logging.New(bytes.NewBuffer(nil), "info"))
	require.NoError(t, err)

	require.Equal(t, 9*time.Hour, snooze, "22:00 EAT -> 07:00 EAT next day")
	require.Zero(t, out.WouldSend)
	_, err = os.Stat(outbox)
	require.True(t, os.IsNotExist(err), "no outbox writes during quiet hours")
}

func TestDispatchRendersDecryptedNamesAndLangs(t *testing.T) {
	q, key, args, outbox := setupDispatch(t)
	ctx := context.Background()

	_, _, err := dispatchAlert(ctx, q, mock.New(outbox), mock.ChannelName,
		key, clock.Fixed{T: noonEAT()}, args, logging.New(bytes.NewBuffer(nil), "info"))
	require.NoError(t, err)

	raw, err := os.ReadFile(outbox)
	require.NoError(t, err)
	content := string(raw)

	// Bilingual: seed alternates guardian languages.
	require.Contains(t, content, "Outbreak risk is HIGH in Kisumu")
	require.Contains(t, content, "Hatari ya mlipuko ni HIGH katika Kisumu")
	// No disease name in any rendered SMS.
	require.NotContains(t, content, "cholera")
	require.NotContains(t, content, "Cholera")
	// No full phone numbers on disk.
	require.NotContains(t, content, "+2547000001")
}

func TestBuildChannel(t *testing.T) {
	cfg := ServiceConfig{MockOutboxPath: "x.jsonl"}

	cfg.Channel = "mock"
	ch, name, err := buildChannel(cfg)
	require.NoError(t, err)
	require.NotNil(t, ch)
	require.Equal(t, "mock", name)

	cfg.Channel = "smpp"
	ch, name, err = buildChannel(cfg)
	require.NoError(t, err)
	require.NotNil(t, ch)
	require.Equal(t, "smpp", name)

	cfg.Channel = "africastalking"
	_, _, err = buildChannel(cfg)
	require.ErrorIs(t, err, notify.ErrNotConfigured)

	cfg.Channel = "carrier-pigeon"
	_, _, err = buildChannel(cfg)
	require.ErrorContains(t, err, "unknown channel")
}
