// SPDX-License-Identifier: Apache-2.0

// Package anthropic is the OPT-IN briefing generator backed by the Claude API
// through the official Go SDK.
//
// It is constructed only when ANTHROPIC_API_KEY is set. Nothing in this
// repository ships a key, `make up` never reaches this code, and no test may
// contact the API — the tests point the SDK at an httptest server with
// option.WithBaseURL and read committed golden responses whose shapes were
// copied by hand from the published API documentation.
//
// The model receives only a fact sheet (see internal/briefing/facts), and its
// draft is served only if the grounding check passes. A refusal, a truncated
// answer or an unreachable API is reported as such; none of them is allowed
// to become a briefing.
package anthropic

import (
	"context"
	"fmt"
	"strings"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/briefing/openaicompat"
)

// Name identifies this generator in stored briefings and on the API.
const Name = "anthropic"

// DefaultModel is the model used when BRIEFING_MODEL is not set.
// claude-sonnet-5 and claude-haiku-4-5 are documented cheaper overrides.
const DefaultModel = "claude-opus-5"

// maxTokens bounds one briefing. A county briefing is a few short paragraphs;
// this is generous, and a reply that hits the ceiling is reported as
// truncated rather than served half-written.
const maxTokens = 2048

// Config configures the adapter.
type Config struct {
	// APIKey is required. The adapter must not be constructed without one:
	// an empty key would produce authentication failures that look like an
	// unavailable model rather than a missing configuration.
	APIKey string
	// Model defaults to DefaultModel.
	Model string
	// BaseURL overrides the API endpoint. It is empty in every deployment and
	// is how the tests point the SDK at a local httptest server.
	BaseURL string
	// Timeout bounds one generation.
	Timeout time.Duration
}

// Client is a facts.Generator backed by the Claude API.
type Client struct {
	client anthropicsdk.Client
	model  string
}

// New builds a client. It returns an error when no API key is configured, so
// a deployment that asks for this generator without a key fails at startup
// instead of silently serving templates that claim a model ran.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("briefing/anthropic: ANTHROPIC_API_KEY is not set")
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.Timeout > 0 {
		opts = append(opts, option.WithRequestTimeout(cfg.Timeout))
	}
	return &Client{client: anthropicsdk.NewClient(opts...), model: cfg.Model}, nil
}

// Name implements facts.Generator.
func (c *Client) Name() string { return Name }

// Model implements facts.Generator.
func (c *Client) Model() string { return c.model }

// PromptVersion implements facts.Generator.
func (c *Client) PromptVersion() string { return facts.PromptVersion }

// Generate implements facts.Generator.
func (c *Client) Generate(ctx context.Context, f facts.FactSheet, lang string) (facts.Draft, error) {
	if !facts.ValidLanguage(lang) {
		return facts.Draft{}, fmt.Errorf("briefing/anthropic: unsupported language %q", lang)
	}
	user, err := openaicompat.UserMessage(f, lang)
	if err != nil {
		return facts.Draft{}, err
	}

	msg, err := c.client.Messages.New(ctx, anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(c.model),
		MaxTokens: maxTokens,
		System:    []anthropicsdk.TextBlockParam{{Text: facts.Prompt()}},
		Messages: []anthropicsdk.MessageParam{
			anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock(user)),
		},
	})
	if err != nil {
		return facts.Draft{}, fmt.Errorf("briefing/anthropic: %w", err)
	}

	// Check why the model stopped BEFORE reading any content. A refusal or a
	// truncation carries content too, and serving it would be publishing half
	// a briefing — or a decline — as if it were the whole answer.
	switch msg.StopReason {
	case anthropicsdk.StopReasonEndTurn, anthropicsdk.StopReasonStopSequence, "":
	case anthropicsdk.StopReasonRefusal:
		return facts.Draft{}, fmt.Errorf(
			"briefing/anthropic: the model declined this request (category %q)",
			string(msg.StopDetails.Category))
	case anthropicsdk.StopReasonMaxTokens:
		return facts.Draft{}, fmt.Errorf(
			"briefing/anthropic: the reply hit the %d-token ceiling and is truncated", maxTokens)
	default:
		return facts.Draft{}, fmt.Errorf(
			"briefing/anthropic: unexpected stop reason %q", string(msg.StopReason))
	}

	var text strings.Builder
	for _, block := range msg.Content {
		if tb, ok := block.AsAny().(anthropicsdk.TextBlock); ok {
			text.WriteString(tb.Text)
		}
	}
	body, err := openaicompat.ExtractBriefing(text.String())
	if err != nil {
		return facts.Draft{}, fmt.Errorf("briefing/anthropic: %w", err)
	}
	return facts.Draft{
		Body: body,
		Usage: map[string]int64{
			"input_tokens":  msg.Usage.InputTokens,
			"output_tokens": msg.Usage.OutputTokens,
		},
	}, nil
}
