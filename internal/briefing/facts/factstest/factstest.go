// SPDX-License-Identifier: Apache-2.0

// Package factstest holds the one fact sheet every briefing test reasons
// about. It lives in its own package so the grounding checker, the template,
// both model adapters and the public API all argue about the SAME county,
// window and numbers: a golden model response is only meaningful against the
// facts it was supposedly given.
//
// The county, the window and the values below are fictional demo data, of the
// same shape the fixture climate scenario produces.
package factstest

import (
	"time"

	"github.com/jarida-io/climateshield/internal/briefing/facts"
)

// Counties is the monitored set the checker is given in tests.
var Counties = []string{"Nairobi", "Kisumu", "Mombasa", "Nakuru", "Eldoret"}

// Sample is Kisumu in a wet window: cholera and malaria HIGH on rainfall,
// pneumonia and meningitis LOW on temperature, one alert count above the k
// threshold and one below it.
func Sample() facts.FactSheet {
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
