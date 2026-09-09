// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/climate"
	"github.com/jarida-io/climateshield/internal/predict"
)

// synthetic builds a contiguous daily record. The values are arbitrary; only
// the day count and the calendar matter to the tests that use it.
func synthetic(t *testing.T, from, to string) []climate.Day {
	t.Helper()
	start, err := time.Parse("2006-01-02", from)
	require.NoError(t, err)
	end, err := time.Parse("2006-01-02", to)
	require.NoError(t, err)

	var days []climate.Day
	for d, i := start, 0; !d.After(end); d, i = d.AddDate(0, 0, 1), i+1 {
		days = append(days, climate.Day{
			Date:               d,
			PrecipitationSumMM: float64((i*7)%50) / 10,
			TempMaxC:           20 + float64((i*3)%120)/10,
			TempMinC:           10 + float64((i*11)%80)/10,
		})
	}
	return days
}

// The generator must produce the committed artifact's SHAPE: the same window
// length, the same 21-step ladder, and — the load-bearing part — the same
// number of windows in every calendar month, county by county. Those counts
// are a pure function of the calendar and the window loop, so they can be
// checked against the committed file with no network and no weather.
func TestWindowCountsPerMonthMatchTheCommittedArtifact(t *testing.T) {
	raw, err := os.ReadFile(committedArtifact)
	require.NoError(t, err)
	var committed predict.Climatology
	require.NoError(t, json.Unmarshal(raw, &committed))

	days := synthetic(t, "2015-01-01", "2024-12-31")
	require.Len(t, days, 3653, "ten years including three leap days")

	months := countyMonths(days, committed.WindowDays)
	require.Len(t, months, 12)

	for _, countyID := range committed.CountyIDs() {
		for month, want := range committed.Counties[countyID].Months {
			got, ok := months[month]
			require.True(t, ok, "month %s missing", month)
			require.Equal(t, want.Samples, got.Samples,
				"%s month %s: window count must match the committed artifact", countyID, month)
			require.Len(t, got.Quantiles, len(want.Quantiles))
			for driver, ladder := range got.Quantiles {
				require.Len(t, ladder, committed.QuantileSteps(),
					"%s must carry the committed 21-step ladder", driver)
				require.True(t, sort.Float64sAreSorted(ladder), "%s ladder must ascend", driver)
			}
		}
	}

	total := 0
	for _, m := range months {
		total += m.Samples
	}
	require.Equal(t, committed.TotalSamples()/len(committed.Counties), total,
		"per-county window total must match the committed artifact")
	require.Equal(t, 3639, total)
}

// The window loop stops one window short of the end of the record. It is a
// quirk of the generator that produced the committed artifact, kept so the
// artifact stays reproducible, and it is pinned here so nobody "fixes" it
// without noticing the artifact would change.
func TestWindowLoopStopsOneWindowShortOfTheRecord(t *testing.T) {
	days := synthetic(t, "2015-01-01", "2015-01-31") // 31 days
	require.Len(t, windowsFrom(days, 14), 31-14)
	require.Len(t, windowsFrom(days, 30), 1)
	require.Empty(t, windowsFrom(days, 31))
	require.Empty(t, windowsFrom(days, 0))
}

// Each window is reduced with exactly the arithmetic predict.FeaturesFrom
// uses at runtime: peak daily rainfall, mean daily max, mean daily min.
func TestWindowFeaturesMatchTheRuntimeFeatureComputation(t *testing.T) {
	days := synthetic(t, "2015-01-01", "2015-02-28")
	windowDays := 14

	got := windowsFrom(days, windowDays)
	require.NotEmpty(t, got)

	for i, w := range got {
		slice := days[i : i+windowDays]
		precip := make([]float64, 0, windowDays)
		tmax := make([]float64, 0, windowDays)
		tmin := make([]float64, 0, windowDays)
		for _, d := range slice {
			precip = append(precip, d.PrecipitationSumMM)
			tmax = append(tmax, d.TempMaxC)
			tmin = append(tmin, d.TempMinC)
		}
		want, err := predict.FeaturesFrom("kisumu", int(slice[0].Date.Month()), precip, tmax, tmin)
		require.NoError(t, err)

		require.InDelta(t, want.PeakRainfallMM, w.peakRain, 1e-12)
		require.InDelta(t, want.MeanMaxTempC, w.meanTmax, 1e-12)
		require.InDelta(t, want.MeanMinTempC, w.meanTmin, 1e-12)
		require.Equal(t, want.Month, w.month)
	}
}

