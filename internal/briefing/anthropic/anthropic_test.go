// SPDX-License-Identifier: Apache-2.0

package anthropic_test

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

	anthropicgen "github.com/jarida-io/climateshield/internal/briefing/anthropic"
	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/briefing/facts/factstest"
)

// The golden files in testdata/golden/llm are Messages API RESPONSE SHAPES,
// written by hand from the published documentation. No test here contacts the
// API: option.WithBaseURL points the SDK at an httptest server, which is why
// the adapter exposes a base URL at all.

func golden(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "golden", "llm", name))
	require.NoError(t, err)
	return body
}

type capture struct {
	path   string
	apiKey string
	body   map[string]any
	raw    string
}

func serve(t *testing.T, status int, response []byte) (*anthropicgen.Client, *capture) {
	t.Helper()
	cap := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		cap.path = r.URL.Path
		cap.apiKey = r.Header.Get("X-Api-Key")
		cap.raw = string(raw)
		require.NoError(t, json.Unmarshal(raw, &cap.body))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(response)
	}))
	t.Cleanup(srv.Close)

	client, err := anthropicgen.New(anthropicgen.Config{
		APIKey:  "test-key-not-a-credential",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	return client, cap
}

func TestGenerateParsesTheGoldenResponse(t *testing.T) {
	client, cap := serve(t, http.StatusOK, golden(t, "anthropic-messages-grounded.json"))

	draft, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.NoError(t, err)
	require.Contains(t, draft.Body, "Kisumu is in a wet forecast window")
	require.NotContains(t, draft.Body, `{"briefing"`, "the JSON envelope is unwrapped, not served raw")
	require.Equal(t, int64(1042), draft.Usage["input_tokens"])
	require.Equal(t, int64(187), draft.Usage["output_tokens"])

	require.Equal(t, "/v1/messages", cap.path)
	require.Equal(t, "test-key-not-a-credential", cap.apiKey)
	require.Equal(t, "claude-opus-5", cap.body["model"], "the documented default model")
	require.Equal(t, float64(2048), cap.body["max_tokens"])
}

// TestRequestCarriesOnlyTheFactSheet asserts on the bytes that leave the
// process: the committed prompt as the system turn, the fact sheet as the
// user turn, and nothing through which a person could travel.
func TestRequestCarriesOnlyTheFactSheet(t *testing.T) {
	client, cap := serve(t, http.StatusOK, golden(t, "anthropic-messages-grounded.json"))
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangSW)
	require.NoError(t, err)

	system, ok := cap.body["system"].([]any)
	require.True(t, ok)
	require.Len(t, system, 1)
	require.Equal(t, facts.Prompt(), system[0].(map[string]any)["text"])

	messages := cap.body["messages"].([]any)
	require.Len(t, messages, 1)
	turn := messages[0].(map[string]any)
	require.Equal(t, "user", turn["role"])
	content := turn["content"].([]any)[0].(map[string]any)["text"].(string)
	require.Contains(t, content, "language: sw")
	require.Contains(t, content, `"area":"Kisumu"`)

	lower := strings.ToLower(content)
	for _, forbidden := range []string{
		"child_id", "guardian", "phone", "national_id", "date_of_birth", "first_name",
		"name_enc", "leaf", "+254",
	} {
		require.NotContains(t, lower, forbidden,
			"a model request must not be able to carry %q", forbidden)
	}
}

// TestRefusalIsReportedNotServed: a decline carries content too, and serving
// it would publish "I can't help with that." as a county briefing.
func TestRefusalIsReportedNotServed(t *testing.T) {
	client, _ := serve(t, http.StatusOK, golden(t, "anthropic-refusal.json"))
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.ErrorContains(t, err, "declined this request")
	require.ErrorContains(t, err, "general_harms")
}

func TestTruncatedAnswerIsReportedNotServed(t *testing.T) {
	client, _ := serve(t, http.StatusOK, golden(t, "anthropic-max-tokens.json"))
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.ErrorContains(t, err, "truncated")
}

func TestHallucinatedDraftIsReturnedForTheCheckerToRefuse(t *testing.T) {
	client, _ := serve(t, http.StatusOK, golden(t, "anthropic-messages-hallucinated.json"))
	draft, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.NoError(t, err, "the adapter parses; judging the content is the checker's job")
	require.Contains(t, draft.Body, "82 mm")
}

func TestProseInsteadOfJSONIsRefused(t *testing.T) {
	client, _ := serve(t, http.StatusOK, []byte(`{
		"id": "msg_01Prose", "type": "message", "role": "assistant", "model": "claude-opus-5",
		"content": [{"type": "text", "text": "Here is your briefing, in prose."}],
		"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 8}
	}`))
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.ErrorContains(t, err, "requested JSON object")
}

func TestUnexpectedStopReasonIsRefused(t *testing.T) {
	client, _ := serve(t, http.StatusOK, []byte(`{
		"id": "msg_01Tool", "type": "message", "role": "assistant", "model": "claude-opus-5",
		"content": [{"type": "text", "text": "{\"briefing\": \"...\"}"}],
		"stop_reason": "tool_use", "usage": {"input_tokens": 10, "output_tokens": 8}
	}`))
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.ErrorContains(t, err, "unexpected stop reason")
}

func TestAPIErrorsAreReported(t *testing.T) {
	client, _ := serve(t, http.StatusUnauthorized, []byte(`{
		"type": "error", "error": {"type": "authentication_error", "message": "invalid x-api-key"}
	}`))
	_, err := client.Generate(context.Background(), factstest.Sample(), facts.LangEN)
	require.ErrorContains(t, err, "briefing/anthropic:")
}

// TestNewRefusesWithoutAKey is the fail-closed rule: a deployment that asks
// for this generator and forgets the key must not start and quietly serve
// templates that claim a model was configured.
func TestNewRefusesWithoutAKey(t *testing.T) {
	_, err := anthropicgen.New(anthropicgen.Config{})
	require.ErrorContains(t, err, "ANTHROPIC_API_KEY is not set")

	_, err = anthropicgen.New(anthropicgen.Config{APIKey: "   "})
	require.ErrorContains(t, err, "ANTHROPIC_API_KEY is not set")
}

func TestIdentity(t *testing.T) {
	client, err := anthropicgen.New(anthropicgen.Config{APIKey: "test-key-not-a-credential"})
	require.NoError(t, err)
	require.Equal(t, "anthropic", client.Name())
	require.Equal(t, "claude-opus-5", client.Model())
	require.Equal(t, facts.PromptVersion, client.PromptVersion())

	override, err := anthropicgen.New(anthropicgen.Config{
		APIKey: "test-key-not-a-credential", Model: "claude-haiku-4-5",
	})
	require.NoError(t, err)
	require.Equal(t, "claude-haiku-4-5", override.Model())
}

func TestUnsupportedLanguageIsRefusedBeforeAnyRequest(t *testing.T) {
	client, cap := serve(t, http.StatusOK, golden(t, "anthropic-messages-grounded.json"))
	_, err := client.Generate(context.Background(), factstest.Sample(), "fr")
	require.ErrorContains(t, err, "unsupported language")
	require.Empty(t, cap.raw, "an unsupported language must not cost a request")
}
