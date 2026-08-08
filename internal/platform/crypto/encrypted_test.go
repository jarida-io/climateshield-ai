// SPDX-License-Identifier: Apache-2.0

package crypto

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testKey(t *testing.T) Key {
	t.Helper()
	k, err := KeyFromHex("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	return k
}

func TestKeyFromHexValidation(t *testing.T) {
	_, err := KeyFromHex("not-hex")
	require.Error(t, err)

	_, err = KeyFromHex("abcd") // too short
	require.Error(t, err)

	_, err = KeyFromHex("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
}

func TestRoundTrip(t *testing.T) {
	key := testKey(t)

	for _, val := range []string{"", "Wanjiku Kamau", "+254712345678", "übergroße-строка-字符串"} {
		enc, err := Seal(key, val)
		require.NoError(t, err)

		got, err := enc.Open(key)
		require.NoError(t, err)
		require.Equal(t, val, got)
	}
}

func TestRoundTripStruct(t *testing.T) {
	type guardian struct {
		Name  string
		Phone string
	}
	key := testKey(t)

	enc, err := Seal(key, guardian{Name: "Achieng", Phone: "0712345678"})
	require.NoError(t, err)

	got, err := enc.Open(key)
	require.NoError(t, err)
	require.Equal(t, guardian{Name: "Achieng", Phone: "0712345678"}, got)
}

func TestWrongKeyFails(t *testing.T) {
	enc, err := Seal(testKey(t), "secret")
	require.NoError(t, err)

	other, err := NewRandomKey()
	require.NoError(t, err)

	_, err = enc.Open(other)
	require.ErrorIs(t, err, ErrDecrypt)
}

func TestTamperFails(t *testing.T) {
	key := testKey(t)
	enc, err := Seal(key, "secret")
	require.NoError(t, err)

	// Flip one byte anywhere in the blob — GCM must reject it.
	for i := range enc.Bytes() {
		mutated := append([]byte(nil), enc.Bytes()...)
		mutated[i] ^= 0x01
		_, err := FromBytes[string](mutated).Open(key)
		require.ErrorIs(t, err, ErrDecrypt, "mutation at byte %d was accepted", i)
	}
}

func TestNonceFreshness(t *testing.T) {
	key := testKey(t)
	a, err := Seal(key, "same value")
	require.NoError(t, err)
	b, err := Seal(key, "same value")
	require.NoError(t, err)
	require.NotEqual(t, a.Bytes(), b.Bytes(), "two seals of the same value must differ (random nonce)")
}

func TestStorageRoundTripAndZero(t *testing.T) {
	key := testKey(t)
	enc, err := Seal(key, "value")
	require.NoError(t, err)

	restored := FromBytes[string](enc.Bytes())
	got, err := restored.Open(key)
	require.NoError(t, err)
	require.Equal(t, "value", got)

	var zero Encrypted[string]
	require.True(t, zero.IsZero())
	_, err = zero.Open(key)
	require.ErrorIs(t, err, ErrDecrypt)
}

func TestStringerNeverLeaks(t *testing.T) {
	enc, err := Seal(testKey(t), "top-secret-name")
	require.NoError(t, err)
	require.Equal(t, "[encrypted]", fmt.Sprintf("%v", enc))
	require.Equal(t, "[encrypted]", enc.String())
	require.NotContains(t, fmt.Sprintf("%+v", enc), "top-secret-name")
}

// The published placeholder must be refused unless local development opts in.
// A deployment that forgets to generate a key has to fail loudly rather than
// run with encryption that protects nothing.
func TestDevKeyRejectedUnlessExplicitlyAllowed(t *testing.T) {
	_, err := KeyFromHexChecked(DevKeyHex, false)
	require.ErrorIs(t, err, ErrDevKey)
	require.Contains(t, err.Error(), "openssl rand -hex 32")

	// Whitespace or case must not smuggle it past the check.
	_, err = KeyFromHexChecked("  "+strings.ToUpper(DevKeyHex)+"\n", false)
	require.ErrorIs(t, err, ErrDevKey)

	// Local development opts in explicitly.
	k, err := KeyFromHexChecked(DevKeyHex, true)
	require.NoError(t, err)
	require.NotEqual(t, Key{}, k)

	// A generated key passes either way.
	real, err := NewRandomKey()
	require.NoError(t, err)
	_, err = KeyFromHexChecked(hex.EncodeToString(real[:]), false)
	require.NoError(t, err)
}
