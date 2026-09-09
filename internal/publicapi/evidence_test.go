// SPDX-License-Identifier: Apache-2.0

package publicapi_test

import (
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/ledger"
	"github.com/jarida-io/climateshield/internal/ledger/anchor"
	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/predict"
	"github.com/jarida-io/climateshield/internal/publicapi"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/seed"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

func TestModelInfoReportsLiveDeploymentAndUnreachableThresholds(t *testing.T) {
	pool := testdb.Pool(t)
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info")).
		WithDeployment("climatology", predict.ClimatologyVersion, "mock", "6h")
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	_, body := get(t, ts, "/v1/model")
	var msg climateshieldv1.GetModelInfoResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))

	// The view must describe the deployment that is running, not a default.
	require.Equal(t, "climatology", msg.GetActivePredictor())
	require.Equal(t, predict.ClimatologyVersion, msg.GetActiveVersion())
	require.Contains(t, msg.GetAvailablePredictors(), "rules")
	require.Equal(t, "2015-01-01..2024-12-31", msg.GetReferencePeriod())
	require.NotEmpty(t, msg.GetReferenceLicence(), "reference data must carry its licence")

	// The interpretation must say what the number is NOT.
	require.Contains(t, msg.GetInterpretation(), "not a probability that an outbreak")

	// All four published rules, with the two unreachable ones flagged.
	require.Len(t, msg.GetRules(), 4)
	byDisease := map[string]*climateshieldv1.ThresholdRule{}
	for _, r := range msg.GetRules() {
		byDisease[r.GetDisease()] = r
	}
	require.True(t, byDisease["cholera"].GetReachableInReferencePeriod())
	require.True(t, byDisease["malaria"].GetReachableInReferencePeriod())
	require.False(t, byDisease["pneumonia"].GetReachableInReferencePeriod(),
		"the pneumonia cutoff cannot be reached in the reference record and must be reported as such")
	require.False(t, byDisease["meningitis"].GetReachableInReferencePeriod())
	require.Contains(t, byDisease["pneumonia"].GetNote(), "never fires")
	require.Contains(t, byDisease["meningitis"].GetNote(), "never fires")

	// Provenance: the reference artifact must be identified precisely enough
	// that a reviewer can hash it and rebuild it, rather than take it on
	// trust. The digest is of the exact bytes embedded in the binary.
	digest, err := predict.ClimatologyDigest()
	require.NoError(t, err)
	require.Equal(t, digest, msg.GetReferenceSha256())
	require.Len(t, msg.GetReferenceSha256(), 64)
	require.Equal(t, predict.DefaultClimatologyFile, msg.GetReferenceFile())
	require.Equal(t, "cmd/buildclimatology", msg.GetReferenceGenerator())
	require.Positive(t, msg.GetReferenceWindows())
	require.EqualValues(t, 21, msg.GetQuantileSteps())

	// The climatology predictor is the deployment here, so the exceedance is
	// what set each level and the view must say so.
	require.Equal(t, predict.ExceedanceRoleDeciding, msg.GetExceedanceRole())
}

// Under the default deployment the published thresholds decide the tiers, and
// /v1/model must say the exceedance beside a score only annotates it.
func TestModelInfoSaysTheExceedanceOnlyAnnotatesRulesScores(t *testing.T) {
	pool := testdb.Pool(t)
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info")).
		WithDeployment("rules", predict.RulesVersion, "mock", "6h")
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	_, body := get(t, ts, "/v1/model")
	var msg climateshieldv1.GetModelInfoResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))

	require.Equal(t, "rules", msg.GetActivePredictor())
	require.Equal(t, predict.ExceedanceRoleAnnotation, msg.GetExceedanceRole())
	require.Contains(t, msg.GetExceedanceRole(), "did not move any tier")
	// The reference provenance is reported whichever predictor is active: the
	// annotation is measured from the same artifact.
	require.NotEmpty(t, msg.GetReferenceSha256())
}

func TestClimateSeriesReportsSourceItWasActuallyIngestedFrom(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ingestFixtureWindow(t, q, "kisumu")

	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info"))
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	_, body := get(t, ts, "/v1/climate/series")
	var msg climateshieldv1.GetClimateSeriesResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))
	require.Len(t, msg.GetSeries(), 1)

	s := msg.GetSeries()[0]
	require.Equal(t, "Kisumu", s.GetArea())
	require.Equal(t, "fixture", s.GetSource(), "the source must be read back, never assumed")
	require.Len(t, s.GetDays(), 14)

	// The window that drives the demo scenario.
	var peak float64
	for _, d := range s.GetDays() {
		if d.GetPrecipitationMm() > peak {
			peak = d.GetPrecipitationMm()
		}
	}
	require.InDelta(t, 74.0, peak, 1e-9)

	// Area filter.
	_, body = get(t, ts, "/v1/climate/series?area=Nowhere")
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))
	require.Empty(t, msg.GetSeries())
}

