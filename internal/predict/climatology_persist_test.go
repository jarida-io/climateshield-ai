// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"context"
	"io"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

// Scoring with the climatology predictor must persist the anomaly and the
// explanation alongside the tier, so an alert can always be traced back to a
// number and a sentence.
func TestScoreAreaPersistsExceedanceAndExplanation(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	ingestFixture(t, q, "kisumu")

	p, err := NewClimatologyPredictor()
	require.NoError(t, err)

	noop := func(context.Context, river.JobArgs, string) error { return nil }
	_, err = scoreArea(ctx, q, p, "kisumu", noop, logging.New(io.Discard, "info"))
	require.NoError(t, err)

	rows, err := q.CurrentRisk(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	for _, r := range rows {
		require.Equal(t, "climatology", r.Predictor)
		require.Equal(t, ClimatologyVersion, r.PredictorVersion)
		require.NotNil(t, r.Exceedance, "%s must record how unusual the driver was", r.Disease)
		require.GreaterOrEqual(t, *r.Exceedance, 0.0)
		require.LessOrEqual(t, *r.Exceedance, 1.0)
		require.NotNil(t, r.Explanation)
		require.NotEmpty(t, *r.Explanation)
	}
}

// The rules engine reports no exceedance — it has no reference distribution to
// measure one against — but must still explain itself.
func TestScoreAreaRulesRecordsExplanationWithoutExceedance(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	ingestFixture(t, q, "kisumu")

	noop := func(context.Context, river.JobArgs, string) error { return nil }
	_, err := scoreArea(ctx, q, NewRulesPredictor(), "kisumu", noop, logging.New(io.Discard, "info"))
	require.NoError(t, err)

	rows, err := q.CurrentRisk(ctx)
	require.NoError(t, err)
	for _, r := range rows {
		require.Equal(t, "rules", r.Predictor)
		require.Nil(t, r.Exceedance)
		require.NotNil(t, r.Explanation)
		require.Contains(t, *r.Explanation, "threshold")
	}
}

// The two predictors disagree about the demo window, which is the whole point
// of shipping both: fixed cutoffs calibrated elsewhere do not transfer.
func TestPredictorsDisagreeOnTheSameWindow(t *testing.T) {
	clim, err := NewClimatologyPredictor()
	require.NoError(t, err)

	// Eldoret in the committed demo scenario: a mild, unremarkable window.
	f := Features{AreaID: "eldoret", Month: 8, PeakRainfallMM: 8, MeanMaxTempC: 17.2, MeanMinTempC: 11.5}

	rulesPneumonia, ok := findPrediction(NewRulesPredictor().Predict(f), Pneumonia)
	require.True(t, ok)
	climPneumonia, ok := findPrediction(clim.Predict(f), Pneumonia)
	require.True(t, ok)

	// The rules read 17.2C as cold enough to be MEDIUM; measured against a
	// decade of Eldoret Augusts the same window is unremarkable.
	require.Equal(t, Medium, rulesPneumonia.Level)
	require.Equal(t, DriverMeanMaxTemp, rulesPneumonia.Driver)
	require.Equal(t, DriverMeanMinTemp, climPneumonia.Driver)
	require.NotNil(t, climPneumonia.Exceedance)
}

func TestReferenceExposesTheLoadedClimatology(t *testing.T) {
	p, err := NewClimatologyPredictor()
	require.NoError(t, err)
	ref := p.Reference()
	require.NotNil(t, ref)
	require.Equal(t, "2015-01-01..2024-12-31", ref.ReferencePeriod)
	require.Len(t, ref.Counties, 5)
	require.Positive(t, ref.Samples("kisumu", 8))
	require.Zero(t, ref.Samples("atlantis", 8))
	require.Zero(t, ref.Samples("kisumu", 99))
}

func TestClimatologyValidationRejectsBadArtifacts(t *testing.T) {
	base := func() *Climatology {
		return &Climatology{
			QuantileStepsPct: []int{0, 50, 100},
			Counties: map[string]County{
				"kisumu": {Months: map[string]Month{
					"1": {Samples: 10, Quantiles: map[string][]float64{driverPeakRain: {1, 2, 3}}},
				}},
			},
		}
	}
	require.NoError(t, base().validate())

	noCounties := base()
	noCounties.Counties = nil
	require.Error(t, noCounties.validate())

	noLadder := base()
	noLadder.QuantileStepsPct = []int{0}
	require.Error(t, noLadder.validate())

	wrongLen := base()
	wrongLen.Counties["kisumu"].Months["1"].Quantiles[driverPeakRain] = []float64{1, 2}
	require.Error(t, wrongLen.validate())

	unsorted := base()
	unsorted.Counties["kisumu"].Months["1"].Quantiles[driverPeakRain] = []float64{3, 2, 1}
	require.ErrorContains(t, unsorted.validate(), "not sorted")
}

func TestLoadClimatologyMissingFile(t *testing.T) {
	_, err := loadClimatologyFile("climatologydata/does-not-exist.json")
	require.Error(t, err)
}
