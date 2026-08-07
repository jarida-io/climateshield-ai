// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/climate"
	"github.com/jarida-io/climateshield/internal/predict"
)

var goldenDir = filepath.Join("..", "..", "..", "testdata", "golden")

func TestAllCountiesHaveGoldenFixtures(t *testing.T) {
	s := New(goldenDir)
	for _, id := range []string{"nairobi", "kisumu", "mombasa", "nakuru", "eldoret"} {
		fc, err := s.FetchDaily(context.Background(), climate.Area{ID: id}, 14)
		require.NoError(t, err, "fixture for %s", id)
		require.Len(t, fc.Days, 14)
		require.Equal(t, SourceName, fc.Source)
	}
}

func TestFixtureReproducesDemoScenario(t *testing.T) {
	s := New(goldenDir)
	want := map[string][2]float64{ // areaID -> {peak rainfall, mean max temp}
		"nairobi": {18, 23.4},
		"kisumu":  {74, 28.1},
		"mombasa": {41, 31.6},
		"nakuru":  {12, 21.0},
		"eldoret": {8, 17.2},
	}
	for id, exp := range want {
		fc, err := s.FetchDaily(context.Background(), climate.Area{ID: id}, 14)
		require.NoError(t, err)
		precip, tmax := make([]float64, 0, 14), make([]float64, 0, 14)
		tmin := make([]float64, 0, 14)
		for _, d := range fc.Days {
			precip = append(precip, d.PrecipitationSumMM)
			tmax = append(tmax, d.TempMaxC)
			tmin = append(tmin, d.TempMinC)
		}
		f, err := predict.FeaturesFrom(id, int(fc.Days[0].Date.Month()), precip, tmax, tmin)
		require.NoError(t, err)
		require.InDelta(t, exp[0], f.PeakRainfallMM, 1e-9, "%s peak rainfall", id)
		require.InDelta(t, exp[1], f.MeanMaxTempC, 1e-9, "%s mean max temp", id)
	}
}

func TestFixtureDeterministicIssuedAt(t *testing.T) {
	s := New(goldenDir)
	a, err := s.FetchDaily(context.Background(), climate.Area{ID: "kisumu"}, 14)
	require.NoError(t, err)
	b, err := s.FetchDaily(context.Background(), climate.Area{ID: "kisumu"}, 14)
	require.NoError(t, err)
	require.Equal(t, a.IssuedAt, b.IssuedAt, "fixture ingestion must be idempotent across runs")
}

func TestFixtureTruncatesToRequestedDays(t *testing.T) {
	s := New(goldenDir)
	fc, err := s.FetchDaily(context.Background(), climate.Area{ID: "kisumu"}, 7)
	require.NoError(t, err)
	require.Len(t, fc.Days, 7)
}

func TestFixtureMissingArea(t *testing.T) {
	s := New(goldenDir)
	_, err := s.FetchDaily(context.Background(), climate.Area{ID: "atlantis"}, 14)
	require.Error(t, err)
}
