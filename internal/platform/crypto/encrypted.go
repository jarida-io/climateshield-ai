// SPDX-License-Identifier: Apache-2.0

// Package crypto provides typed encryption-at-rest for PII columns.
// Child names, guardian names, guardian phones and national IDs are stored
// only as Encrypted[T] blobs (AES-256-GCM); the key comes from the
// PII_KEY_HEX environment variable and never lives in the database.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	keySize   = 32 // AES-256
	nonceSize = 12 // GCM standard
)

// ErrDecrypt is returned when a blob cannot be authenticated or decoded —
// wrong key, truncated data, or tampering. Callers get no finer detail by
// design.
var ErrDecrypt = errors.New("crypto: decryption failed")

// Key is a 32-byte AES-256 key.
type Key [keySize]byte

// KeyFromHex parses a 64-character hex string into a Key.
func KeyFromHex(s string) (Key, error) {
	var k Key
	b, err := hex.DecodeString(s)
	if err != nil {
		return k, fmt.Errorf("crypto: key is not valid hex: %w", err)
	}
	if len(b) != keySize {
		return k, fmt.Errorf("crypto: key must be %d bytes (%d hex chars), got %d bytes", keySize, keySize*2, len(b))
	}
	copy(k[:], b)
	return k, nil
}

// NewRandomKey generates a fresh random key (used for per-child HMAC keys and
// in tests).
func NewRandomKey() (Key, error) {
	var k Key
	if _, err := rand.Read(k[:]); err != nil {
		return k, fmt.Errorf("crypto: %w", err)
	}
	return k, nil
}

// Encrypted is a value of type T encrypted at rest. The wire/storage form is
// nonce || AES-256-GCM(json(T)), kept in a bytea column. The zero value is
// "no data".
type Encrypted[T any] struct {
	blob []byte
}

// Seal encrypts v under key with a fresh random nonce.
func Seal[T any](key Key, v T) (Encrypted[T], error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return Encrypted[T]{}, fmt.Errorf("crypto: %w", err)
	}
	aead, err := newAEAD(key)
	if err != nil {
		return Encrypted[T]{}, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return Encrypted[T]{}, fmt.Errorf("crypto: %w", err)
	}
	return Encrypted[T]{blob: aead.Seal(nonce, nonce, plain, nil)}, nil
}

// Open decrypts and returns the wrapped value.
func (e Encrypted[T]) Open(key Key) (T, error) {
	var v T
	if len(e.blob) < nonceSize+1 {
		return v, ErrDecrypt
	}
	aead, err := newAEAD(key)
	if err != nil {
		return v, err
	}
	plain, err := aead.Open(nil, e.blob[:nonceSize], e.blob[nonceSize:], nil)
	if err != nil {
		return v, ErrDecrypt
	}
	if err := json.Unmarshal(plain, &v); err != nil {
		return v, ErrDecrypt
	}
	return v, nil
}

// Bytes returns the storage form (for writing to a bytea column).
func (e Encrypted[T]) Bytes() []byte { return e.blob }

// FromBytes wraps a storage blob read back from the database.
func FromBytes[T any](blob []byte) Encrypted[T] {
	return Encrypted[T]{blob: blob}
}

// IsZero reports whether no data is present.
func (e Encrypted[T]) IsZero() bool { return len(e.blob) == 0 }

// String implements fmt.Stringer so an Encrypted value can never leak its
// contents through printf or a log line.
func (Encrypted[T]) String() string { return "[encrypted]" }

func newAEAD(key Key) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: %w", err)
	}
	return aead, nil
}
