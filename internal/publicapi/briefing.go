// SPDX-License-Identifier: Apache-2.0

package publicapi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/jarida-io/climateshield/internal/briefing/facts"
	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// The briefing view. It publishes the text a reader sees AND the fact sheet
// that text was generated from, so every sentence can be checked against the
// numbers behind it without trusting this service.
//
// Three things this endpoint will not do:
//   - it will not generate. Generation is a background job in the briefing
//     service; a public read never waits on a language model.
//   - it will not attribute text to a model that did not write it. The
//     provenance line is computed from the stored row, and a rejected or
//     unavailable generation is reported as such, with the reasons.
//   - it will not publish anything person-derived. The fact sheet contains
//     county-level scores, the forecast window and k>=10 suppressed
//     system-wide message counts, and nothing else.

// briefingNotes is the stored grounding-note shape.
type briefingNote struct {
	Kind    string `json:"kind"`
	Detail  string `json:"detail"`
	Excerpt string `json:"excerpt,omitempty"`
}

// buildBriefing serves one county's briefing in one language.
func (s *Server) buildBriefing(ctx context.Context, area, lang string) (*climateshieldv1.GetBriefingResponse, error) {
	if lang == "" {
		lang = facts.LangEN
	}
	if !facts.ValidLanguage(lang) {
		return nil, errBadRequest{fmt.Sprintf("unsupported language %q (want %q or %q)", lang, facts.LangEN, facts.LangSW)}
	}

	resp := &climateshieldv1.GetBriefingResponse{
		Lang:           lang,
		Area:           area,
		Status:         "none",
		AvailableLangs: facts.Languages,
		GeneratedAt:    timestamppb.Now(),
	}

	available, err := s.q.ListBriefingAreas(ctx, lang)
	if err != nil {
		return nil, fmt.Errorf("publicapi: briefing areas: %w", err)
	}
	for _, a := range available {
		resp.AvailableAreas = append(resp.AvailableAreas, a.AreaName)
	}

	if area == "" {
		resp.Note = "Ask for one county: /v1/briefings?area=<county>&lang=en. " +
			"Counties with a briefing in this language are listed in available_areas."
		return resp, nil
	}
	areaID, ok := s.areaIDFor(ctx, area)
	if !ok {
		return nil, errBadRequest{fmt.Sprintf("unknown county %q", area)}
	}

	row, err := s.q.LatestBriefing(ctx, db.LatestBriefingParams{AreaID: areaID, Lang: lang})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			resp.Note = "No briefing has been generated for this county and language yet. " +
				"Briefings are written by the briefing service when a county's fact sheet changes; " +
				"this is not an error and nothing is being withheld."
			return resp, nil
		}
		return nil, fmt.Errorf("publicapi: briefing: %w", err)
	}

	resp.Body = row.Body
	resp.Generator = row.Generator
	resp.Model = row.Model
	resp.PromptVersion = row.PromptVersion
	resp.FactsHashHex = hex.EncodeToString(row.FactsHash)
	resp.Grounded = row.Grounded
	resp.Status = row.Status
	if row.CreatedAt.Valid {
		resp.CreatedAt = timestamppb.New(row.CreatedAt.Time)
	}

	var notes []briefingNote
	if len(row.GroundingNotes) > 0 {
		if err := json.Unmarshal(row.GroundingNotes, &notes); err != nil {
			// A note that cannot be decoded is reported as one that cannot be
			// decoded — never dropped, which would hide a rejection.
			notes = []briefingNote{{Kind: "unreadable_note", Detail: "the stored grounding notes could not be decoded"}}
		}
	}
	for _, n := range notes {
		resp.GroundingNotes = append(resp.GroundingNotes, &climateshieldv1.GroundingNote{
			Kind: n.Kind, Detail: n.Detail, Excerpt: n.Excerpt,
		})
	}

	var sheet facts.FactSheet
	if err := json.Unmarshal(row.FactsJson, &sheet); err == nil {
		resp.Facts = briefingFacts(sheet)
	}
	resp.Provenance = provenance(row, notes)
	return resp, nil
}

// briefingFacts republishes the stored fact sheet. It is a straight
// projection: the sheet served here is byte-for-byte the sheet the generator
// was given, which is what makes "check every number" a real instruction
// rather than an invitation to trust.
func briefingFacts(f facts.FactSheet) *climateshieldv1.BriefingFacts {
	out := &climateshieldv1.BriefingFacts{
		Area:         f.Area,
		WindowFrom:   f.Window.From,
		WindowTo:     f.Window.To,
		WindowSource: f.Window.Source,
		WindowDays:   int32(f.Window.Days),
		ChannelSends: f.ChannelSends,
		ChannelNote:  f.ChannelNote,
	}
	for _, s := range f.Scores {
		score := &climateshieldv1.BriefingScore{
			Disease: s.Disease, Level: s.Level, Driver: s.Driver,
			DriverValue: s.DriverValue, Explanation: s.Explanation,
			Predictor: s.Predictor, PredictorVersion: s.Version,
		}
		if s.Exceedance != nil {
			e := *s.Exceedance
			score.Exceedance = &e
		}
		out.Scores = append(out.Scores, score)
	}
	for _, a := range f.AlertsAllCounties {
		count := &climateshieldv1.BriefingAlertCount{Status: a.Status, Suppressed: a.Suppressed}
		if a.Count != nil {
			v := *a.Count
			count.Count = &v
		}
		out.AlertsAllCounties = append(out.AlertsAllCounties, count)
	}
	return out
}

// provenance states, in one line, what produced the text above it. It is
// computed from the stored row: a briefing can never be labelled with a model
// that did not write it, because the label is derived from the same row as
// the words.
func provenance(row db.Briefing, notes []briefingNote) string {
	hash := hex.EncodeToString(row.FactsHash)
	if len(hash) > 12 {
		hash = hash[:12]
	}
	var kinds []string
	for _, n := range notes {
		kinds = append(kinds, n.Kind)
	}
	reasons := strings.Join(kinds, ", ")

	switch row.Status {
	case "rejected":
		return fmt.Sprintf(
			"No language model text is shown. %s (%s) produced a draft and the grounding check "+
				"refused it (%s), so the deterministic template is served instead. Prompt %s, facts %s.",
			row.Model, row.Generator, reasons, row.PromptVersion, hash)
	case "unavailable":
		return fmt.Sprintf(
			"No language model ran: %s (%s) could not be reached, so the deterministic template is "+
				"served instead. Prompt %s, facts %s.",
			row.Model, row.Generator, row.PromptVersion, hash)
	default:
		if row.Generator == "mock" {
			return fmt.Sprintf(
				"Generated by a deterministic template — no language model ran. Template %s, facts %s.",
				row.PromptVersion, hash)
		}
		return fmt.Sprintf(
			"Generated by %s via the %s adapter, prompt %s, facts %s; every number, county, "+
				"disease and risk level in it was checked against the fact sheet below.",
			row.Model, row.Generator, row.PromptVersion, hash)
	}
}
