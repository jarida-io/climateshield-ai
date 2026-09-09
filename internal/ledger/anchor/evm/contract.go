// SPDX-License-Identifier: Apache-2.0

package evm

import (
	_ "embed"
	"encoding/hex"
	"strings"
)

// The RootAnchor artifacts are compiled ONCE by `make contract` with a solc
// image pinned by digest (see contract/BUILD.txt) and committed. Nothing at
// build, test or run time compiles Solidity. contract_test.go asserts these
// bytes still hash to what BUILD.txt records, so an edited .sol without a
// recompile fails the suite instead of shipping stale bytecode.

//go:embed contract/RootAnchor.sol
var contractSource string

//go:embed contract/build/RootAnchor.abi
var contractABI string

//go:embed contract/build/RootAnchor.bin
var creationHex string

//go:embed contract/build/RootAnchor.bin-runtime
var runtimeHex string

//go:embed contract/BUILD.txt
var buildRecord string

// Source is the Solidity source of RootAnchor.
func Source() string { return contractSource }

// ABI is the contract's JSON ABI.
func ABI() string { return contractABI }

// BuildRecord is contract/BUILD.txt: compiler version, image digest, flags
// and the SHA-256 of every artifact.
func BuildRecord() string { return buildRecord }

// Bytecode is the creation bytecode sent in the deployment transaction.
func Bytecode() []byte { return mustHex(creationHex) }

// RuntimeBytecode is what eth_getCode must return for a genuine deployment.
// The contract uses no immutables and was compiled with --metadata-hash
// none, so the deployed code equals this byte for byte.
func RuntimeBytecode() []byte { return mustHex(runtimeHex) }

// ParseBuildRecord turns BUILD.txt into a key/value map. Hash lines become
// "sha256 <path>" keys.
func ParseBuildRecord(record string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(record, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "sha256 ") {
			fields := strings.Fields(line)
			if len(fields) == 3 {
				out["sha256 "+fields[1]] = fields[2]
			}
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			out[k] = v
		}
	}
	return out
}

func mustHex(s string) []byte {
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		// The artifacts are compiled in and hash-checked by a test; a decode
		// failure here means a corrupted build, which cannot be recovered from
		// at runtime.
		panic("evm: committed bytecode is not valid hex: " + err.Error())
	}
	return b
}
