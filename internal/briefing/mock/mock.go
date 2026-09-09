// SPDX-License-Identifier: Apache-2.0

// Package mock is the DEFAULT briefing generator: a deterministic template
// that says, on its first line, that no language model ran.
//
// It exists for the same reason internal/notify/mock exists. The old
// prototype printed "SMS sent…" while sending nothing; the rule that came out
// of that is that output never implies an action that did not happen. Text
// produced by string concatenation is not text produced by a language model,
// and this package says so every time.
//
// It is also the fallback. When a configured model cannot be reached, or
// produces a draft that fails the grounding check, this template is what gets
// served — with a first line naming which of those happened.
package mock

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jarida-io/climateshield/internal/briefing/facts"
)

// Name identifies this generator in stored briefings and on the API.
const Name = "mock"

// Model is what is reported as the model: nothing ran.
const Model = "none"

// TemplateVersion identifies the template wording. It takes the place of a
// prompt version in the cache key, so editing the template regenerates
// briefings instead of silently serving the old wording.
const TemplateVersion = "template-v1"

// Generator implements facts.Generator with no model, no network and no
// credentials.
type Generator struct{}

// New returns the deterministic template generator.
func New() Generator { return Generator{} }

// Name implements facts.Generator.
func (Generator) Name() string { return Name }

// Model implements facts.Generator.
func (Generator) Model() string { return Model }

// PromptVersion implements facts.Generator.
func (Generator) PromptVersion() string { return TemplateVersion }

// Generate implements facts.Generator: the plain "no language model ran"
// briefing.
func (Generator) Generate(_ context.Context, f facts.FactSheet, lang string) (facts.Draft, error) {
	body, err := Template(f, lang, Notice{Kind: NoticeNoModel})
	if err != nil {
		return facts.Draft{}, err
	}
	return facts.Draft{Body: body}, nil
}

// Notice kinds. Each one is a different true statement about why the reader is
// looking at a template instead of generated prose.
const (
	// NoticeNoModel: no language model is configured, so none ran.
	NoticeNoModel = "no_model"
	// NoticeRejected: a model ran and its draft failed the grounding check.
	// The draft is not shown, stored or logged.
	NoticeRejected = "rejected"
	// NoticeUnavailable: a model is configured but could not be reached.
	NoticeUnavailable = "unavailable"
)

// Notice is the first line of a template briefing: what did or did not
// happen. It never carries model text — only this system's own words plus the
// generator name, the model identifier and the grounding violation kinds.
type Notice struct {
	Kind      string
	Generator string
	Model     string
	// Reasons are grounding violation kinds (see the grounding checker), used
	// with NoticeRejected.
	Reasons []string
}

// Line renders the notice in one language.
func (n Notice) Line(lang string) string {
	model := n.Model
	if model == "" {
		model = "the configured model"
	}
	via := n.Generator
	if via == "" {
		via = "the configured generator"
	}
	reasons := strings.Join(n.Reasons, ", ")
	if reasons == "" {
		reasons = "no reason recorded"
	}
	if lang == facts.LangSW {
		switch n.Kind {
		case NoticeRejected:
			return fmt.Sprintf(
				"[mock] rasimu ya %s (%s) ilikataliwa na ukaguzi wa uthibitisho (%s). "+
					"Hakuna maandishi ya modeli yanayoonyeshwa hapa; haya ni maandishi ya kiolezo.",
				model, via, reasons)
		case NoticeUnavailable:
			return fmt.Sprintf(
				"[mock] %s (%s) haikupatikana, hivyo hakuna modeli ya lugha iliyotumika — maandishi ya kiolezo.",
				model, via)
		default:
			return "[mock] hakuna modeli ya lugha iliyotumika — maandishi ya kiolezo."
		}
	}
	switch n.Kind {
	case NoticeRejected:
		return fmt.Sprintf(
			"[mock] the %s draft (%s) was rejected by the grounding check (%s). "+
				"No model text is shown here; this is the deterministic template.",
			model, via, reasons)
	case NoticeUnavailable:
		return fmt.Sprintf(
			"[mock] %s (%s) could not be reached, so no language model ran — deterministic template.",
			model, via)
	default:
		return "[mock] no language model ran — deterministic template."
	}
}

