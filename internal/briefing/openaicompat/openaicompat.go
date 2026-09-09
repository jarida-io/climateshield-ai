// SPDX-License-Identifier: Apache-2.0

// Package openaicompat is the OPT-IN briefing generator for a locally hosted,
// open-weights model exposing the OpenAI-compatible chat API — Ollama or
// llama.cpp's server, running beside this stack.
//
// It is opt-in for a reason: the default `make up` is offline and
// credential-free, and this adapter is only reached when BRIEFING_GENERATOR
// says so. There is no OpenAI SDK here — one request struct and one response
// struct, in the same shape as internal/climate/openmeteo — so the dependency
// footprint of an optional feature stays at net/http, and the tests point the
// adapter at an httptest server with a committed golden response.
//
// The model receives only a fact sheet (see internal/briefing/facts): no
// child, no guardian, no phone number, no count below the k>=10 threshold.
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jarida-io/climateshield/internal/briefing/facts"
)

// Name identifies this generator in stored briefings and on the API.
const Name = "openai-compatible"

// DefaultModel is a small open-weights model under an OSI-approved licence
// (Apache-2.0). Models under research or community licences are deliberately
// not defaulted to in a project that promises open source throughout.
const DefaultModel = "qwen2.5:1.5b"

// DefaultBaseURL is the local Ollama endpoint on the host. The compose
// override sets the in-network address instead.
const DefaultBaseURL = "http://localhost:11434/v1"

// Config configures the adapter.
type Config struct {
	// BaseURL is the OpenAI-compatible root, e.g. http://ollama:11434/v1.
	BaseURL string
	// Model is the model name the server knows, e.g. qwen2.5:1.5b.
	Model string
	// APIKey is sent as a bearer token. Local servers ignore it; it exists so
	// the adapter also works against a self-hosted gateway that wants one. It
	// is NOT required and the stack ships without one.
	APIKey string
	// Timeout bounds one generation. A 1.5B model on a laptop CPU takes tens
	// of seconds, which is exactly why generation runs as a background job and
	// never in the request path.
	Timeout time.Duration
}

// Client is a facts.Generator backed by an OpenAI-compatible chat endpoint.
type Client struct {
	cfg Config
	hc  *http.Client
}

// New builds a client. An empty model or base URL falls back to the defaults.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 120 * time.Second
	}
	return &Client{cfg: cfg, hc: &http.Client{Timeout: cfg.Timeout}}
}

// Name implements facts.Generator.
func (c *Client) Name() string { return Name }

// Model implements facts.Generator.
func (c *Client) Model() string { return c.cfg.Model }

// PromptVersion implements facts.Generator.
func (c *Client) PromptVersion() string { return facts.PromptVersion }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	// Temperature 0: the same facts should produce the same briefing, and a
	// cached briefing that cannot be reproduced is not evidence of anything.
	Temperature    float64        `json:"temperature"`
	Stream         bool           `json:"stream"`
	ResponseFormat responseFormat `json:"response_format"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage"`
}

// Generate implements facts.Generator.
func (c *Client) Generate(ctx context.Context, f facts.FactSheet, lang string) (facts.Draft, error) {
	if !facts.ValidLanguage(lang) {
		return facts.Draft{}, fmt.Errorf("openaicompat: unsupported language %q", lang)
	}
	user, err := UserMessage(f, lang)
	if err != nil {
		return facts.Draft{}, err
	}
	payload, err := json.Marshal(chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: facts.Prompt()},
			{Role: "user", Content: user},
		},
		Temperature:    0,
		Stream:         false,
		ResponseFormat: responseFormat{Type: "json_object"},
	})
	if err != nil {
		return facts.Draft{}, fmt.Errorf("openaicompat: encode request: %w", err)
	}

	url := strings.TrimSuffix(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return facts.Draft{}, fmt.Errorf("openaicompat: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return facts.Draft{}, fmt.Errorf("openaicompat: %s unreachable: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return facts.Draft{}, fmt.Errorf("openaicompat: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// The body is not quoted: it is remote text, and an error string ends
		// up in logs. The status is enough to act on.
		return facts.Draft{}, fmt.Errorf("openaicompat: %s returned status %d", url, resp.StatusCode)
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return facts.Draft{}, fmt.Errorf("openaicompat: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return facts.Draft{}, fmt.Errorf("openaicompat: %s returned no choices", url)
	}
	choice := parsed.Choices[0]
	// The OpenAI-compatible twin of checking a stop reason: anything other
	// than a natural stop means the text is truncated or filtered, and a
	// truncated briefing must not be served as a briefing.
	if reason := choice.FinishReason; reason != "" && reason != "stop" {
		return facts.Draft{}, fmt.Errorf("openaicompat: generation stopped with reason %q", reason)
	}
	text, err := ExtractBriefing(choice.Message.Content)
	if err != nil {
		return facts.Draft{}, fmt.Errorf("openaicompat: %w", err)
	}
	return facts.Draft{
		Body: text,
		Usage: map[string]int64{
			"prompt_tokens":     parsed.Usage.PromptTokens,
			"completion_tokens": parsed.Usage.CompletionTokens,
			"total_tokens":      parsed.Usage.TotalTokens,
		},
	}, nil
}

// UserMessage is the user turn: the requested language and the fact sheet,
// and nothing else. It is exported because both model adapters send exactly
// the same thing, and a test asserts that the bytes leaving this process
// contain no field that could carry a person.
func UserMessage(f facts.FactSheet, lang string) (string, error) {
	canon, _, err := facts.Canonical(f)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("language: %s\n\nfact sheet:\n%s", lang, string(canon)), nil
}

// briefingEnvelope is the JSON object the prompt asks for.
type briefingEnvelope struct {
	Briefing string `json:"briefing"`
}

// ExtractBriefing reads the briefing text out of a model's reply. It accepts
// the JSON object the prompt asks for, optionally wrapped in a markdown code
// fence, and refuses anything else rather than guessing what the model meant.
func ExtractBriefing(content string) (string, error) {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "```") {
		if i := strings.Index(trimmed, "\n"); i >= 0 {
			trimmed = trimmed[i+1:]
		}
		trimmed = strings.TrimSuffix(strings.TrimSpace(trimmed), "```")
		trimmed = strings.TrimSpace(trimmed)
	}
	var env briefingEnvelope
	if err := json.Unmarshal([]byte(trimmed), &env); err != nil {
		return "", fmt.Errorf("the model did not answer with the requested JSON object")
	}
	if strings.TrimSpace(env.Briefing) == "" {
		return "", fmt.Errorf("the model returned an empty briefing")
	}
	return strings.TrimSpace(env.Briefing), nil
}
