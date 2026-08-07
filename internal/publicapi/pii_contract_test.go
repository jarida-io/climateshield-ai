// SPDX-License-Identifier: Apache-2.0

// CONTRACT TESTS — DO NOT DELETE OR RENAME.
//
// TestContract_PIILeak and TestContract_KAnonymity enforce funding-agreement
// commitments (no personal data on any public surface; k>=10 suppression).
// CI runs them BY NAME and fails if they do not run; scripts/
// contract-checks.sh fails if they are missing from this package. Weakening
// either test is a release-blocking change requiring sign-off.

package publicapi_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/publicapi"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/seed"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

// Sentinel values: if any of these strings ever appears in a public
// response, PII has leaked. They are deliberately weird so a hit is
// unambiguous.
const (
	sentinelChildName    = "ZEPHYRINE-XKCD-CHILD"
	sentinelGuardianName = "QUIXOTIC-XKCD-GUARDIAN"
	sentinelPhone        = "+254999888777"
	sentinelNationalID   = "98765432"
)

// forbiddenFieldNames must never appear as JSON keys, CSV headers or GeoJSON
// property names in any public response.
var forbiddenFieldNames = []string{
	"child_id", "childId", "guardian", "phone", "national_id", "nationalId",
	"date_of_birth", "dateOfBirth", "first_name", "firstName", "name_enc", "phone_enc",
}

// seedWithSentinels loads the demo population plus one guardian/child pair
// carrying sentinel PII in every encrypted field, an immunization event, an
// alert row, and risk scores — so every public code path has person-derived
// data behind it when the tests probe for leaks.
func seedWithSentinels(t *testing.T) *httptest.Server {
	t.Helper()
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)

	seal := func(v string) []byte {
		enc, err := crypto.Seal(key, v)
		require.NoError(t, err)
		return enc.Bytes()
	}
	gid, err := q.CreateGuardian(ctx, db.CreateGuardianParams{
		NameEnc:       seal(sentinelGuardianName),
		PhoneEnc:      seal(sentinelPhone),
		NationalIDEnc: seal(sentinelNationalID),
		Lang:          "en",
	})
	require.NoError(t, err)
	require.NoError(t, q.AppendConsent(ctx, db.AppendConsentParams{GuardianID: gid, Action: "OPT_IN", Channel: "sms"}))
	cid, err := q.CreateChild(ctx, db.CreateChildParams{
		GuardianID: gid, AreaID: "kisumu", NameEnc: seal(sentinelChildName),
		DateOfBirth: pgtype.Date{Time: time.Now().AddDate(0, -5, 0), Valid: true},
	})
	require.NoError(t, err)
	_, err = q.InsertImmunizationEvent(ctx, db.InsertImmunizationEventParams{
		ChildID: cid, VaccineCode: "bcg",
		AdministeredAt: pgtype.Timestamptz{Time: time.Now().AddDate(0, -5, 0), Valid: true},
	})
	require.NoError(t, err)

	scoreID, err := q.UpsertRiskScore(ctx, db.UpsertRiskScoreParams{
		AreaID: "kisumu", Disease: "cholera", Level: "HIGH",
		Driver: "peak_rainfall_mm_14d", DriverValue: 74,
		ForecastDate: pgtype.Date{Time: time.Now(), Valid: true},
		WindowDays:   14, Predictor: "rules", PredictorVersion: "1.0.0",
	})
	require.NoError(t, err)
	_, err = q.InsertAlert(ctx, db.InsertAlertParams{
		RiskScoreID: &scoreID, ChildID: cid, GuardianID: gid, AreaID: "kisumu",
		VaccineCode: "opv3", Lang: "en", Channel: "mock", Status: "would_send",
	})
	require.NoError(t, err)

	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info"))
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)
	return ts
}

func get(t *testing.T, ts *httptest.Server, path string) (int, string) {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

// TestContract_PIILeak probes every public endpoint in every format and
// asserts that no sentinel PII value and no forbidden field name appears
// anywhere in any response body.
func TestContract_PIILeak(t *testing.T) {
	ts := seedWithSentinels(t)

	paths := []string{
		"/v1/risk/current",
		"/v1/risk/current?format=csv",
		"/v1/risk/current?format=geojson",
		"/v1/risk/history",
		"/v1/risk/history?format=csv",
		"/v1/risk/history?format=geojson",
		"/v1/risk/history?area=Kisumu&disease=cholera",
		"/v1/stats",
		"/v1/stats?format=csv",
		"/health",
	}
	sentinels := []string{
		sentinelChildName, sentinelGuardianName, sentinelPhone, sentinelNationalID,
		// Seed-population names must not leak either.
		"Amina", "Odhiambo", "+2547000001",
	}

	for _, path := range paths {
		status, body := get(t, ts, path)
		require.Equal(t, http.StatusOK, status, "GET %s", path)

		lower := strings.ToLower(body)
		for _, s := range sentinels {
			require.NotContains(t, body, s, "PII value leaked into %s", path)
			require.NotContains(t, lower, strings.ToLower(s), "PII value leaked into %s (case-folded)", path)
		}
		for _, f := range forbiddenFieldNames {
			require.NotContains(t, lower, strings.ToLower(f), "forbidden field name %q in %s", f, path)
		}
	}

	// Connect surface too: the same rule holds for the RPC responses.
	for _, procedure := range []string{"GetCurrentRisk", "GetStats"} {
		resp, err := http.Post(
			ts.URL+"/climateshield.v1.PublicService/"+procedure,
			"application/json", strings.NewReader("{}"))
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		for _, s := range sentinels {
			require.NotContains(t, string(body), s, "PII value leaked from Connect %s", procedure)
		}
	}
}

// TestContract_KAnonymity asserts the k>=10 suppression rule end to end:
// counties under the threshold expose no number, counties at/above it do,
// and zero passes through.
func TestContract_KAnonymity(t *testing.T) {
	ts := seedWithSentinels(t) // Kisumu 13 (12 + sentinel child), Eldoret 11, Mombasa 3, Nakuru 2, Nairobi 0

	status, body := get(t, ts, "/v1/stats")
	require.Equal(t, http.StatusOK, status)

	stats := parseStats(t, body)

	require.Equal(t, int64(13), *stats["Kisumu"].registered, "k>=10 must be shown")
	require.False(t, stats["Kisumu"].suppressed)
	require.Equal(t, int64(11), *stats["Eldoret"].registered)

	require.Nil(t, stats["Mombasa"].registered, "0<n<10 must be withheld")
	require.True(t, stats["Mombasa"].suppressed)
	require.Nil(t, stats["Nakuru"].registered)
	require.True(t, stats["Nakuru"].suppressed)

	require.NotNil(t, stats["Nairobi"].registered, "zero is not suppressed")
	require.Zero(t, *stats["Nairobi"].registered)
	require.False(t, stats["Nairobi"].suppressed)

	// The suppressed counts must not be derivable from CSV either.
	_, csvBody := get(t, ts, "/v1/stats?format=csv")
	for _, line := range strings.Split(strings.TrimSpace(csvBody), "\n") {
		if strings.HasPrefix(line, "Mombasa") || strings.HasPrefix(line, "Nakuru") {
			fields := strings.Split(line, ",")
			require.Empty(t, fields[1], "suppressed CSV cell must be empty: %s", line)
			require.Equal(t, "true", fields[2])
		}
	}
}
