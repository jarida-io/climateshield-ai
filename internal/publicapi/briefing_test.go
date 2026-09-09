// SPDX-License-Identifier: Apache-2.0

package publicapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/jarida-io/climateshield/internal/briefing"
	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/briefing/facts/factstest"
	"github.com/jarida-io/climateshield/internal/briefing/mock"
	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/gen/climateshield/v1/climateshieldv1connect"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/publicapi"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

// storeBriefing writes one briefing row the way the briefing service does.
func storeBriefing(t *testing.T, q *db.Queries, area, lang, body, generator, model, status string, grounded bool, notes any) {
	t.Helper()
	sheet := factstest.Sample()
	canon, hash, err := facts.Canonical(sheet)
	require.NoError(t, err)
	notesJSON, err := json.Marshal(notes)
	require.NoError(t, err)

	_, err = q.UpsertBriefing(context.Background(), db.UpsertBriefingParams{
		AreaID: area, Lang: lang, FactsHash: hash[:], FactsJson: canon,
		Generator: generator, Model: model, PromptVersion: "v1",
		Body: body, Grounded: grounded, GroundingNotes: notesJSON, Status: status,
	})
	require.NoError(t, err)
}

func briefingServer(t *testing.T) (*httptest.Server, *db.Queries) {
	t.Helper()
	pool := testdb.Pool(t)
	srv := publicapi.NewServer(pool, logging.New(io.Discard, "info"))
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)
	return ts, db.New(pool)
}

func fetchBriefing(t *testing.T, ts *httptest.Server, path string) *climateshieldv1.GetBriefingResponse {
	t.Helper()
	status, body := get(t, ts, path)
	require.Equal(t, http.StatusOK, status)
	var msg climateshieldv1.GetBriefingResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))
	return &msg
}

// TestBriefingServesTheTemplateWithItsFactSheet is the default deployment's
// public surface: the text says no model ran, and the fact sheet it came from
// is published beside it.
func TestBriefingServesTheTemplateWithItsFactSheet(t *testing.T) {
	ts, q := briefingServer(t)

	body, err := mock.Template(factstest.Sample(), facts.LangEN, mock.Notice{Kind: mock.NoticeNoModel})
	require.NoError(t, err)
	storeBriefing(t, q, "kisumu", "en", body, "mock", "none", briefing.StatusServed, true, nil)

	msg := fetchBriefing(t, ts, "/v1/briefings?area=Kisumu&lang=en")
	require.Equal(t, "Kisumu", msg.GetArea())
	require.Equal(t, "en", msg.GetLang())
	require.Equal(t, briefing.StatusServed, msg.GetStatus())
	require.Equal(t, "mock", msg.GetGenerator())
	require.Equal(t, "none", msg.GetModel())
	require.True(t, msg.GetGrounded())
	require.Contains(t, msg.GetBody(), "[mock] no language model ran")
	require.Contains(t, msg.GetProvenance(), "no language model ran")
	require.Len(t, msg.GetFactsHashHex(), 64)

	f := msg.GetFacts()
	require.NotNil(t, f, "the fact sheet must be published beside the text")
	require.Equal(t, "Kisumu", f.GetArea())
	require.Equal(t, "2026-09-10", f.GetWindowFrom())
	require.Equal(t, "fixture", f.GetWindowSource())
	require.Len(t, f.GetScores(), 4)
	require.Equal(t, "cholera", f.GetScores()[0].GetDisease())
	require.Equal(t, "HIGH", f.GetScores()[0].GetLevel())
	require.InDelta(t, 0.02, f.GetScores()[0].GetExceedance(), 1e-9)
	require.False(t, f.GetChannelSends())
	require.Contains(t, f.GetChannelNote(), "no SMS is sent")

	// The suppressed alert count must arrive suppressed, not as a number.
	var suppressedSeen bool
	for _, a := range f.GetAlertsAllCounties() {
		if a.GetSuppressed() {
			suppressedSeen = true
			require.Nil(t, a.Count, "a suppressed count must carry no value")
		}
	}
	require.True(t, suppressedSeen)
}

