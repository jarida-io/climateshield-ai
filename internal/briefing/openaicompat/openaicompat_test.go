// SPDX-License-Identifier: Apache-2.0

package openaicompat_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/briefing/facts/factstest"
	"github.com/jarida-io/climateshield/internal/briefing/openaicompat"
)

// The golden files in testdata/golden/llm are the RESPONSE SHAPES of the
// OpenAI-compatible chat API, written by hand from the published
// documentation. No test here contacts a model server: the adapter is pointed
// at an httptest server, which is the whole reason the base URL is
// configurable.

func golden(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "golden", "llm", name))
	require.NoError(t, err)
	return body
}

// capture records the request the adapter made, so the test can assert what
// left this process — including that nothing person-shaped did.
type capture struct {
	path        string
	auth        string
	contentType string
	body        map[string]any
	raw         string
}

func serve(t *testing.T, status int, response []byte) (*openaicompat.Client, *capture) {
	t.Helper()
	cap := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		cap.path = r.URL.Path
		cap.auth = r.Header.Get("Authorization")
		cap.contentType = r.Header.Get("Content-Type")
		cap.raw = string(raw)
		require.NoError(t, json.Unmarshal(raw, &cap.body))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(response)
	}))
	t.Cleanup(srv.Close)

	return openaicompat.New(openaicompat.Config{
		BaseURL: srv.URL + "/v1",
		Model:   "qwen2.5:1.5b",
		APIKey:  "local-server-ignores-this",
		Timeout: 5 * time.Second,
	}), cap
}

func TestGenerateParsesTheGoldenResponse(t *testing.T) {
	client, cap := serve(t, http.StatusOK, golden(t, "openai-chat-grounded.json"))

	draft, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.NoError(t, err)
	require.Contains(t, draft.Body, "Kisumu is in a wet forecast window")
	require.NotContains(t, draft.Body, "briefing\":", "the JSON envelope is unwrapped, not served raw")
	require.Equal(t, int64(966), draft.Usage["total_tokens"])

	require.Equal(t, "/v1/chat/completions", cap.path)
	require.Equal(t, "application/json", cap.contentType)
	require.Equal(t, "Bearer local-server-ignores-this", cap.auth)
	require.Equal(t, "qwen2.5:1.5b", cap.body["model"])
	require.Equal(t, float64(0), cap.body["temperature"], "generation must be reproducible")
	require.Equal(t, false, cap.body["stream"])
	require.Equal(t, map[string]any{"type": "json_object"}, cap.body["response_format"])
}

// TestRequestCarriesOnlyTheFactSheet is the privacy guarantee, asserted on the
// actual bytes that leave the process: the request contains the committed
// prompt and the fact sheet, and no field through which a person could travel.
func TestRequestCarriesOnlyTheFactSheet(t *testing.T) {
	client, cap := serve(t, http.StatusOK, golden(t, "openai-chat-grounded.json"))
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangSW)
	require.NoError(t, err)

	messages, ok := cap.body["messages"].([]any)
	require.True(t, ok)
	require.Len(t, messages, 2)
	system := messages[0].(map[string]any)
	user := messages[1].(map[string]any)
	require.Equal(t, "system", system["role"])
	require.Equal(t, facts.Prompt(), system["content"])
	require.Equal(t, "user", user["role"])
	require.Contains(t, user["content"], "language: sw")
	require.Contains(t, user["content"], `"area":"Kisumu"`)

	// The system turn is the committed prompt, asserted above. The user turn
	// is the only place data travels, and it must carry nothing person-shaped.
	sent := strings.ToLower(user["content"].(string))
	for _, forbidden := range []string{
		"child_id", "guardian", "phone", "national_id", "date_of_birth", "first_name",
		"name_enc", "leaf", "+254",
	} {
		require.NotContains(t, sent, forbidden,
			"a model request must not be able to carry %q", forbidden)
	}
	require.Contains(t, cap.raw, `"temperature":0`)
}

