// SPDX-License-Identifier: Apache-2.0

package mock_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/briefing/facts/factstest"
	"github.com/jarida-io/climateshield/internal/briefing/mock"
)

func TestGeneratorIdentifiesItselfAsNoModel(t *testing.T) {
	g := mock.New()
	require.Equal(t, "mock", g.Name())
	require.Equal(t, "none", g.Model())
	require.Equal(t, "template-v1", g.PromptVersion())
}

// TestGenerateSaysNoModelRan is the rule that the old prototype broke: output
// never implies work that did not happen.
func TestGenerateSaysNoModelRan(t *testing.T) {
	draft, err := mock.New().Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(draft.Body, "[mock] no language model ran"),
		"the first line must say no model ran; got %q", firstLine(draft.Body))
	require.Empty(t, draft.Usage)
}

func TestTemplateEnglishStatesEveryScoreAndTheChannel(t *testing.T) {
	body, err := mock.Template(factstest.Sample(), facts.LangEN, mock.Notice{Kind: mock.NoticeNoModel})
	require.NoError(t, err)

	require.Contains(t, body, "Kisumu, forecast window 2026-09-10 to 2026-09-23 (14 days, source: fixture)")
	require.Contains(t, body, "Cholera: HIGH.")
	require.Contains(t, body, "Malaria: HIGH.")
	require.Contains(t, body, "Pneumonia: LOW.")
	require.Contains(t, body, "Meningitis: LOW.")
	require.Contains(t, body, "peak 14-day rainfall of 74.0mm is at or above the HIGH threshold of 60mm")
	require.Contains(t, body, "Elevated now: Cholera (HIGH), Malaria (HIGH).")
	require.Contains(t, body, "no SMS is sent")
	require.Contains(t, body, "They do not forecast an outbreak")
	require.Contains(t, body, "Scored by rules v1.0.0.")
}

func TestTemplateKiswahiliIsWrittenNotTranslatedFromTheStoredNote(t *testing.T) {
	body, err := mock.Template(factstest.Sample(), facts.LangSW, mock.Notice{Kind: mock.NoticeNoModel})
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(body, "[mock] hakuna modeli ya lugha iliyotumika"))
	require.Contains(t, body, "Kipindupindu: HIGH.")
	require.Contains(t, body, "Homa ya uti wa mgongo: LOW.")
	require.Contains(t, body, "mvua ya juu zaidi ya siku 14 ni 74.0 mm")
	require.Contains(t, body, "hakuna SMS inayotumwa",
		"the Kiswahili text must state the mock channel itself, not carry the English note through")
	require.Contains(t, body, "Havitabiri mlipuko")
	require.NotContains(t, body, "The mock channel is active",
		"the English channel note must not leak into the Kiswahili briefing")
}

// TestNoticeLinesAreDifferentTruths: three different things can put a template
// in front of a reader, and the reader is told which one happened.
func TestNoticeLinesAreDifferentTruths(t *testing.T) {
	sheet := factstest.Sample()

	rejected, err := mock.Template(sheet, facts.LangEN, mock.Notice{
		Kind: mock.NoticeRejected, Generator: "anthropic", Model: "claude-opus-5",
		Reasons: []string{"unknown_number", "possible_name"},
	})
	require.NoError(t, err)
	require.Contains(t, rejected, "[mock] the claude-opus-5 draft (anthropic) was rejected by the grounding check (unknown_number, possible_name)")
	require.Contains(t, rejected, "No model text is shown here")

	unavailable, err := mock.Template(sheet, facts.LangEN, mock.Notice{
		Kind: mock.NoticeUnavailable, Generator: "openai-compatible", Model: "qwen2.5:1.5b",
	})
	require.NoError(t, err)
	require.Contains(t, unavailable, "could not be reached, so no language model ran")

	swRejected, err := mock.Template(sheet, facts.LangSW, mock.Notice{
		Kind: mock.NoticeRejected, Generator: "anthropic", Model: "claude-opus-5",
		Reasons: []string{"forbidden_claim"},
	})
	require.NoError(t, err)
	require.Contains(t, swRejected, "ilikataliwa na ukaguzi wa uthibitisho (forbidden_claim)")

	swUnavailable, err := mock.Template(sheet, facts.LangSW, mock.Notice{Kind: mock.NoticeUnavailable})
	require.NoError(t, err)
	require.Contains(t, swUnavailable, "haikupatikana")
	require.Contains(t, swUnavailable, "the configured model")
}

func TestNoticeFallsBackWhenNothingIsNamed(t *testing.T) {
	line := mock.Notice{Kind: mock.NoticeRejected}.Line(facts.LangEN)
	require.Contains(t, line, "the configured model")
	require.Contains(t, line, "the configured generator")
	require.Contains(t, line, "no reason recorded")
}

func TestTemplateWithNoScoresSaysSo(t *testing.T) {
	sheet := facts.FactSheet{Area: "Nairobi", ChannelNote: "The mock channel is active: alerts are rendered and recorded, and no SMS is sent."}
	for _, lang := range facts.Languages {
		body, err := mock.Template(sheet, lang, mock.Notice{Kind: mock.NoticeNoModel})
		require.NoError(t, err)
		require.Contains(t, body, "Nairobi")
		require.NotContains(t, body, "Elevated now")
		require.NotContains(t, body, "Hatari iliyopanda")
	}
}

func TestTemplateWithNothingElevated(t *testing.T) {
	sheet := factstest.Sample()
	for i := range sheet.Scores {
		sheet.Scores[i].Level = "LOW"
	}
	en, err := mock.Template(sheet, facts.LangEN, mock.Notice{Kind: mock.NoticeNoModel})
	require.NoError(t, err)
	require.Contains(t, en, "No disease is above the MEDIUM cutoff in Kisumu")

	sw, err := mock.Template(sheet, facts.LangSW, mock.Notice{Kind: mock.NoticeNoModel})
	require.NoError(t, err)
	require.Contains(t, sw, "Hakuna ugonjwa ulio juu ya kiwango cha MEDIUM")
}

func TestTemplateUsesTheDriverWhenNoExplanationWasStored(t *testing.T) {
	sheet := factstest.Sample()
	for i := range sheet.Scores {
		sheet.Scores[i].Explanation = ""
	}
	body, err := mock.Template(sheet, facts.LangEN, mock.Notice{Kind: mock.NoticeNoModel})
	require.NoError(t, err)
	require.Contains(t, body, "peak 14-day rainfall is 74.0 mm")
}

func TestTemplateRefusesAnUnsupportedLanguage(t *testing.T) {
	_, err := mock.Template(factstest.Sample(), "fr", mock.Notice{})
	require.Error(t, err)

	_, err = mock.New().Generate(context.Background(), factstest.Sample(), "fr")
	require.Error(t, err)
}

// TestChannelThatSendsIsNotDescribedAsMock guards the reverse mistake: a live
// channel must not be described as sending nothing.
func TestChannelThatSendsIsNotDescribedAsMock(t *testing.T) {
	sheet := factstest.Sample()
	sheet.ChannelSends = true
	sheet.ChannelNote = `Channel "smpp" is active and delivers messages.`

	en, err := mock.Template(sheet, facts.LangEN, mock.Notice{Kind: mock.NoticeNoModel})
	require.NoError(t, err)
	require.Contains(t, en, "delivers messages")

	sw, err := mock.Template(sheet, facts.LangSW, mock.Notice{Kind: mock.NoticeNoModel})
	require.NoError(t, err)
	require.Contains(t, sw, "inatuma ujumbe kwa sasa")
	require.NotContains(t, sw, "hakuna SMS inayotumwa")
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}
