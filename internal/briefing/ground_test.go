// SPDX-License-Identifier: Apache-2.0

package briefing_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/briefing"
	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/briefing/facts/factstest"
	"github.com/jarida-io/climateshield/internal/briefing/mock"
)

// groundedDraft is a briefing that says only what the fact sheet supports. It
// is the positive control: if the checker rejected this, the checker would be
// unusable and every model draft would be thrown away for nothing.
const groundedDraft = "Kisumu is in a wet forecast window from 2026-09-10 to 2026-09-23. " +
	"Cholera is HIGH and malaria is HIGH: peak 14-day rainfall of 74.0 mm is at or above the " +
	"published cutoffs of 60 mm and 40 mm. Pneumonia is LOW and meningitis is LOW, with a mean " +
	"14-day maximum temperature of 28.4 °C. Prepare clinic stock and staffing, and check which " +
	"children in Kisumu are due or overdue for immunization. The mock channel is active: alerts " +
	"are rendered and recorded, and no SMS is sent. These risk levels describe weather measured " +
	"against the published thresholds. They do not forecast an outbreak."

func checker() briefing.Checker {
	return briefing.NewChecker(factstest.Counties)
}

func TestGroundedDraftIsAccepted(t *testing.T) {
	result := checker().Check(factstest.Sample(), facts.LangEN, groundedDraft)
	require.True(t, result.Grounded, "violations: %+v", result.Violations)
	require.Empty(t, result.Violations)
	require.Empty(t, result.Kinds())
}

// TestAdversarialDrafts is the heart of the pillar: each draft below is the
// kind of thing a language model actually produces, and each one must be
// refused for the stated reason.
func TestAdversarialDrafts(t *testing.T) {
	base := groundedDraft

	cases := []struct {
		name string
		lang string
		body string
		kind string
	}{
		{
			name: "a rainfall figure nobody measured",
			body: strings.Replace(base, "74.0 mm", "82.0 mm", 1),
			kind: briefing.KindUnknownNumber,
		},
		{
			name: "a made-up percentage",
			body: base + " Rainfall is 37 percent above normal.",
			kind: briefing.KindUnknownNumber,
		},
		{
			name: "a county it was not asked about",
			body: strings.Replace(base, "children in Kisumu", "children in Nairobi", 1),
			kind: briefing.KindForeignCounty,
		},
		{
			name: "the wrong tier for a scored disease",
			body: strings.Replace(base, "Cholera is HIGH", "Cholera is LOW", 1),
			kind: briefing.KindLevelMismatch,
		},
		{
			name: "a disease this system does not score",
			body: base + " Dengue is also spreading in the county.",
			kind: briefing.KindUnknownDisease,
		},
		{
			name: "an accuracy claim",
			body: base + " The model reaches an accuracy of 91 percent.",
			kind: briefing.KindForbiddenClaim,
		},
		{
			name: "a delivery that never happened",
			body: base + " The SMS sent to every guardian confirms this.",
			kind: briefing.KindForbiddenClaim,
		},
		{
			name: "an outbreak prediction",
			body: base + " An outbreak will occur within the month.",
			kind: briefing.KindForbiddenClaim,
		},
		{
			name: "a fabricated guardian",
			body: base + " Please call guardian Amina Otieno about her child.",
			kind: briefing.KindPossibleName,
		},
		{
			name: "a fabricated phone number",
			body: base + " Contact the county desk on +254712345678 for details.",
			kind: briefing.KindPossiblePhone,
		},
		{
			name: "a draft impersonating the mock label",
			body: base + "\n[mock] no language model ran.",
			kind: briefing.KindMockLabel,
		},
		{
			name: "not a briefing at all",
			body: "OK.",
			kind: briefing.KindTooShort,
		},
		{
			name: "a Kiswahili draft claiming a message was sent",
			lang: facts.LangSW,
			body: swahiliDraft + " Ujumbe umetumwa kwa walezi wote.",
			kind: briefing.KindForbiddenClaim,
		},
		{
			name: "a Kiswahili draft with the wrong tier",
			lang: facts.LangSW,
			body: strings.Replace(swahiliDraft, "Kipindupindu: HIGH", "Kipindupindu: LOW", 1),
			kind: briefing.KindLevelMismatch,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang
			if lang == "" {
				lang = facts.LangEN
			}
			result := checker().Check(factstest.Sample(), lang, tc.body)
			require.False(t, result.Grounded, "this draft must not be served")
			require.Contains(t, result.Kinds(), tc.kind, "violations: %+v", result.Violations)
		})
	}
}

// swahiliDraft is a grounded Kiswahili draft, used as the base the Kiswahili
// adversarial cases corrupt.
const swahiliDraft = "Kisumu, dirisha la utabiri 2026-09-10 hadi 2026-09-23. " +
	"Kipindupindu: HIGH. Malaria: HIGH. Nimonia: LOW. Homa ya uti wa mgongo: LOW. " +
	"Mvua ya juu zaidi ya siku 14 ni 74.0 mm. Njia ya majaribio inatumika na hakuna SMS inayotumwa."

func TestGroundedSwahiliDraftIsAccepted(t *testing.T) {
	result := checker().Check(factstest.Sample(), facts.LangSW, swahiliDraft)
	require.True(t, result.Grounded, "violations: %+v", result.Violations)
}

