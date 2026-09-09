// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/predict"
)

const committedArtifact = "../../internal/predict/climatologydata/kenya-5county-2015-2024.json"

// The strongest check available without a network: take the committed
// reference artifact, decode it, re-encode it with this tool's writer, and
// require the bytes back. If it holds, a regenerated artifact differs from
// the committed one only where a measured number differs — never in layout,
// key order or float formatting — so the published SHA-256 stays meaningful.
func TestEncoderReproducesTheCommittedArtifactByteForByte(t *testing.T) {
	raw, err := os.ReadFile(committedArtifact)
	require.NoError(t, err)

	var c predict.Climatology
	require.NoError(t, json.Unmarshal(raw, &c))

	require.Equal(t, string(raw), string(encodeClimatology(&c)))
}

// The layout itself, spelled out on a tiny document so a reader can see what
// "the committed layout" means without diffing 63KB.
func TestEncoderLayout(t *testing.T) {
	c := &predict.Climatology{
		SchemaVersion:    1,
		ReferencePeriod:  "2015-01-01..2015-01-31",
		Source:           "test",
		SourceLicence:    "test",
		WindowDays:       2,
		GeneratedBy:      "cmd/buildclimatology",
		QuantileStepsPct: []int{0, 100},
		Counties: map[string]predict.County{
			"kisumu": {Months: map[string]predict.Month{
				"1": {Samples: 2, Quantiles: map[string][]float64{"peak_rain_mm": {0, 1.5}}},
			}},
		},
	}
	want := `{
 "counties": {
  "kisumu": {
   "months": {
    "1": {
     "quantiles": {
      "peak_rain_mm": [
       0.0,
       1.5
      ]
     },
     "samples": 2
    }
   }
  }
 },
 "generated_by": "cmd/buildclimatology",
 "quantile_steps_pct": [
  0,
  100
 ],
 "reference_period": "2015-01-01..2015-01-31",
 "schema_version": 1,
 "source": "test",
 "source_licence": "test",
 "window_days": 2
}
`
	require.Equal(t, want, string(encodeClimatology(c)))
}

// Whatever the encoder writes must be loadable by the predictor that consumes
// it, including its validation rules.
func TestEncodedArtifactDecodesBackIntoTheSameValues(t *testing.T) {
	raw, err := os.ReadFile(committedArtifact)
	require.NoError(t, err)
	var original predict.Climatology
	require.NoError(t, json.Unmarshal(raw, &original))

	var round predict.Climatology
	require.NoError(t, json.Unmarshal(encodeClimatology(&original), &round))
	require.Equal(t, original, round)
}

// A whole number is a measurement, not an integer: it must keep its decimal
// point so the file reads the way the committed one does.
func TestFloatsAlwaysCarryADecimalPoint(t *testing.T) {
	require.Equal(t, "0.0", formatFloat(0))
	require.Equal(t, "28.9", formatFloat(28.9))
	require.Equal(t, "25.5", formatFloat(25.5))
	require.Equal(t, "22.314", formatFloat(22.314))
	require.Equal(t, "-3.0", formatFloat(-3))
}
