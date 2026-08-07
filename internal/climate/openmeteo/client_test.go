// SPDX-License-Identifier: Apache-2.0

package openmeteo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/climate"
	"github.com/jarida-io/climateshield/internal/platform/clock"
	"github.com/jarida-io/climateshield/internal/predict"
)

// The client is tested exclusively against a local httptest server replaying
// the committed golden payload — no test touches the real API.
func TestFetchDailyParsesGoldenPayload(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "golden", "openmeteo_kisumu.json"))
	require.NoError(t, err)

	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/forecast", r.URL.Path)
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(golden)
	}))
	defer srv.Close()

	issued := time.Date(2026, 8, 7, 5, 30, 0, 0, time.UTC)
	c := New(srv.URL, clock.Fixed{T: issued})

	fc, err := c.FetchDaily(context.Background(), climate.Area{ID: "kisumu", Lat: -0.1022, Lon: 34.7617}, 14)
	require.NoError(t, err)

	require.Equal(t, "kisumu", fc.AreaID)
	require.Equal(t, SourceName, fc.Source)
	require.Equal(t, issued, fc.IssuedAt)
	require.Len(t, fc.Days, 14)

	// Query contract with Open-Meteo (mirrors the Python prototype).
	require.Equal(t, []string{"-0.1022"}, gotQuery["latitude"])
	require.Equal(t, []string{"34.7617"}, gotQuery["longitude"])
	require.Equal(t, []string{"Africa/Nairobi"}, gotQuery["timezone"])
	require.Equal(t, []string{"14"}, gotQuery["forecast_days"])
	require.Equal(t,
		[]string{"precipitation_sum,temperature_2m_max,temperature_2m_min,relative_humidity_2m_max"},
		gotQuery["daily"])

	// The golden Kisumu window must reproduce the demo scenario features.
	precip := make([]float64, 0, 14)
	tmax := make([]float64, 0, 14)
	for _, d := range fc.Days {
		precip = append(precip, d.PrecipitationSumMM)
		tmax = append(tmax, d.TempMaxC)
	}
	f, err := predict.FeaturesFrom(precip, tmax)
	require.NoError(t, err)
	require.InDelta(t, 74.0, f.PeakRainfallMM, 1e-9)
	require.InDelta(t, 28.1, f.MeanMaxTempC, 1e-9)
}

func TestFetchDailyNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New(srv.URL, clock.Real{})
	_, err := c.FetchDaily(context.Background(), climate.Area{ID: "kisumu"}, 14)
	require.ErrorContains(t, err, "429")
}
