// SPDX-License-Identifier: Apache-2.0

package predict

// SINGLE SOURCE OF TRUTH for the v1 risk thresholds, exactly as published in
// the funding proposal and docs/diagrams/risk-scoring.md. Do not change these
// numbers without a proposal amendment.
const (
	// Cholera: 14-day peak rainfall (mm).
	CholeraHighMM   = 60.0
	CholeraMediumMM = 30.0
	// Malaria: 14-day peak rainfall (mm).
	MalariaHighMM   = 40.0
	MalariaMediumMM = 20.0
	// Pneumonia: 14-day mean max temp (°C), cold stress — tiers are at-or-below.
	PneumoniaHighC   = 16.0
	PneumoniaMediumC = 19.0
	// Meningitis: 14-day mean max temp (°C), heat stress.
	MeningitisHighC   = 39.0
	MeningitisMediumC = 36.0
)

// RulesVersion identifies this ruleset in risk_scores.predictor_version.
const RulesVersion = "1.0.0"

// RulesPredictor is the deterministic v1 predictor.
type RulesPredictor struct{}

// NewRulesPredictor returns the v1 rules engine.
func NewRulesPredictor() RulesPredictor { return RulesPredictor{} }

// Name implements Predictor.
func (RulesPredictor) Name() string { return "rules" }

// Version implements Predictor.
func (RulesPredictor) Version() string { return RulesVersion }

// Predict implements Predictor: one prediction per disease, always.
func (RulesPredictor) Predict(f Features) []Prediction {
	return []Prediction{
		{
			Disease:     Cholera,
			Level:       tierAtOrAbove(f.PeakRainfallMM, CholeraHighMM, CholeraMediumMM),
			Driver:      DriverPeakRainfall,
			DriverValue: f.PeakRainfallMM,
		},
		{
			Disease:     Malaria,
			Level:       tierAtOrAbove(f.PeakRainfallMM, MalariaHighMM, MalariaMediumMM),
			Driver:      DriverPeakRainfall,
			DriverValue: f.PeakRainfallMM,
		},
		{
			Disease:     Pneumonia,
			Level:       tierAtOrBelow(f.MeanMaxTempC, PneumoniaHighC, PneumoniaMediumC),
			Driver:      DriverMeanMaxTemp,
			DriverValue: f.MeanMaxTempC,
		},
		{
			Disease:     Meningitis,
			Level:       tierAtOrAbove(f.MeanMaxTempC, MeningitisHighC, MeningitisMediumC),
			Driver:      DriverMeanMaxTemp,
			DriverValue: f.MeanMaxTempC,
		},
	}
}

func tierAtOrAbove(value, high, medium float64) Level {
	switch {
	case value >= high:
		return High
	case value >= medium:
		return Medium
	default:
		return Low
	}
}

// tierAtOrBelow is the cold-stress variant: lower values are worse, so the
// HIGH cutoff (16) is numerically below the MEDIUM cutoff (19).
func tierAtOrBelow(value, high, medium float64) Level {
	switch {
	case value <= high:
		return High
	case value <= medium:
		return Medium
	default:
		return Low
	}
}
