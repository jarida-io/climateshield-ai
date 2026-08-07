// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
)

//go:embed climatologydata/*.json
var climatologyFS embed.FS

// DefaultClimatologyFile is the reference climatology shipped with the binary.
const DefaultClimatologyFile = "climatologydata/kenya-5county-2015-2024.json"

// Climatology holds, for each county and calendar month, the empirical
// distribution of each 14-day climate driver over a multi-year reference
// period. These are measured distributions, not assumptions: they are what
// the weather in that county actually did.
type Climatology struct {
	SchemaVersion    int               `json:"schema_version"`
	ReferencePeriod  string            `json:"reference_period"`
	Source           string            `json:"source"`
	SourceLicence    string            `json:"source_licence"`
	WindowDays       int               `json:"window_days"`
	GeneratedBy      string            `json:"generated_by"`
	QuantileStepsPct []int             `json:"quantile_steps_pct"`
	Counties         map[string]County `json:"counties"`
}

// County holds one county's monthly distributions.
type County struct {
	Months map[string]Month `json:"months"`
}

// Month holds the sample count and quantile ladders for one calendar month.
type Month struct {
	Samples   int                  `json:"samples"`
	Quantiles map[string][]float64 `json:"quantiles"`
}

// Driver keys inside the climatology quantile ladders.
const (
	driverPeakRain = "peak_rain_mm"
	driverMeanTmax = "mean_tmax_c"
	driverMeanTmin = "mean_tmin_c"
)

// LoadClimatology reads the embedded reference climatology.
func LoadClimatology() (*Climatology, error) {
	return loadClimatologyFile(DefaultClimatologyFile)
}

func loadClimatologyFile(name string) (*Climatology, error) {
	raw, err := climatologyFS.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("predict: climatology: %w", err)
	}
	var c Climatology
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("predict: climatology: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Climatology) validate() error {
	if len(c.Counties) == 0 {
		return fmt.Errorf("predict: climatology has no counties")
	}
	if len(c.QuantileStepsPct) < 2 {
		return fmt.Errorf("predict: climatology has no quantile ladder")
	}
	for name, county := range c.Counties {
		for m, month := range county.Months {
			for driver, q := range month.Quantiles {
				if len(q) != len(c.QuantileStepsPct) {
					return fmt.Errorf("predict: climatology %s/%s/%s has %d quantiles, want %d",
						name, m, driver, len(q), len(c.QuantileStepsPct))
				}
				if !sort.Float64sAreSorted(q) {
					return fmt.Errorf("predict: climatology %s/%s/%s quantiles are not sorted",
						name, m, driver)
				}
			}
		}
	}
	return nil
}

// ErrNoClimatology means no reference distribution exists for that county and
// month, so no anomaly can be computed. Callers must not substitute a guess.
var ErrNoClimatology = fmt.Errorf("predict: no climatology for this county and month")

// cdf returns the empirical proportion of reference windows at or below x for
// one county, month and driver — i.e. the percentile rank of x, in [0,1].
func (c *Climatology) cdf(areaID string, month int, driver string, x float64) (float64, error) {
	county, ok := c.Counties[areaID]
	if !ok {
		return 0, ErrNoClimatology
	}
	m, ok := county.Months[fmt.Sprintf("%d", month)]
	if !ok {
		return 0, ErrNoClimatology
	}
	q, ok := m.Quantiles[driver]
	if !ok || len(q) != len(c.QuantileStepsPct) {
		return 0, ErrNoClimatology
	}

	// Below the observed minimum or above the observed maximum of ten years
	// of reference data: clamp, rather than extrapolate a distribution we have
	// no evidence for.
	if x <= q[0] {
		return 0, nil
	}
	last := len(q) - 1
	if x >= q[last] {
		return 1, nil
	}
	for i := 0; i < last; i++ {
		lo, hi := q[i], q[i+1]
		if x < lo || x > hi {
			continue
		}
		pLo := float64(c.QuantileStepsPct[i]) / 100
		pHi := float64(c.QuantileStepsPct[i+1]) / 100
		if hi == lo {
			return pHi, nil
		}
		return pLo + (pHi-pLo)*(x-lo)/(hi-lo), nil
	}
	return 1, nil
}

// Exceedance returns how unusual x is in the tail that matters for harm:
// for upperTail drivers (rainfall, heat) the share of reference windows at or
// above x; for lower-tail drivers (cold) the share at or below x. Smaller
// means rarer. 0.02 reads as "a 1-in-50 window for this county and month".
func (c *Climatology) Exceedance(areaID string, month int, driver string, x float64, upperTail bool) (float64, error) {
	rank, err := c.cdf(areaID, month, driver, x)
	if err != nil {
		return 0, err
	}
	if upperTail {
		return 1 - rank, nil
	}
	return rank, nil
}

// Samples reports how many reference windows back a county-month, so the UI
// can show what the anomaly is measured against.
func (c *Climatology) Samples(areaID string, month int) int {
	county, ok := c.Counties[areaID]
	if !ok {
		return 0
	}
	m, ok := county.Months[fmt.Sprintf("%d", month)]
	if !ok {
		return 0
	}
	return m.Samples
}

// QuantileLadder returns the stored quantiles for one county, month and
// driver, or nil when there is no reference distribution for them.
func (c *Climatology) QuantileLadder(areaID string, month int, driver string) []float64 {
	county, ok := c.Counties[areaID]
	if !ok {
		return nil
	}
	m, ok := county.Months[fmt.Sprintf("%d", month)]
	if !ok {
		return nil
	}
	return m.Quantiles[driver]
}
