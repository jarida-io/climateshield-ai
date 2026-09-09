// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/jobs"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

// The contractual guarantee of the whole annotation layer: wrapping the rules
// engine must not move a single tier, driver or driver value. If this ever
// fails, an annotation has become a scorer.
func TestAnnotationNeverChangesWhatTheRulesDecided(t *testing.T) {
	clim, err := LoadClimatology()
	require.NoError(t, err)
	annotated := Annotate(NewRulesPredictor(), clim)

	windows := []Features{
		{AreaID: "kisumu", Month: 5, PeakRainfallMM: 74.0, MeanMaxTempC: 29.4, MeanMinTempC: 18.1},
		{AreaID: "eldoret", Month: 8, PeakRainfallMM: 8, MeanMaxTempC: 17.2, MeanMinTempC: 11.5},
		{AreaID: "mombasa", Month: 11, PeakRainfallMM: 0, MeanMaxTempC: 31.0, MeanMinTempC: 23.4},
		// Exactly on every published cutoff.
		{AreaID: "nairobi", Month: 1, PeakRainfallMM: CholeraHighMM, MeanMaxTempC: MeningitisHighC, MeanMinTempC: 14},
		{AreaID: "nakuru", Month: 3, PeakRainfallMM: MalariaMediumMM, MeanMaxTempC: PneumoniaMediumC, MeanMinTempC: 9},
		// A county the reference record does not cover at all.
		{AreaID: "atlantis", Month: 6, PeakRainfallMM: 90, MeanMaxTempC: 40, MeanMinTempC: 2},
	}

	for _, f := range windows {
		base := NewRulesPredictor().Predict(f)
		got := annotated.Predict(f)
		require.Len(t, got, len(base))
		for i := range base {
			require.Equal(t, base[i].Disease, got[i].Disease)
			require.Equal(t, base[i].Level, got[i].Level, "annotation moved a tier for %v", f)
			require.Equal(t, base[i].Driver, got[i].Driver)
			require.Equal(t, base[i].DriverValue, got[i].DriverValue)
			// The original sentence survives verbatim at the front.
			require.True(t, strings.HasPrefix(got[i].Explanation, base[i].Explanation),
				"annotation must append, never rewrite: %q", got[i].Explanation)
		}
	}
}

// Provenance must stay the wrapped predictor's, because it is the one that
// decided the tier that an alert is sent on.
func TestAnnotationKeepsTheWrappedPredictorsProvenance(t *testing.T) {
	clim, err := LoadClimatology()
	require.NoError(t, err)

	annotated := Annotate(NewRulesPredictor(), clim)
	require.Equal(t, "rules", annotated.Name())
	require.Equal(t, RulesVersion, annotated.Version())

	wrapper, ok := annotated.(*AnnotatedPredictor)
	require.True(t, ok)
	require.Equal(t, "rules", wrapper.Inner().Name())
}

// A rules score in a covered county-month gains an exceedance and a sentence
// that says the figure did not set the level.
func TestAnnotationAddsExceedanceAndSaysItDidNotSetTheLevel(t *testing.T) {
	clim, err := LoadClimatology()
	require.NoError(t, err)
	f := Features{AreaID: "kisumu", Month: 5, PeakRainfallMM: 74.0, MeanMaxTempC: 29.4, MeanMinTempC: 18.1}

	for _, p := range Annotate(NewRulesPredictor(), clim).Predict(f) {
		require.NotNil(t, p.Exceedance, "%s should carry an annotation", p.Disease)
		require.GreaterOrEqual(t, *p.Exceedance, 0.0)
		require.LessOrEqual(t, *p.Exceedance, 1.0)
		require.Contains(t, p.Explanation, "reference windows")
		require.Contains(t, p.Explanation, "the published threshold, not this figure, set the level")
	}
}

// The annotation reads the tail the published rule itself reasons about.
// Pneumonia and meningitis share a driver value but not a hazard direction:
// pneumonia is a cold-stress rule, so for a hot window it must report that
// value as ordinary at the cold end, not as rare.
func TestAnnotationUsesTheTailThePublishedRuleTreatsAsHazard(t *testing.T) {
	clim, err := LoadClimatology()
	require.NoError(t, err)
	// A hot Mombasa window, inside the record's range at both ends.
	f := Features{AreaID: "mombasa", Month: 2, PeakRainfallMM: 1, MeanMaxTempC: 32.0, MeanMinTempC: 24}

	preds := Annotate(NewRulesPredictor(), clim).Predict(f)
	pneumonia, ok := findPrediction(preds, Pneumonia)
	require.True(t, ok)
	meningitis, ok := findPrediction(preds, Meningitis)
	require.True(t, ok)

	// Same driver value, opposite tails: the two must not report the same number.
	require.Equal(t, pneumonia.DriverValue, meningitis.DriverValue)
	require.NotNil(t, pneumonia.Exceedance)
	require.NotNil(t, meningitis.Exceedance)
	require.InDelta(t, 1.0, *pneumonia.Exceedance+*meningitis.Exceedance, 1e-9,
		"the two tails of one distribution must sum to 1")
	require.Contains(t, pneumonia.Explanation, "this low")
	require.Contains(t, meningitis.Explanation, "this high")
}

// No reference distribution means no annotation — never a fabricated zero.
func TestAnnotationIsOmittedWhereTheRecordHasNothingToSay(t *testing.T) {
	clim, err := LoadClimatology()
	require.NoError(t, err)
	f := Features{AreaID: "atlantis", Month: 6, PeakRainfallMM: 90, MeanMaxTempC: 40, MeanMinTempC: 2}

	for _, p := range Annotate(NewRulesPredictor(), clim).Predict(f) {
		require.Nil(t, p.Exceedance, "%s must not invent an annotation", p.Disease)
		require.NotContains(t, p.Explanation, "reference windows")
	}
}

