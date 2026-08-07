// SPDX-License-Identifier: Apache-2.0

// Package httpx provides the shared HTTP server shape: a chi router with
// /health and /metrics, sane timeouts, and graceful shutdown.
package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// HealthFunc reports readiness; return an error to fail /health.
type HealthFunc func(ctx context.Context) error

// NewRouter builds the base router every service shares. metricsHandler
// serves /metrics; healthy (optional) gates /health.
func NewRouter(healthy HealthFunc, metricsHandler http.Handler) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		if healthy != nil {
			if err := healthy(req.Context()); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"status":"unhealthy"}`))
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	if metricsHandler != nil {
		r.Handle("/metrics", metricsHandler)
	}
	return r
}

// Serve runs an HTTP server until ctx is canceled, then shuts down
// gracefully. Timeouts are deliberately conservative for a public tier.
func Serve(ctx context.Context, addr string, handler http.Handler, log *slog.Logger) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		log.Info("http server stopped", "addr", addr)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
