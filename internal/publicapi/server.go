// SPDX-License-Identifier: Apache-2.0

package publicapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/gen/climateshield/v1/climateshieldv1connect"
	"github.com/jarida-io/climateshield/internal/platform/config"
	"github.com/jarida-io/climateshield/internal/platform/httpx"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/platform/metrics"
	"github.com/jarida-io/climateshield/internal/predict"
	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// ServiceConfig configures cmd/publicapi.
type ServiceConfig struct {
	config.Common
	config.DB
	Addr string `env:"PUBLICAPI_ADDR" envDefault:":8080"`
	// Mirrors of the other services' configuration, reported by the evidence
	// views so they describe the deployment rather than a guess.
	PredictorName  string `env:"PREDICTOR" envDefault:"rules"`
	Channel        string `env:"NOTIFY_CHANNEL" envDefault:"mock"`
	IngestInterval string `env:"INGEST_INTERVAL" envDefault:"6h"`
}

func predictorVersionFor(name string) string {
	if name == "climatology" {
		return predict.ClimatologyVersion
	}
	return predict.RulesVersion
}

// Server is the public read-only tier: REST (JSON/CSV/GeoJSON) and Connect,
// both fed by the same proto messages.
type Server struct {
	q     *db.Queries
	pool  *pgxpool.Pool // nil unless a pool was supplied; only for River's own tables
	cache *staleCache
	log   *slog.Logger

	// Deployment facts reported by the evidence views. They are read from the
	// running configuration rather than assumed, so a view cannot claim a
	// predictor or channel that is not the one in use.
	predictorName    string
	predictorVersion string
	channel          string
	ingestInterval   string
	climatology      *predict.Climatology
}

// NewServer builds the public server over a database handle.
func NewServer(dbtx db.DBTX, log *slog.Logger) *Server {
	s := &Server{
		q: db.New(dbtx), cache: newStaleCache(), log: log,
		predictorName: "rules", predictorVersion: predict.RulesVersion,
		channel: "mock", ingestInterval: "6h",
	}
	if pool, ok := dbtx.(*pgxpool.Pool); ok {
		s.pool = pool
	}
	if clim, err := predict.LoadClimatology(); err == nil {
		s.climatology = clim
	}
	return s
}

// WithDeployment records which predictor and channel are actually running, so
// the evidence views report the live configuration instead of a default.
func (s *Server) WithDeployment(predictorName, predictorVersion, channel, ingestInterval string) *Server {
	s.predictorName = predictorName
	s.predictorVersion = predictorVersion
	s.channel = channel
	s.ingestInterval = ingestInterval
	return s
}

// Router assembles /health, /metrics, the /v1 REST surface and the Connect
// service. healthy may be nil.
func (s *Server) Router(healthy httpx.HealthFunc, metricsHandler http.Handler) chi.Router {
	r := httpx.NewRouter(healthy, metricsHandler)
	r.Get("/v1/risk/current", func(w http.ResponseWriter, req *http.Request) {
		s.serveREST(w, req, "current", func(ctx context.Context) (proto.Message, error) {
			return s.buildCurrentRisk(ctx)
		}, &climateshieldv1.GetCurrentRiskResponse{})
	})
	r.Get("/v1/risk/history", func(w http.ResponseWriter, req *http.Request) {
		p := historyParams{
			Area:     req.URL.Query().Get("area"),
			Disease:  req.URL.Query().Get("disease"),
			FromDate: req.URL.Query().Get("from"),
			ToDate:   req.URL.Query().Get("to"),
		}
		if raw := req.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				http.Error(w, "bad limit", http.StatusBadRequest)
				return
			}
			p.Limit = int32(n)
		}
		s.serveREST(w, req, "history", func(ctx context.Context) (proto.Message, error) {
			return s.buildRiskHistory(ctx, p)
		}, &climateshieldv1.GetRiskHistoryResponse{})
	})
	r.Get("/v1/stats", func(w http.ResponseWriter, req *http.Request) {
		s.serveREST(w, req, "stats", func(ctx context.Context) (proto.Message, error) {
			return s.buildStats(ctx)
		}, &climateshieldv1.GetStatsResponse{})
	})

	// Evidence views. Each is a plain GET so a reviewer can curl it.
	r.Get("/v1/model", func(w http.ResponseWriter, req *http.Request) {
		s.serveREST(w, req, "model", func(ctx context.Context) (proto.Message, error) {
			return s.buildModelInfo(ctx)
		}, &climateshieldv1.GetModelInfoResponse{})
	})
	r.Get("/v1/climate/series", func(w http.ResponseWriter, req *http.Request) {
		area := req.URL.Query().Get("area")
		s.serveREST(w, req, "climate", func(ctx context.Context) (proto.Message, error) {
			return s.buildClimateSeries(ctx, area)
		}, &climateshieldv1.GetClimateSeriesResponse{})
	})
	r.Get("/v1/ledger/summary", func(w http.ResponseWriter, req *http.Request) {
		s.serveREST(w, req, "ledger", func(ctx context.Context) (proto.Message, error) {
			return s.buildLedgerSummary(ctx)
		}, &climateshieldv1.GetLedgerSummaryResponse{})
	})
	r.Get("/v1/alerts/summary", func(w http.ResponseWriter, req *http.Request) {
		s.serveREST(w, req, "alerts", func(ctx context.Context) (proto.Message, error) {
			return s.buildAlertSummary(ctx)
		}, &climateshieldv1.GetAlertSummaryResponse{})
	})
	r.Get("/v1/pipeline", func(w http.ResponseWriter, req *http.Request) {
		s.serveREST(w, req, "pipeline", func(ctx context.Context) (proto.Message, error) {
			return s.buildPipelineStatus(ctx)
		}, &climateshieldv1.GetPipelineStatusResponse{})
	})

	path, handler := climateshieldv1connect.NewPublicServiceHandler(s)
	r.Mount(path, handler)
	return r
}

