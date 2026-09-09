// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/climate"
	"github.com/jarida-io/climateshield/internal/climate/fixture"
	"github.com/jarida-io/climateshield/internal/predict"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

// The demo's comparison cells must show the level, the value it was read
// from, and — only when one was measured — how rare that value is. A missing
// rarity must read as missing, never as zero.
func TestScorerCellShowsRarityOnlyWhenOneWasMeasured(t *testing.T) {
	exc := 0.062
	withRarity := scorerCell(predict.Prediction{
		Disease: predict.Cholera, Level: predict.Medium,
		Driver: predict.DriverPeakRainfall, DriverValue: 44.0, Exceedance: &exc,
	})
	require.Contains(t, withRarity, "MEDIUM")
	require.Contains(t, withRarity, "44.0mm")
	require.Contains(t, withRarity, "[top 6.2%]")

	zero := 0.0
	atExtreme := scorerCell(predict.Prediction{
		Disease: predict.Cholera, Level: predict.High,
		Driver: predict.DriverPeakRainfall, DriverValue: 74.0, Exceedance: &zero,
	})
	require.Contains(t, atExtreme, "[at the record extreme]")
	require.NotContains(t, atExtreme, "top 0.0%")

	without := scorerCell(predict.Prediction{
		Disease: predict.Meningitis, Level: predict.Low,
		Driver: predict.DriverMeanMaxTemp, DriverValue: 29.4,
	})
	require.Contains(t, without, "LOW")
	require.Contains(t, without, "29.4C")
	require.NotContains(t, without, "top")
	require.NotContains(t, without, "0.0%")
}

func TestDriverUnitPerPublishedDriver(t *testing.T) {
	require.Equal(t, "mm", driverUnit(predict.DriverPeakRainfall))
	require.Equal(t, "C", driverUnit(predict.DriverMeanMaxTemp))
	require.Equal(t, "C", driverUnit(predict.DriverMeanMinTemp))
}

// The county the comparison uses must be one the demo actually seeds, or the
// section would print "nothing to compare" on every run.
func TestDemoScorerCountyIsSeeded(t *testing.T) {
	clim, err := predict.LoadClimatology()
	require.NoError(t, err)
	require.Contains(t, clim.CountyIDs(), DemoScorerCounty)
	require.Equal(t, strings.ToLower(DemoScorerCounty), DemoScorerCounty)
}

// The demo section over the committed fixture window: both columns are
// printed, the active predictor is read back from the rows that were actually
// written, and the output says plainly that the comparison column scored
// nothing and sent nothing.
func TestReportBothScorersPrintsBothColumnsAndSaysWhichOneCounted(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	src := fixture.New(filepath.Join("..", "..", "testdata", "golden"))
	fc, err := src.FetchDaily(ctx, climate.Area{ID: DemoScorerCounty}, 14)
	require.NoError(t, err)
	_, err = climate.UpsertForecast(ctx, q, fc)
	require.NoError(t, err)

	// Before anything has scored, the header must not name a predictor.
	var before bytes.Buffer
	require.NoError(t, reportBothScorers(ctx, q, DemoScorerCounty, &before))
	require.Contains(t, before.String(), "no predictor has scored this county yet")

	// Write the rows the predictor service would have written.
	for _, p := range predict.NewRulesPredictor().Predict(predict.Features{
		AreaID: DemoScorerCounty, Month: int(fc.Days[0].Date.Month()),
		PeakRainfallMM: 74, MeanMaxTempC: 29.4, MeanMinTempC: 18.1,
	}) {
		_, err := q.UpsertRiskScore(ctx, db.UpsertRiskScoreParams{
			AreaID:           DemoScorerCounty,
			Disease:          string(p.Disease),
			Level:            string(p.Level),
			Driver:           p.Driver,
			DriverValue:      p.DriverValue,
			ForecastDate:     pgtype.Date{Time: fc.Days[0].Date, Valid: true},
			WindowDays:       14,
			Predictor:        "rules",
			PredictorVersion: predict.RulesVersion,
		})
		require.NoError(t, err)
	}

	var out bytes.Buffer
	require.NoError(t, reportBothScorers(ctx, q, DemoScorerCounty, &out))
	got := out.String()

	require.Contains(t, got, "Same weather, both scorers")
	require.Contains(t, got, "rules v"+predict.RulesVersion)
	require.Contains(t, got, "published thresholds")
	require.Contains(t, got, "reference climatology")
	for _, d := range predict.Diseases {
		require.Contains(t, got, string(d))
	}
	require.Contains(t, got, "neither column is validated against disease outcomes")
	require.Contains(t, got, "mean MINIMUM")
	require.Contains(t, got, "sent nothing")
	// No accuracy or learned-model language may reach the demo transcript.
	for _, banned := range []string{"accura", "trained", "machine learning"} {
		require.NotContains(t, strings.ToLower(got), banned)
	}

	// A county with no observations says so instead of printing an empty table.
	var empty bytes.Buffer
	require.NoError(t, reportBothScorers(ctx, q, "nakuru", &empty))
	require.Contains(t, empty.String(), "nothing to compare")

	// An unknown county is an error from the query layer, not a fabricated row.
	_, err = q.LatestObservationWindow(ctx, "atlantis")
	require.NoError(t, err, "an unknown area simply has no window")
	var unknown bytes.Buffer
	require.NoError(t, reportBothScorers(ctx, q, "atlantis", &unknown))
	require.Contains(t, unknown.String(), "nothing to compare")
}
