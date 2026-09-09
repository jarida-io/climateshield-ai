// SPDX-License-Identifier: Apache-2.0

package briefing_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/briefing"
	"github.com/jarida-io/climateshield/internal/briefing/facts/factstest"
	"github.com/jarida-io/climateshield/internal/platform/logging"
)

func TestSelectGeneratorDefaultsToNoModel(t *testing.T) {
	for _, selection := range []string{"", briefing.GeneratorMock} {
		gen, err := briefing.SelectGenerator(briefing.ServiceConfig{Generator: selection})
		require.NoError(t, err)
		require.Equal(t, "mock", gen.Name())
		require.Equal(t, "none", gen.Model(),
			"the default deployment must not claim a model")
	}
}

func TestSelectGeneratorOpenAICompatible(t *testing.T) {
	gen, err := briefing.SelectGenerator(briefing.ServiceConfig{
		Generator:     briefing.GeneratorOpenAI,
		OpenAIBaseURL: "http://ollama:11434/v1",
		Model:         "qwen2.5:1.5b",
	})
	require.NoError(t, err)
	require.Equal(t, "openai-compatible", gen.Name())
	require.Equal(t, "qwen2.5:1.5b", gen.Model())
	require.Equal(t, "v1", gen.PromptVersion())
}

// TestSelectGeneratorAnthropicNeedsAKey is the fail-closed rule: asking for a
// model without a credential must stop the service, not silently downgrade it
// to templates that would then be labelled as if a model were configured.
func TestSelectGeneratorAnthropicNeedsAKey(t *testing.T) {
	_, err := briefing.SelectGenerator(briefing.ServiceConfig{Generator: briefing.GeneratorAnthropic})
	require.ErrorContains(t, err, "ANTHROPIC_API_KEY is not set")

	gen, err := briefing.SelectGenerator(briefing.ServiceConfig{
		Generator: briefing.GeneratorAnthropic, AnthropicAPIKey: "test-key-not-a-credential",
	})
	require.NoError(t, err)
	require.Equal(t, "anthropic", gen.Name())
	require.Equal(t, "claude-opus-5", gen.Model())
}

func TestSelectGeneratorRejectsAnUnknownSelection(t *testing.T) {
	_, err := briefing.SelectGenerator(briefing.ServiceConfig{Generator: "gpt-please"})
	require.ErrorContains(t, err, "BRIEFING_GENERATOR")
}

func TestNewSweeperWiresTheChecker(t *testing.T) {
	store := newFakeStore()
	s := briefing.NewSweeper(store, mockGenerator(), factstest.Counties, "mock", time.Second,
		logging.New(io.Discard, "error"))
	require.Equal(t, factstest.Counties, s.Checker.Counties)
	require.False(t, s.Checker.AllowSystemNotice,
		"a model draft may never write the [mock] label itself")

	sum, err := s.Sweep(context.Background(), "kisumu")
	require.NoError(t, err)
	require.Equal(t, 2, sum.Served)
}

// TestRunFailsFastOnBadConfiguration: a misconfigured briefing service must
// refuse to start rather than run in a state its own output would misdescribe.
func TestRunFailsFastOnBadConfiguration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Setenv("DATABASE_URL", "postgres://nobody:nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	t.Setenv("BRIEFING_ADDR", "127.0.0.1:0")

	t.Setenv("BRIEFING_GENERATOR", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "")
	require.ErrorContains(t, briefing.Run(ctx), "ANTHROPIC_API_KEY is not set")

	t.Setenv("BRIEFING_GENERATOR", "not-a-generator")
	require.ErrorContains(t, briefing.Run(ctx), "BRIEFING_GENERATOR")

	t.Setenv("BRIEFING_GENERATOR", "mock")
	require.Error(t, briefing.Run(ctx), "an unreachable database must fail startup")
}
