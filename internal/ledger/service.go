// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/jarida-io/climateshield/internal/jobs"
	"github.com/jarida-io/climateshield/internal/ledger/anchor"
	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm"
	"github.com/jarida-io/climateshield/internal/platform/config"
	"github.com/jarida-io/climateshield/internal/platform/httpx"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/platform/metrics"
	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// Anchor modes accepted by ANCHOR_MODE.
const (
	AnchorModeLocal = "local"
	AnchorModeEVM   = "evm"
)

// ServiceConfig configures cmd/ledger.
type ServiceConfig struct {
	config.Common
	config.DB
	Addr          string        `env:"LEDGER_ADDR" envDefault:":8093"`
	SweepInterval time.Duration `env:"LEDGER_SWEEP_INTERVAL" envDefault:"1h"`

	// AnchorMode is "local" (roots recorded in the anchors table only) or
	// "evm" (roots ALSO written to the RootAnchor contract on the chain at
	// AnchorRPCURL and read back). The code default is local; the compose
	// stack and .env.example set evm and start a development chain.
	AnchorMode string `env:"ANCHOR_MODE" envDefault:"local"`
	// AnchorRPCURL is the chain's JSON-RPC endpoint (evm mode only).
	AnchorRPCURL string `env:"ANCHOR_RPC_URL" envDefault:"http://localhost:8545"`
	// AnchorFrom is the sending account; empty uses the node's first
	// unlocked account, so no key is ever configured.
	AnchorFrom string `env:"ANCHOR_FROM" envDefault:""`
	// AnchorContractAddress pins an existing RootAnchor; empty deploys once
	// per chain and remembers the address in anchor_contracts.
	AnchorContractAddress string `env:"ANCHOR_CONTRACT_ADDRESS" envDefault:""`
	// AnchorConfirmTimeout bounds the wait for a transaction receipt.
	AnchorConfirmTimeout time.Duration `env:"ANCHOR_CONFIRM_TIMEOUT" envDefault:"30s"`
}

// sweepWorker is the River worker for ledger_daily_root jobs.
type sweepWorker struct {
	river.WorkerDefaults[jobs.LedgerDailyRootArgs]
	pool interface {
		db.DBTX
	}
	anchors anchor.Multi
	log     *slog.Logger
}

// Work implements river.Worker.
func (w *sweepWorker) Work(ctx context.Context, _ *river.Job[jobs.LedgerDailyRootArgs]) error {
	q := db.New(w.pool)
	_, _, err := Sweep(ctx, q, w.anchors, w.log)
	return err
}

// buildAnchors assembles the anchors for the configured mode. In evm mode
// the chain is contacted and the contract deployed (once) or verified right
// here, so a configured chain that is not there fails startup loudly instead
// of degrading to local-only in silence — the same rule as an ONNX model
// path that cannot load.
func buildAnchors(ctx context.Context, cfg ServiceConfig, store evm.ContractStore, log *slog.Logger) (anchor.Multi, error) {
	local := anchor.NewLocal()
	switch cfg.AnchorMode {
	case AnchorModeLocal:
		return anchor.Multi{local}, nil
	case AnchorModeEVM:
		chain := evm.New(evm.Config{
			RPCURL:          cfg.AnchorRPCURL,
			From:            cfg.AnchorFrom,
			ContractAddress: cfg.AnchorContractAddress,
			ConfirmTimeout:  cfg.AnchorConfirmTimeout,
		}, store, log)
		dep, err := chain.Ensure(ctx)
		if err != nil {
			return nil, fmt.Errorf("ledger: ANCHOR_MODE=evm at %s: %w", cfg.AnchorRPCURL, err)
		}
		log.Info("chain anchor ready", "chain_id", dep.ChainID, "chain_label", dep.ChainLabel,
			"contract", dep.ContractAddress, "from", dep.From, "deployed_now", dep.Deployed)
		return anchor.Multi{local, chain}, nil
	default:
		return nil, fmt.Errorf("ledger: ANCHOR_MODE %q: want %q or %q", cfg.AnchorMode, AnchorModeLocal, AnchorModeEVM)
	}
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

	anchors, err := buildAnchors(ctx, cfg, db.New(pool), log)
	if err != nil {
		return err
	}

	workers := river.NewWorkers()
	river.AddWorker(workers, &sweepWorker{pool: pool, anchors: anchors, log: log})

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

	log.Info("ledger started", "sweep_interval", cfg.SweepInterval.String(), "anchors", anchors.Types())
	return httpx.Serve(ctx, cfg.Addr, httpx.NewRouter(func(c context.Context) error {
		return pool.Ping(c)
	}, m.Handler()), log)
}
