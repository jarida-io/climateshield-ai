// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlerServesMetrics(t *testing.T) {
	m := New("test")
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "go_goroutines")
}

func TestMiddlewareRecordsDuration(t *testing.T) {
	m := New("test")
	h := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/risk/current", nil))
	require.Equal(t, http.StatusTeapot, rec.Code)

	metricsRec := httptest.NewRecorder()
	m.Handler().ServeHTTP(metricsRec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metricsRec.Body.String()
	require.Contains(t, body, "climateshield_http_request_duration_seconds")
	require.Contains(t, body, `status="418"`)
}
