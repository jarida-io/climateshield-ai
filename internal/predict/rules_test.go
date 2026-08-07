// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Boundary coverage for every published threshold: at the boundary, just
// below, just above. The thresholds are contractual (published in the funding
// proposal) — if one of these cases fails, the code has drifted from the
// proposal, not the other way around.
func TestRulesThresholdBoundaries(t *testing.T) {
	cases := []struct {
		name     string
		disease  Disease
		features Features
		want     Level
	}{
		// Cholera — 14-day peak rainfall: HIGH >= 60, MEDIUM >= 30.
		{"cholera at high", Cholera, rain(60), High},
		{"cholera just below high", Cholera, rain(59.9), Medium},
		{"cholera just above high", Cholera, rain(60.1), High},
		{"cholera at medium", Cholera, rain(30), Medium},
		{"cholera just below medium", Cholera, rain(29.9), Low},
		{"cholera just above medium", Cholera, rain(30.1), Medium},
		{"cholera zero", Cholera, rain(0), Low},

		// Malaria — 14-day peak rainfall: HIGH >= 40, MEDIUM >= 20.
		{"malaria at high", Malaria, rain(40), High},
		{"malaria just below high", Malaria, rain(39.9), Medium},
		{"malaria just above high", Malaria, rain(40.1), High},
		{"malaria at medium", Malaria, rain(20), Medium},
		{"malaria just below medium", Malaria, rain(19.9), Low},
		{"malaria just above medium", Malaria, rain(20.1), Medium},

		// Pneumonia — 14-day mean max temp, inverted: HIGH <= 16, MEDIUM <= 19.
		{"pneumonia at high", Pneumonia, temp(16), High},
		{"pneumonia just below high (colder)", Pneumonia, temp(15.9), High},
		{"pneumonia just above high", Pneumonia, temp(16.1), Medium},
		{"pneumonia at medium", Pneumonia, temp(19), Medium},
		{"pneumonia just above medium (warmer)", Pneumonia, temp(19.1), Low},
		{"pneumonia just below medium", Pneumonia, temp(18.9), Medium},

		// Meningitis — 14-day mean max temp: HIGH >= 39, MEDIUM >= 36.
		{"meningitis at high", Meningitis, temp(39), High},
		{"meningitis just below high", Meningitis, temp(38.9), Medium},
		{"meningitis just above high", Meningitis, temp(39.1), High},
		{"meningitis at medium", Meningitis, temp(36), Medium},
		{"meningitis just below medium", Meningitis, temp(35.9), Low},
		{"meningitis just above medium", Meningitis, temp(36.1), Medium},
	}

	p := NewRulesPredictor()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			preds := p.Predict(tc.features)
			require.Len(t, preds, 4, "one prediction per disease")
			got, ok := findPrediction(preds, tc.disease)
			require.True(t, ok, "no prediction for %s", tc.disease)
			require.Equal(t, tc.want, got.Level)
		})
	}
}

// rain builds features that keep temperature neutral (no temp tier fires
// either way matters — only the disease under test is asserted).
func rain(mm float64) Features { return Features{PeakRainfallMM: mm, MeanMaxTempC: 25} }

func temp(c float64) Features { return Features{PeakRainfallMM: 0, MeanMaxTempC: c} }

func findPrediction(preds []Prediction, d Disease) (Prediction, bool) {
	for _, p := range preds {
		if p.Disease == d {
			return p, true
		}
	}
	return Prediction{}, false
}

func TestRulesPredictorMetadata(t *testing.T) {
	p := NewRulesPredictor()
	require.Equal(t, "rules", p.Name())
	require.Equal(t, RulesVersion, p.Version())

	preds := p.Predict(rain(74))
	for _, pr := range preds {
		switch pr.Disease {
		case Cholera, Malaria:
			require.Equal(t, DriverPeakRainfall, pr.Driver)
			require.InDelta(t, 74.0, pr.DriverValue, 1e-9)
		case Pneumonia, Meningitis:
			require.Equal(t, DriverMeanMaxTemp, pr.Driver)
			require.InDelta(t, 25.0, pr.DriverValue, 1e-9)
		}
	}
}

// Demo-scenario regression: the committed fixture values must reproduce the
// documented outcome (Kisumu cholera+malaria HIGH, Mombasa cholera MEDIUM +
// malaria HIGH, Eldoret pneumonia MEDIUM, Nairobi/Nakuru all LOW-ish).
func TestRulesDemoScenario(t *testing.T) {
	p := NewRulesPredictor()

	expect := func(f Features, want map[Disease]Level) {
		t.Helper()
		preds := p.Predict(f)
		for d, lvl := range want {
			got, ok := findPrediction(preds, d)
			require.True(t, ok)
			require.Equal(t, lvl, got.Level, "disease %s", d)
		}
	}

	// Kisumu: rain 74, temp 28.1
	expect(Features{PeakRainfallMM: 74, MeanMaxTempC: 28.1}, map[Disease]Level{
		Cholera: High, Malaria: High, Pneumonia: Low, Meningitis: Low,
	})
	// Mombasa: rain 41, temp 31.6
	expect(Features{PeakRainfallMM: 41, MeanMaxTempC: 31.6}, map[Disease]Level{
		Cholera: Medium, Malaria: High, Pneumonia: Low, Meningitis: Low,
	})
	// Eldoret: rain 8, temp 17.2
	expect(Features{PeakRainfallMM: 8, MeanMaxTempC: 17.2}, map[Disease]Level{
		Cholera: Low, Malaria: Low, Pneumonia: Medium, Meningitis: Low,
	})
	// Nairobi: rain 18, temp 23.4 — nothing elevated.
	expect(Features{PeakRainfallMM: 18, MeanMaxTempC: 23.4}, map[Disease]Level{
		Cholera: Low, Malaria: Low, Pneumonia: Low, Meningitis: Low,
	})
}

func TestFeaturesFrom(t *testing.T) {
	f, err := FeaturesFrom(
		[]float64{0, 12, 74, 3},
		[]float64{26, 28, 30.3},
	)
	require.NoError(t, err)
	require.InDelta(t, 74.0, f.PeakRainfallMM, 1e-9)
	require.InDelta(t, (26+28+30.3)/3, f.MeanMaxTempC, 1e-9)

	// Empty inputs are an error, not a silent default: the Python prototype's
	// fallback (assume 25°C) could mask a data outage as "no risk".
	_, err = FeaturesFrom(nil, []float64{20})
	require.Error(t, err)
	_, err = FeaturesFrom([]float64{1}, nil)
	require.Error(t, err)
}