func TestLedgerSummaryPublishesRootsButNeverLeaves(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)
	_, _, err = ledger.Sweep(ctx, q, anchor.Multi{anchor.NewLocal()}, logging.New(io.Discard, "info"))
	require.NoError(t, err)

	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info"))
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	_, body := get(t, ts, "/v1/ledger/summary")
	var msg climateshieldv1.GetLedgerSummaryResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))

	require.NotEmpty(t, msg.GetRoots())
	require.Positive(t, msg.GetTotalDays())
	// With a local anchor the note must not let anyone infer a chain, and the
	// mode is reported as configured.
	require.Equal(t, "local", msg.GetAnchorMode())
	require.Contains(t, msg.GetAnchorNote(), "No blockchain is written to by this system")
	require.False(t, msg.GetRoots()[0].GetReadbackMatches(), "a local anchor reads nothing back and must not claim to")
	require.Zero(t, msg.GetRoots()[0].GetChainId())
	require.Equal(t, hex.EncodeToString(func() []byte {
		days, err := q.ListLeafDays(ctx)
		require.NoError(t, err)
		stored, err := q.GetDailyRoot(ctx, days[0])
		require.NoError(t, err)
		return stored.Root
	}()), msg.GetRoots()[0].GetAnchorReference())

	// Every published root must match what the ledger actually stored, and
	// the leaf count (people-derived) must obey the k rule.
	days, err := q.ListLeafDays(ctx)
	require.NoError(t, err)
	stored, err := q.GetDailyRoot(ctx, days[0])
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(stored.Root), msg.GetRoots()[0].GetRootHex())
	require.Equal(t, "local", msg.GetRoots()[0].GetAnchorType())

	// NO leaf hash may appear anywhere in the response: a leaf is a per-child
	// HMAC, and publishing one would put a per-child artifact on a public
	// surface even though it looks opaque.
	leaves, err := q.LeavesForDay(ctx, days[0])
	require.NoError(t, err)
	require.NotEmpty(t, leaves)
	for _, l := range leaves {
		require.NotContains(t, body, hex.EncodeToString(l.LeafHash),
			"a per-child ledger leaf reached the public API")
	}
}

func TestLedgerSummarySuppressesSmallLeafCounts(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	// A single quiet day: one immunization means one identifiable family.
	require.NoError(t, q.UpsertDailyRoot(ctx, db.UpsertDailyRootParams{
		LeafDay:   pgtype.Date{Time: time.Now(), Valid: true},
		Root:      []byte{0xde, 0xad, 0xbe, 0xef},
		LeafCount: 1,
	}))

	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info"))
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	_, body := get(t, ts, "/v1/ledger/summary")
	var msg climateshieldv1.GetLedgerSummaryResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))
	require.Len(t, msg.GetRoots(), 1)
	require.True(t, msg.GetRoots()[0].GetLeafCountSuppressed())
	require.Nil(t, msg.GetRoots()[0].LeafCount)
	// The root itself is still published: it discloses nothing about anyone.
	require.Equal(t, "deadbeef", msg.GetRoots()[0].GetRootHex())
}

func TestAlertSummaryStatesTheMockChannelSendsNothing(t *testing.T) {
	pool := testdb.Pool(t)
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info")).
		WithDeployment("rules", predict.RulesVersion, "mock", "6h")
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	_, body := get(t, ts, "/v1/alerts/summary")
	var msg climateshieldv1.GetAlertSummaryResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))

	require.Equal(t, "mock", msg.GetChannel())
	require.False(t, msg.GetChannelSends(), "the mock channel must never report that it sends")
	require.Contains(t, msg.GetChannelNote(), "no SMS is sent")
	require.Contains(t, msg.GetQuietHours(), "21:00")

	// Templates are rendered with placeholders, within the single-segment
	// budget, and must never contain a disease name.
	require.Len(t, msg.GetTemplates(), 2)
	langs := map[string]bool{}
	for _, tpl := range msg.GetTemplates() {
		langs[tpl.GetLang()] = true
		require.Contains(t, tpl.GetBody(), "<CHILD>")
		require.Contains(t, tpl.GetBody(), "STOP")
		require.Positive(t, tpl.GetSeptets())
		require.LessOrEqual(t, tpl.GetSeptets(), tpl.GetMaxSeptets())
		for _, d := range []string{"cholera", "malaria", "pneumonia", "meningitis"} {
			require.NotContains(t, strings.ToLower(tpl.GetBody()), d)
		}
	}
	require.True(t, langs["en"] && langs["sw"], "both languages must be shown")
}

