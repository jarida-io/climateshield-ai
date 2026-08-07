// SPDX-License-Identifier: Apache-2.0

// Package chirps declares the CHIRPS rainfall source.
// TODO(Q1): implement satellite-derived rainfall ingestion from CHIRPS;
// interface-only until then per the walking-skeleton scope.
package chirps

import (
	"github.com/jarida-io/climateshield/internal/climate"
)

// New reports ErrNotImplemented: CHIRPS ingestion is Q1 scope.
func New() (climate.Source, error) {
	return nil, climate.ErrNotImplemented
}
