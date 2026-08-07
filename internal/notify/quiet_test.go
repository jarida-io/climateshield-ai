// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/platform/clock"
)

func eat(h, m int) time.Time {
	return time.Date(2026, 8, 7, h, m, 0, 0, clock.EAT)
}

func TestQuietHoursBoundaries(t *testing.T) {
	require.False(t, InQuietHours(eat(20, 59)), "20:59 EAT is allowed")
	require.True(t, InQuietHours(eat(21, 0)), "21:00 EAT starts quiet hours")
	require.True(t, InQuietHours(eat(23, 59)))
	require.True(t, InQuietHours(eat(0, 0)))
	require.True(t, InQuietHours(eat(6, 59)), "06:59 EAT is still quiet")
	require.False(t, InQuietHours(eat(7, 0)), "07:00 EAT ends quiet hours")
	require.False(t, InQuietHours(eat(12, 0)))
}

func TestQuietHoursUsesEATNotUTC(t *testing.T) {
	// 18:30 UTC is 21:30 EAT — inside the window even though UTC says evening.
	utc := time.Date(2026, 8, 7, 18, 30, 0, 0, time.UTC)
	require.True(t, InQuietHours(utc))

	// 04:30 UTC is 07:30 EAT — allowed.
	require.False(t, InQuietHours(time.Date(2026, 8, 7, 4, 30, 0, 0, time.UTC)))
}

func TestNextAllowedTime(t *testing.T) {
	// Daytime passes through untouched.
	noon := eat(12, 0)
	require.Equal(t, noon, NextAllowedTime(noon))

	// Late evening rolls to 07:00 the NEXT day.
	got := NextAllowedTime(eat(21, 30)).In(clock.EAT)
	require.Equal(t, 7, got.Hour())
	require.Equal(t, 8, got.Day())

	// Early morning rolls to 07:00 the SAME day.
	got = NextAllowedTime(eat(5, 0)).In(clock.EAT)
	require.Equal(t, 7, got.Hour())
	require.Equal(t, 7, got.Day())
}
