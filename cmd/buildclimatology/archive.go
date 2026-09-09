// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/jarida-io/climateshield/internal/climate"
	"github.com/jarida-io/climateshield/internal/store/seed"
)

// archiveClient reads the Open-Meteo historical archive (ERA5 reanalysis).
// It is free and keyless, like the forecast client in
// internal/climate/openmeteo, and it is the ONLY reader of the archive in
// this repository — used only when a person runs this command. No test may
// touch it: the tests here drive it against httptest.
type archiveClient struct {
	baseURL string
	hc      *http.Client
}

func newArchiveClient(baseURL string, timeout time.Duration) *archiveClient {
	return &archiveClient{baseURL: baseURL, hc: &http.Client{Timeout: timeout}}
}

// DailyVariables are the archive variables requested, matching the three
// features the runtime predictor computes.
const DailyVariables = "precipitation_sum,temperature_2m_max,temperature_2m_min"

// daily fetches one county's daily record for the closed date range. Dates
// are YYYY-MM-DD in the Africa/Nairobi calendar, the same calendar the
// ingestor labels forecast days with.
func (c *archiveClient) daily(ctx context.Context, area seed.County, from, to string) ([]climate.Day, error) {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", area.Lat))
	q.Set("longitude", fmt.Sprintf("%.4f", area.Lon))
	q.Set("start_date", from)
	q.Set("end_date", to)
	q.Set("daily", DailyVariables)
	q.Set("timezone", "Africa/Nairobi")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/archive?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("buildclimatology: %s: %w", area.ID, err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("buildclimatology: %s: %w", area.ID, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("buildclimatology: %s: archive returned status %d", area.ID, resp.StatusCode)
	}

	// The archive answers in the same daily shape as the forecast API, so it
	// is parsed by the same code the ingestor uses.
	days, err := climate.ParseOpenMeteo(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("buildclimatology: %s: %w", area.ID, err)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Date.Before(days[j].Date) })
	if err := checkContiguous(days); err != nil {
		return nil, fmt.Errorf("buildclimatology: %s: %w", area.ID, err)
	}
	return days, nil
}

// checkContiguous refuses a record with a gap. A missing day would silently
// shorten a window and shift every later window's month, so the tool stops
// rather than produce a distribution nobody can account for.
func checkContiguous(days []climate.Day) error {
	if len(days) == 0 {
		return fmt.Errorf("archive returned no days")
	}
	for i := 1; i < len(days); i++ {
		want := days[i-1].Date.AddDate(0, 0, 1)
		if !days[i].Date.Equal(want) {
			return fmt.Errorf("gap in the daily record: %s is followed by %s",
				days[i-1].Date.Format("2006-01-02"), days[i].Date.Format("2006-01-02"))
		}
	}
	return nil
}
