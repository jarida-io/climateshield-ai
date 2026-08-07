// SPDX-License-Identifier: Apache-2.0

package climate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOpenMeteoRejectsGarbage(t *testing.T) {
	cases := map[string]string{
		"not json":          `{"daily": nope`,
		"no daily data":     `{"daily":{"time":[]}}`,
		"mismatched arrays": `{"daily":{"time":["2026-08-07","2026-08-08"],"precipitation_sum":[1.0],"temperature_2m_max":[20,21],"temperature_2m_min":[10,11]}}`,
		"bad date":          `{"daily":{"time":["yesterday"],"precipitation_sum":[1],"temperature_2m_max":[20],"temperature_2m_min":[10]}}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseOpenMeteo(strings.NewReader(payload))
			require.Error(t, err)
		})
	}
}

func TestParseOpenMeteoOptionalHumidity(t *testing.T) {
	days, err := ParseOpenMeteo(strings.NewReader(
		`{"daily":{"time":["2026-08-07"],"precipitation_sum":[3.5],"temperature_2m_max":[27],"temperature_2m_min":[17]}}`))
	require.NoError(t, err)
	require.Len(t, days, 1)
	require.Nil(t, days[0].HumidityMaxPct)
	require.InDelta(t, 3.5, days[0].PrecipitationSumMM, 1e-9)
}
