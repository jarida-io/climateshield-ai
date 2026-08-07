// SPDX-License-Identifier: Apache-2.0

// Package openmeteo implements climate.Source against the Open-Meteo
// forecast API — free, no API key, no credentials (funding constraint).
package openmeteo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jarida-io/climateshield/internal/climate"
	"github.com/jarida-io/climateshield/internal/platform/clock"
)

// SourceName tags observations ingested from Open-Meteo.
const SourceName = "openmeteo"

// Client is a climate.Source backed by api.open-meteo.com (or a test server).
type Client struct {
	baseURL string
	hc      *http.Client
	clk     clock.Clock
}

// New builds a client. baseURL is the server origin, e.g.
// "https://api.open-meteo.com".
func New(baseURL string, clk clock.Clock) *Client {
	return &Client{
		baseURL: baseURL,
		hc:      &http.Client{Timeout: 30 * time.Second},
		clk:     clk,
	}
}

// FetchDaily implements climate.Source.
func (c *Client) FetchDaily(ctx context.Context, area climate.Area, days int) (climate.Forecast, error) {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", area.Lat))
	q.Set("longitude", fmt.Sprintf("%.4f", area.Lon))
	q.Set("daily", "precipitation_sum,temperature_2m_max,temperature_2m_min,relative_humidity_2m_max")
	q.Set("timezone", "Africa/Nairobi")
	q.Set("forecast_days", fmt.Sprintf("%d", days))

	reqURL := c.baseURL + "/v1/forecast?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return climate.Forecast{}, fmt.Errorf("openmeteo: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return climate.Forecast{}, fmt.Errorf("openmeteo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return climate.Forecast{}, fmt.Errorf("openmeteo: unexpected status %d", resp.StatusCode)
	}

	fdays, err := climate.ParseOpenMeteo(resp.Body)
	if err != nil {
		return climate.Forecast{}, err
	}
	return climate.Forecast{
		AreaID:   area.ID,
		Source:   SourceName,
		IssuedAt: c.clk.Now().UTC().Truncate(time.Second),
		Days:     fdays,
	}, nil
}