// An extreme value clamps to the end of the ladder, and the sentence says so
// in words rather than printing "0.0%".
func TestAnnotationNamesTheRecordExtremeInsteadOfZeroPercent(t *testing.T) {
	clim, err := LoadClimatology()
	require.NoError(t, err)
	f := Features{AreaID: "kisumu", Month: 5, PeakRainfallMM: 10_000, MeanMaxTempC: 25, MeanMinTempC: 18}

	cholera, ok := findPrediction(Annotate(NewRulesPredictor(), clim).Predict(f), Cholera)
	require.True(t, ok)
	require.NotNil(t, cholera.Exceedance)
	require.Zero(t, *cholera.Exceedance)
	require.Contains(t, cholera.Explanation, "the highest value on record")
}

// The climatology predictor already measures an exceedance, so the wrapper
// must leave its output completely alone.
func TestAnnotationLeavesTheClimatologyPredictorUntouched(t *testing.T) {
	clim, err := LoadClimatology()
	require.NoError(t, err)
	inner, err := NewClimatologyPredictor()
	require.NoError(t, err)
	f := Features{AreaID: "kisumu", Month: 5, PeakRainfallMM: 74.0, MeanMaxTempC: 29.4, MeanMinTempC: 18.1}

	base := inner.Predict(f)
	got := Annotate(inner, clim).Predict(f)
	require.Equal(t, base, got)
}

// A missing artifact must degrade to the unwrapped predictor, not to a nil
// one and not to a silent zero.
func TestAnnotateWithoutAClimatologyReturnsThePredictorUnchanged(t *testing.T) {
	p := NewRulesPredictor()
	require.Equal(t, Predictor(p), Annotate(p, nil))
	require.Nil(t, Annotate(nil, &Climatology{}))
}

// A driver the reference record has no ladder for is left unannotated.
func TestAnnotationSkipsUnknownDriversAndDiseases(t *testing.T) {
	clim, err := LoadClimatology()
	require.NoError(t, err)
	a := &AnnotatedPredictor{inner: NewRulesPredictor(), clim: clim}
	f := Features{AreaID: "kisumu", Month: 5}

	_, ok := a.exceedanceFor(Prediction{Disease: Cholera, Driver: "humidity_pct"}, f)
	require.False(t, ok, "unknown driver must not be annotated")
	_, ok = a.exceedanceFor(Prediction{Disease: Disease("dengue"), Driver: DriverPeakRainfall}, f)
	require.False(t, ok, "unknown disease has no declared hazard tail")
}

// Select must hand back an annotating predictor whose provenance is still the
// chosen one, for both predictors and for the default.
func TestSelectAnnotatesEveryPredictor(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New(&buf, "info")

	for _, tc := range []struct{ name, want, version string }{
		{"", "rules", RulesVersion},
		{"rules", "rules", RulesVersion},
		{"climatology", "climatology", ClimatologyVersion},
	} {
		p, err := Select(tc.name, "", log)
		require.NoError(t, err)
		require.Equal(t, tc.want, p.Name())
		require.Equal(t, tc.version, p.Version())
		require.IsType(t, &AnnotatedPredictor{}, p)
	}
	require.Contains(t, buf.String(), "climatology_annotation")

	_, err := Select("astrology", "", logging.New(io.Discard, "info"))
	require.ErrorContains(t, err, "unknown predictor")

	_, err = Select("rules", "/no/such/model.onnx", logging.New(io.Discard, "info"))
	require.ErrorContains(t, err, "ONNX model configured")
}

func TestAppendAnnotationOnAnEmptyExplanation(t *testing.T) {
	require.Equal(t, "annotation", appendAnnotation("", "annotation"))
	require.Equal(t, "base. annotation", appendAnnotation("base", "annotation"))
}

// End to end through the persistence layer: with the annotation in place a
// rules deployment still enqueues exactly the alerts the published thresholds
// call for, still stamps the rows "rules", and now also records how unusual
// each driver value was.
func TestScoreAreaAnnotatesRulesRowsWithoutChangingTheAlerts(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	ingestFixture(t, q, "kisumu")

	clim, err := LoadClimatology()
	require.NoError(t, err)

	var enqueued []jobs.AlertDispatchArgs
	capture := func(_ context.Context, args river.JobArgs, _ string) error {
		enqueued = append(enqueued, args.(jobs.AlertDispatchArgs))
		return nil
	}
	n, err := scoreArea(ctx, q, Annotate(NewRulesPredictor(), clim), "kisumu", capture,
		logging.New(io.Discard, "info"))
	require.NoError(t, err)
	require.Equal(t, 2, n, "Kisumu demo scenario: cholera HIGH + malaria HIGH, exactly as before")
	require.Len(t, enqueued, 2)

	rows, err := q.CurrentRisk(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	for _, r := range rows {
		require.Equal(t, "rules", r.Predictor, "the annotation must not claim to be the scorer")
		require.Equal(t, RulesVersion, r.PredictorVersion)
		require.NotNil(t, r.Exceedance, "%s should record how unusual its driver value was", r.Disease)
		require.NotNil(t, r.Explanation)
		require.Contains(t, *r.Explanation, "threshold")
		require.Contains(t, *r.Explanation, "not this figure, set the level")
	}
}
