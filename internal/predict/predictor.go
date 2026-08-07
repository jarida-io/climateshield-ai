// SPDX-License-Identifier: Apache-2.0

// Package predict scores outbreak risk from climate features. v1 is a
// deterministic rules engine whose thresholds are published in the funding
// proposal; an ONNX model predictor is interface-only until a model exists.
package predict

import (
	"errors"
	"fmt"
	"log/slog"
)

// Disease values match the risk_scores.disease CHECK constraint and the
// protobuf Disease enum (lowercased).
type Disease string

// The four climate-sensitive diseases in scope.
const (
	Cholera    Disease = "cholera"
	Malaria    Disease = "malaria"
	Pneumonia  Disease = "pneumonia"
	Meningitis Disease = "meningitis"
)

// Diseases lists all scored diseases in stable order.
var Diseases = []Disease{Cholera, Malaria, Pneumonia, Meningitis}

// Level is the three-tier risk classification.
type Level string

// Risk tiers.
const (
	Low    Level = "LOW"
	Medium Level = "MEDIUM"
	High   Level = "HIGH"
)

// Driver identifiers persisted with every score.
const (
	DriverPeakRainfall = "peak_rainfall_mm_14d"
	DriverMeanMaxTemp  = "mean_max_temp_c_14d"
)

// Features are the climate inputs for one area over a 14-day forecast window.
type Features struct {
	PeakRainfallMM float64
	MeanMaxTempC   float64
}

// FeaturesFrom computes window features from daily series. Empty input is an
// error by design: the Python prototype defaulted missing temperatures to
// 25°C, which silently turned a data outage into "no risk".
func FeaturesFrom(dailyPrecipMM, dailyTempMaxC []float64) (Features, error) {
	if len(dailyPrecipMM) == 0 || len(dailyTempMaxC) == 0 {
		return Features{}, errors.New("predict: empty climate series")
	}
	peak := dailyPrecipMM[0]
	for _, v := range dailyPrecipMM[1:] {
		if v > peak {
			peak = v
		}
	}
	sum := 0.0
	for _, v := range dailyTempMaxC {
		sum += v
	}
	return Features{
		PeakRainfallMM: peak,
		MeanMaxTempC:   sum / float64(len(dailyTempMaxC)),
	}, nil
}

// Prediction is one disease's scored risk.
type Prediction struct {
	Disease     Disease
	Level       Level
	Driver      string
	DriverValue float64
}

// Predictor scores all diseases for one feature window.
type Predictor interface {
	Name() string
	Version() string
	Predict(f Features) []Prediction
}

// ErrNotImplemented marks predictors that are declared but not yet built.
var ErrNotImplemented = errors.New("predict: not implemented")

// Select returns the active predictor: the ONNX predictor when a model path
// is configured, the rules engine otherwise. It logs which one is active so
// every deployment's scoring provenance is explicit.
func Select(modelPath string, log *slog.Logger) (Predictor, error) {
	if modelPath != "" {
		p, err := NewONNXPredictor(modelPath)
		if err != nil {
			return nil, fmt.Errorf("predict: ONNX model configured at %q but unavailable: %w", modelPath, err)
		}
		log.Info("predictor selected", "predictor", p.Name(), "version", p.Version())
		return p, nil
	}
	p := NewRulesPredictor()
	log.Info("predictor selected", "predictor", p.Name(), "version", p.Version())
	return p, nil
}
