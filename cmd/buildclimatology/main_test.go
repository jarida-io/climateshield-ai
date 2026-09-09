// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/predict"
	"github.com/jarida-io/climateshield/internal/store/seed"
)

// goldenArchive serves the committed SYNTHETIC archive responses (see
// testdata/golden/README.md) keyed by the requested latitude, so the whole
// tool can run end to end without a network.
func goldenArchive(t *testing.T) *httptest.Server {
	t.Helper()
	byLatitude := map[string]string{}
	for _, c := range seed.Counties {
		byLatitude[fmt.Sprintf("%.4f", c.Lat)] = filepath.Join("testdata", "golden", "archive-"+c.ID+".json")
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/archive" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if q.Get("daily") != DailyVariables || q.Get("timezone") != "Africa/Nairobi" ||
			q.Get("start_date") == "" || q.Get("end_date") == "" {
			http.Error(w, "unexpected query: "+r.URL.RawQuery, http.StatusBadRequest)
			return
		}
		path, ok := byLatitude[q.Get("latitude")]
		if !ok {
			http.Error(w, "no fixture for latitude "+q.Get("latitude"), http.StatusNotFound)
			return
		}
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
}

// The whole tool, end to end against the golden sample: it must write a file
// the predictor can load, with the committed artifact's shape — 21 quantile
// steps, one distribution per driver, per-month window counts that follow the
// calendar — and report the digest of what it wrote.
func TestBuildWritesAnArtifactWithTheCommittedShape(t *testing.T) {
	ts := goldenArchive(t)
	t.Cleanup(ts.Close)

	out := filepath.Join(t.TempDir(), "golden-climatology.json")
	var log bytes.Buffer
	require.NoError(t, run([]string{
		"-base", ts.URL, "-out", out,
		"-from", "2015-01-01", "-to", "2015-03-15", "-window", "14",
	}, &log))

	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	var got predict.Climatology
	require.NoError(t, json.Unmarshal(raw, &got))

	require.Equal(t, 1, got.SchemaVersion)
	require.Equal(t, "2015-01-01..2015-03-15", got.ReferencePeriod)
	require.Equal(t, generatorName, got.GeneratedBy)
	require.Equal(t, licenceLabel, got.SourceLicence)
	require.Equal(t, 14, got.WindowDays)
	require.Equal(t, 21, got.QuantileSteps())
	require.Equal(t, []string{"eldoret", "kisumu", "mombasa", "nairobi", "nakuru"}, got.CountyIDs())

	// 74 days, 14-day windows: 60 windows, split by the month each starts in.
	for _, id := range got.CountyIDs() {
		months := got.Counties[id].Months
		require.Len(t, months, 3)
		require.Equal(t, 31, months["1"].Samples)
		require.Equal(t, 28, months["2"].Samples)
		require.Equal(t, 1, months["3"].Samples)
		for _, m := range months {
			require.Len(t, m.Quantiles, 3)
			for driver, ladder := range m.Quantiles {
				require.Len(t, ladder, 21, driver)
			}
		}
	}
	require.Equal(t, 5*60, got.TotalSamples())

	// The digest printed is the digest of the bytes on disk.
	sum := sha256.Sum256(raw)
	require.Contains(t, log.String(), hex.EncodeToString(sum[:]))
	require.Contains(t, log.String(), "no credentials are used")
	require.Contains(t, log.String(), "5 counties, 300 14-day windows")
}

// A byte-identical rerun: the tool must be deterministic, or the published
// SHA-256 says nothing.
func TestBuildIsDeterministic(t *testing.T) {
	ts := goldenArchive(t)
	t.Cleanup(ts.Close)

	dir := t.TempDir()
	first := filepath.Join(dir, "a.json")
	second := filepath.Join(dir, "b.json")
	for _, out := range []string{first, second} {
		require.NoError(t, run([]string{"-base", ts.URL, "-out", out,
			"-from", "2015-01-01", "-to", "2015-03-15"}, &bytes.Buffer{}))
	}
	a, err := os.ReadFile(first)
	require.NoError(t, err)
	b, err := os.ReadFile(second)
	require.NoError(t, err)
	require.Equal(t, string(a), string(b))
}

// -digest hashes an existing file and makes no request at all. It is how the
// digest published on /v1/model is checked offline.
func TestDigestModeMatchesShasumAndMakesNoRequest(t *testing.T) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	t.Cleanup(ts.Close)

	var out bytes.Buffer
	require.NoError(t, run([]string{"-digest", "-out", committedArtifact, "-base", ts.URL}, &out))

	raw, err := os.ReadFile(committedArtifact)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	require.Equal(t, hex.EncodeToString(sum[:])+"  "+committedArtifact+"\n", out.String())
	require.Zero(t, requests)

	// The same digest the API publishes for the embedded bytes.
	embedded, err := predict.ClimatologyDigest()
	require.NoError(t, err)
	require.Contains(t, out.String(), embedded)

	require.Error(t, run([]string{"-digest", "-out", filepath.Join(t.TempDir(), "missing.json")}, &out))
}

