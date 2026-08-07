// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"time"

	"github.com/jarida-io/climateshield/internal/platform/clock"
)

// Quiet hours: no guardian receives an SMS between 21:00 and 07:00 East
// Africa Time. Jobs that land in the window are snoozed to the next 07:00.
const (
	quietStartHour = 21
	quietEndHour   = 7
)

// InQuietHours reports whether t falls inside the quiet window.
func InQuietHours(t time.Time) bool {
	h := t.In(clock.EAT).Hour()
	return h >= quietStartHour || h < quietEndHour
}

// NextAllowedTime returns t unchanged when sending is allowed, otherwise the
// next 07:00 EAT.
func NextAllowedTime(t time.Time) time.Time {
	if !InQuietHours(t) {
		return t
	}
	eat := t.In(clock.EAT)
	next := time.Date(eat.Year(), eat.Month(), eat.Day(), quietEndHour, 0, 0, 0, clock.EAT)
	if eat.Hour() >= quietStartHour {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