// Every stored quantile must be a value the record actually produced. The
// committed artifact has that property — its temperature quantiles are all
// exact multiples of 1/140, i.e. means of fourteen one-decimal readings — and
// the generator must keep it, because an interpolated quantile would be a
// number no window ever had.
func TestQuantilesAreOrderStatisticsNotInterpolations(t *testing.T) {
	sample := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, p := range quantileSteps() {
		q := quantile(sample, p)
		require.Contains(t, sample, q, "p%d must be a value from the sample", p)
	}
	require.InDelta(t, 1.0, quantile(sample, 0), 0)
	require.InDelta(t, 10.0, quantile(sample, 100), 0)
	require.InDelta(t, 5.0, quantile(sample, 50), 0, "virtual index 4.5 rounds half to even, to 4")
	require.InDelta(t, 0.0, quantile(nil, 50), 0)
}

func TestQuantileStepsAreTheCommittedLadder(t *testing.T) {
	raw, err := os.ReadFile(committedArtifact)
	require.NoError(t, err)
	var committed predict.Climatology
	require.NoError(t, json.Unmarshal(raw, &committed))
	require.Equal(t, committed.QuantileStepsPct, quantileSteps())
	require.Len(t, quantileSteps(), 21)
}

func TestRoundingToTheStoredPrecision(t *testing.T) {
	require.InDelta(t, 22.314, round3(312.4/14), 1e-12)
	require.InDelta(t, 25.5, round3(357.0/14), 1e-12)
	require.InDelta(t, 0.1, round3(0.1), 1e-12)
}

func TestRoundHalfToEven(t *testing.T) {
	require.Equal(t, 4, roundHalfToEven(4.5))
	require.Equal(t, 6, roundHalfToEven(5.5))
	require.Equal(t, 5, roundHalfToEven(5.4))
	require.Equal(t, 0, roundHalfToEven(0))
}

// The assembled artifact must satisfy the loader that consumes it, carry the
// provenance it claims, and refuse to be built from nothing.
func TestBuildClimatologyProducesALoadableArtifact(t *testing.T) {
	records := []countyRecord{
		{ID: "kisumu", Days: synthetic(t, "2015-01-01", "2015-03-15")},
		{ID: "nairobi", Days: synthetic(t, "2015-01-01", "2015-03-15")},
	}
	c, err := buildClimatology(records, 14, "2015-01-01..2015-03-15", sourceLabel, licenceLabel, generatorName)
	require.NoError(t, err)

	require.Equal(t, generatorName, c.GeneratedBy)
	require.Equal(t, licenceLabel, c.SourceLicence)
	require.Equal(t, 1, c.SchemaVersion)
	require.Equal(t, 21, c.QuantileSteps())
	require.Equal(t, []string{"kisumu", "nairobi"}, c.CountyIDs())
	require.Equal(t, 31+28+1, c.Counties["kisumu"].Months["1"].Samples+
		c.Counties["kisumu"].Months["2"].Samples+c.Counties["kisumu"].Months["3"].Samples)

	// What the encoder writes must decode back to exactly what was built.
	var round predict.Climatology
	require.NoError(t, json.Unmarshal(encodeClimatology(c), &round))
	require.Equal(t, *c, round)

	_, err = buildClimatology(nil, 14, "", "", "", "")
	require.ErrorContains(t, err, "no county records")

	_, err = buildClimatology([]countyRecord{{ID: "kisumu", Days: synthetic(t, "2015-01-01", "2015-01-10")}},
		14, "", "", "", "")
	require.ErrorContains(t, err, "yielded no windows")
}

// A county-month with no windows must yield a full, flat ladder rather than
// a short one the loader would reject.
func TestLadderOnAnEmptySample(t *testing.T) {
	l := ladder(nil)
	require.Len(t, l, len(quantileSteps()))
	for _, v := range l {
		require.InDelta(t, 0.0, v, 0)
	}
}
