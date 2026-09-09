// SPDX-License-Identifier: Apache-2.0

package facts_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/publicapi"
)

// sample is the fact sheet the whole package's tests are written against: one
// county, four scored diseases, one suppressed alert count.
func sample() facts.FactSheet {
	exceed := 0.02
	count := int64(24)
	return facts.FactSheet{
		Area: "Kisumu",
		Window: facts.Window{
			From: "2026-09-10", To: "2026-09-23", Days: 14, Source: "fixture",
		},
		Scores: []facts.Score{
			{
				Disease: "cholera", Level: "HIGH", Driver: "peak_rainfall_mm_14d",
				DriverValue: 74, Exceedance: &exceed,
				Explanation: "peak 14-day rainfall of 74.0mm is at or above the HIGH threshold of 60mm",
				Predictor:   "rules", Version: "1.0.0",
			},
			{
				Disease: "malaria", Level: "HIGH", Driver: "peak_rainfall_mm_14d",
				DriverValue: 74,
				Explanation: "peak 14-day rainfall of 74.0mm is at or above the HIGH threshold of 40mm",
				Predictor:   "rules", Version: "1.0.0",
			},
			{
				Disease: "pneumonia", Level: "LOW", Driver: "mean_max_temp_c_14d",
				DriverValue: 28.4,
				Explanation: "mean 14-day maximum temperature of 28.4°C is above the MEDIUM threshold of 19°C",
				Predictor:   "rules", Version: "1.0.0",
			},
			{
				Disease: "meningitis", Level: "LOW", Driver: "mean_max_temp_c_14d",
				DriverValue: 28.4,
				Explanation: "mean 14-day maximum temperature of 28.4°C is below the MEDIUM threshold of 36°C",
				Predictor:   "rules", Version: "1.0.0",
			},
		},
		AlertsAllCounties: []facts.AlertCount{
			{Status: "would_send", Count: &count},
			{Status: "skipped_consent", Suppressed: true},
		},
		ChannelSends: false,
		ChannelNote:  "The mock channel is active: alerts are rendered and recorded, and no SMS is sent.",
		GeneratedAt:  time.Date(2026, 9, 10, 6, 0, 0, 0, time.UTC),
	}
}

// Sample is the shared fixture for tests in other packages of this module.
func Sample() facts.FactSheet { return sample() }

func TestCanonicalIsStableAndExcludesTheTimestamp(t *testing.T) {
	a := sample()
	b := sample()
	b.GeneratedAt = a.GeneratedAt.Add(48 * time.Hour)

	canonA, hashA, err := facts.Canonical(a)
	require.NoError(t, err)
	canonB, hashB, err := facts.Canonical(b)
	require.NoError(t, err)

	require.Equal(t, string(canonA), string(canonB),
		"the assembly time is not a fact about the county and must not change the sheet")
	require.Equal(t, hashA, hashB,
		"a changing hash would regenerate a briefing that still describes the world correctly")
	require.NotContains(t, string(canonA), "generated_at")
}

func TestCanonicalChangesWhenAFactChanges(t *testing.T) {
	a := sample()
	_, hashA, err := facts.Canonical(a)
	require.NoError(t, err)

	b := sample()
	b.Scores[0].Level = "MEDIUM"
	_, hashB, err := facts.Canonical(b)
	require.NoError(t, err)
	require.NotEqual(t, hashA, hashB, "a changed risk level must regenerate the briefing")
}

func TestCanonicalNormalisesEmptySlices(t *testing.T) {
	empty := facts.FactSheet{Area: "Nairobi"}
	canon, _, err := facts.Canonical(empty)
	require.NoError(t, err)
	require.Contains(t, string(canon), `"scores":[]`)
	require.Contains(t, string(canon), `"alerts_all_counties":[]`)

	var back map[string]any
	require.NoError(t, json.Unmarshal(canon, &back))
}

// TestFactSheetHasNoPersonFields is a structural check on the promise that a
// generator cannot be told about a person: the JSON a generator receives must
// carry no field that could name, identify or contact one.
func TestFactSheetHasNoPersonFields(t *testing.T) {
	canon, _, err := facts.Canonical(sample())
	require.NoError(t, err)
	lower := strings.ToLower(string(canon))
	for _, forbidden := range []string{
		"child", "guardian", "phone", "national_id", "date_of_birth",
		"first_name", "name_enc", "phone_enc", "leaf", "hmac",
	} {
		require.NotContains(t, lower, forbidden,
			"a fact sheet must not carry %q — it is what a language model is shown", forbidden)
	}
}

// TestSuppressMatchesPublicAPI keeps this package's copy of the k>=10 rule
// honest. It cannot import publicapi in non-test code (publicapi imports this
// package), so the agreement is asserted here instead of assumed.
func TestSuppressMatchesPublicAPI(t *testing.T) {
	require.Equal(t, publicapi.K, facts.K)
	for n := int64(0); n <= 25; n++ {
		gotValue, gotSuppressed := facts.Suppress(n)
		wantValue, wantSuppressed := publicapi.Suppress(n)
		require.Equal(t, wantSuppressed, gotSuppressed, "n=%d", n)
		if wantValue == nil {
			require.Nil(t, gotValue, "n=%d", n)
			continue
		}
		require.NotNil(t, gotValue, "n=%d", n)
		require.Equal(t, *wantValue, *gotValue, "n=%d", n)
	}
}

func TestValidLanguage(t *testing.T) {
	require.True(t, facts.ValidLanguage("en"))
	require.True(t, facts.ValidLanguage("sw"))
	require.False(t, facts.ValidLanguage("fr"))
	require.False(t, facts.ValidLanguage(""))
	require.Equal(t, []string{"en", "sw"}, facts.Languages)
}

func TestPromptIsCommittedAndVersioned(t *testing.T) {
	require.Equal(t, "v1", facts.PromptVersion)
	prompt := facts.Prompt()
	require.Contains(t, prompt, "Use only numbers that appear in the fact sheet")
	require.Contains(t, prompt, "Name no person")
	require.Contains(t, prompt, `{"briefing"`)
}

func TestLexicon(t *testing.T) {
	require.Equal(t, "Cholera", facts.DiseaseName("cholera", facts.LangEN))
	require.Equal(t, "Kipindupindu", facts.DiseaseName("cholera", facts.LangSW))
	require.Equal(t, "dengue", facts.DiseaseName("dengue", facts.LangEN), "unknown diseases are not invented")
	require.Equal(t, "cholera", facts.DiseaseName("cholera", "fr"), "an unknown language falls back to the identifier")

	aliases := facts.DiseaseAliases()
	require.Equal(t, "cholera", aliases["Kipindupindu"])
	require.Equal(t, "cholera", aliases["Cholera"])
	require.Equal(t, "cholera", aliases["cholera"])

	require.Equal(t, "peak 14-day rainfall", facts.DriverPhrase("peak_rainfall_mm_14d", facts.LangEN))
	require.Contains(t, facts.DriverPhrase("peak_rainfall_mm_14d", facts.LangSW), "mvua")
	require.Equal(t, "peak 14-day rainfall", facts.DriverPhrase("peak_rainfall_mm_14d", "fr"))
	require.Equal(t, "unknown_driver", facts.DriverPhrase("unknown_driver", facts.LangEN))

	require.Equal(t, "mm", facts.DriverUnit("peak_rainfall_mm_14d"))
	require.Equal(t, "°C", facts.DriverUnit("mean_max_temp_c_14d"))
	require.Equal(t, "74.0 mm", facts.FormatDriverValue("peak_rainfall_mm_14d", 74))
}
