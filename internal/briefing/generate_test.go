// SPDX-License-Identifier: Apache-2.0

package briefing_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/briefing"
	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/briefing/facts/factstest"
	"github.com/jarida-io/climateshield/internal/briefing/mock"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// --- a fake store -------------------------------------------------------

type fakeStore struct {
	areas  []db.ListAreasRow
	risk   []db.CurrentRiskRow
	series []db.LatestSeriesForAllAreasRow
	alerts []db.CountAlertsByStatusRow

	rows   map[string]db.Briefing
	writes int
	upsert error
}

func newFakeStore() *fakeStore {
	sheet := factstest.Sample()
	s := &fakeStore{
		areas: []db.ListAreasRow{{ID: "kisumu", Name: "Kisumu"}, {ID: "nairobi", Name: "Nairobi"}},
		alerts: []db.CountAlertsByStatusRow{
			{Status: "would_send", N: 24},
			{Status: "skipped_consent", N: 3},
		},
		rows: map[string]db.Briefing{},
	}
	for i, day := range []string{"2026-09-10", "2026-09-23"} {
		t, err := time.Parse("2006-01-02", day)
		if err != nil {
			panic(err)
		}
		_ = i
		s.series = append(s.series, db.LatestSeriesForAllAreasRow{
			AreaID: "kisumu", AreaName: "Kisumu", Source: "fixture",
			ForecastDate: pgtype.Date{Time: t, Valid: true},
		})
	}
	for _, score := range sheet.Scores {
		explanation := score.Explanation
		s.risk = append(s.risk, db.CurrentRiskRow{
			AreaID: "kisumu", Disease: score.Disease, Level: score.Level,
			Driver: score.Driver, DriverValue: score.DriverValue,
			Predictor: score.Predictor, PredictorVersion: score.Version,
			Explanation: &explanation,
		})
	}
	return s
}

func (f *fakeStore) ListAreas(context.Context) ([]db.ListAreasRow, error) { return f.areas, nil }
func (f *fakeStore) CurrentRisk(context.Context) ([]db.CurrentRiskRow, error) {
	return f.risk, nil
}

func (f *fakeStore) LatestSeriesForAllAreas(context.Context) ([]db.LatestSeriesForAllAreasRow, error) {
	return f.series, nil
}

func (f *fakeStore) CountAlertsByStatus(context.Context) ([]db.CountAlertsByStatusRow, error) {
	return f.alerts, nil
}

func key(area, lang string, hash []byte, gen, model, prompt string) string {
	return strings.Join([]string{area, lang, string(hash), gen, model, prompt}, "|")
}

func (f *fakeStore) GetBriefingForKey(_ context.Context, arg db.GetBriefingForKeyParams) (db.Briefing, error) {
	row, ok := f.rows[key(arg.AreaID, arg.Lang, arg.FactsHash, arg.Generator, arg.Model, arg.PromptVersion)]
	if !ok {
		return db.Briefing{}, pgx.ErrNoRows
	}
	return row, nil
}

