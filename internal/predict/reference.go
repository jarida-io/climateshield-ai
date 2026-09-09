// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// This file exposes the provenance of the reference climatology artifact.
// The point is recomputation, not reassurance: a reviewer who has the file
// can hash it themselves, rebuild it with the generator named inside it, and
// compare. Everything here is measured from the embedded bytes.

// ClimatologyDigest returns the SHA-256, in lowercase hex, of the reference
// climatology bytes embedded in this binary. It is the same digest as
// `shasum -a 256` over the committed file, because go:embed copies the file
// verbatim.
func ClimatologyDigest() (string, error) {
	return climatologyDigestOf(DefaultClimatologyFile)
}

func climatologyDigestOf(name string) (string, error) {
	raw, err := climatologyFS.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("predict: climatology digest: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// TotalSamples is how many reference windows the whole artifact was measured
// from, summed over every county and calendar month.
func (c *Climatology) TotalSamples() int {
	total := 0
	for _, county := range c.Counties {
		for _, month := range county.Months {
			total += month.Samples
		}
	}
	return total
}

// QuantileSteps is how many percentile points each stored distribution has.
func (c *Climatology) QuantileSteps() int { return len(c.QuantileStepsPct) }

// LadderKey maps a persisted driver label (the value stored in
// risk_scores.driver) to the reference climatology's quantile ladder key for
// the same quantity. The second result is false for drivers the reference
// record holds no distribution for.
func LadderKey(driver string) (string, bool) {
	key, ok := annotationLadder[driver]
	return key, ok
}

// CountyIDs lists the counties in the artifact, sorted, so callers render a
// stable order.
func (c *Climatology) CountyIDs() []string {
	out := make([]string, 0, len(c.Counties))
	for id := range c.Counties {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