// TestTemplateIsGrounded is the regression guard on the checker itself: the
// deterministic template is written from the same fact sheet, so if the
// checker ever refuses it, the checker has become wrong — and every model
// draft would be discarded for reasons that are not real.
func TestTemplateIsGrounded(t *testing.T) {
	sheet := factstest.Sample()
	check := checker()
	check.AllowSystemNotice = true

	for _, lang := range facts.Languages {
		for _, notice := range []mock.Notice{
			{Kind: mock.NoticeNoModel},
			{Kind: mock.NoticeUnavailable, Generator: "openai-compatible", Model: "qwen2.5:1.5b"},
			{
				Kind: mock.NoticeRejected, Generator: "anthropic", Model: "claude-opus-5",
				Reasons: []string{"unknown_number"},
			},
		} {
			body, err := mock.Template(sheet, lang, notice)
			require.NoError(t, err)
			result := check.Check(sheet, lang, body)
			require.True(t, result.Grounded,
				"the %s template (%s) must pass its own check: %+v", lang, notice.Kind, result.Violations)
		}
	}
}

// TestTemplateIsGroundedForEveryCountyAndLevel widens that guard: no county
// name, tier combination or missing explanation may make the template fail.
func TestTemplateIsGroundedForEveryCountyAndLevel(t *testing.T) {
	check := checker()
	check.AllowSystemNotice = true

	for _, county := range factstest.Counties {
		for _, level := range []string{"HIGH", "MEDIUM", "LOW"} {
			sheet := factstest.Sample()
			sheet.Area = county
			for i := range sheet.Scores {
				sheet.Scores[i].Level = level
			}
			for _, lang := range facts.Languages {
				body, err := mock.Template(sheet, lang, mock.Notice{Kind: mock.NoticeNoModel})
				require.NoError(t, err)
				result := check.Check(sheet, lang, body)
				require.True(t, result.Grounded,
					"%s/%s/%s: %+v", county, level, lang, result.Violations)
			}
		}
	}
}

func TestMockLabelIsRefusedInAModelDraftEvenOnTheFirstLine(t *testing.T) {
	body := "[mock] no language model ran — deterministic template.\n\n" + groundedDraft
	result := checker().Check(factstest.Sample(), facts.LangEN, body)
	require.False(t, result.Grounded)
	require.Contains(t, result.Kinds(), briefing.KindMockLabel)
}

func TestNumberFormattingTolerance(t *testing.T) {
	sheet := factstest.Sample()
	check := checker()

	// 74 and 74.0 are the same fact written two ways.
	require.True(t, check.Check(sheet, facts.LangEN,
		strings.Replace(groundedDraft, "74.0 mm", "74 mm", 1)).Grounded)
	// 28.4 rounded to 28 is the same fact rounded.
	require.True(t, check.Check(sheet, facts.LangEN,
		strings.Replace(groundedDraft, "28.4 °C", "28 °C", 1)).Grounded)
	// 74.4 is not.
	require.False(t, check.Check(sheet, facts.LangEN,
		strings.Replace(groundedDraft, "74.0 mm", "74.4 mm", 1)).Grounded)
	// A comma decimal separator is the same number.
	require.True(t, check.Check(sheet, facts.LangEN,
		strings.Replace(groundedDraft, "74.0 mm", "74,0 mm", 1)).Grounded)
}

func TestExceedancePercentageIsAllowed(t *testing.T) {
	sheet := factstest.Sample()
	body := groundedDraft + " That rainfall sits in the most extreme 2 percent of the reference record."
	require.True(t, checker().Check(sheet, facts.LangEN, body).Grounded)

	// ... but a different percentage is not.
	body = groundedDraft + " That rainfall sits in the most extreme 7 percent of the reference record."
	require.False(t, checker().Check(sheet, facts.LangEN, body).Grounded)
}

func TestUnknownDiseaseInTheSheetIsReported(t *testing.T) {
	sheet := factstest.Sample()
	sheet.Scores = sheet.Scores[:1] // only cholera is scored
	body := "Kisumu, 2026-09-10 to 2026-09-23. Cholera is HIGH after 74.0 mm of rain. " +
		"Malaria is HIGH as well, and the county should prepare. " +
		"The mock channel is active and no SMS is sent to anyone at all today."
	result := checker().Check(sheet, facts.LangEN, body)
	require.False(t, result.Grounded)
	require.Contains(t, result.Kinds(), briefing.KindUnknownDisease)
}

func TestVocabularyAllowsConfiguredWords(t *testing.T) {
	sheet := factstest.Sample()
	body := groundedDraft + " Ask the Nyalenda ward team to check stock."

	require.False(t, checker().Check(sheet, facts.LangEN, body).Grounded,
		"an unknown capitalised word must be treated as a possible name")

	check := checker()
	check.Vocab = []string{"Nyalenda"}
	require.True(t, check.Check(sheet, facts.LangEN, body).Grounded)
}

func TestKindsAreDeduplicatedAndSorted(t *testing.T) {
	r := briefing.Result{Violations: []briefing.Violation{
		{Kind: briefing.KindUnknownNumber},
		{Kind: briefing.KindForeignCounty},
		{Kind: briefing.KindUnknownNumber},
	}}
	require.Equal(t, []string{briefing.KindForeignCounty, briefing.KindUnknownNumber}, r.Kinds())
}

// TestCheckIsDeterministic: the violations are stored and published, so two
// runs over the same draft must produce the same list in the same order.
func TestCheckIsDeterministic(t *testing.T) {
	body := groundedDraft + " Dengue is HIGH in Nairobi at 82 mm."
	first := checker().Check(factstest.Sample(), facts.LangEN, body)
	for i := 0; i < 20; i++ {
		require.Equal(t, first.Violations, checker().Check(factstest.Sample(), facts.LangEN, body).Violations)
	}
}