func (f *fakeStore) UpsertBriefing(_ context.Context, arg db.UpsertBriefingParams) (int64, error) {
	if f.upsert != nil {
		return 0, f.upsert
	}
	f.writes++
	f.rows[key(arg.AreaID, arg.Lang, arg.FactsHash, arg.Generator, arg.Model, arg.PromptVersion)] = db.Briefing{
		ID: int64(f.writes), AreaID: arg.AreaID, Lang: arg.Lang, FactsHash: arg.FactsHash,
		FactsJson: arg.FactsJson, Generator: arg.Generator, Model: arg.Model,
		PromptVersion: arg.PromptVersion, Body: arg.Body, Grounded: arg.Grounded,
		GroundingNotes: arg.GroundingNotes, Status: arg.Status,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	return int64(f.writes), nil
}

func (f *fakeStore) only(t *testing.T, area, lang string) db.Briefing {
	t.Helper()
	for _, row := range f.rows {
		if row.AreaID == area && row.Lang == lang {
			return row
		}
	}
	t.Fatalf("no briefing stored for %s/%s", area, lang)
	return db.Briefing{}
}

// --- fake generators ----------------------------------------------------

type fakeGen struct {
	name, model, prompt string
	body                string
	err                 error
	calls               int
}

func (g *fakeGen) Name() string          { return g.name }
func (g *fakeGen) Model() string         { return g.model }
func (g *fakeGen) PromptVersion() string { return g.prompt }
func (g *fakeGen) Generate(context.Context, facts.FactSheet, string) (facts.Draft, error) {
	g.calls++
	if g.err != nil {
		return facts.Draft{}, g.err
	}
	return facts.Draft{Body: g.body}, nil
}

// mockGenerator is the real deterministic template generator, used where a
// test needs the default path rather than a stub.
func mockGenerator() briefing.Generator { return mock.New() }

func modelGen(body string) *fakeGen {
	return &fakeGen{name: "anthropic", model: "claude-opus-5", prompt: "v1", body: body}
}

func sweeper(store briefing.Store, gen briefing.Generator) *briefing.Sweeper {
	return &briefing.Sweeper{
		Store:   store,
		Gen:     gen,
		Checker: briefing.NewChecker(factstest.Counties),
		Channel: "mock",
		Now:     func() time.Time { return time.Date(2026, 9, 10, 6, 0, 0, 0, time.UTC) },
	}
}

// --- tests --------------------------------------------------------------

// TestSweepWithNoModelServesTheLabelledTemplate is the default path: `make up`
// with no model configured produces briefings that say no model ran.
func TestSweepWithNoModelServesTheLabelledTemplate(t *testing.T) {
	store := newFakeStore()
	s := sweeper(store, mock.New())

	sum, err := s.Sweep(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, 2, sum.Areas)
	require.Equal(t, 4, sum.Served, "two counties x two languages")
	require.Zero(t, sum.Cached)

	row := store.only(t, "kisumu", "en")
	require.Equal(t, briefing.StatusServed, row.Status)
	require.Equal(t, "mock", row.Generator)
	require.Equal(t, "none", row.Model)
	require.True(t, row.Grounded)
	require.True(t, strings.HasPrefix(row.Body, "[mock] no language model ran"))
	require.JSONEq(t, "null", string(row.GroundingNotes))
}

// TestSweepIsIdempotent is what lets the sweep run on a short interval with a
// model configured: unchanged facts cost a hash comparison, not a generation.
func TestSweepIsIdempotent(t *testing.T) {
	store := newFakeStore()
	gen := modelGen(groundedDraft)
	s := sweeper(store, gen)

	first, err := s.Sweep(context.Background(), "kisumu")
	require.NoError(t, err)
	require.Equal(t, 2, first.Served)
	require.Equal(t, 2, gen.calls)
	writes := store.writes

	second, err := s.Sweep(context.Background(), "kisumu")
	require.NoError(t, err)
	require.Equal(t, 2, second.Cached)
	require.Zero(t, second.Served)
	require.Equal(t, 2, gen.calls, "an unchanged fact sheet must not call the model again")
	require.Equal(t, writes, store.writes, "and must not write again")
}

func TestSweepRegeneratesWhenTheFactsChange(t *testing.T) {
	store := newFakeStore()
	gen := modelGen(groundedDraft)
	s := sweeper(store, gen)

	_, err := s.Sweep(context.Background(), "kisumu")
	require.NoError(t, err)

	// The rain stops: cholera drops to LOW.
	for i := range store.risk {
		if store.risk[i].Disease == "cholera" {
			store.risk[i].Level = "LOW"
		}
	}
	sum, err := s.Sweep(context.Background(), "kisumu")
	require.NoError(t, err)
	require.Equal(t, 4, gen.calls, "changed facts must be put to the generator again")
	require.Zero(t, sum.Cached)
	require.Equal(t, 2, sum.Rejected+sum.Served,
		"changed facts must produce a fresh verdict, not reuse the old one")
}

func TestSweepServesAGroundedModelDraft(t *testing.T) {
	store := newFakeStore()
	s := sweeper(store, modelGen(groundedDraft))

	sum, err := s.Sweep(context.Background(), "kisumu")
	require.NoError(t, err)
	require.Equal(t, 2, sum.Served)

	row := store.only(t, "kisumu", "en")
	require.Equal(t, briefing.StatusServed, row.Status)
	require.Equal(t, "anthropic", row.Generator)
	require.Equal(t, "claude-opus-5", row.Model)
	require.True(t, row.Grounded)
	require.Equal(t, groundedDraft, row.Body)
	require.NotContains(t, row.Body, "[mock]")
}

// TestUngroundedDraftIsRejectedAndTheTemplateIsServed is the rule the whole
// pillar rests on: a draft that fails the check is not served, the reader is
// told a model draft was refused and why, and no model text is stored.
func TestUngroundedDraftIsRejectedAndTheTemplateIsServed(t *testing.T) {
	hallucinated := groundedDraft +
		" Dengue is HIGH in Nairobi at 82 mm, guardian Amina Otieno was called on +254712345678, " +
		"and the model reaches an accuracy of 91 percent."

	store := newFakeStore()
	s := sweeper(store, modelGen(hallucinated))

	sum, err := s.Sweep(context.Background(), "kisumu")
	require.NoError(t, err)
	require.Equal(t, 2, sum.Rejected)
	require.Zero(t, sum.Served)

	row := store.only(t, "kisumu", "en")
	require.Equal(t, briefing.StatusRejected, row.Status)
	require.False(t, row.Grounded)
	require.Equal(t, "anthropic", row.Generator, "the attempted generator is recorded")
	require.Contains(t, row.Body, "[mock] the claude-opus-5 draft (anthropic) was rejected")
	require.Contains(t, row.Body, "No model text is shown here")

	// Nothing the model wrote may survive anywhere in the stored row.
	stored := row.Body + string(row.GroundingNotes)
	// The kinds and the offending tokens are published so a rejection is
	// visible; the model's sentences, invented people and phone numbers are
	// not stored anywhere.
	for _, invented := range []string{
		"Amina", "Otieno", "+254712345678", "254712345678",
		"Dengue is HIGH", "accuracy of 91 percent", "lake basin",
	} {
		require.NotContains(t, stored, invented,
			"a rejected draft's text must not be stored, not even in the notes")
	}

	var notes []map[string]string
	require.NoError(t, json.Unmarshal(row.GroundingNotes, &notes))
	kinds := map[string]bool{}
	for _, n := range notes {
		kinds[n["kind"]] = true
	}
	require.True(t, kinds[briefing.KindForeignCounty])
	require.True(t, kinds[briefing.KindUnknownNumber])
	require.True(t, kinds[briefing.KindForbiddenClaim])
	require.True(t, kinds[briefing.KindPossibleName])
	require.True(t, kinds[briefing.KindPossiblePhone])
}

// TestUnavailableGeneratorIsRetried: an unreachable model is a temporary
// condition, so its row is not treated as a final verdict.
func TestUnavailableGeneratorIsRetried(t *testing.T) {
	store := newFakeStore()
	gen := modelGen(groundedDraft)
	gen.err = errors.New("dial tcp 127.0.0.1:11434: connect: connection refused")
	s := sweeper(store, gen)

	sum, err := s.Sweep(context.Background(), "kisumu")
	require.NoError(t, err)
	require.Equal(t, 2, sum.Unavailable)

	row := store.only(t, "kisumu", "en")
	require.Equal(t, briefing.StatusUnavailable, row.Status)
	require.False(t, row.Grounded)
	require.Contains(t, row.Body, "could not be reached, so no language model ran")

	var notes []map[string]string
	require.NoError(t, json.Unmarshal(row.GroundingNotes, &notes))
	require.Len(t, notes, 1)
	require.Equal(t, briefing.KindUnavailable, notes[0]["kind"])
	require.Contains(t, notes[0]["detail"], "connection refused")

	// The model comes back on the next sweep, and the same facts are retried.
	gen.err = nil
	sum, err = s.Sweep(context.Background(), "kisumu")
	require.NoError(t, err)
	require.Equal(t, 2, sum.Served, "an unavailable attempt must be retried, not cached")
	require.Equal(t, briefing.StatusServed, store.only(t, "kisumu", "en").Status)
}

func TestSweepScopesToOneCounty(t *testing.T) {
	store := newFakeStore()
	s := sweeper(store, mock.New())

	sum, err := s.Sweep(context.Background(), "nairobi")
	require.NoError(t, err)
	require.Equal(t, 1, sum.Areas)
	require.Equal(t, 2, sum.Served)
	for _, row := range store.rows {
		require.Equal(t, "nairobi", row.AreaID)
	}
}

func TestSweepReportsStorageFailures(t *testing.T) {
	store := newFakeStore()
	store.upsert = errors.New("disk on fire")
	s := sweeper(store, mock.New())
	_, err := s.Sweep(context.Background(), "kisumu")
	require.ErrorContains(t, err, "disk on fire")
}

func TestSweeperHonoursALanguageRestriction(t *testing.T) {
	store := newFakeStore()
	s := sweeper(store, mock.New())
	s.Languages = []string{facts.LangSW}

	sum, err := s.Sweep(context.Background(), "kisumu")
	require.NoError(t, err)
	require.Equal(t, 1, sum.Served)
	require.Equal(t, "sw", store.only(t, "kisumu", "sw").Lang)
}

// TestPublicNotesWithholdInventedText: publishing a rejection must not
// publish what was rejected.
func TestPublicNotesWithholdInventedText(t *testing.T) {
	notes := briefing.PublicNotes([]briefing.Violation{
		{Kind: briefing.KindPossibleName, Detail: "a capitalised word", Excerpt: "Amina Otieno"},
		{Kind: briefing.KindPossiblePhone, Detail: "phone shaped", Excerpt: "+254712345678"},
		{Kind: briefing.KindUnknownNumber, Detail: "not in the fact sheet", Excerpt: "82"},
		{Kind: briefing.KindUnknownNumber, Detail: "not in the fact sheet", Excerpt: "254712345678"},
		{Kind: briefing.KindForeignCounty, Detail: "wrong county", Excerpt: "Nairobi"},
		{Kind: briefing.KindForbiddenClaim, Detail: "unsupportable", Excerpt: strings.Repeat("x", 80)},
	})
	require.Len(t, notes, 6)
	require.Empty(t, notes[0].Excerpt, "a possible name is never republished")
	require.Empty(t, notes[1].Excerpt, "a possible phone number is never republished")
	require.Equal(t, "82", notes[2].Excerpt)
	require.Empty(t, notes[3].Excerpt, "a long digit run is withheld even as a number")
	require.Equal(t, "Nairobi", notes[4].Excerpt)
	require.Len(t, []rune(notes[5].Excerpt), 40, "excerpts are truncated")
	for _, n := range notes {
		require.NotEmpty(t, n.Kind)
		require.NotEmpty(t, n.Detail, "the reason is always published")
	}
}
