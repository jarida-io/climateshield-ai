// SPDX-License-Identifier: Apache-2.0

package clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEATIsFixedUTCPlus3(t *testing.T) {
	_, offset := time.Date(2026, 8, 7, 12, 0, 0, 0, EAT).Zone()
	require.Equal(t, 3*60*60, offset)

	// No DST: offset identical in January and July.
	_, jan := time.Date(2026, 1, 15, 12, 0, 0, 0, EAT).Zone()
	_, jul := time.Date(2026, 7, 15, 12, 0, 0, 0, EAT).Zone()
	require.Equal(t, jan, jul)
}

func TestFixedClock(t *testing.T) {
	at := time.Date(2026, 8, 7, 21, 0, 0, 0, EAT)
	require.Equal(t, at, Fixed{T: at}.Now())
}

func TestRealClockAdvances(t *testing.T) {
	a := Real{}.Now()
	b := Real{}.Now()
	require.False(t, b.Before(a))
}
