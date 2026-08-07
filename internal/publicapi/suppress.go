// SPDX-License-Identifier: Apache-2.0

// Package publicapi is the public read-only tier. Everything it serves is
// aggregate (county x disease x date); counts derived from people pass
// through k-anonymity suppression before leaving the process; reads never
// return 500 — a failing database serves the last good response, marked
// stale.
package publicapi

// K is the k-anonymity threshold for people-derived counts, a funding
// commitment: no public number may allow singling out fewer than K children.
const K = 10

// Suppress applies the k>=10 rule to one people-derived count:
//
//	0        -> shown (population-level absence, not individual-inferable)
//	1..K-1   -> suppressed (value withheld, flag set)
//	>=K      -> shown
//
// The returned pointer is nil exactly when suppressed is true.
func Suppress(n int64) (value *int64, suppressed bool) {
	if n > 0 && n < K {
		return nil, true
	}
	return &n, false
}
