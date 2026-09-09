// SPDX-License-Identifier: Apache-2.0

package facts

import (
	"fmt"

	"github.com/jarida-io/climateshield/internal/predict"
)

// The words a briefing is allowed to use for the things the scoring code
// names in English identifiers. Both the deterministic template and the
// grounding checker read them from here, so a disease can never be written in
// the template under a name the checker does not recognise.
//
// The Kiswahili column has NOT been reviewed by a Kiswahili speaker. Every
// surface that shows Kiswahili text says so, and it stays that way until a
// named reviewer signs it off.

// DiseaseName returns the display name for one disease in one language.
// Unknown diseases fall back to the identifier: inventing a translation would
// be worse than showing the raw word.
func DiseaseName(disease, lang string) string {
	if names, ok := diseaseNames[lang]; ok {
		if n, ok := names[disease]; ok {
			return n
		}
	}
	return disease
}

var diseaseNames = map[string]map[string]string{
	LangEN: {
		string(predict.Cholera):    "Cholera",
		string(predict.Malaria):    "Malaria",
		string(predict.Pneumonia):  "Pneumonia",
		string(predict.Meningitis): "Meningitis",
	},
	LangSW: {
		string(predict.Cholera):    "Kipindupindu",
		string(predict.Malaria):    "Malaria",
		string(predict.Pneumonia):  "Nimonia",
		string(predict.Meningitis): "Homa ya uti wa mgongo",
	},
}

// DiseaseAliases returns every written form of every scored disease, in every
// language, mapped to the disease identifier. The grounding checker uses it to
// notice a disease being written about — in either language — so that a draft
// cannot escape the level check by using the Kiswahili name.
func DiseaseAliases() map[string]string {
	out := make(map[string]string, len(diseaseNames)*len(predict.Diseases))
	for _, d := range predict.Diseases {
		out[string(d)] = string(d)
	}
	for _, names := range diseaseNames {
		for disease, name := range names {
			out[name] = disease
		}
	}
	return out
}

// DriverPhrase names a score's driver in prose. The value's unit comes from
// DriverUnit; together they let the template state a driver value without
// restating a threshold it has not been given.
func DriverPhrase(driver, lang string) string {
	phrases, ok := driverPhrases[lang]
	if !ok {
		phrases = driverPhrases[LangEN]
	}
	if p, ok := phrases[driver]; ok {
		return p
	}
	return driver
}

var driverPhrases = map[string]map[string]string{
	LangEN: {
		predict.DriverPeakRainfall: "peak 14-day rainfall",
		predict.DriverMeanMaxTemp:  "mean 14-day maximum temperature",
		predict.DriverMeanMinTemp:  "mean 14-day minimum temperature",
	},
	LangSW: {
		predict.DriverPeakRainfall: "mvua ya juu zaidi ya siku 14",
		predict.DriverMeanMaxTemp:  "wastani wa joto la juu la siku 14",
		predict.DriverMeanMinTemp:  "wastani wa joto la chini la siku 14",
	},
}

// DriverUnit returns the unit a driver value is measured in.
func DriverUnit(driver string) string {
	if driver == predict.DriverPeakRainfall {
		return "mm"
	}
	return "°C"
}

// FormatDriverValue renders a driver value the way the scoring code's own
// explanations render it, so the two never disagree by a decimal place.
func FormatDriverValue(driver string, value float64) string {
	return fmt.Sprintf("%.1f %s", value, DriverUnit(driver))
}
