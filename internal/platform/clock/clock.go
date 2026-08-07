// SPDX-License-Identifier: Apache-2.0

// Package clock provides an injectable time source and the East Africa Time
// zone. Kenya does not observe DST, so a fixed UTC+3 zone is correct and
// avoids a tzdata dependency in minimal containers.
package clock

import "time"

// EAT is East Africa Time (UTC+3, no DST).
var EAT = time.FixedZone("EAT", 3*60*60)

// Clock is an injectable time source so quiet-hours logic is testable.
type Clock interface {
	Now() time.Time
}

// Real is the wall clock.
type Real struct{}

// Now implements Clock.
func (Real) Now() time.Time { return time.Now() }

// Fixed is a test clock frozen at a single instant.
type Fixed struct{ T time.Time }

// Now implements Clock.
func (f Fixed) Now() time.Time { return f.T }
