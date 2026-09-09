// SPDX-License-Identifier: Apache-2.0

// Package briefing generates the county risk briefings served by the public
// API: the aggregate facts this system already publishes, restated in plain
// language for a county health officer, in English and Kiswahili.
//
// Four rules govern everything here, and they are the reason a language model
// is admissible in a child health system at all:
//
//  1. The DEFAULT generator is a deterministic template that says on its
//     first line that no language model ran. A model is opt-in, and the
//     default stack is offline and credential-free.
//  2. A generator only ever sees a facts.FactSheet, which has no field for a
//     child, a guardian, a phone number or a people-derived count below the
//     k>=10 threshold. The guarantee is structural, not procedural.
//  3. Every model draft is checked against its own fact sheet (ground.go).
//     A draft with a number, county, disease or tier the facts do not support
//     is REJECTED, the template is served instead, and the reasons are
//     published. Serving model-labelled text a model did not produce would be
//     the old prototype's "SMS sent" lie in new clothes.
//  4. No generated text ever reaches a guardian. Guardian SMS comes from the
//     fixed, length-checked templates in internal/notify and nowhere else.
package briefing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/briefing/mock"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// Statuses a stored briefing can carry. They mirror the CHECK constraint in
// migration 0011 and are published on the API.
const (
	// StatusServed: this generator produced the body and it passed grounding.
	StatusServed = "served"
	// StatusRejected: a model draft failed grounding; the body is the
	// labelled template and the reasons are recorded.
	StatusRejected = "rejected"
	// StatusUnavailable: the generator could not be reached; the body is the
	// labelled template.
	StatusUnavailable = "unavailable"
	// StatusNone is not stored: it is what the API reports when no briefing
	// exists yet for a county and language.
	StatusNone = "none"
)

// KindUnavailable records that the configured generator could not be reached.
// It is a grounding-note kind so that the API has one place to look for "why
// is this a template?".
const KindUnavailable = "generator_unavailable"

// Store is the database surface the generator needs: aggregate reads for the
// fact sheet, plus the briefing cache. Nothing here can read a person.
type Store interface {
	facts.FactQuerier
	GetBriefingForKey(ctx context.Context, arg db.GetBriefingForKeyParams) (db.Briefing, error)
	UpsertBriefing(ctx context.Context, arg db.UpsertBriefingParams) (int64, error)
}

// Generator is re-exported so callers need one import, not two.
type Generator = facts.Generator

// Sweeper regenerates briefings whose facts have changed.
type Sweeper struct {
	Store     Store
	Gen       Generator
	Checker   Checker
	Channel   string
	Timeout   time.Duration
	Now       func() time.Time
	Log       *slog.Logger
	Languages []string
}

// Summary counts what one sweep did. It is returned rather than logged so a
// test can assert that an unchanged fact sheet regenerates nothing.
type Summary struct {
	Areas       int
	Cached      int
	Served      int
	Rejected    int
	Unavailable int
}

// Sweep regenerates every county and language whose facts have changed.
// areaID limits the sweep to one county; empty sweeps all of them.
//
// It is idempotent by construction: the fact sheet's hash is the cache key, so
// a county whose facts have not changed costs one hash comparison and no
// generation at all. That is what makes it safe to run on a short interval
// even with a language model configured.
func (s *Sweeper) Sweep(ctx context.Context, areaID string) (Summary, error) {
	areas, err := facts.Areas(ctx, s.Store)
	if err != nil {
		return Summary{}, err
	}
	langs := s.Languages
	if len(langs) == 0 {
		langs = facts.Languages
	}

	var sum Summary
	for _, area := range areas {
		if areaID != "" && area.ID != areaID {
			continue
		}
		sum.Areas++
		sheet, err := facts.BuildFactSheet(ctx, s.Store, area, s.Channel, s.now())
		if err != nil {
			return sum, err
		}
		for _, lang := range langs {
			outcome, err := s.generateOne(ctx, area, sheet, lang)
			if err != nil {
				return sum, err
			}
			switch outcome {
			case outcomeCached:
				sum.Cached++
			case outcomeServed:
				sum.Served++
			case outcomeRejected:
				sum.Rejected++
			case outcomeUnavailable:
				sum.Unavailable++
			}
		}
	}
	return sum, nil
}

type outcome int

const (
	outcomeCached outcome = iota
	outcomeServed
	outcomeRejected
	outcomeUnavailable
)