// GetModelInfo implements climateshieldv1connect.PublicServiceHandler.
func (s *Server) GetModelInfo(
	ctx context.Context, _ *connect.Request[climateshieldv1.GetModelInfoRequest],
) (*connect.Response[climateshieldv1.GetModelInfoResponse], error) {
	msg, err := s.buildModelInfo(ctx)
	if err != nil {
		return staleConnect[climateshieldv1.GetModelInfoResponse](s, "connect:model")
	}
	s.cache.storeProto("connect:model", msg)
	return connect.NewResponse(msg), nil
}

// GetClimateSeries implements climateshieldv1connect.PublicServiceHandler.
func (s *Server) GetClimateSeries(
	ctx context.Context, req *connect.Request[climateshieldv1.GetClimateSeriesRequest],
) (*connect.Response[climateshieldv1.GetClimateSeriesResponse], error) {
	msg, err := s.buildClimateSeries(ctx, req.Msg.GetArea())
	if err != nil {
		return staleConnect[climateshieldv1.GetClimateSeriesResponse](s, "connect:climate")
	}
	s.cache.storeProto("connect:climate", msg)
	return connect.NewResponse(msg), nil
}

// GetLedgerSummary implements climateshieldv1connect.PublicServiceHandler.
func (s *Server) GetLedgerSummary(
	ctx context.Context, _ *connect.Request[climateshieldv1.GetLedgerSummaryRequest],
) (*connect.Response[climateshieldv1.GetLedgerSummaryResponse], error) {
	msg, err := s.buildLedgerSummary(ctx)
	if err != nil {
		return staleConnect[climateshieldv1.GetLedgerSummaryResponse](s, "connect:ledger")
	}
	s.cache.storeProto("connect:ledger", msg)
	return connect.NewResponse(msg), nil
}

// GetAlertSummary implements climateshieldv1connect.PublicServiceHandler.
func (s *Server) GetAlertSummary(
	ctx context.Context, _ *connect.Request[climateshieldv1.GetAlertSummaryRequest],
) (*connect.Response[climateshieldv1.GetAlertSummaryResponse], error) {
	msg, err := s.buildAlertSummary(ctx)
	if err != nil {
		return staleConnect[climateshieldv1.GetAlertSummaryResponse](s, "connect:alerts")
	}
	s.cache.storeProto("connect:alerts", msg)
	return connect.NewResponse(msg), nil
}

// GetPipelineStatus implements climateshieldv1connect.PublicServiceHandler.
func (s *Server) GetPipelineStatus(
	ctx context.Context, _ *connect.Request[climateshieldv1.GetPipelineStatusRequest],
) (*connect.Response[climateshieldv1.GetPipelineStatusResponse], error) {
	msg, err := s.buildPipelineStatus(ctx)
	if err != nil {
		return staleConnect[climateshieldv1.GetPipelineStatusResponse](s, "connect:pipeline")
	}
	s.cache.storeProto("connect:pipeline", msg)
	return connect.NewResponse(msg), nil
}

// serveREST is the never-500 read path: build fresh -> cache -> serve; on
// backend failure serve the cached response (or an empty valid payload)
// with X-Data-Stale: true. Only client errors return 4xx.
func (s *Server) serveREST(
	w http.ResponseWriter,
	req *http.Request,
	endpoint string,
	build func(ctx context.Context) (proto.Message, error),
	empty proto.Message,
) {
	format := req.URL.Query().Get("format")
	key := endpoint + "|" + format + "|" + req.URL.RawQuery

	msg, err := build(req.Context())
	if err != nil {
		var bad errBadRequest
		if errors.As(err, &bad) {
			http.Error(w, bad.Error(), http.StatusBadRequest)
			return
		}
		s.log.Warn("backend unavailable, serving stale", "endpoint", endpoint, "error", err.Error())
		s.serveStale(w, key, format, empty)
		return
	}

	body, contentType, err := encode(msg, format)
	if err != nil {
		var bad errBadRequest
		if errors.As(err, &bad) {
			http.Error(w, bad.Error(), http.StatusBadRequest)
			return
		}
		s.log.Warn("encode failed, serving stale", "endpoint", endpoint, "error", err.Error())
		s.serveStale(w, key, format, empty)
		return
	}
	s.cache.storeBody(key, body, contentType)
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(body)
}

