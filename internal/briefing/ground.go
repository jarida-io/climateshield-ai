// SPDX-License-Identifier: Apache-2.0

package briefing

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/predict"
)

// The grounding check. This is the part that makes a language model
// admissible here at all: a draft is only served if every number, county,
// disease and risk tier in it is traceable to the fact sheet the model was
// given. A draft that fails is REJECTED and the deterministic template is
// served in its place, with the reasons published on the API — a rejection
// that nobody can see is a rejection that will quietly stop happening.
//
// The check is deliberately biased towards refusal. A false rejection costs a
// paragraph of prose; a false acceptance puts an invented number in front of a
// county health officer.

// Violation kinds. These strings are published on the API and are part of its
// contract; add kinds rather than renaming them.
const (
	KindUnknownNumber  = "unknown_number"
	KindForeignCounty  = "foreign_county"
	KindUnknownDisease = "unknown_disease"
	KindLevelMismatch  = "level_mismatch"
	KindForbiddenClaim = "forbidden_claim"
	KindPossibleName   = "possible_name"
	KindPossiblePhone  = "possible_phone"
	KindTooShort       = "too_short"
	KindMockLabel      = "mock_label"
)

// Violation is one reason a draft was refused.
type Violation struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	// Excerpt is the offending text, and is EMPTY for the kinds whose
	// offending text could itself be a fabricated name or phone number. What
	// is published about those is that they happened, not what they said.
	Excerpt string `json:"excerpt,omitempty"`
}

// Result is the outcome of checking one draft.
type Result struct {
	Grounded   bool
	Violations []Violation
}

// Kinds lists the distinct violation kinds, sorted, for logging and for the
// one-line reason shown on a rejected briefing.
func (r Result) Kinds() []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range r.Violations {
		if !seen[v.Kind] {
			seen[v.Kind] = true
			out = append(out, v.Kind)
		}
	}
	sort.Strings(out)
	return out
}

// Checker validates drafts against a fact sheet.
type Checker struct {
	// Counties is every monitored county name. A draft that writes about a
	// county other than the one it was given is refused even though that name
	// is a real place: this briefing is about one county.
	Counties []string
	// AllowSystemNotice permits a leading "[mock] …" line — the label this
	// system itself writes on template output. It is false for model drafts,
	// where a "[mock]" label would be a model impersonating that label.
	AllowSystemNotice bool
	// Vocab holds extra capitalised words that are not person names
	// (deployment-specific place or programme names, for example).
	Vocab []string
}

// NewChecker builds a checker for model drafts over the given counties.
func NewChecker(counties []string) Checker {
	return Checker{Counties: counties}
}

var numberPattern = regexp.MustCompile(`\d+(?:[.,]\d+)?`)

// capitalisedWord matches a word that starts with a capital and continues in
// lower case — the shape of a person's name in both languages. All-caps
// tokens (HIGH, MEDIUM, LOW, SMS, KEPI) are not names and are not matched.
var capitalisedWord = regexp.MustCompile(`\p{Lu}[\p{Ll}'\x{2019}\-]{2,}`)

// Check validates one draft body against the fact sheet it was generated
// from. It is pure: same inputs, same verdict, on any machine.
func (c Checker) Check(f facts.FactSheet, lang, body string) Result {
	var vs []Violation
	trimmed := strings.TrimSpace(body)

	if len([]rune(trimmed)) < 80 {
		vs = append(vs, Violation{
			Kind:   KindTooShort,
			Detail: fmt.Sprintf("a briefing of %d characters is not a briefing", len([]rune(trimmed))),
		})
		return Result{Grounded: false, Violations: vs}
	}

	scanned, noticeVs := c.stripSystemNotice(trimmed)
	vs = append(vs, noticeVs...)

	vs = append(vs, c.checkNumbers(f, scanned)...)
	vs = append(vs, c.checkCounties(f, scanned)...)
	vs = append(vs, checkDiseasesAndLevels(f, scanned)...)
	vs = append(vs, checkUnscoredDiseases(scanned)...)
	vs = append(vs, checkForbidden(lang, scanned)...)
	vs = append(vs, c.checkPersonLike(f, scanned)...)

	return Result{Grounded: len(vs) == 0, Violations: vs}
}

// stripSystemNotice removes a leading "[mock] …" label from the text that is
// checked. That line is this system's own words about what did not happen, so
// checking it against the fact sheet would be checking the wrong author. When
// the checker is not configured to allow it, its presence is itself a
// violation: only this system may write that label.
func (c Checker) stripSystemNotice(body string) (string, []Violation) {
	lines := strings.Split(body, "\n")
	var vs []Violation
	if c.AllowSystemNotice && strings.HasPrefix(lines[0], "[mock]") {
		lines = lines[1:]
	}
	rest := strings.Join(lines, "\n")
	if strings.Contains(rest, "[mock]") {
		vs = append(vs, Violation{
			Kind:   KindMockLabel,
			Detail: "the draft writes the [mock] label, which only this system may write",
		})
	}
	return rest, vs
}