// Template renders one county's briefing from the fact sheet alone.
//
// Every number it prints comes from the fact sheet, which is what makes it
// safe to serve and what makes it the reference the grounding checker is
// tested against: if the template itself ever failed that check, the check
// would be wrong.
func Template(f facts.FactSheet, lang string, notice Notice) (string, error) {
	if !facts.ValidLanguage(lang) {
		return "", fmt.Errorf("briefing/mock: unsupported language %q", lang)
	}
	var b strings.Builder
	b.WriteString(notice.Line(lang))
	b.WriteString("\n\n")

	if lang == facts.LangSW {
		writeSwahili(&b, f)
	} else {
		writeEnglish(&b, f)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func writeEnglish(b *strings.Builder, f facts.FactSheet) {
	if f.Window.From != "" {
		fmt.Fprintf(b, "%s, forecast window %s to %s (%d days, source: %s).\n\n",
			f.Area, f.Window.From, f.Window.To, f.Window.Days, f.Window.Source)
	} else {
		fmt.Fprintf(b, "%s. No forecast window has been ingested yet, so there is nothing to score.\n\n", f.Area)
	}

	if len(f.Scores) == 0 {
		b.WriteString("No risk scores have been computed for this county yet.\n\n")
	}
	for _, s := range f.Scores {
		reason := s.Explanation
		if reason == "" {
			reason = fmt.Sprintf("%s is %s",
				facts.DriverPhrase(s.Driver, facts.LangEN),
				facts.FormatDriverValue(s.Driver, s.DriverValue))
		}
		fmt.Fprintf(b, "%s: %s. %s.\n",
			facts.DiseaseName(s.Disease, facts.LangEN), s.Level, strings.TrimRight(reason, "."))
	}
	if len(f.Scores) > 0 {
		b.WriteString("\n")
	}

	elevated := elevatedNames(f, facts.LangEN)
	if len(elevated) > 0 {
		fmt.Fprintf(b, "Elevated now: %s. For those, prepare clinic stock and staffing, "+
			"and check which children in %s are due or overdue for immunization.\n",
			strings.Join(elevated, ", "), f.Area)
	} else if len(f.Scores) > 0 {
		fmt.Fprintf(b, "No disease is above the MEDIUM cutoff in %s in this window. "+
			"Routine immunization catch-up is the useful action.\n", f.Area)
	}

	fmt.Fprintf(b, "%s\n", f.ChannelNote)
	b.WriteString(closingEN)
	if len(f.Scores) > 0 {
		fmt.Fprintf(b, "\nScored by %s v%s.\n", f.Scores[0].Predictor, f.Scores[0].Version)
	}
}

func writeSwahili(b *strings.Builder, f facts.FactSheet) {
	if f.Window.From != "" {
		fmt.Fprintf(b, "%s, dirisha la utabiri %s hadi %s (siku %d, chanzo: %s).\n\n",
			f.Area, f.Window.From, f.Window.To, f.Window.Days, f.Window.Source)
	} else {
		fmt.Fprintf(b, "%s. Bado hakuna dirisha la utabiri lililoingizwa, hivyo hakuna alama za hatari.\n\n", f.Area)
	}

	if len(f.Scores) == 0 {
		b.WriteString("Bado hakuna alama za hatari kwa kaunti hii.\n\n")
	}
	for _, s := range f.Scores {
		fmt.Fprintf(b, "%s: %s. %s ni %s.\n",
			facts.DiseaseName(s.Disease, facts.LangSW), s.Level,
			facts.DriverPhrase(s.Driver, facts.LangSW),
			facts.FormatDriverValue(s.Driver, s.DriverValue))
	}
	if len(f.Scores) > 0 {
		b.WriteString("\n")
	}

	elevated := elevatedNames(f, facts.LangSW)
	if len(elevated) > 0 {
		fmt.Fprintf(b, "Hatari iliyopanda sasa: %s. Kwa hizo, andaa dawa na wafanyakazi wa kliniki, "+
			"na angalia ni watoto gani katika %s wanaohitaji chanjo.\n",
			strings.Join(elevated, ", "), f.Area)
	} else if len(f.Scores) > 0 {
		fmt.Fprintf(b, "Hakuna ugonjwa ulio juu ya kiwango cha MEDIUM katika %s kwa dirisha hili. "+
			"Kazi muhimu ni kuwafikia watoto waliokosa chanjo.\n", f.Area)
	}

	fmt.Fprintf(b, "%s\n", channelNoteSW(f.ChannelSends))
	b.WriteString(closingSW)
	if len(f.Scores) > 0 {
		fmt.Fprintf(b, "\nAlama zimetolewa na %s v%s.\n", f.Scores[0].Predictor, f.Scores[0].Version)
	}
}

// The closing sentence is fixed in both languages: risk levels describe
// weather measured against published thresholds, and nothing more. The prompt
// asks language models to end the same way, so the claim is identical whoever
// wrote the paragraph above it.
const (
	closingEN = "These risk levels describe weather measured against the published thresholds. " +
		"They do not forecast an outbreak, and this system holds no outbreak surveillance data.\n"
	closingSW = "Viwango hivi vya hatari vinaeleza hali ya hewa ikilinganishwa na vigezo vilivyochapishwa. " +
		"Havitabiri mlipuko, na mfumo huu hauna data ya ufuatiliaji wa milipuko.\n"
)

// channelNoteSW states the messaging position in Kiswahili. It is written
// here rather than translated from the stored English note so that the mock
// channel's "nothing was sent" is stated, not implied.
func channelNoteSW(sends bool) string {
	if sends {
		return "Njia ya ujumbe inatuma ujumbe kwa sasa."
	}
	return "Njia ya majaribio (mock) inatumika: arifa zinaandikwa na kuhifadhiwa, na hakuna SMS inayotumwa."
}

// elevatedNames lists the diseases at HIGH or MEDIUM, worst first, in the
// requested language.
func elevatedNames(f facts.FactSheet, lang string) []string {
	type row struct {
		name  string
		level string
	}
	var rows []row
	for _, s := range f.Scores {
		if s.Level == "HIGH" || s.Level == "MEDIUM" {
			rows = append(rows, row{name: facts.DiseaseName(s.Disease, lang), level: s.Level})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].level == "HIGH" && rows[j].level != "HIGH"
	})
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s (%s)", r.name, r.level))
	}
	return out
}
