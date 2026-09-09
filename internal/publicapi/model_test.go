// SPDX-License-Identifier: Apache-2.0

package publicapi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/predict"
)

// Every published cutoff is now checked against the record, not just the two
// temperature ones, and each verdict carries the number it was measured
// against so a reviewer can redo the comparison.
func TestThresholdRulesShowTheMeasurementBehindEveryVerdict(t *testing.T) {
	clim, err := predict.LoadClimatology()
	require.NoError(t, err)

	rules := thresholdRules(clim)
	require.Len(t, rules, 4)

	byDisease := map[string]*climateshieldv1.ThresholdRule{}
	for _, r := range rules {
		byDisease[r.GetDisease()] = r
		require.NotEmpty(t, r.GetNote(), "%s must say how it was checked", r.GetDisease())
	}

	require.True(t, byDisease["cholera"].GetReachableInReferencePeriod())
	require.True(t, byDisease["malaria"].GetReachableInReferencePeriod())
	require.False(t, byDisease["pneumonia"].GetReachableInReferencePeriod())
	require.False(t, byDisease["meningitis"].GetReachableInReferencePeriod())

	require.Contains(t, byDisease["cholera"].GetNote(), "Reachable:")
	require.Contains(t, byDisease["cholera"].GetNote(), "mm")
	require.Contains(t, byDisease["cholera"].GetNote(),
		"no threshold here has been checked against disease outcomes")
	require.Contains(t, byDisease["pneumonia"].GetNote(), "never fires")
	require.Contains(t, byDisease["meningitis"].GetNote(), "never fires")

	// The published numbers themselves are untouched.
	require.InDelta(t, predict.CholeraHighMM, byDisease["cholera"].GetHigh(), 0)
	require.InDelta(t, predict.MeningitisHighC, byDisease["meningitis"].GetHigh(), 0)
	require.InDelta(t, predict.PneumoniaMediumC, byDisease["pneumonia"].GetMedium(), 0)
}

// With no reference record loaded, the view must say the check did not run —
// the old code claimed every rule was reachable instead.
func TestThresholdRulesWithoutAReferenceRecordSayTheCheckDidNotRun(t *testing.T) {
	for _, r := range thresholdRules(nil) {
		require.False(t, r.GetReachableInReferencePeriod())
		require.Contains(t, r.GetNote(), "Not checked")
	}
}

// A driver with no distribution in the artifact is reported as unchecked,
// never as reachable.
func TestReachabilityReportsAnUnmeasurableDriverAsUnchecked(t *testing.T) {
	clim := &predict.Climatology{
		QuantileStepsPct: []int{0, 100},
		Counties: map[string]predict.County{
			"kisumu": {Months: map[string]predict.Month{
				"1": {Samples: 1, Quantiles: map[string][]float64{}},
			}},
		},
	}
	for _, r := range thresholdRules(clim) {
		require.False(t, r.GetReachableInReferencePeriod())
		require.Contains(t, r.GetNote(), "Not checked")
	}

	_, _, measured := reachable(clim, &climateshieldv1.ThresholdRule{Driver: "humidity_pct"})
	require.False(t, measured, "an unknown driver cannot be checked")
}

// The wording of what the exceedance figure did must follow the deployment,
// because under the published thresholds it decides nothing.
func TestExceedanceRoleFollowsTheActivePredictor(t *testing.T) {
	require.Equal(t, predict.ExceedanceRoleAnnotation, exceedanceRole("rules", true))
	require.Contains(t, exceedanceRole("rules", true), "did not move any tier")
	require.Equal(t, predict.ExceedanceRoleDeciding, exceedanceRole("climatology", true))
	require.Contains(t, exceedanceRole("climatology", true), "not a probability")
	require.Contains(t, exceedanceRole("rules", false), "No reference climatology is loaded")
}

// No surface anywhere may describe this as trained, AI or accurate.
func TestModelWordingMakesNoLearnedOrAccuracyClaim(t *testing.T) {
	clim, err := predict.LoadClimatology()
	require.NoError(t, err)

	text := exceedanceRole("rules", true) + " " + exceedanceRole("climatology", true)
	for _, r := range thresholdRules(clim) {
		text += " " + r.GetNote()
	}
	for _, banned := range []string{
		"accuracy", "accurate", "sensitivity", "specificity", "trained", "training",
		"machine learning", "artificial intelligence", " ai ", "validated against outbreak",
	} {
		require.NotContains(t, strings.ToLower(text), banned)
	}
}

func TestUnitForEveryPublishedDriver(t *testing.T) {
	require.Equal(t, " mm", unitFor(predict.DriverPeakRainfall))
	require.Equal(t, "°C", unitFor(predict.DriverMeanMaxTemp))
	require.Equal(t, "°C", unitFor(predict.DriverMeanMinTemp))
}