// Bad input must stop the tool rather than produce a distribution nobody can
// account for.
func TestBadInputIsRefused(t *testing.T) {
	var out bytes.Buffer
	require.ErrorContains(t, run([]string{"-window", "1"}, &out), "-window must be at least 2")
	require.ErrorContains(t, run([]string{"-from", "yesterday"}, &out), "YYYY-MM-DD")
	require.Error(t, run([]string{"-nonsense"}, &out))
}

func TestArchiveErrorsAreReportedWithTheCounty(t *testing.T) {
	ctx := context.Background()
	county := seed.Counties[0]

	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	t.Cleanup(notFound.Close)
	_, err := newArchiveClient(notFound.URL, time.Second).daily(ctx, county, "2015-01-01", "2015-01-31")
	require.ErrorContains(t, err, county.ID)
	require.ErrorContains(t, err, "status 404")

	garbage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	t.Cleanup(garbage.Close)
	_, err = newArchiveClient(garbage.URL, time.Second).daily(ctx, county, "2015-01-01", "2015-01-31")
	require.ErrorContains(t, err, county.ID)

	// An unreachable origin, and a build that therefore writes nothing.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	dead.Close()
	out := filepath.Join(t.TempDir(), "never-written.json")
	require.Error(t, run([]string{"-base", dead.URL, "-out", out}, &bytes.Buffer{}))
	_, err = os.Stat(out)
	require.True(t, os.IsNotExist(err))
}

// A day missing from the record would silently shorten a window and shift
// every later window's month, so it is refused rather than smoothed over.
func TestAGapInTheDailyRecordIsRefused(t *testing.T) {
	gapped := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"daily":{"time":["2015-01-01","2015-01-02","2015-01-04"],
			"precipitation_sum":[0,1,2],"temperature_2m_max":[20,21,22],"temperature_2m_min":[10,11,12]}}`))
	}))
	t.Cleanup(gapped.Close)

	_, err := newArchiveClient(gapped.URL, time.Second).
		daily(context.Background(), seed.Counties[0], "2015-01-01", "2015-01-04")
	require.ErrorContains(t, err, "gap in the daily record")
	require.ErrorContains(t, err, "2015-01-02 is followed by 2015-01-04")

	require.ErrorContains(t, checkContiguous(nil), "no days")
}

// A failure to write must be reported, not swallowed.
func TestAnUnwritableDestinationIsReported(t *testing.T) {
	ts := goldenArchive(t)
	t.Cleanup(ts.Close)
	err := run([]string{"-base", ts.URL, "-out", filepath.Join(t.TempDir(), "no-such-dir", "x.json"),
		"-from", "2015-01-01", "-to", "2015-03-15"}, &bytes.Buffer{})
	require.ErrorContains(t, err, "write")
}

// The tool's own help text must not describe the artifact as trained, learned
// or accurate — it is a measured distribution and nothing more.
func TestToolOutputMakesNoLearnedOrAccuracyClaim(t *testing.T) {
	ts := goldenArchive(t)
	t.Cleanup(ts.Close)
	var out bytes.Buffer
	dest := filepath.Join(t.TempDir(), "a.json")
	require.NoError(t, run([]string{"-base", ts.URL, "-out", dest,
		"-from", "2015-01-01", "-to", "2015-03-15"}, &out))

	// The temporary path is the test harness's wording, not the tool's.
	lower := strings.ToLower(strings.ReplaceAll(out.String(), dest, "<out>"))
	for _, banned := range []string{"train", "learn", "accura", "ai ", "model of disease", "predict outbreak"} {
		require.NotContains(t, lower, banned)
	}
}
