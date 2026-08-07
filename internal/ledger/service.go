// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/jarida-io/climateshield/internal/jobs"
	"github.com/jarida-io/climateshield/internal/ledger/anchor"
	"github.com/jarida-io/climateshield/internal/platform/config"
	"github.com/jarida-io/climateshield/internal/platform/httpx"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/platform/metrics"
	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// ServiceConfig configures cmd/ledger.
type ServiceConfig struct {
	config.Common
	config.DB
	Addr          string        `env:"LEDGER_ADDR" envDefault:":8093"`
	SweepInterval time.Duration `env:"LEDGER_SWEEP_INTERVAL" envDefault:"1h"`
}

// sweepWorker is the River worker for ledger_daily_root jobs.
type sweepWorker struct {
	river.WorkerDefaults[jobs.LedgerDailyRootArgs]
	pool interface {
		db.DBTX
	}
	log *slog.Logger
}

// Work implements river.Worker.
func (w *sweepWorker) Work(ctx context.Context, _ *river.Job[jobs.LedgerDailyRootArgs]) error {
	q := db.New(w.pool)
	_, _, err := Sweep(ctx, q, anchor.NewLocal(w.pool), w.log)
	return err
}

// Run starts the ledger service: periodic sweeps plus on-demand jobs.
func Run(ctx context.Context) error {
	cfg, err := config.Load[ServiceConfig]()
	if err != nil {
		return err
	}
	log := logging.New(os.Stdout, cfg.LogLevel)
	m := metrics.New("ledger")

	pool, err := store.Connect(ctx, cfg.URL)
	if err != nil {
		return err
	}
	defer pool.Close()

	workers := river.NewWorkers()
	river.AddWorker(workers, &sweepWorker{pool: pool, log: log})

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{jobs.QueueLedger: {MaxWorkers: 1}},
		Workers: workers,
	})
	if err != nil {
		return err
	}
	if err := riverClient.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = riverClient.Stop(context.Background()) }()

	// Own schedule rather than a River periodic job — see jobs.Schedule for
	// why leader-elected scheduling does not work across services.
	go jobs.Schedule(ctx, log, jobs.LedgerDailyRootArgs{}.Kind(), cfg.SweepInterval, func(c context.Context) error {
		_, err := riverClient.Insert(c, jobs.LedgerDailyRootArgs{}, &river.InsertOpts{
			Queue:      jobs.QueueLedger,
			UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: cfg.SweepInterval},
		})
		return err
	})

	log.Info("ledger started", "sweep_interval", cfg.SweepInterval.String())
	return httpx.Serve(ctx, cfg.Addr, httpx.NewRouter(func(c context.Context) error {
		return pool.Ping(c)
	}, m.Handler()), log)
}