func TestAlertSummaryReportsALiveChannelHonestly(t *testing.T) {
	pool := testdb.Pool(t)
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info")).
		WithDeployment("rules", predict.RulesVersion, "smpp", "6h")
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	_, body := get(t, ts, "/v1/alerts/summary")
	var msg climateshieldv1.GetAlertSummaryResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))
	require.True(t, msg.GetChannelSends())
	require.Contains(t, msg.GetChannelNote(), "delivers")
}

func TestPipelineStatusReportsRealCounts(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ingestFixtureWindow(t, q, "kisumu")

	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info")).
		WithDeployment("rules", predict.RulesVersion, "mock", "6h")
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	_, body := get(t, ts, "/v1/pipeline")
	var msg climateshieldv1.GetPipelineStatusResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))

	require.EqualValues(t, 14, msg.GetClimateObservations())
	require.Equal(t, "6h", msg.GetIngestInterval())
	require.NotNil(t, msg.GetLatestObservationAt())
}

func TestEvidenceEndpointsNeverFailWhenBackendIsDown(t *testing.T) {
	pool, _ := testdb.PoolDSN(t)
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info"))
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)
	pool.Close()

	// The availability promise covers every read, including the new views.
	for _, path := range []string{
		"/v1/model", "/v1/climate/series", "/v1/ledger/summary",
		"/v1/ledger/anchors/verify", "/v1/ledger/anchors/verify?day=2026-08-07",
		"/v1/alerts/summary", "/v1/pipeline",
	} {
		resp, err := http.Get(ts.URL + path)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s must not 500", path)
	}
}

func TestClimatologyExplorerReturnsDistributionsAndMarker(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ingestFixtureWindow(t, q, "kisumu")

	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info")).
		WithDeployment("climatology", predict.ClimatologyVersion, "mock", "6h")
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	status, body := get(t, ts, "/v1/climatology?area=Kisumu&month=8")
	require.Equal(t, http.StatusOK, status)

	var msg climateshieldv1.GetClimatologyResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))
	require.Equal(t, "Kisumu", msg.GetArea())
	require.EqualValues(t, 8, msg.GetMonth())
	require.Positive(t, msg.GetSamples())
	require.Equal(t, "2015-01-01..2024-12-31", msg.GetReferencePeriod())
	require.Len(t, msg.GetDistributions(), 3)

	byDriver := map[string]*climateshieldv1.ClimatologyDistribution{}
	for _, d := range msg.GetDistributions() {
		byDriver[d.GetDriver()] = d
		require.NotEmpty(t, d.GetQuantiles())
		require.Len(t, d.GetPercentileSteps(), len(d.GetQuantiles()))
		require.True(t, sort.Float64sAreSorted(d.GetQuantiles()), "%s ladder must ascend", d.GetDriver())
	}

	// Cold stress is the only lower-tail driver.
	require.True(t, byDriver["14-day mean minimum temperature"].GetLowerTailIsHazard())
	require.False(t, byDriver["14-day peak rainfall"].GetLowerTailIsHazard())

	// The ingested window is marked on the distribution: the committed Kisumu
	// scenario is 74mm, beyond anything in the reference decade.
	rain := byDriver["14-day peak rainfall"]
	require.NotNil(t, rain.Observed)
	require.InDelta(t, 74.0, rain.GetObserved(), 1e-9)
	require.NotNil(t, rain.ObservedExceedance)
	require.Zero(t, rain.GetObservedExceedance())
}

func TestClimatologyExplorerRejectsBadInput(t *testing.T) {
	pool := testdb.Pool(t)
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info"))
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)

	for _, path := range []string{
		"/v1/climatology?area=Kisumu",           // no month
		"/v1/climatology?area=Kisumu&month=abc", // unparseable
		"/v1/climatology?area=Kisumu&month=0",   // out of range
		"/v1/climatology?area=Kisumu&month=13",
		"/v1/climatology?area=Atlantis&month=3", // unknown county
	} {
		status, _ := get(t, ts, path)
		require.Equal(t, http.StatusBadRequest, status, "%s must be a client error, not a 500", path)
	}
}
