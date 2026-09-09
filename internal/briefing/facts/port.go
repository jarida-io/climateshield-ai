// SPDX-License-Identifier: Apache-2.0

package facts

import (
	"context"
	_ "embed"
)

// Draft is what a generator returns: the text, and how much it cost.
//
// There is deliberately no field for the raw provider response. A draft that
// fails the grounding check may contain a fabricated person or number, and
// keeping the raw body around invites storing or logging it. What is kept
// about a refused draft is the list of violation KINDS (see the grounding
// checker), which are this system's own words.
type Draft struct {
	Body string
	// Usage carries provider-reported token counts, for cost visibility only.
	Usage map[string]int64
}

// Generator writes one county briefing in one language from one fact sheet.
//
// Name, Model and PromptVersion are recorded with every stored briefing and
// published on the API: a reader always knows what produced the words in
// front of them, and "[mock] no language model ran" is itself an answer.
type Generator interface {
	// Name identifies the adapter: "mock", "openai-compatible", "anthropic".
	Name() string
	// Model is the model identifier, or "none" when no model is involved.
	Model() string
	// PromptVersion identifies the prompt text this generator was built with.
	PromptVersion() string
	// Generate writes the briefing body for one language. It must return an
	// error rather than an approximation: an unreachable model is reported,
	// never papered over.
	Generate(ctx context.Context, f FactSheet, lang string) (Draft, error)
}

//go:embed prompt_v1.txt
var promptV1 string

// PromptVersion identifies the committed prompt text. Bump it whenever
// prompt_v1.txt changes meaningfully: it is part of the cache key, so an
// edited prompt regenerates rather than silently reusing older wording.
const PromptVersion = "v1"

// Prompt returns the system prompt given to every language-model generator.
// It is committed to the repository and versioned so a reviewer can read the
// instructions the model was actually given.
func Prompt() string { return promptV1 }
