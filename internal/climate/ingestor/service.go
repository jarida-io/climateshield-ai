// SPDX-License-Identifier: Apache-2.0

package ingestor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/jarida-io/climateshield/internal/climate"
	"github.com/jarida-io/climateshield/internal/climate/fixture"
	"github.com/jarida-io/climateshield/internal/climate/openmeteo"
	"github.com/jarida-io/climateshield/internal/jobs"
	"github.com/jarida-io/climateshield/internal/platform/clock"
	"github.com/jarida-io/climateshield/internal/platform/config"
	"github.com/jarida-io/climateshield/internal/platform/httpx"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/platform/metrics"
	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// ServiceConfig configures cmd/ingestor.
type ServiceConfig struct {
	config.Common
	config.DB
	Addr             string        `env:"INGESTOR_ADDR" envDefault:":8090"`
	Source           string        `env:"CLIMATE_SOURCE" envDefault:"openmeteo"`
	OpenMeteoBaseURL string        `env:"OPENMETEO_BASE_URL" envDefault:"https://api.open-meteo.com"`
	FixtureDir       string        `env:"CLIMATE_FIXTURE_DIR" envDefault:"testdata/golden"`
	ForecastDays     int           `env:"FORECAST_DAYS" envDefault:"14"`
	IngestInterval   time.Duration `env:"INGEST_INTERVAL" envDefault:"6h"`
}

// enqueuer abstracts River insertion so worker logic is unit-testable.
type enqueuer func(ctx context.Context, args river.JobArgs, queue string) error

// buildSource resolves a source name to an implementation.
func buildSource(name string, cfg ServiceConfig) (climate.Source, error) {
	switch name {
	case openmeteo.SourceName:
		return openmeteo.New(cfg.OpenMeteoBaseURL, clock.Real{}), nil
	case fixture.SourceName:
		return fixture.New(cfg.FixtureDir), nil
	default:
		return nil, fmt.Errorf("climate: unknown source %q", name)
	}
}

// runIngestSweep fetches and upserts the forecast for every area through the
// given source, then enqueues one risk_predict job per area.
func runIngestSweep(
	ctx context.Context,
	q *db.Queries,
	src climate.Source,
	days int,
	enqueue enqueuer,
	log *slog.Logger,
) error {
	areas, err := q.ListAreas(ctx)
	if err != nil {
		return fmt.Errorf("climate: list areas: %w", err)
	}
	if len(areas) == 0 {
		return fmt.Errorf("climate: no areas configured (run cmd/migrate)")
	}
	for _, a := range areas {
		fc, err := src.FetchDaily(ctx, climate.Area{ID: a.ID, Lat: a.Latitude, Lon: a.Longitude}, days)
		if err != nil {
			return fmt.Errorf("climate: fetch %s: %w", a.ID, err)
		}
		n, err := climate.UpsertForecast(ctx, q, fc)
		if err != nil {
			return err
		}
		log.Info("forecast ingested",
			"area", a.ID, "source", fc.Source, "days", n, "issued_at", fc.IssuedAt)

		if err := enqueue(ctx, jobs.RiskPredictArgs{AreaID: a.ID}, jobs.QueuePredict); err != nil {
			return fmt.Errorf("climate: enqueue predict for %s: %w", a.ID, err)
		}
	}
	return nil
}

// ingestWorker is the River worker for climate_ingest jobs.
type ingestWorker struct {
	river.WorkerDefaults[jobs.ClimateIngestArgs]
	cfg  ServiceConfig
	pool interface {
		db.DBTX
	}
	log *slog.Logger
}

// Work implements river.Worker.
func (w *ingestWorker) Work(ctx context.Context, job *river.Job[jobs.ClimateIngestArgs]) error {
	name := w.cfg.Source
	if job.Args.Source != "" {
		name = job.Args.Source
	}
	src, err := buildSource(name, w.cfg)
	if err != nil {
		return err
	}
	client := river.ClientFromContext[pgx.Tx](ctx)
	enqueue := func(ctx context.Context, args river.JobArgs, queue string) error {
		_, err := client.Insert(ctx, args, &river.InsertOpts{Queue: queue})
		return err
	}
	return runIngestSweep(ctx, db.New(w.pool), src, w.cfg.ForecastDays, enqueue, w.log)
}

// Run starts the ingestor service: a River client working the ingest queue
// with a periodic sweep, plus /health and /metrics.
func Run(ctx context.Context) error {
	cfg, err := config.Load[ServiceConfig]()
	if err != nil {
		return err
	}
	log := logging.New(os.Stdout, cfg.LogLevel)
	m := metrics.New("ingestor")

	pool, err := store.Connect(ctx, cfg.URL)
	if err != nil {
		return err
	}
	defer pool.Close()

	workers := river.NewWorkers()
	river.AddWorker(workers, &ingestWorker{cfg: cfg, pool: pool, log: log})

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{jobs.QueueIngest: {MaxWorkers: 1}},
		Workers: workers,
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(cfg.IngestInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return jobs.ClimateIngestArgs{}, &river.InsertOpts{Queue: jobs.QueueIngest}
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
		},
	})
	if err != nil {
		return err
	}
	if err := riverClient.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = riverClient.Stop(context.Background()) }()

	log.Info("ingestor started", "source", cfg.Source, "interval", cfg.IngestInterval.String())
	return httpx.Serve(ctx, cfg.Addr, httpx.NewRouter(func(c context.Context) error {
		return pool.Ping(c)
	}, m.Handler()), log)
}
