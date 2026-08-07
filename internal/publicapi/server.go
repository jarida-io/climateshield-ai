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
	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// ServiceConfig configures cmd/publicapi.
type ServiceConfig struct {
	config.Common
	config.DB
	Addr string `env:"PUBLICAPI_ADDR" envDefault:":8080"`
}

// Server is the public read-only tier: REST (JSON/CSV/GeoJSON) and Connect,
// both fed by the same proto messages.
type Server struct {
	q     *db.Queries
	cache *staleCache
	log   *slog.Logger
}

// NewServer builds the public server over a database handle.
func NewServer(dbtx db.DBTX, log *slog.Logger) *Server {
	return &Server{q: db.New(dbtx), cache: newStaleCache(), log: log}
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

	path, handler := climateshieldv1connect.NewPublicServiceHandler(s)
	r.Mount(path, handler)
	return r
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

	srv := NewServer(pool, log)
	router := srv.Router(healthFunc(pool), m.Handler())

	log.Info("public api started", "addr", cfg.Addr)
	return httpx.Serve(ctx, cfg.Addr, m.Middleware(router), log)
}

func healthFunc(pool *pgxpool.Pool) httpx.HealthFunc {
	return func(ctx context.Context) error { return pool.Ping(ctx) }
}
