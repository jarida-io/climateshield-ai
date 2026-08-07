// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/jarida-io/climateshield/internal/jobs"
	"github.com/jarida-io/climateshield/internal/platform/config"
	"github.com/jarida-io/climateshield/internal/platform/httpx"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/platform/metrics"
	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// ServiceConfig configures cmd/predictor.
type ServiceConfig struct {
	config.Common
	config.DB
	Addr      string `env:"PREDICTOR_ADDR" envDefault:":8091"`
	ModelPath string `env:"ONNX_MODEL_PATH" envDefault:""`
}

type enqueuer func(ctx context.Context, args river.JobArgs, queue string) error

// scoreArea scores one area's latest observation window and persists one
// risk_scores row per disease; every HIGH or MEDIUM score enqueues an
// alert_dispatch job. Returns the number of alerts enqueued.
func scoreArea(
	ctx context.Context,
	q *db.Queries,
	p Predictor,
	areaID string,
	enqueue enqueuer,
	log *slog.Logger,
) (int, error) {
	window, err := q.LatestObservationWindow(ctx, areaID)
	if err != nil {
		return 0, fmt.Errorf("predict: window for %s: %w", areaID, err)
	}
	if len(window) == 0 {
		log.Warn("no observations for area, skipping", "area", areaID)
		return 0, nil
	}
	precip := make([]float64, 0, len(window))
	tmax := make([]float64, 0, len(window))
	for _, row := range window {
		precip = append(precip, row.PrecipitationSumMm)
		tmax = append(tmax, row.TempMaxC)
	}
	feats, err := FeaturesFrom(precip, tmax)
	if err != nil {
		return 0, err
	}

	forecastDate := window[0].ForecastDate
	alerts := 0
	for _, pred := range p.Predict(feats) {
		id, err := q.UpsertRiskScore(ctx, db.UpsertRiskScoreParams{
			AreaID:           areaID,
			Disease:          string(pred.Disease),
			Level:            string(pred.Level),
			Driver:           pred.Driver,
			DriverValue:      pred.DriverValue,
			ForecastDate:     pgtype.Date{Time: forecastDate.Time, Valid: true},
			WindowDays:       int32(len(window)),
			Predictor:        p.Name(),
			PredictorVersion: p.Version(),
		})
		if err != nil {
			return alerts, fmt.Errorf("predict: persist %s/%s: %w", areaID, pred.Disease, err)
		}
		log.Info("risk scored",
			"area", areaID, "disease", pred.Disease, "level", pred.Level,
			"driver", pred.Driver, "driver_value", pred.DriverValue,
			"predictor", p.Name(), "version", p.Version())

		if pred.Level == High || pred.Level == Medium {
			err := enqueue(ctx, jobs.AlertDispatchArgs{
				RiskScoreID: id,
				AreaID:      areaID,
				Disease:     string(pred.Disease),
				Level:       string(pred.Level),
			}, jobs.QueueNotify)
			if err != nil {
				return alerts, fmt.Errorf("predict: enqueue alert: %w", err)
			}
			alerts++
		}
	}
	return alerts, nil
}

// predictWorker is the River worker for risk_predict jobs.
type predictWorker struct {
	river.WorkerDefaults[jobs.RiskPredictArgs]
	pool interface {
		db.DBTX
	}
	predictor Predictor
	log       *slog.Logger
}

// Work implements river.Worker.
func (w *predictWorker) Work(ctx context.Context, job *river.Job[jobs.RiskPredictArgs]) error {
	client := river.ClientFromContext[pgx.Tx](ctx)
	enqueue := func(ctx context.Context, args river.JobArgs, queue string) error {
		_, err := client.Insert(ctx, args, &river.InsertOpts{Queue: queue})
		return err
	}
	_, err := scoreArea(ctx, db.New(w.pool), w.predictor, job.Args.AreaID, enqueue, w.log)
	return err
}

// Run starts the predictor service.
func Run(ctx context.Context) error {
	cfg, err := config.Load[ServiceConfig]()
	if err != nil {
		return err
	}
	log := logging.New(os.Stdout, cfg.LogLevel)
	m := metrics.New("predictor")

	predictor, err := Select(cfg.ModelPath, log)
	if err != nil {
		return err
	}

	pool, err := store.Connect(ctx, cfg.URL)
	if err != nil {
		return err
	}
	defer pool.Close()

	workers := river.NewWorkers()
	river.AddWorker(workers, &predictWorker{pool: pool, predictor: predictor, log: log})

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{jobs.QueuePredict: {MaxWorkers: 2}},
		Workers: workers,
	})
	if err != nil {
		return err
	}
	if err := riverClient.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = riverClient.Stop(context.Background()) }()

	log.Info("predictor started")
	return httpx.Serve(ctx, cfg.Addr, httpx.NewRouter(func(c context.Context) error {
		return pool.Ping(c)
	}, m.Handler()), log)
}