// TestRejectedBriefingSaysSoInPublic: a refused model draft is visible as a
// refusal, with its reasons — a rejection nobody can see is a rejection that
// will quietly stop happening.
func TestRejectedBriefingSaysSoInPublic(t *testing.T) {
	ts, q := briefingServer(t)

	body, err := mock.Template(factstest.Sample(), facts.LangEN, mock.Notice{
		Kind: mock.NoticeRejected, Generator: "anthropic", Model: "claude-opus-5",
		Reasons: []string{"unknown_number", "possible_name"},
	})
	require.NoError(t, err)
	storeBriefing(t, q, "kisumu", "en", body, "anthropic", "claude-opus-5",
		briefing.StatusRejected, false, []map[string]string{
			{"kind": "unknown_number", "detail": "this number is not in the fact sheet", "excerpt": "82"},
			{"kind": "possible_name", "detail": "a capitalised word may be a person's name"},
		})

	msg := fetchBriefing(t, ts, "/v1/briefings?area=Kisumu")
	require.Equal(t, briefing.StatusRejected, msg.GetStatus())
	require.False(t, msg.GetGrounded())
	require.Contains(t, msg.GetProvenance(), "No language model text is shown")
	require.Contains(t, msg.GetProvenance(), "unknown_number, possible_name")
	require.Contains(t, msg.GetBody(), "[mock] the claude-opus-5 draft (anthropic) was rejected")

	require.Len(t, msg.GetGroundingNotes(), 2)
	require.Equal(t, "unknown_number", msg.GetGroundingNotes()[0].GetKind())
	require.Equal(t, "82", msg.GetGroundingNotes()[0].GetExcerpt())
	require.Empty(t, msg.GetGroundingNotes()[1].GetExcerpt(),
		"a possible name is never republished, only counted")
}

func TestBriefingReportsAnUnavailableGenerator(t *testing.T) {
	ts, q := briefingServer(t)

	body, err := mock.Template(factstest.Sample(), facts.LangSW, mock.Notice{
		Kind: mock.NoticeUnavailable, Generator: "openai-compatible", Model: "qwen2.5:1.5b",
	})
	require.NoError(t, err)
	storeBriefing(t, q, "kisumu", "sw", body, "openai-compatible", "qwen2.5:1.5b",
		briefing.StatusUnavailable, false, []map[string]string{
			{"kind": briefing.KindUnavailable, "detail": "connection refused"},
		})

	msg := fetchBriefing(t, ts, "/v1/briefings?area=Kisumu&lang=sw")
	require.Equal(t, briefing.StatusUnavailable, msg.GetStatus())
	require.Contains(t, msg.GetProvenance(), "No language model ran")
	require.Contains(t, msg.GetProvenance(), "could not be reached")
	require.Contains(t, msg.GetBody(), "haikupatikana")
}

func TestBriefingSaysWhenNothingHasBeenGeneratedYet(t *testing.T) {
	ts, _ := briefingServer(t)

	msg := fetchBriefing(t, ts, "/v1/briefings?area=Kisumu&lang=en")
	require.Equal(t, "none", msg.GetStatus())
	require.Empty(t, msg.GetBody())
	require.Contains(t, msg.GetNote(), "No briefing has been generated")
	require.Contains(t, msg.GetNote(), "not an error")
}

func TestBriefingWithoutACountyListsWhatIsAvailable(t *testing.T) {
	ts, q := briefingServer(t)
	body, err := mock.Template(factstest.Sample(), facts.LangEN, mock.Notice{Kind: mock.NoticeNoModel})
	require.NoError(t, err)
	storeBriefing(t, q, "kisumu", "en", body, "mock", "none", briefing.StatusServed, true, nil)

	msg := fetchBriefing(t, ts, "/v1/briefings")
	require.Equal(t, "none", msg.GetStatus())
	require.Contains(t, msg.GetNote(), "Ask for one county")
	require.Equal(t, []string{"Kisumu"}, msg.GetAvailableAreas())
	require.Equal(t, []string{"en", "sw"}, msg.GetAvailableLangs())
}

func TestBriefingRejectsBadParameters(t *testing.T) {
	ts, _ := briefingServer(t)

	status, body := get(t, ts, "/v1/briefings?area=Kisumu&lang=fr")
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, body, "unsupported language")

	status, body = get(t, ts, "/v1/briefings?area=Atlantis")
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, body, "unknown county")
}

func TestBriefingOverConnect(t *testing.T) {
	ts, q := briefingServer(t)
	body, err := mock.Template(factstest.Sample(), facts.LangEN, mock.Notice{Kind: mock.NoticeNoModel})
	require.NoError(t, err)
	storeBriefing(t, q, "kisumu", "en", body, "mock", "none", briefing.StatusServed, true, nil)

	client := climateshieldv1connect.NewPublicServiceClient(http.DefaultClient, ts.URL)
	resp, err := client.GetBriefing(context.Background(), connect.NewRequest(&climateshieldv1.GetBriefingRequest{
		Area: "Kisumu", Lang: "en",
	}))
	require.NoError(t, err)
	require.Equal(t, briefing.StatusServed, resp.Msg.GetStatus())
	require.Contains(t, resp.Msg.GetBody(), "[mock] no language model ran")
	require.NotNil(t, resp.Msg.GetFacts())

	_, err = client.GetBriefing(context.Background(), connect.NewRequest(&climateshieldv1.GetBriefingRequest{
		Area: "Kisumu", Lang: "fr",
	}))
	require.Error(t, err, "an unsupported language is a client error, not a stale response")
}
