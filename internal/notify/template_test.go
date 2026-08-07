// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Worst-case bindings: the longest KEPI vaccine name in the seed schedule, a
// generously long first name, the longest county, MEDIUM (longest tier).
var worstCase = TemplateData{
	RiskLevel:   "MEDIUM",
	County:      "Elgeyo-Marakwet", // longer than any monitored county today
	FirstName:   "Wanjikuwambui",   // 13 chars, longer than any seed name
	VaccineName: "Measles-Rubella 1",
}

// One test per template, per the spec: worst-case render must fit a single
// GSM-7 segment.
func TestTemplateENFits160Septets(t *testing.T) {
	d := worstCase
	d.Lang = "en"
	body, err := Render(d)
	require.NoError(t, err)
	n, err := SeptetLength(body)
	require.NoError(t, err)
	require.LessOrEqual(t, n, MaxSeptets, "EN template overflows: %q", body)
}

func TestTemplateSWFits160Septets(t *testing.T) {
	d := worstCase
	d.Lang = "sw"
	body, err := Render(d)
	require.NoError(t, err)
	n, err := SeptetLength(body)
	require.NoError(t, err)
	require.LessOrEqual(t, n, MaxSeptets, "SW template overflows: %q", body)
}

func TestTemplatesContainMandatoryContent(t *testing.T) {
	for _, lang := range []string{"en", "sw"} {
		d := TemplateData{Lang: lang, RiskLevel: "HIGH", County: "Kisumu", FirstName: "Amina", VaccineName: "OPV 3"}
		body, err := Render(d)
		require.NoError(t, err)
		require.Contains(t, body, "HIGH")
		require.Contains(t, body, "Kisumu")
		require.Contains(t, body, "Amina")
		require.Contains(t, body, "OPV 3")
		require.Contains(t, body, "STOP")
	}
}

// Forbidden content: no disease name (EN or common SW equivalents) and no
// ID-number-shaped digit runs may ever appear in a rendered message. The
// template has no disease input at all — this test keeps it that way.
func TestTemplatesNeverContainForbiddenContent(t *testing.T) {
	forbidden := []string{
		"cholera", "malaria", "pneumonia", "meningitis",
		"kipindupindu", // cholera (sw)
		"homa",         // fever/disease framing (sw)
	}
	for _, lang := range []string{"en", "sw"} {
		d := TemplateData{Lang: lang, RiskLevel: "HIGH", County: "Kisumu", FirstName: "Amina", VaccineName: "BCG"}
		body, err := Render(d)
		require.NoError(t, err)
		lower := strings.ToLower(body)
		for _, f := range forbidden {
			require.NotContains(t, lower, f, "%s template leaked %q", lang, f)
		}
		// No long digit runs (national IDs are 8 digits).
		require.NotRegexp(t, `\d{7,}`, body)
	}
}

func TestRenderValidation(t *testing.T) {
	_, err := Render(TemplateData{Lang: "fr", RiskLevel: "HIGH", County: "x", FirstName: "y", VaccineName: "z"})
	require.Error(t, err)

	_, err = Render(TemplateData{Lang: "en"})
	require.Error(t, err)

	// A name outside GSM-7 must be refused, not mangled or silently sent as
	// a multi-segment UCS-2 message.
	_, err = Render(TemplateData{Lang: "en", RiskLevel: "HIGH", County: "Kisumu", FirstName: "李明", VaccineName: "BCG"})
	require.Error(t, err)
}

func TestRenderErrorsNeverContainTheName(t *testing.T) {
	_, err := Render(TemplateData{Lang: "en", RiskLevel: "", County: "Kisumu", FirstName: "Secretname", VaccineName: "BCG"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "Secretname")
}
