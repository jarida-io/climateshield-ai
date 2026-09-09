// SPDX-License-Identifier: Apache-2.0

package evm_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm"
	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm/evmtest"
)

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// TestCommittedArtifactsMatchBuildRecord is the guard against editing the
// Solidity source (or an artifact) without recompiling: every hash BUILD.txt
// records must still be the hash of the committed file, and the bytes the
// binary embeds must be those files.
func TestCommittedArtifactsMatchBuildRecord(t *testing.T) {
	rec := evm.ParseBuildRecord(evm.BuildRecord())
	require.NotEmpty(t, rec["solc_version"])
	require.Contains(t, rec["solc_image"], "@sha256:", "the compiler image must be pinned by digest")
	require.Contains(t, rec["solc_flags"], "--metadata-hash none",
		"without this, deployed code would not equal bin-runtime and eth_getCode verification would fail")

	for _, rel := range []string{
		"RootAnchor.sol", "build/RootAnchor.abi", "build/RootAnchor.bin",
		"build/RootAnchor.bin-runtime", "build/RootAnchor.signatures",
	} {
		want, ok := rec["sha256 "+rel]
		require.True(t, ok, "BUILD.txt lacks a hash for %s", rel)
		require.Equal(t, want, fileSHA256(t, filepath.Join("contract", rel)), "%s changed since the last `make contract`", rel)
	}

	// The embedded bytes are those files.
	src, err := os.ReadFile(filepath.Join("contract", "RootAnchor.sol"))
	require.NoError(t, err)
	require.Equal(t, string(src), evm.Source())
	require.Contains(t, evm.Source(), "SPDX-License-Identifier: Apache-2.0")

	binHex, err := os.ReadFile(filepath.Join("contract", "build", "RootAnchor.bin"))
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(string(binHex)), hex.EncodeToString(evm.Bytecode()))
	runHex, err := os.ReadFile(filepath.Join("contract", "build", "RootAnchor.bin-runtime"))
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(string(runHex)), hex.EncodeToString(evm.RuntimeBytecode()))

	// Creation code carries the runtime code, and the runtime is small.
	require.Contains(t, hex.EncodeToString(evm.Bytecode()), hex.EncodeToString(evm.RuntimeBytecode()))
	require.Less(t, len(evm.RuntimeBytecode()), 2048)
}

// TestABIMatchesTheHandPackedSelectors ties the compiled ABI to the Go
// packer: the selector derived from every ABI function signature must equal
// the constant the anchor sends.
func TestABIMatchesTheHandPackedSelectors(t *testing.T) {
	var entries []struct {
		Type   string `json:"type"`
		Name   string `json:"name"`
		Inputs []struct {
			Type string `json:"type"`
		} `json:"inputs"`
	}
	require.NoError(t, json.Unmarshal([]byte(evm.ABI()), &entries))

	want := map[string][]byte{
		"anchor":    evm.SelectorAnchor,
		"rootOf":    evm.SelectorRootOf,
		"versions":  evm.SelectorVersions,
		"publisher": evm.SelectorPublisher,
	}
	seen := map[string]bool{}
	for _, e := range entries {
		types := make([]string, 0, len(e.Inputs))
		for _, in := range e.Inputs {
			types = append(types, in.Type)
		}
		sig := e.Name + "(" + strings.Join(types, ",") + ")"
		switch e.Type {
		case "function":
			if w, ok := want[e.Name]; ok {
				require.Equal(t, w, evm.Selector(sig), sig)
				seen[e.Name] = true
			}
		case "event":
			require.Equal(t, "Anchored", e.Name)
			require.Equal(t, evm.AnchoredTopic, evm.Keccak256([]byte(sig)))
			seen["Anchored"] = true
		}
	}
	for name := range want {
		require.True(t, seen[name], "ABI lacks %s", name)
	}
	require.True(t, seen["Anchored"], "ABI lacks the Anchored event")
}

// TestBuildRecordParser covers the small parser the hash test relies on.
func TestBuildRecordParser(t *testing.T) {
	rec := evm.ParseBuildRecord("# comment\n\nkey=value\nsha256 a/b.bin abc\nsha256 malformed\nnoequals\n")
	require.Equal(t, "value", rec["key"])
	require.Equal(t, "abc", rec["sha256 a/b.bin"])
	require.Len(t, rec, 2)
}

// TestComposePinsTheSameAnvilImage keeps the container the stack runs and the
// container the tests run from drifting apart.
func TestComposePinsTheSameAnvilImage(t *testing.T) {
	compose, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docker-compose.yml"))
	require.NoError(t, err)
	require.Contains(t, string(compose), "image: "+evmtest.Image)
	require.Contains(t, evmtest.Image, "@sha256:")
}
