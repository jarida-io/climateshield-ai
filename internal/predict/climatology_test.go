// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func loadRef(t *testing.T) *Climatology {
	t.Helper()
	c, err := LoadClimatology()
	require.NoError(t, err)
	return c
}

func TestClimatologyArtifactIsWellFormed(t *testing.T) {
	c := loadRef(t)

	require.Equal(t, 1, c.SchemaVersion)
	require.Equal(t, 14, c.WindowDays)
	require.Contains(t, c.Source, "Open-Meteo")
	require.NotEmpty(t, c.SourceLicence, "reference data must carry its licence")
	require.Equal(t, "2015-01-01..2024-12-31", c.ReferencePeriod)

	// Every monitored county, every month, every driver.
	for _, county := range []string{"nairobi", "kisumu", "mombasa", "nakuru", "eldoret"} {
		cty, ok := c.Counties[county]
		require.True(t, ok, "missing county %s", county)
		require.Len(t, cty.Months, 12, "%s must cover all 12 months", county)
		for m := 1; m <= 12; m++ {
			month := cty.Months[itoa(m)]
			require.Positive(t, month.Samples, "%s month %d has no samples", county, m)
			for _, d := range []string{driverPeakRain, driverMeanTmax, driverMeanTmin} {
				require.Len(t, month.Quantiles[d], len(c.QuantileStepsPct),
					"%s month %d driver %s", county, m, d)
			}
		}
	}
}

func TestExceedanceIsMonotonicAndBounded(t *testing.T) {
	c := loadRef(t)

	// Upper tail: more rain is rarer, so exceedance must not increase.
	prev := 1.0
	for _, mm := range []float64{0, 5, 10, 20, 40, 80, 200} {
		e, err := c.Exceedance("kisumu", 4, driverPeakRain, mm, true)
		require.NoError(t, err)
		require.GreaterOrEqual(t, e, 0.0)
		require.LessOrEqual(t, e, 1.0)
		require.LessOrEqual(t, e, prev+1e-9, "exceedance rose with rainfall at %.0fmm", mm)
		prev = e
	}

	// Lower tail: colder is rarer, so exceedance must not decrease with temp.
	prev = 0.0
	for _, c1 := range []float64{5, 10, 14, 17, 19, 25} {
		e, err := c.Exceedance("eldoret", 7, driverMeanTmin, c1, false)
		require.NoError(t, err)
		require.GreaterOrEqual(t, e, prev-1e-9, "exceedance fell as it warmed at %.0fC", c1)
		prev = e
	}
}

func TestExceedanceClampsBeyondObservedRange(t *testing.T) {
	c := loadRef(t)

	// Far beyond a decade of evidence: clamp to the extreme, never extrapolate.
	e, err := c.Exceedance("kisumu", 4, driverPeakRain, 10_000, true)
	require.NoError(t, err)
	require.Zero(t, e)

	e, err = c.Exceedance("kisumu", 4, driverPeakRain, -5, true)
	require.NoError(t, err)
	require.Equal(t, 1.0, e)
}

func TestUnknownCountyOrMonthIsAnError(t *testing.T) {
	c := loadRef(t)

	_, err := c.Exceedance("atlantis", 4, driverPeakRain, 10, true)
	require.ErrorIs(t, err, ErrNoClimatology)

	_, err = c.Exceedance("kisumu", 13, driverPeakRain, 10, true)
	require.ErrorIs(t, err, ErrNoClimatology)
}

func TestTierFromExceedance(t *testing.T) {
	require.Equal(t, High, tierFromExceedance(0))
	require.Equal(t, High, tierFromExceedance(HighExceedance))
	require.Equal(t, Medium, tierFromExceedance(HighExceedance+0.001))
	require.Equal(t, Medium, tierFromExceedance(MediumExceedance))
	require.Equal(t, Low, tierFromExceedance(MediumExceedance+0.001))
	require.Equal(t, Low, tierFromExceedance(1))
}

func TestClimatologyPredictorScoresDemoScenario(t *testing.T) {
	p, err := NewClimatologyPredictor()
	require.NoError(t, err)
	require.Equal(t, "climatology", p.Name())
	require.Equal(t, ClimatologyVersion, p.Version())

	// The committed Kisumu demo window: 74mm peak rainfall in August, which
	// the reference decade says is far outside anything normal there.
	preds := p.Predict(Features{
		AreaID: "kisumu", Month: 8,
		PeakRainfallMM: 74, MeanMaxTempC: 28.1, MeanMinTempC: 17.9,
	})
	require.Len(t, preds, 4)

	cholera, ok := findPrediction(preds, Cholera)
	require.True(t, ok)
	require.Equal(t, High, cholera.Level)
	require.NotNil(t, cholera.Exceedance)
	require.LessOrEqual(t, *cholera.Exceedance, HighExceedance)
	require.Contains(t, cholera.Explanation, "most extreme")
	require.Contains(t, cholera.Explanation, "reference windows")

	// Every prediction must carry a driver value and an explanation.
	for _, pr := range preds {
		require.NotEmpty(t, pr.Driver)
		require.NotEmpty(t, pr.Explanation)
	}
}

func TestClimatologyPredictorUsesMinimumTempForColdStress(t *testing.T) {
	p, err := NewClimatologyPredictor()
	require.NoError(t, err)

	preds := p.Predict(Features{
		AreaID: "eldoret", Month: 7,
		PeakRainfallMM: 8, MeanMaxTempC: 23.5, MeanMinTempC: 10.4,
	})
	pneumonia, ok := findPrediction(preds, Pneumonia)
	require.True(t, ok)
	require.Equal(t, DriverMeanMinTemp, pneumonia.Driver,
		"cold stress must be measured on minimum temperature")
	require.InDelta(t, 10.4, pneumonia.DriverValue, 1e-9)
	require.NotNil(t, pneumonia.Exceedance)
}

func TestClimatologyPredictorRefusesToGuess(t *testing.T) {
	// An unmonitored county has no reference distribution. The predictor must
	// say so rather than emit a confident LOW.
	p, err := NewClimatologyPredictor()
	require.NoError(t, err)

	preds := p.Predict(Features{AreaID: "turkana", Month: 3, PeakRainfallMM: 90})
	require.Len(t, preds, 4)
	for _, pr := range preds {
		require.Nil(t, pr.Exceedance, "no exceedance may be reported without a reference")
		require.Contains(t, pr.Explanation, "not scored")
	}
}

// This test encodes the validation finding that motivates this predictor: two
// of the four published thresholds cannot be reached in ANY monitored county,
// so as absolute cutoffs they never fire. If a future reference period makes
// them reachable, this test fails and the finding must be revisited.
func TestPublishedTemperatureThresholdsAreUnreachableInReferenceDecade(t *testing.T) {
	c := loadRef(t)

	for county, cty := range c.Counties {
		for m := 1; m <= 12; m++ {
			month := cty.Months[itoa(m)]
			tmax := month.Quantiles[driverMeanTmax]
			coldest, hottest := tmax[0], tmax[len(tmax)-1]

			require.Greater(t, coldest, PneumoniaHighC,
				"%s month %d: a 14-day mean max of %.1f°C would reach the pneumonia HIGH cutoff of %.0f°C",
				county, m, coldest, PneumoniaHighC)
			require.Less(t, hottest, MeningitisHighC,
				"%s month %d: a 14-day mean max of %.1f°C would reach the meningitis HIGH cutoff of %.0f°C",
				county, m, hottest, MeningitisHighC)
		}
	}
}

func itoa(i int) string { return strconv.Itoa(i) }
