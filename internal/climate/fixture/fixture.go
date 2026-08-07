// SPDX-License-Identifier: Apache-2.0

// Package fixture implements climate.Source from committed golden JSON files
// (testdata/golden/openmeteo_<area>.json). It is the CI and demo default:
// deterministic, offline, and shaped exactly like a real Open-Meteo response.
package fixture

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jarida-io/climateshield/internal/climate"
)

// SourceName tags observations ingested from fixtures. The name is loud on
// purpose: output derived from it must never read as live weather.
const SourceName = "fixture"

// Source reads golden forecast files from a directory.
type Source struct {
	dir string
}

// New builds a fixture source rooted at dir.
func New(dir string) *Source { return &Source{dir: dir} }

// FetchDaily implements climate.Source. IssuedAt is derived from the first
// forecast date, so repeated ingests of the same fixture upsert the same rows
// (deterministic demo, idempotent by construction).
func (s *Source) FetchDaily(_ context.Context, area climate.Area, days int) (climate.Forecast, error) {
	path := filepath.Join(s.dir, fmt.Sprintf("openmeteo_%s.json", area.ID))
	f, err := os.Open(path)
	if err != nil {
		return climate.Forecast{}, fmt.Errorf("fixture: %w", err)
	}
	defer func() { _ = f.Close() }()

	fdays, err := climate.ParseOpenMeteo(f)
	if err != nil {
		return climate.Forecast{}, err
	}
	if days > 0 && days < len(fdays) {
		fdays = fdays[:days]
	}
	return climate.Forecast{
		AreaID:   area.ID,
		Source:   SourceName,
		IssuedAt: fdays[0].Date.Add(6 * time.Hour).UTC(),
		Days:     fdays,
	}, nil
}
