// SPDX-License-Identifier: Apache-2.0

// Package climate defines the climate-data contract: a Source yields a daily
// forecast window per area, and ingest.go persists it idempotently. The
// Open-Meteo implementation is live; the fixture implementation replays
// committed golden JSON so tests and the demo never touch the network.
package climate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// ErrNotImplemented marks declared-but-unbuilt sources (CHIRPS, ERA5).
var ErrNotImplemented = errors.New("climate: not implemented")

// Area is the minimal area shape a source needs.
type Area struct {
	ID  string
	Lat float64
	Lon float64
}

// Day is one forecast day for one area.
type Day struct {
	Date               time.Time // calendar date in Africa/Nairobi
	PrecipitationSumMM float64
	TempMaxC           float64
	TempMinC           float64
	HumidityMaxPct     *float64
}

// Forecast is a daily window plus provenance.
type Forecast struct {
	AreaID   string
	Source   string
	IssuedAt time.Time
	Days     []Day
}

// Source fetches a daily forecast window for an area.
type Source interface {
	// FetchDaily returns up to days forecast days for the area.
	FetchDaily(ctx context.Context, area Area, days int) (Forecast, error)
}

// openMeteoPayload is the wire shape of an Open-Meteo /v1/forecast response
// (daily variables only). The fixture files use the identical shape.
type openMeteoPayload struct {
	Daily struct {
		Time             []string  `json:"time"`
		PrecipitationSum []float64 `json:"precipitation_sum"`
		TempMax          []float64 `json:"temperature_2m_max"`
		TempMin          []float64 `json:"temperature_2m_min"`
		HumidityMax      []float64 `json:"relative_humidity_2m_max"`
	} `json:"daily"`
}

// ParseOpenMeteo decodes an Open-Meteo daily forecast response into Days.
func ParseOpenMeteo(r io.Reader) ([]Day, error) {
	var p openMeteoPayload
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return nil, fmt.Errorf("climate: decode open-meteo payload: %w", err)
	}
	n := len(p.Daily.Time)
	if n == 0 {
		return nil, errors.New("climate: open-meteo payload has no daily data")
	}
	if len(p.Daily.PrecipitationSum) != n || len(p.Daily.TempMax) != n || len(p.Daily.TempMin) != n {
		return nil, errors.New("climate: open-meteo daily arrays have mismatched lengths")
	}
	days := make([]Day, 0, n)
	for i := 0; i < n; i++ {
		date, err := time.Parse("2006-01-02", p.Daily.Time[i])
		if err != nil {
			return nil, fmt.Errorf("climate: bad date %q: %w", p.Daily.Time[i], err)
		}
		d := Day{
			Date:               date,
			PrecipitationSumMM: p.Daily.PrecipitationSum[i],
			TempMaxC:           p.Daily.TempMax[i],
			TempMinC:           p.Daily.TempMin[i],
		}
		if len(p.Daily.HumidityMax) == n {
			h := p.Daily.HumidityMax[i]
			d.HumidityMaxPct = &h
		}
		days = append(days, d)
	}
	return days, nil
}
