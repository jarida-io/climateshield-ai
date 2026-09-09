// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/jarida-io/climateshield/internal/climate"
	"github.com/jarida-io/climateshield/internal/predict"
)

// This file is the whole method. It is deliberately small and pure so that
// the numbers in the reference artifact can be checked by reading it:
// slide a fixed-length window along the daily record, reduce each window to
// the same three features the predictor computes at runtime, group the
// windows by the calendar month of their first day, and store an evenly
// spaced ladder of order statistics per group.
//
// Nothing here is fitted to health outcomes. There are none in this project.

// QuantileStepPct is the spacing of the stored percentile ladder, in
// percentage points: 0, 5, 10 … 100, so 21 points per distribution.
const QuantileStepPct = 5

// DecimalPlaces is the precision the stored quantiles are rounded to.
const DecimalPlaces = 3

// quantileSteps is the ladder stored with every distribution.
func quantileSteps() []int {
	out := make([]int, 0, 100/QuantileStepPct+1)
	for p := 0; p <= 100; p += QuantileStepPct {
		out = append(out, p)
	}
	return out
}

// window is one 14-day slice of the record, reduced to the features the
// runtime predictor uses. Month is the calendar month of the window's FIRST
// day, which is how internal/predict/service.go labels a forecast window.
type window struct {
	month    int
	peakRain float64
	meanTmax float64
	meanTmin float64
}

// windowsFrom slides a window of windowDays along days.
//
// The loop stops one window short of the end of the record: a window is taken
// only while at least one further day follows it. That is how the committed
// reference artifact was built — it is why December carries 296 windows per
// decade rather than 297 — and it is preserved here so the artifact stays
// reproducible. It costs the single last window of the record and biases
// nothing. It is recorded in docs/model-card.md under limitations.
func windowsFrom(days []climate.Day, windowDays int) []window {
	if windowDays <= 0 {
		return nil
	}
	out := make([]window, 0, max(len(days)-windowDays, 0))
	for i := 0; i+windowDays < len(days); i++ {
		slice := days[i : i+windowDays]
		peak := slice[0].PrecipitationSumMM
		sumMax, sumMin := 0.0, 0.0
		for _, d := range slice {
			if d.PrecipitationSumMM > peak {
				peak = d.PrecipitationSumMM
			}
			sumMax += d.TempMaxC
			sumMin += d.TempMinC
		}
		n := float64(windowDays)
		out = append(out, window{
			month:    int(slice[0].Date.Month()),
			peakRain: peak,
			meanTmax: sumMax / n,
			meanTmin: sumMin / n,
		})
	}
	return out
}

// quantile returns the pct-th percentile of an ascending sample as an ORDER
// STATISTIC: it is always a value the record actually produced, never an
// interpolation between two of them. The index is the virtual position
// pct/100 * (n-1) rounded half to even — the definition NumPy calls
// method="nearest". p0 is the minimum and p100 the maximum by construction.
func quantile(ascending []float64, pct int) float64 {
	if len(ascending) == 0 {
		return 0
	}
	virtual := float64(pct) / 100 * float64(len(ascending)-1)
	return ascending[roundHalfToEven(virtual)]
}

// roundHalfToEven rounds x to the nearest integer, breaking exact halves
// towards the even neighbour.
func roundHalfToEven(x float64) int {
	return int(math.RoundToEven(x))
}

// round3 rounds to the stored precision.
func round3(v float64) float64 {
	scale := math.Pow(10, DecimalPlaces)
	return math.Round(v*scale) / scale
}

// countyMonths reduces one county's daily record to its per-month
// distributions.
func countyMonths(days []climate.Day, windowDays int) map[string]predict.Month {
	byMonth := map[int][]window{}
	for _, w := range windowsFrom(days, windowDays) {
		byMonth[w.month] = append(byMonth[w.month], w)
	}
	months := make(map[string]predict.Month, len(byMonth))
	for m, ws := range byMonth {
		rain := make([]float64, 0, len(ws))
		tmax := make([]float64, 0, len(ws))
		tmin := make([]float64, 0, len(ws))
		for _, w := range ws {
			rain = append(rain, w.peakRain)
			tmax = append(tmax, w.meanTmax)
			tmin = append(tmin, w.meanTmin)
		}
		months[fmt.Sprintf("%d", m)] = predict.Month{
			Samples: len(ws),
			Quantiles: map[string][]float64{
				"peak_rain_mm": ladder(rain),
				"mean_tmax_c":  ladder(tmax),
				"mean_tmin_c":  ladder(tmin),
			},
		}
	}
	return months
}

// ladder sorts a sample and reads the stored percentile points off it.
func ladder(values []float64) []float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	steps := quantileSteps()
	out := make([]float64, 0, len(steps))
	for _, p := range steps {
		out = append(out, round3(quantile(sorted, p)))
	}
	return out
}

// countyRecord is one county's fetched daily record.
type countyRecord struct {
	ID   string
	Days []climate.Day
}

// buildClimatology assembles the artifact from the fetched records. The
// metadata fields are the artifact's own provenance: they describe where the
// numbers came from and what built them.
func buildClimatology(records []countyRecord, windowDays int, period, source, licence, generator string) (*predict.Climatology, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("buildclimatology: no county records")
	}
	c := &predict.Climatology{
		SchemaVersion:    1,
		ReferencePeriod:  period,
		Source:           source,
		SourceLicence:    licence,
		WindowDays:       windowDays,
		GeneratedBy:      generator,
		QuantileStepsPct: quantileSteps(),
		Counties:         make(map[string]predict.County, len(records)),
	}
	for _, r := range records {
		months := countyMonths(r.Days, windowDays)
		if len(months) == 0 {
			return nil, fmt.Errorf("buildclimatology: %s yielded no windows from %d days", r.ID, len(r.Days))
		}
		c.Counties[r.ID] = predict.County{Months: months}
	}
	return c, nil
}
