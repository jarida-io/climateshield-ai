// SPDX-License-Identifier: Apache-2.0

package crypto

import (
	"fmt"
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