// checkNumbers requires every number in the draft to be traceable to the fact
// sheet: either it appears in the canonical fact JSON (so the model was told
// it), or it is a formatting of a value in it (74 for 74.0, 2 for an
// exceedance of 0.02), or it is one of the published thresholds the scoring
// explanations already quote.
func (c Checker) checkNumbers(f facts.FactSheet, body string) []Violation {
	allowed := allowedNumbers(f)
	var vs []Violation
	seen := map[string]bool{}
	for _, token := range numberPattern.FindAllString(body, -1) {
		if seen[token] {
			continue
		}
		seen[token] = true
		value, err := strconv.ParseFloat(strings.ReplaceAll(token, ",", "."), 64)
		if err != nil {
			continue
		}
		if matchesAllowed(value, token, allowed) {
			continue
		}
		vs = append(vs, Violation{
			Kind:    KindUnknownNumber,
			Detail:  "this number is not in the fact sheet the briefing was generated from",
			Excerpt: token,
		})
	}
	return vs
}

// allowedNumbers is every number the draft may use: the ones written in the
// fact sheet itself, the derived formattings a writer would reasonably
// produce, and the published thresholds.
func allowedNumbers(f facts.FactSheet) []float64 {
	var out []float64
	add := func(v float64) { out = append(out, v) }

	// Everything the generator was literally given. Scanning the canonical
	// JSON with the same pattern the draft is scanned with means dates,
	// driver identifiers ("peak_rainfall_mm_14d") and version strings are all
	// covered by construction rather than by a list that can go stale.
	if canon, _, err := facts.Canonical(f); err == nil {
		for _, token := range numberPattern.FindAllString(string(canon), -1) {
			if v, err := strconv.ParseFloat(token, 64); err == nil {
				add(v)
			}
		}
	}
	for _, s := range f.Scores {
		add(s.DriverValue)
		add(math.Round(s.DriverValue))
		add(math.Round(s.DriverValue*10) / 10)
		if s.Exceedance != nil {
			pct := *s.Exceedance * 100
			add(pct)
			add(math.Round(pct))
			add(math.Round(pct*10) / 10)
		}
	}
	for _, a := range f.AlertsAllCounties {
		if a.Count != nil {
			add(float64(*a.Count))
		}
	}
	// The published thresholds. They are quoted by the scoring explanations
	// already; listing them here keeps a draft from being refused for
	// restating the rule it was given.
	add(predict.CholeraHighMM)
	add(predict.CholeraMediumMM)
	add(predict.MalariaHighMM)
	add(predict.MalariaMediumMM)
	add(predict.PneumoniaHighC)
	add(predict.PneumoniaMediumC)
	add(predict.MeningitisHighC)
	add(predict.MeningitisMediumC)
	return out
}

// matchesAllowed reports whether a number in the draft is one of the allowed
// values, within the tolerance of how it was written: "74" matches 74.0, and
// "74.4" does not match 74.
func matchesAllowed(value float64, token string, allowed []float64) bool {
	decimals := 0
	if i := strings.IndexAny(token, ".,"); i >= 0 {
		decimals = len(token) - i - 1
	}
	scale := math.Pow(10, float64(decimals))
	for _, a := range allowed {
		if math.Abs(value-a) < 1e-9 {
			return true
		}
		if math.Abs(math.Round(a*scale)/scale-value) < 1e-9 {
			return true
		}
	}
	return false
}

// checkCounties refuses a draft that writes about a county it was not given.
func (c Checker) checkCounties(f facts.FactSheet, body string) []Violation {
	lower := strings.ToLower(body)
	var vs []Violation
	for _, county := range c.Counties {
		if strings.EqualFold(county, f.Area) {
			continue
		}
		if containsWord(lower, strings.ToLower(county)) {
			vs = append(vs, Violation{
				Kind:    KindForeignCounty,
				Detail:  fmt.Sprintf("the briefing is about %s, not %s", f.Area, county),
				Excerpt: county,
			})
		}
	}
	return vs
}

