// SPDX-License-Identifier: Apache-2.0

package briefing

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	anthropicgen "github.com/jarida-io/climateshield/internal/briefing/anthropic"
	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/briefing/mock"
	"github.com/jarida-io/climateshield/internal/briefing/openaicompat"
	"github.com/jarida-io/climateshield/internal/jobs"
	"github.com/jarida-io/climateshield/internal/platform/config"
	"github.com/jarida-io/climateshield/internal/platform/httpx"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/platform/metrics"
	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// Generator selections accepted by BRIEFING_GENERATOR.
const (
	// GeneratorMock is the default: a deterministic template, no model, no
	// network, no credentials.
	GeneratorMock = "mock"
	// GeneratorOpenAI is a locally hosted open-weights model behind an
	// OpenAI-compatible endpoint (the `ai` compose profile runs one).
	GeneratorOpenAI = "openai"
	// GeneratorAnthropic is the Claude API, and requires ANTHROPIC_API_KEY.
	GeneratorAnthropic = "anthropic"
)

// ServiceConfig configures cmd/briefing.
type ServiceConfig struct {
	config.Common
	config.DB
	Addr string `env:"BRIEFING_ADDR" envDefault:":8094"`

	// Generator selects who writes the prose: mock (default), openai or
	// anthropic. The default keeps `make up` offline and credential-free, and
	// every briefing it produces says on its first line that no language
	// model ran.
	Generator string `env:"BRIEFING_GENERATOR" envDefault:"mock"`
	// Model overrides the generator's default model identifier.
	Model string `env:"BRIEFING_MODEL" envDefault:""`
	// SweepInterval is how often the service checks whether any county's fact
	// sheet has changed. An unchanged sheet regenerates nothing.
	SweepInterval time.Duration `env:"BRIEFING_SWEEP_INTERVAL" envDefault:"15m"`
	// Timeout bounds one generation. A small model on a laptop CPU takes tens
	// of seconds; this is why generation is a background job.
	Timeout time.Duration `env:"BRIEFING_TIMEOUT" envDefault:"120s"`

	// OpenAI-compatible endpoint (Ollama, llama.cpp server). No credential is
	// required; the key exists only for gateways that demand one.
	OpenAIBaseURL string `env:"BRIEFING_OPENAI_BASE_URL" envDefault:"http://localhost:11434/v1"`
	OpenAIAPIKey  string `env:"BRIEFING_OPENAI_API_KEY" envDefault:""`

	// Claude API. The key is never committed and is empty in every checked-in
	// environment file; setting it is what opts a deployment in.
	AnthropicAPIKey  string `env:"ANTHROPIC_API_KEY" envDefault:""`
	AnthropicBaseURL string `env:"ANTHROPIC_BASE_URL" envDefault:""`

	// Channel mirrors the notifier's configuration so the fact sheet states
	// what the messaging path actually does rather than assuming.
	Channel string `env:"NOTIFY_CHANNEL" envDefault:"mock"`
}

// SelectGenerator builds the configured generator. An unknown selection, or a
// model generator without its credential, is a startup error: a deployment
// that asked for a model and silently got templates would be telling its
// operator something untrue.
func SelectGenerator(cfg ServiceConfig) (Generator, error) {
	switch cfg.Generator {
	case GeneratorMock, "":
		return mock.New(), nil
	case GeneratorOpenAI:
		return openaicompat.New(openaicompat.Config{
			BaseURL: cfg.OpenAIBaseURL,
			Model:   cfg.Model,
			APIKey:  cfg.OpenAIAPIKey,
			Timeout: cfg.Timeout,
		}), nil
	case GeneratorAnthropic:
		return anthropicgen.New(anthropicgen.Config{
			APIKey:  cfg.AnthropicAPIKey,
			Model:   cfg.Model,
			BaseURL: cfg.AnthropicBaseURL,
			Timeout: cfg.Timeout,
		})
	default:
		return nil, fmt.Errorf("briefing: BRIEFING_GENERATOR %q: want %q, %q or %q",
			cfg.Generator, GeneratorMock, GeneratorOpenAI, GeneratorAnthropic)
	}
}

// NewSweeper assembles the sweeper for a running service.
func NewSweeper(q Store, gen Generator, counties []string, channel string, timeout time.Duration, log *slog.Logger) *Sweeper {
	return &Sweeper{
		Store:   q,
		Gen:     gen,
		Checker: NewChecker(counties),
		Channel: channel,
		Timeout: timeout,
		Log:     log,
	}
}

// sweepWorker is the River worker for briefing_sweep jobs.
type sweepWorker struct {
	river.WorkerDefaults[jobs.BriefingSweepArgs]
	sweeper *Sweeper
	log     *slog.Logger
}

// Work implements river.Worker.
func (w *sweepWorker) Work(ctx context.Context, job *river.Job[jobs.BriefingSweepArgs]) error {
	sum, err := w.sweeper.Sweep(ctx, job.Args.AreaID)
	if err != nil {
		return err
	}
	w.log.Info("briefing sweep",
		"areas", sum.Areas, "cached", sum.Cached, "served", sum.Served,
		"rejected", sum.Rejected, "unavailable", sum.Unavailable,
		"generator", w.sweeper.Gen.Name(), "model", w.sweeper.Gen.Model())
	return nil
}

// Run starts the briefing service: periodic sweeps plus on-demand jobs.
func Run(ctx context.Context) error {
	cfg, err := config.Load[ServiceConfig]()
	if err != nil {
		return err
	}
	log := logging.New(os.Stdout, cfg.LogLevel)
	m := metrics.New("briefing")

	gen, err := SelectGenerator(cfg)
	if err != nil {
		return err
	}

	pool, err := store.Connect(ctx, cfg.URL)
	if err != nil {
		return err
	}
	defer pool.Close()
	q := db.New(pool)

	areas, err := facts.Areas(ctx, q)
	if err != nil {
		return err
	}
	counties := make([]string, 0, len(areas))
	for _, a := range areas {
		counties = append(counties, a.Name)
	}

	sweeper := NewSweeper(q, gen, counties, cfg.Channel, cfg.Timeout, log)

	workers := river.NewWorkers()
	river.AddWorker(workers, &sweepWorker{sweeper: sweeper, log: log})

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{jobs.QueueBriefing: {MaxWorkers: 1}},
		Workers: workers,
	})
	if err != nil {
		return err
	}
	if err := riverClient.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = riverClient.Stop(context.Background()) }()

	// This service owns its own schedule, exactly as the ledger does: River
	// elects one leader per database, so a shared periodic schedule would fire
	// only in whichever service happened to win leadership. See jobs.Schedule.
	go jobs.Schedule(ctx, log, jobs.BriefingSweepArgs{}.Kind(), cfg.SweepInterval, func(c context.Context) error {
		_, err := riverClient.Insert(c, jobs.BriefingSweepArgs{}, &river.InsertOpts{
			Queue:      jobs.QueueBriefing,
			UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: cfg.SweepInterval},
		})
		return err
	})

	log.Info("briefing started",
		"generator", gen.Name(), "model", gen.Model(), "prompt_version", gen.PromptVersion(),
		"sweep_interval", cfg.SweepInterval.String(), "languages", facts.Languages)
	return httpx.Serve(ctx, cfg.Addr, httpx.NewRouter(func(c context.Context) error {
		return pool.Ping(c)
	}, m.Handler()), log)
}
