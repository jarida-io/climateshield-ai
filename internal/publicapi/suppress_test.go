// SPDX-License-Identifier: Apache-2.0

package publicapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSuppressBoundaries(t *testing.T) {
	cases := []struct {
		n          int64
		suppressed bool
	}{
		{0, false}, // zero is population info, not individual-inferable
		{1, true},
		{5, true},
		{9, true},   // K-1: suppressed
		{10, false}, // K: shown
		{11, false},
		{10_000, false},
	}
	for _, tc := range cases {
		v, sup := Suppress(tc.n)
		require.Equal(t, tc.suppressed, sup, "n=%d", tc.n)
		if sup {
			require.Nil(t, v)
		} else {
			require.NotNil(t, v)
			require.Equal(t, tc.n, *v)
		}
	}
}