// checkDiseasesAndLevels holds the draft to the levels it was given: a
// disease it was not told about is refused, and a disease written next to the
// wrong tier is refused. Both languages' disease names are recognised, so a
// Kiswahili draft cannot slip past by using the Kiswahili word.
func checkDiseasesAndLevels(f facts.FactSheet, body string) []Violation {
	levels := map[string]string{}
	for _, s := range f.Scores {
		levels[s.Disease] = s.Level
	}
	aliases := facts.DiseaseAliases()
	// Stable order: the violations are stored and published, so two runs over
	// the same draft must produce the same list.
	names := make([]string, 0, len(aliases))
	for alias := range aliases {
		names = append(names, alias)
	}
	sort.Strings(names)

	var vs []Violation
	reported := map[string]bool{}
	for _, sentence := range splitSentences(body) {
		lower := strings.ToLower(sentence)
		tiers := tierPositions(sentence)
		for _, alias := range names {
			disease := aliases[alias]
			positions := wordPositions(lower, strings.ToLower(alias))
			if len(positions) == 0 {
				continue
			}
			level, known := levels[disease]
			if !known {
				if !reported[KindUnknownDisease+disease] {
					reported[KindUnknownDisease+disease] = true
					vs = append(vs, Violation{
						Kind:    KindUnknownDisease,
						Detail:  "this disease is not scored in the fact sheet",
						Excerpt: disease,
					})
				}
				continue
			}
			// Each mention is judged against the tier token NEAREST to it, so
			// "cholera is LOW and malaria is HIGH" is caught even though both
			// tiers appear in the sentence.
			for _, p := range positions {
				nearest, ok := nearestTier(tiers, p)
				if !ok || nearest == level || reported[KindLevelMismatch+disease] {
					continue
				}
				reported[KindLevelMismatch+disease] = true
				vs = append(vs, Violation{
					Kind: KindLevelMismatch,
					Detail: fmt.Sprintf("%s is %s in the fact sheet, and this sentence puts it at %s",
						disease, level, nearest),
					Excerpt: disease,
				})
			}
		}
	}
	return vs
}

// tierPosition is one HIGH/MEDIUM/LOW token and where it sits in a sentence.
type tierPosition struct {
	at   int
	tier string
}

func tierPositions(sentence string) []tierPosition {
	var out []tierPosition
	for _, t := range tierTokens {
		for _, at := range wordPositions(sentence, t) {
			out = append(out, tierPosition{at: at, tier: t})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].at < out[j].at })
	return out
}

func nearestTier(tiers []tierPosition, at int) (string, bool) {
	best, bestDist := "", 0
	for i, t := range tiers {
		d := t.at - at
		if d < 0 {
			d = -d
		}
		if i == 0 || d < bestDist {
			best, bestDist = t.tier, d
		}
	}
	return best, best != ""
}

// unscoredDiseases are diseases this system does NOT score. A briefing that
// writes about one is writing about something the fact sheet cannot support,
// however plausible it sounds for the region. The list is not exhaustive and
// does not need to be: an unrecognised claim about a scored disease is caught
// by the level check, and one about an unlisted disease is usually caught by
// its numbers.
var unscoredDiseases = []string{
	"dengue", "measles", "covid", "covid-19", "ebola", "typhoid", "polio",
	"tuberculosis", "chikungunya", "yellow fever", "hepatitis", "diphtheria",
	"rift valley fever", "surua", "kifua kikuu", "homa ya manjano",
}

func checkUnscoredDiseases(body string) []Violation {
	lower := strings.ToLower(body)
	var vs []Violation
	for _, d := range unscoredDiseases {
		if containsWord(lower, d) {
			vs = append(vs, Violation{
				Kind:    KindUnknownDisease,
				Detail:  "this system does not score this disease, so the fact sheet says nothing about it",
				Excerpt: d,
			})
		}
	}
	return vs
}

var tierTokens = []string{string(predict.High), string(predict.Medium), string(predict.Low)}

// forbiddenClaims are the claims this system may not make in any language:
// performance numbers it has never measured, outbreak predictions it cannot
// make, deliveries the mock channel never performs, and verification the
// ledger does not provide.
var forbiddenClaims = map[string][]string{
	facts.LangEN: {
		"accuracy", "accurate to", "precision of", "sensitivity of", "specificity",
		"% chance", "percent chance", "chance of an outbreak", "probability of an outbreak",
		"will occur", "predicted outbreak", "predicts an outbreak", "outbreak is predicted",
		"outbreak is likely", "guaranteed", "clinically validated", "has been validated",
		"sms sent", "sms was sent", "sms were sent", "messages were sent", "we sent",
		"blockchain-verified", "verified on the blockchain", "proven to reduce",
	},
	facts.LangSW: {
		"usahihi wa", "asilimia ya uwezekano", "uwezekano wa mlipuko",
		"mlipuko utatokea", "utatokea", "imethibitishwa kitabibu", "imethibitishwa kupunguza",
		"ujumbe umetumwa", "sms imetumwa", "tumetuma ujumbe",
	},
}

