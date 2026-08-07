// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"fmt"
)

// ClimatologyVersion identifies this predictor in risk_scores.predictor_version.
const ClimatologyVersion = "1.0.0"

// Tier cut-points on the exceedance scale. A HIGH is a roughly 1-in-50
// window for that county and month; a MEDIUM is roughly 1-in-10. These are a
// stated operating point, NOT a fit to health outcomes — there is no outbreak
// surveillance data in this system to fit against. They are declared here so
// they can be argued with, and changed in one place.
const (
	HighExceedance   = 0.02
	MediumExceedance = 0.10
)

// driverSpec says which climate variable stands in for each disease's hazard
// and which tail of the distribution is the dangerous one.
type driverSpec struct {
	key       string // climatology quantile ladder key
	name      string // persisted driver label
	upperTail bool   // true when LARGE values are the hazard
	value     func(Features) float64
}

// Drivers per disease. Cholera and malaria track heavy rainfall; meningitis
// tracks heat. Pneumonia tracks MINIMUM temperature, not maximum: the hazard
// is cold stress, and in the five monitored counties the daily maximum never
// falls low enough to express it (see docs/threshold-validation.md).
var climatologyDrivers = map[Disease]driverSpec{
	Cholera: {
		key: driverPeakRain, name: DriverPeakRainfall, upperTail: true,
		value: func(f Features) float64 { return f.PeakRainfallMM },
	},
	Malaria: {
		key: driverPeakRain, name: DriverPeakRainfall, upperTail: true,
		value: func(f Features) float64 { return f.PeakRainfallMM },
	},
	Pneumonia: {
		key: driverMeanTmin, name: DriverMeanMinTemp, upperTail: false,
		value: func(f Features) float64 { return f.MeanMinTempC },
	},
	Meningitis: {
		key: driverMeanTmax, name: DriverMeanMaxTemp, upperTail: true,
		value: func(f Features) float64 { return f.MeanMaxTempC },
	},
}

// ClimatologyPredictor scores how far a forecast window departs from what that
// county normally experiences in that month, using empirical distributions
// measured from a decade of reanalysis.
//
// Why this exists alongside the published rules: fixed absolute thresholds do
// not transfer between climates. Validated against the reference period, two
// of the four published cutoffs cannot be reached in any monitored county, so
// they never fire. A percentile is defined everywhere, adapts to each county's
// own seasonal baseline, and degrades honestly (ErrNoClimatology) rather than
// silently scoring LOW where it has no evidence.
type ClimatologyPredictor struct {
	clim *Climatology
}

// NewClimatologyPredictor builds the predictor over the embedded reference
// climatology.
func NewClimatologyPredictor() (*ClimatologyPredictor, error) {
	c, err := LoadClimatology()
	if err != nil {
		return nil, err
	}
	return &ClimatologyPredictor{clim: c}, nil
}

// Name implements Predictor.
func (*ClimatologyPredictor) Name() string { return "climatology" }

// Version implements Predictor.
func (*ClimatologyPredictor) Version() string { return ClimatologyVersion }

// Reference exposes the loaded climatology (for the explainability endpoint).
func (p *ClimatologyPredictor) Reference() *Climatology { return p.clim }

// Predict implements Predictor: one prediction per disease, always.
func (p *ClimatologyPredictor) Predict(f Features) []Prediction {
	out := make([]Prediction, 0, len(Diseases))
	for _, d := range Diseases {
		spec := climatologyDrivers[d]
		value := spec.value(f)

		exc, err := p.clim.Exceedance(f.AreaID, f.Month, spec.key, value, spec.upperTail)
		if err != nil {
			// No reference distribution: say so rather than inventing a LOW.
			out = append(out, Prediction{
				Disease: d, Level: Low, Driver: spec.name, DriverValue: value,
				Explanation: fmt.Sprintf(
					"no reference climatology for %s in month %d — not scored", f.AreaID, f.Month),
			})
			continue
		}

		e := exc
		out = append(out, Prediction{
			Disease:     d,
			Level:       tierFromExceedance(exc),
			Driver:      spec.name,
			DriverValue: value,
			Exceedance:  &e,
			Explanation: explain(spec, value, exc, f, p.clim.Samples(f.AreaID, f.Month)),
		})
	}
	return out
}

func tierFromExceedance(exc float64) Level {
	switch {
	case exc <= HighExceedance:
		return High
	case exc <= MediumExceedance:
		return Medium
	default:
		return Low
	}
}

func explain(spec driverSpec, value, exc float64, f Features, samples int) string {
	direction := "above"
	if !spec.upperTail {
		direction = "below"
	}
	pct := exc * 100
	if exc <= 0 {
		return fmt.Sprintf(
			"%s of %.1f is the most extreme %s value on record for %s in month %d (%d reference windows)",
			spec.name, value, direction, f.AreaID, f.Month, samples)
	}
	return fmt.Sprintf(
		"%s of %.1f sits in the most extreme %.1f%% of %s windows for %s in month %d (%d reference windows)",
		spec.name, value, pct, direction, f.AreaID, f.Month, samples)
}
