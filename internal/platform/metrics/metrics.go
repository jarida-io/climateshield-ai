// SPDX-License-Identifier: Apache-2.0

// Package metrics provides each service's Prometheus registry and HTTP
// instrumentation. Uptime is a contractual obligation, so every service
// exposes /metrics from day one.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns a service's private Prometheus registry.
type Metrics struct {
	registry     *prometheus.Registry
	httpDuration *prometheus.HistogramVec
}

// New creates a registry with Go runtime/process collectors and the shared
// HTTP duration histogram.
func New(service string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	dur := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   "climateshield",
		Subsystem:   "http",
		Name:        "request_duration_seconds",
		Help:        "HTTP request latency by path and status.",
		ConstLabels: prometheus.Labels{"service": service},
		Buckets:     prometheus.DefBuckets,
	}, []string{"path", "status"})
	reg.MustRegister(dur)
	return &Metrics{registry: reg, httpDuration: dur}
}

// Registry exposes the underlying registry for service-specific collectors.
func (m *Metrics) Registry() *prometheus.Registry { return m.registry }

// Handler serves the /metrics endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Middleware records request durations.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		m.httpDuration.
			WithLabelValues(r.URL.Path, strconv.Itoa(sw.status)).
			Observe(time.Since(start).Seconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
