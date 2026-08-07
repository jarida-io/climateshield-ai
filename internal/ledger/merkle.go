// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"bytes"
	"crypto/sha256"
	"fmt"
)

// Merkle tree in the RFC 6962 (Certificate Transparency) structure: leaf
// hashes are domain-separated with 0x00, interior nodes with 0x01, and a
// tree of n leaves splits at the largest power of two smaller than n. Domain
// separation prevents leaf/node confusion attacks; the structure gives
// O(log n) inclusion proofs.

const (
	leafPrefix     = byte(0x00)
	interiorPrefix = byte(0x01)
)

// Root computes the Merkle tree hash over the ordered leaves.
func Root(leaves [][]byte) []byte {
	if len(leaves) == 0 {
		h := sha256.Sum256(nil)
		return h[:]
	}
	return subtreeRoot(leaves)
}

func subtreeRoot(leaves [][]byte) []byte {
	if len(leaves) == 1 {
		return hashLeaf(leaves[0])
	}
	k := largestPowerOfTwoBelow(len(leaves))
	return hashInterior(subtreeRoot(leaves[:k]), subtreeRoot(leaves[k:]))
}

// Proof is an inclusion proof for the leaf at Index in a tree of N leaves.
type Proof struct {
	Index int
	N     int
	Path  [][]byte
}

// BuildProof returns the audit path for the leaf at index.
func BuildProof(leaves [][]byte, index int) (Proof, error) {
	if index < 0 || index >= len(leaves) {
		return Proof{}, fmt.Errorf("ledger: proof index %d out of range [0,%d)", index, len(leaves))
	}
	path := auditPath(leaves, index)
	return Proof{Index: index, N: len(leaves), Path: path}, nil
}

func auditPath(leaves [][]byte, index int) [][]byte {
	if len(leaves) == 1 {
		return nil
	}
	k := largestPowerOfTwoBelow(len(leaves))
	if index < k {
		return append(auditPath(leaves[:k], index), subtreeRoot(leaves[k:]))
	}
	return append(auditPath(leaves[k:], index-k), subtreeRoot(leaves[:k]))
}

// VerifyProof checks that leaf (the raw leaf value, pre-hash) is included
// under root via proof. This is the public verifier: given a published daily
// root and one leaf plus its path, anyone can confirm inclusion without
// seeing any other leaf.
func VerifyProof(leaf []byte, proof Proof, root []byte) bool {
	if proof.N <= 0 || proof.Index < 0 || proof.Index >= proof.N {
		return false
	}
	derived, ok := rootFromPath(hashLeaf(leaf), proof.Index, proof.N, proof.Path)
	return ok && bytes.Equal(derived, root)
}

// rootFromPath mirrors auditPath's recursion exactly: the audit path is
// ordered deepest-sibling-first, so the top-level sibling is consumed from
// the end at each level on the way down.
func rootFromPath(leafHash []byte, index, n int, path [][]byte) ([]byte, bool) {
	if n == 1 {
		if len(path) != 0 {
			return nil, false // path longer than the tree is deep
		}
		return leafHash, true
	}
	if len(path) == 0 {
		return nil, false // path shorter than the tree is deep
	}
	sibling := path[len(path)-1]
	rest := path[:len(path)-1]
	k := largestPowerOfTwoBelow(n)
	if index < k {
		left, ok := rootFromPath(leafHash, index, k, rest)
		if !ok {
			return nil, false
		}
		return hashInterior(left, sibling), true
	}
	right, ok := rootFromPath(leafHash, index-k, n-k, rest)
	if !ok {
		return nil, false
	}
	return hashInterior(sibling, right), true
}

func hashLeaf(leaf []byte) []byte {
	h := sha256.New()
	h.Write([]byte{leafPrefix})
	h.Write(leaf)
	return h.Sum(nil)
}

func hashInterior(left, right []byte) []byte {
	h := sha256.New()
	h.Write([]byte{interiorPrefix})
	h.Write(left)
	h.Write(right)
	return h.Sum(nil)
}

func largestPowerOfTwoBelow(n int) int {
	k := 1
	for k*2 < n {
		k *= 2
	}
	return k
}