func checkForbidden(lang string, body string) []Violation {
	lower := strings.ToLower(body)
	var vs []Violation
	claims := append([]string{}, forbiddenClaims[facts.LangEN]...)
	if lang != facts.LangEN {
		claims = append(claims, forbiddenClaims[lang]...)
	}
	for _, claim := range claims {
		if strings.Contains(lower, claim) {
			vs = append(vs, Violation{
				Kind:    KindForbiddenClaim,
				Detail:  "this system cannot support that claim",
				Excerpt: claim,
			})
		}
	}
	return vs
}

// checkPersonLike refuses anything shaped like a person. There is no person in
// a fact sheet, so a name or a phone number in a draft is invented — and an
// invented name beside a health risk is the worst thing this system could
// print. The capitalised-word rule ignores words that open a sentence, since
// those are ordinary prose in both languages.
func (c Checker) checkPersonLike(f facts.FactSheet, body string) []Violation {
	var vs []Violation
	if logging.RedactString(body) != body {
		vs = append(vs, Violation{
			Kind:   KindPossiblePhone,
			Detail: "the draft contains something shaped like a phone number",
		})
	}
	allowed := c.wordVocabulary(f)
	for _, m := range capitalisedWord.FindAllStringIndex(body, -1) {
		word := body[m[0]:m[1]]
		if allowed[strings.ToLower(word)] {
			continue
		}
		if sentenceInitial(body, m[0]) {
			continue
		}
		vs = append(vs, Violation{
			Kind:   KindPossibleName,
			Detail: "a capitalised word that is not a county, disease, month or programme name may be a person's name",
		})
		break // one is enough; the draft is refused either way.
	}
	return vs
}

// wordVocabulary is every capitalised word a briefing may legitimately use.
func (c Checker) wordVocabulary(f facts.FactSheet) map[string]bool {
	out := map[string]bool{}
	addWords := func(s string) {
		for _, w := range strings.FieldsFunc(s, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}) {
			out[strings.ToLower(w)] = true
		}
	}
	addWords(f.Area)
	for _, county := range c.Counties {
		addWords(county)
	}
	for alias := range facts.DiseaseAliases() {
		addWords(alias)
	}
	for _, s := range f.Scores {
		addWords(s.Predictor)
	}
	addWords(f.Window.Source)
	for _, w := range []string{
		"ClimateShield", "KEPI", "Kenya", "Kiswahili", "English", "SMS",
		"January", "February", "March", "April", "May", "June", "July",
		"August", "September", "October", "November", "December",
		"Januari", "Februari", "Machi", "Aprili", "Mei", "Juni", "Julai",
		"Agosti", "Septemba", "Oktoba", "Novemba", "Desemba",
		"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday",
	} {
		addWords(w)
	}
	for _, w := range c.Vocab {
		addWords(w)
	}
	return out
}

// sentenceInitial reports whether the word at index i opens a sentence, a
// line, a list item or a parenthesis — positions where a capital letter is
// grammar rather than a name.
func sentenceInitial(body string, i int) bool {
	for j := i - 1; j >= 0; j-- {
		switch body[j] {
		case ' ', '\t', '"', '\'', '(', '[':
			continue
		case '.', '!', '?', ':', ';', '\n', '\r':
			return true
		default:
			return false
		}
	}
	return true
}

// sentenceBreak splits on sentence punctuation FOLLOWED BY whitespace, so a
// decimal point inside a number ("74.0 mm") does not cut a sentence in half
// and separate a disease from its risk tier.
var sentenceBreak = regexp.MustCompile(`[.!?;]\s|\n`)

func splitSentences(body string) []string {
	return sentenceBreak.Split(body, -1)
}

// containsWord reports whether needle appears in haystack on word boundaries,
// so "malaria" matches "Malaria." but not "malarial".
func containsWord(haystack, needle string) bool {
	return len(wordPositions(haystack, needle)) > 0
}

// wordPositions lists the byte offsets at which needle appears in haystack on
// word boundaries.
func wordPositions(haystack, needle string) []int {
	if needle == "" {
		return nil
	}
	var out []int
	from := 0
	for from < len(haystack) {
		i := strings.Index(haystack[from:], needle)
		if i < 0 {
			return out
		}
		start := from + i
		end := start + len(needle)
		beforeOK := start == 0 || !isWordRune(rune(haystack[start-1]))
		afterOK := end == len(haystack) || !isWordRune(rune(haystack[end]))
		if beforeOK && afterOK {
			out = append(out, start)
		}
		from = start + 1
	}
	return out
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
