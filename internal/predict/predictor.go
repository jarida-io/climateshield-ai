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
	DriverMeanMinTemp  = "mean_min_temp_c_14d"
)

// Features are the climate inputs for one area over a 14-day forecast window.
// AreaID and Month are carried so a predictor can compare the window against
// that county's own seasonal history rather than a fixed global cutoff.
type Features struct {
	PeakRainfallMM float64
	MeanMaxTempC   float64
	MeanMinTempC   float64
	AreaID         string
	Month          int // 1-12, calendar month of the first forecast day
}

// FeaturesFrom computes window features from daily series. Empty input is an
// error by design: the Python prototype defaulted missing temperatures to
// 25°C, which silently turned a data outage into "no risk".
func FeaturesFrom(areaID string, month int, dailyPrecipMM, dailyTempMaxC, dailyTempMinC []float64) (Features, error) {
	if len(dailyPrecipMM) == 0 || len(dailyTempMaxC) == 0 || len(dailyTempMinC) == 0 {
		return Features{}, errors.New("predict: empty climate series")
	}
	peak := dailyPrecipMM[0]
	for _, v := range dailyPrecipMM[1:] {
		if v > peak {
			peak = v
		}
	}
	return Features{
		PeakRainfallMM: peak,
		MeanMaxTempC:   mean(dailyTempMaxC),
		MeanMinTempC:   mean(dailyTempMinC),
		AreaID:         areaID,
		Month:          month,
	}, nil
}

func mean(v []float64) float64 {
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

// Prediction is one disease's scored risk.
type Prediction struct {
	Disease     Disease
	Level       Level
	Driver      string
	DriverValue float64

	// Exceedance is how rare this driver value is for this county and month,
	// measured against the reference climatology: 0.02 means the window sits
	// in the most extreme 2% of the last decade. It is a property of the
	// WEATHER, not a probability that an outbreak will occur — no outbreak
	// surveillance data exists in this system to estimate that. Nil for
	// predictors that do not compute it.
	Exceedance *float64

	// Explanation is a one-line, human-readable reason a health officer can
	// act on or challenge.
	Explanation string
}

// Predictor scores all diseases for one feature window.
type Predictor interface {
	Name() string
	Version() string
	Predict(f Features) []Prediction
}

// ErrNotImplemented marks predictors that are declared but not yet built.
var ErrNotImplemented = errors.New("predict: not implemented")

// Select returns the active predictor.
//
//	PREDICTOR=rules       the published proposal thresholds (default)
//	PREDICTOR=climatology per-county seasonal percentiles
//
// A configured ONNX model path is a hard startup error while no model exists:
// silently falling back to rules would misreport scoring provenance.
// The choice is logged so every deployment's provenance is explicit.
//
// Whichever predictor is chosen, its output is wrapped by Annotate so that
// every score carries the reference climatology's view of its own driver
// value. The wrapper never changes a level, a driver or a driver value; see
// annotate.go. The wrapped predictor's name and version are what get
// persisted, because it is the one that decided the tier.
func Select(name, modelPath string, log *slog.Logger) (Predictor, error) {
	var p Predictor
	switch {
	case modelPath != "":
		onnx, err := NewONNXPredictor(modelPath)
		if err != nil {
			return nil, fmt.Errorf("predict: ONNX model configured at %q but unavailable: %w", modelPath, err)
		}
		p = onnx
	case name == "" || name == "rules":
		p = NewRulesPredictor()
	case name == "climatology":
		c, err := NewClimatologyPredictor()
		if err != nil {
			return nil, err
		}
		p = c
	default:
		return nil, fmt.Errorf("predict: unknown predictor %q (want rules or climatology)", name)
	}

	annotated, annotating := p, false
	clim, err := LoadClimatology()
	if err != nil {
		// An annotation that cannot be measured is left off, not invented.
		log.Warn("reference climatology unavailable: scores will carry no exceedance annotation",
			"error", err)
	} else {
		annotated, annotating = Annotate(p, clim), true
	}
	log.Info("predictor selected",
		"predictor", p.Name(), "version", p.Version(), "climatology_annotation", annotating)
	return annotated, nil
}
