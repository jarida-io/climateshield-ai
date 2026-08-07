// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// deterministic pseudo-random leaves; fixed seed keeps the test reproducible.
func randomLeaves(rng *rand.Rand, n int) [][]byte {
	leaves := make([][]byte, n)
	for i := range leaves {
		leaves[i] = make([]byte, 32)
		rng.Read(leaves[i])
	}
	return leaves
}

func TestRootEmptyAndSingle(t *testing.T) {
	empty := sha256.Sum256(nil)
	require.Equal(t, empty[:], Root(nil))

	leaf := []byte("leaf-0")
	withPrefix := sha256.Sum256(append([]byte{0x00}, leaf...))
	require.Equal(t, withPrefix[:], Root([][]byte{leaf}))
}

func TestRootDependsOnOrder(t *testing.T) {
	a, b := []byte("a"), []byte("b")
	require.NotEqual(t, Root([][]byte{a, b}), Root([][]byte{b, a}))
}

// The property the whole ledger stands on: flipping ANY single bit of ANY
// leaf changes the root.
func TestAnySingleByteMutationChangesRoot(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for _, n := range []int{1, 2, 3, 5, 8, 13} {
		leaves := randomLeaves(rng, n)
		base := Root(leaves)
		for i := range leaves {
			for pos := range leaves[i] {
				mutated := make([][]byte, n)
				for j := range leaves {
					mutated[j] = append([]byte(nil), leaves[j]...)
				}
				mutated[i][pos] ^= 1 << uint(rng.Intn(8))
				require.NotEqual(t, base, Root(mutated),
					"n=%d: mutation of leaf %d byte %d left the root unchanged", n, i, pos)
			}
		}
	}
}

func TestInclusionProofsVerifyForEveryLeaf(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, n := range []int{1, 2, 3, 4, 5, 7, 8, 9, 16, 31} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			leaves := randomLeaves(rng, n)
			root := Root(leaves)
			for i := 0; i < n; i++ {
				proof, err := BuildProof(leaves, i)
				require.NoError(t, err)
				require.True(t, VerifyProof(leaves[i], proof, root),
					"proof for leaf %d/%d failed", i, n)
			}
		})
	}
}

func TestInclusionProofRejections(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	leaves := randomLeaves(rng, 9)
	root := Root(leaves)

	proof, err := BuildProof(leaves, 4)
	require.NoError(t, err)

	// Wrong leaf under a valid proof.
	require.False(t, VerifyProof(leaves[5], proof, root))

	// Tampered root.
	badRoot := append([]byte(nil), root...)
	badRoot[0] ^= 0xff
	require.False(t, VerifyProof(leaves[4], proof, badRoot))

	// Truncated and extended paths.
	short := proof
	short.Path = proof.Path[:len(proof.Path)-1]
	require.False(t, VerifyProof(leaves[4], short, root))
	long := proof
	long.Path = append(append([][]byte{}, proof.Path...), leaves[0])
	require.False(t, VerifyProof(leaves[4], long, root))

	// Out-of-range index.
	_, err = BuildProof(leaves, 9)
	require.Error(t, err)
	require.False(t, VerifyProof(leaves[0], Proof{Index: -1, N: 9}, root))
}
