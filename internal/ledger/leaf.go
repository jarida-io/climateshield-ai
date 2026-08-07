// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"crypto/hmac"
	"crypto/sha256"
)

// Leaf computes the per-child leaf hash: HMAC-SHA256(childKey, canonical).
// The key never leaves sealed.child_keys; destroying it (ForgetChild) makes
// every leaf it produced permanently unlinkable to the child.
func Leaf(childKey []byte, canonical []byte) []byte {
	mac := hmac.New(sha256.New, childKey)
	mac.Write(canonical)
	return mac.Sum(nil)
}
