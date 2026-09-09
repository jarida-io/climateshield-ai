// SPDX-License-Identifier: Apache-2.0

package facts_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// fakeQuerier stands in for the database. It returns the aggregate rows the
// real queries return, so the builder is tested without Docker.
type fakeQuerier struct {
	areas  []db.ListAreasRow
	risk   []db.CurrentRiskRow
	series []db.LatestSeriesForAllAreasRow
	alerts []db.CountAlertsByStatusRow

	areasErr, riskErr, seriesErr, alertsErr error
}

func (f fakeQuerier) ListAreas(context.Context) ([]db.ListAreasRow, error) {
	return f.areas, f.areasErr
}

func (f fakeQuerier) CurrentRisk(context.Context) ([]db.CurrentRiskRow, error) {
	return f.risk, f.riskErr
}

func (f fakeQuerier) LatestSeriesForAllAreas(context.Context) ([]db.LatestSeriesForAllAreasRow, error) {
	return f.series, f.seriesErr
}

func (f fakeQuerier) CountAlertsByStatus(context.Context) ([]db.CountAlertsByStatusRow, error) {
	return f.alerts, f.alertsErr
}

func date(s string) pgtype.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return pgtype.Date{Time: t, Valid: true}
}

func explained(s string) *string { return &s }

func populated() fakeQuerier {
	return fakeQuerier{
		areas: []db.ListAreasRow{
			{ID: "kisumu", Name: "Kisumu"},
			{ID: "nairobi", Name: "Nairobi"},
		},
		series: []db.LatestSeriesForAllAreasRow{
			{AreaID: "kisumu", AreaName: "Kisumu", ForecastDate: date("2026-09-11"), Source: "fixture"},
			{AreaID: "kisumu", AreaName: "Kisumu", ForecastDate: date("2026-09-10"), Source: "fixture"},
			{AreaID: "kisumu", AreaName: "Kisumu", ForecastDate: date("2026-09-12"), Source: "fixture"},
			{AreaID: "nairobi", AreaName: "Nairobi", ForecastDate: date("2026-09-10"), Source: "openmeteo"},
		},
		risk: []db.CurrentRiskRow{
			// Deliberately out of the predictor's order, to prove the builder
			// sorts rather than depending on the query's ORDER BY.
			{
				AreaID: "kisumu", Disease: "meningitis", Level: "LOW", Driver: "mean_max_temp_c_14d",
				DriverValue: 28.4, Predictor: "rules", PredictorVersion: "1.0.0",
				Explanation: explained("below the MEDIUM threshold of 36°C"),
			},
			{
				AreaID: "kisumu", Disease: "cholera", Level: "HIGH", Driver: "peak_rainfall_mm_14d",
				DriverValue: 74, Predictor: "rules", PredictorVersion: "1.0.0",
			},
			{AreaID: "nairobi", Disease: "cholera", Level: "LOW", Driver: "peak_rainfall_mm_14d"},
		},
		alerts: []db.CountAlertsByStatusRow{
			{Status: "would_send", N: 24},
			{Status: "skipped_consent", N: 3},
			{Status: "failed", N: 0},
		},
	}
}

func TestBuildFactSheet(t *testing.T) {
	sheet, err := facts.BuildFactSheet(
		context.Background(), populated(), facts.Area{ID: "kisumu", Name: "Kisumu"},
		"mock", time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	require.Equal(t, "Kisumu", sheet.Area)
	require.Equal(t, "2026-09-10", sheet.Window.From)
	require.Equal(t, "2026-09-12", sheet.Window.To)
	require.Equal(t, 3, sheet.Window.Days, "only this county's window is counted")
	require.Equal(t, "fixture", sheet.Window.Source)

	require.Len(t, sheet.Scores, 2, "another county's scores must not appear")
	require.Equal(t, "cholera", sheet.Scores[0].Disease, "scores follow the predictor's order")
	require.Equal(t, "meningitis", sheet.Scores[1].Disease)
	require.Equal(t, "below the MEDIUM threshold of 36°C", sheet.Scores[1].Explanation)
	require.Empty(t, sheet.Scores[0].Explanation, "a missing explanation is left empty, not invented")

	require.False(t, sheet.ChannelSends)
	require.Contains(t, sheet.ChannelNote, "no SMS is sent")

	require.Len(t, sheet.AlertsAllCounties, 3)
	require.Equal(t, int64(24), *sheet.AlertsAllCounties[0].Count)
	require.False(t, sheet.AlertsAllCounties[0].Suppressed)
	require.Nil(t, sheet.AlertsAllCounties[1].Count, "3 is below k and must be withheld")
	require.True(t, sheet.AlertsAllCounties[1].Suppressed)
	require.Equal(t, int64(0), *sheet.AlertsAllCounties[2].Count, "zero is not suppressed")
}

func TestBuildFactSheetWithNoData(t *testing.T) {
	sheet, err := facts.BuildFactSheet(
		context.Background(), fakeQuerier{}, facts.Area{ID: "kisumu", Name: "Kisumu"},
		"mock", time.Now())
	require.NoError(t, err)
	require.Empty(t, sheet.Scores)
	require.Empty(t, sheet.Window.From)
	require.Equal(t, 0, sheet.Window.Days)
}

func TestBuildFactSheetSortsUnknownDiseasesLast(t *testing.T) {
	q := populated()
	q.risk = append(q.risk, db.CurrentRiskRow{
		AreaID: "kisumu", Disease: "zzz-unscored", Level: "LOW",
	})
	sheet, err := facts.BuildFactSheet(
		context.Background(), q, facts.Area{ID: "kisumu", Name: "Kisumu"}, "mock", time.Now())
	require.NoError(t, err)
	require.Equal(t, "zzz-unscored", sheet.Scores[len(sheet.Scores)-1].Disease)
}

func TestBuildFactSheetReportsQueryFailures(t *testing.T) {
	boom := errors.New("database is down")
	for name, q := range map[string]fakeQuerier{
		"series": {seriesErr: boom},
		"risk":   {riskErr: boom},
		"alerts": {alertsErr: boom},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := facts.BuildFactSheet(
				context.Background(), q, facts.Area{ID: "kisumu", Name: "Kisumu"}, "mock", time.Now())
			require.ErrorIs(t, err, boom)
		})
	}
}

func TestAreas(t *testing.T) {
	areas, err := facts.Areas(context.Background(), populated())
	require.NoError(t, err)
	require.Equal(t, []facts.Area{{ID: "kisumu", Name: "Kisumu"}, {ID: "nairobi", Name: "Nairobi"}}, areas)

	_, err = facts.Areas(context.Background(), fakeQuerier{areasErr: errors.New("nope")})
	require.Error(t, err)
}

// TestChannelNote is the honesty rule in one assertion: a channel that sends
// nothing must say so, and one that sends must not be described as mock.
func TestChannelNote(t *testing.T) {
	sends, note := facts.ChannelNote("mock")
	require.False(t, sends)
	require.Contains(t, note, "no SMS is sent")

	sends, note = facts.ChannelNote("")
	require.False(t, sends)
	require.Contains(t, note, "no SMS is sent")

	sends, note = facts.ChannelNote("smpp")
	require.True(t, sends)
	require.Contains(t, note, "smpp")
	require.NotContains(t, note, "no SMS is sent")
}
