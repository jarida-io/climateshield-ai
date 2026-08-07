// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeptetLengthBasic(t *testing.T) {
	n, err := SeptetLength("Reply STOP to opt out.")
	require.NoError(t, err)
	require.Equal(t, 22, n)

	n, err = SeptetLength("")
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestSeptetLengthExtensionCharsCostTwo(t *testing.T) {
	n, err := SeptetLength("a[b]c")
	require.NoError(t, err)
	require.Equal(t, 3+2*2, n)

	n, err = SeptetLength("€")
	require.NoError(t, err)
	require.Equal(t, 2, n)
}

func TestSeptetLengthRejectsNonGSM(t *testing.T) {
	for _, s := range []string{"emoji 🙂", "chinese 中", "curly ’quote"} {
		_, err := SeptetLength(s)
		require.Error(t, err, "%q must be rejected", s)
	}
}

func TestSeptetLength160Boundary(t *testing.T) {
	s := strings.Repeat("a", 160)
	n, err := SeptetLength(s)
	require.NoError(t, err)
	require.Equal(t, 160, n)
}
