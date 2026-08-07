// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"fmt"
	"strings"
)

// Templates deliberately do NOT take a disease name. A named disease next to
// a named child in a plaintext SMS readable by anyone holding the phone is
// diagnosis-adjacent and stigma-prone; the spec's mandatory content list
// (risk level, county, child first name, vaccine due, STOP) omits disease,
// and the forbidden-content test enforces its absence. Disease-level detail
// stays on the public aggregate API where it is population-scoped.
//
// The risk level token stays in uppercase English in both languages (it is a
// tier label, not prose — recorded as a simplification in NOTES.md).
const (
	templateEN = "ClimateShield: Outbreak risk is {RISK} in {COUNTY}. {FIRSTNAME} is due for {VACCINE}. Visit your nearest clinic. Reply STOP to opt out."
	templateSW = "ClimateShield: Hatari ya mlipuko ni {RISK} katika {COUNTY}. {FIRSTNAME} anahitaji chanjo ya {VACCINE}. Tembelea kliniki. Jibu STOP kujiondoa."
)

// MaxSeptets is the single-segment GSM-7 SMS budget.
const MaxSeptets = 160

// TemplateData binds one alert message.
type TemplateData struct {
	Lang        string // "en" or "sw"
	RiskLevel   string // "HIGH" or "MEDIUM"
	County      string
	FirstName   string // child first name ONLY — never the full name
	VaccineName string // human-readable KEPI name, e.g. "Measles-Rubella 1"
}

// Render produces the SMS body, guaranteeing GSM-7 encodability and the
// 160-septet single-segment budget. Refusing at render time means an
// over-length or non-encodable message can never reach a channel.
func Render(d TemplateData) (string, error) {
	var tmpl string
	switch d.Lang {
	case "en":
		tmpl = templateEN
	case "sw":
		tmpl = templateSW
	default:
		return "", fmt.Errorf("notify: unsupported language %q", d.Lang)
	}
	if d.RiskLevel == "" || d.County == "" || d.FirstName == "" || d.VaccineName == "" {
		return "", fmt.Errorf("notify: incomplete template data %+v", redactedData(d))
	}

	body := strings.NewReplacer(
		"{RISK}", d.RiskLevel,
		"{COUNTY}", d.County,
		"{FIRSTNAME}", d.FirstName,
		"{VACCINE}", d.VaccineName,
	).Replace(tmpl)

	n, err := SeptetLength(body)
	if err != nil {
		return "", err
	}
	if n > MaxSeptets {
		return "", fmt.Errorf("notify: rendered message is %d septets (max %d)", n, MaxSeptets)
	}
	return body, nil
}

// redactedData keeps names out of error strings (which end up in logs).
func redactedData(d TemplateData) TemplateData {
	if d.FirstName != "" {
		d.FirstName = "[redacted-name]"
	}
	return d
}