func (s *Sweeper) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// generateOne produces (or reuses) one county's briefing in one language.
func (s *Sweeper) generateOne(
	ctx context.Context, area facts.Area, sheet facts.FactSheet, lang string,
) (outcome, error) {
	canon, hash, err := facts.Canonical(sheet)
	if err != nil {
		return outcomeCached, err
	}
	key := db.GetBriefingForKeyParams{
		AreaID: area.ID, Lang: lang, FactsHash: hash[:],
		Generator: s.Gen.Name(), Model: s.Gen.Model(), PromptVersion: s.Gen.PromptVersion(),
	}
	existing, err := s.Store.GetBriefingForKey(ctx, key)
	switch {
	case err == nil:
		// A previous attempt that could not reach the generator is retried;
		// a served or rejected verdict on identical facts is final, because
		// the same facts and the same prompt would ask the same question.
		if existing.Status != StatusUnavailable {
			return outcomeCached, nil
		}
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return outcomeCached, fmt.Errorf("briefing: cache lookup: %w", err)
	}

	body, status, grounded, notes, err := s.write(ctx, sheet, lang)
	if err != nil {
		return outcomeCached, err
	}
	notesJSON, err := json.Marshal(notes)
	if err != nil {
		return outcomeCached, fmt.Errorf("briefing: encode grounding notes: %w", err)
	}
	if _, err := s.Store.UpsertBriefing(ctx, db.UpsertBriefingParams{
		AreaID: area.ID, Lang: lang, FactsHash: hash[:], FactsJson: canon,
		Generator: s.Gen.Name(), Model: s.Gen.Model(), PromptVersion: s.Gen.PromptVersion(),
		Body: body, Grounded: grounded, GroundingNotes: notesJSON, Status: status,
	}); err != nil {
		return outcomeCached, fmt.Errorf("briefing: store: %w", err)
	}

	switch status {
	case StatusRejected:
		return outcomeRejected, nil
	case StatusUnavailable:
		return outcomeUnavailable, nil
	default:
		return outcomeServed, nil
	}
}

// write produces the text to store: the generator's draft when it passes the
// grounding check, and the labelled template when it does not or when the
// generator cannot be reached.
func (s *Sweeper) write(
	ctx context.Context, sheet facts.FactSheet, lang string,
) (body, status string, grounded bool, notes []Violation, err error) {
	genCtx := ctx
	if s.Timeout > 0 {
		var cancel context.CancelFunc
		genCtx, cancel = context.WithTimeout(ctx, s.Timeout)
		defer cancel()
	}

	draft, genErr := s.Gen.Generate(genCtx, sheet, lang)
	if genErr != nil {
		// The generator's error text is this system's own wording plus a URL
		// and a status code; it is still passed through the PII redactor
		// before being stored, because it is the one place remote input could
		// reach a stored string.
		notes = []Violation{{
			Kind:   KindUnavailable,
			Detail: logging.RedactString(genErr.Error()),
		}}
		s.logf().Warn("briefing generator unavailable, serving the template",
			"area", sheet.Area, "lang", lang,
			"generator", s.Gen.Name(), "model", s.Gen.Model(), "error", genErr.Error())
		body, err = mock.Template(sheet, lang, mock.Notice{
			Kind: mock.NoticeUnavailable, Generator: s.Gen.Name(), Model: s.Gen.Model(),
		})
		return body, StatusUnavailable, false, notes, err
	}

	// The deterministic template is not a draft to be checked — it is what a
	// failed check falls back to. Checking it here would be checking this
	// system's own words against themselves; TestTemplateIsGrounded does that
	// deliberately, as a regression guard on the checker.
	if s.Gen.Name() == mock.Name {
		return draft.Body, StatusServed, true, nil, nil
	}

	result := s.Checker.Check(sheet, lang, draft.Body)
	if result.Grounded {
		return draft.Body, StatusServed, true, nil, nil
	}
	notes = PublicNotes(result.Violations)
	// The draft itself is never logged or stored: it may contain a fabricated
	// name, and that is exactly the thing this system must not repeat.
	s.logf().Warn("briefing draft rejected by the grounding check, serving the template",
		"area", sheet.Area, "lang", lang,
		"generator", s.Gen.Name(), "model", s.Gen.Model(),
		"kinds", strings.Join(result.Kinds(), ","))
	body, err = mock.Template(sheet, lang, mock.Notice{
		Kind: mock.NoticeRejected, Generator: s.Gen.Name(), Model: s.Gen.Model(),
		Reasons: result.Kinds(),
	})
	return body, StatusRejected, false, notes, err
}

func (s *Sweeper) logf() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.New(slog.DiscardHandler)
}

// PublicNotes prepares violations for storage and publication. The kinds and
// this system's own explanations are published in full; the offending text is
// withheld whenever it could itself be a fabricated name or phone number,
// because publishing a rejection must not publish what was rejected.
func PublicNotes(vs []Violation) []Violation {
	out := make([]Violation, 0, len(vs))
	for _, v := range vs {
		clean := Violation{Kind: v.Kind, Detail: v.Detail}
		switch v.Kind {
		case KindPossibleName, KindPossiblePhone:
			// No excerpt at all: the excerpt IS the invented person.
		default:
			// A long digit run is withheld whatever kind it arrived as: a
			// fabricated phone number reaches the checker as an unknown
			// NUMBER as readily as as a phone.
			excerpt := v.Excerpt
			if digitCount(excerpt) >= 7 {
				excerpt = ""
			}
			excerpt = logging.RedactString(excerpt)
			if r := []rune(excerpt); len(r) > 40 {
				excerpt = string(r[:40])
			}
			clean.Excerpt = excerpt
		}
		out = append(out, clean)
	}
	return out
}

func digitCount(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			n++
		}
	}
	return n
}