func TestGenerateReturnsTheHallucinatedDraftForTheCheckerToRefuse(t *testing.T) {
	client, _ := serve(t, http.StatusOK, golden(t, "openai-chat-hallucinated.json"))
	draft, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.NoError(t, err, "the adapter parses; judging the content is the checker's job")
	require.Contains(t, draft.Body, "82 mm")
}

func TestGenerateRefusesATruncatedAnswer(t *testing.T) {
	client, _ := serve(t, http.StatusOK, golden(t, "openai-chat-truncated.json"))
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.ErrorContains(t, err, `stopped with reason "length"`)
}

func TestGenerateRefusesProseInsteadOfTheRequestedJSON(t *testing.T) {
	client, _ := serve(t, http.StatusOK, golden(t, "openai-chat-prose.json"))
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.ErrorContains(t, err, "did not answer with the requested JSON object")
}

func TestGenerateReportsServerFailures(t *testing.T) {
	client, _ := serve(t, http.StatusInternalServerError, []byte(`{"error":"model not loaded"}`))
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.ErrorContains(t, err, "status 500")
	require.NotContains(t, err.Error(), "model not loaded",
		"a remote error body must not be pasted into a log line")
}

func TestGenerateReportsAnEmptyChoiceList(t *testing.T) {
	client, _ := serve(t, http.StatusOK, []byte(`{"choices":[]}`))
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.ErrorContains(t, err, "no choices")
}

func TestGenerateReportsUndecodableResponses(t *testing.T) {
	client, _ := serve(t, http.StatusOK, []byte(`not json at all`))
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.ErrorContains(t, err, "decode response")
}

func TestGenerateReportsAnUnreachableServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	client := openaicompat.New(openaicompat.Config{BaseURL: url + "/v1", Timeout: time.Second})
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.ErrorContains(t, err, "unreachable")
}

func TestGenerateRefusesAnUnsupportedLanguage(t *testing.T) {
	client, _ := serve(t, http.StatusOK, golden(t, "openai-chat-grounded.json"))
	_, err := client.Generate(context.Background(), factstest.Sample(), "fr")
	require.ErrorContains(t, err, "unsupported language")
}

func TestDefaultsAndIdentity(t *testing.T) {
	client := openaicompat.New(openaicompat.Config{})
	require.Equal(t, "openai-compatible", client.Name())
	require.Equal(t, openaicompat.DefaultModel, client.Model())
	require.Equal(t, facts.PromptVersion, client.PromptVersion())
	require.Equal(t, "qwen2.5:1.5b", openaicompat.DefaultModel,
		"the default model must stay on an OSI-approved licence")
}

func TestNoAuthorizationHeaderWithoutAKey(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.auth = r.Header.Get("Authorization")
		_, _ = w.Write(golden(t, "openai-chat-grounded.json"))
	}))
	t.Cleanup(srv.Close)

	client := openaicompat.New(openaicompat.Config{BaseURL: srv.URL + "/v1/"})
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.NoError(t, err)
	require.Empty(t, cap.auth, "the default deployment sends no credential at all")
}

func TestExtractBriefing(t *testing.T) {
	body, err := openaicompat.ExtractBriefing(`{"briefing": "  a county briefing  "}`)
	require.NoError(t, err)
	require.Equal(t, "a county briefing", body)

	body, err = openaicompat.ExtractBriefing("```json\n{\"briefing\": \"fenced\"}\n```")
	require.NoError(t, err)
	require.Equal(t, "fenced", body)

	_, err = openaicompat.ExtractBriefing(`{"briefing": "   "}`)
	require.ErrorContains(t, err, "empty briefing")

	_, err = openaicompat.ExtractBriefing("plain prose")
	require.ErrorContains(t, err, "requested JSON object")
}

func TestUserMessage(t *testing.T) {
	msg, err := openaicompat.UserMessage(factstest.Sample(), facts.LangEN)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(msg, "language: en\n\nfact sheet:\n"))
	canon, _, err := facts.Canonical(factstest.Sample())
	require.NoError(t, err)
	require.Contains(t, msg, string(canon))
}
