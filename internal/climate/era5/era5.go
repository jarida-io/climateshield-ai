// SPDX-License-Identifier: Apache-2.0

// Package era5 declares the ERA5 reanalysis source.
// TODO(Q1): implement historical reanalysis ingestion (model training needs
// it); interface-only until then per the walking-skeleton scope.
package era5

import (
	"github.com/jarida-io/climateshield/internal/climate"
)

// New reports ErrNotImplemented: ERA5 ingestion is Q1 scope.
func New() (climate.Source, error) {
	return nil, climate.ErrNotImplemented
}