func (s *Server) serveStale(w http.ResponseWriter, key, format string, empty proto.Message) {
	w.Header().Set("X-Data-Stale", "true")
	if cached, ok := s.cache.getBody(key); ok {
		w.Header().Set("Content-Type", cached.contentType)
		_, _ = w.Write(cached.body)
		return
	}
	// Cold start with a dead backend: an empty-but-valid payload, still 200.
	body, contentType, err := encode(empty, format)
	if err != nil {
		body, contentType = []byte("{}"), "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(body)
}

// --- Connect handlers (same builders, same stale semantics) ---

// GetCurrentRisk implements climateshieldv1connect.PublicServiceHandler.
func (s *Server) GetCurrentRisk(
	ctx context.Context,
	_ *connect.Request[climateshieldv1.GetCurrentRiskRequest],
) (*connect.Response[climateshieldv1.GetCurrentRiskResponse], error) {
	msg, err := s.buildCurrentRisk(ctx)
	if err != nil {
		return staleConnect[climateshieldv1.GetCurrentRiskResponse](s, "connect:current")
	}
	s.cache.storeProto("connect:current", msg)
	return connect.NewResponse(msg), nil
}

// GetRiskHistory implements climateshieldv1connect.PublicServiceHandler.
func (s *Server) GetRiskHistory(
	ctx context.Context,
	req *connect.Request[climateshieldv1.GetRiskHistoryRequest],
) (*connect.Response[climateshieldv1.GetRiskHistoryResponse], error) {
	p := historyParams{
		Area:     req.Msg.GetArea(),
		FromDate: req.Msg.GetFromDate(),
		ToDate:   req.Msg.GetToDate(),
		Limit:    req.Msg.GetLimit(),
	}
	if req.Msg.GetDisease() != climateshieldv1.Disease_DISEASE_UNSPECIFIED {
		p.Disease = shortDisease(req.Msg.GetDisease())
	}
	msg, err := s.buildRiskHistory(ctx, p)
	if err != nil {
		var bad errBadRequest
		if errors.As(err, &bad) {
			return nil, connect.NewError(connect.CodeInvalidArgument, bad)
		}
		return staleConnect[climateshieldv1.GetRiskHistoryResponse](s, "connect:history")
	}
	s.cache.storeProto("connect:history", msg)
	return connect.NewResponse(msg), nil
}

// GetStats implements climateshieldv1connect.PublicServiceHandler.
func (s *Server) GetStats(
	ctx context.Context,
	_ *connect.Request[climateshieldv1.GetStatsRequest],
) (*connect.Response[climateshieldv1.GetStatsResponse], error) {
	msg, err := s.buildStats(ctx)
	if err != nil {
		return staleConnect[climateshieldv1.GetStatsResponse](s, "connect:stats")
	}
	s.cache.storeProto("connect:stats", msg)
	return connect.NewResponse(msg), nil
}

// staleConnect serves the last good proto (or an empty one) with the stale
// header — the Connect twin of serveStale.
func staleConnect[T any](s *Server, key string) (*connect.Response[T], error) {
	msg := new(T)
	if cached, ok := s.cache.getProto(key); ok {
		if typed, ok := any(cached).(*T); ok {
			msg = typed
		}
	}
	resp := connect.NewResponse(msg)
	resp.Header().Set("X-Data-Stale", "true")
	return resp, nil
}

// Run starts the public API service.
func Run(ctx context.Context) error {
	cfg, err := config.Load[ServiceConfig]()
	if err != nil {
		return err
	}
	log := logging.New(os.Stdout, cfg.LogLevel)
	m := metrics.New("publicapi")

	pool, err := store.Connect(ctx, cfg.URL)
	if err != nil {
		return err
	}
	defer pool.Close()

	srv := NewServer(pool, log).WithDeployment(
		cfg.PredictorName, predictorVersionFor(cfg.PredictorName), cfg.Channel, cfg.IngestInterval)
	router := srv.Router(healthFunc(pool), m.Handler())

	log.Info("public api started", "addr", cfg.Addr)
	return httpx.Serve(ctx, cfg.Addr, m.Middleware(router), log)
}

func healthFunc(pool *pgxpool.Pool) httpx.HealthFunc {
	return func(ctx context.Context) error { return pool.Ping(ctx) }
}
