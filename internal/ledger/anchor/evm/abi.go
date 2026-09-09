// SPDX-License-Identifier: Apache-2.0

// Package evm anchors daily Merkle roots to the RootAnchor contract on an EVM
// chain through a hand-rolled JSON-RPC client. There is no Ethereum client
// library here: the contract's ABI is small enough to pack by hand, and the
// four selectors and one event topic are pinned by golden vectors generated
// with `cast` (testdata/golden/evm/vectors.json).
//
// Transactions are sent with eth_sendTransaction from an account the node
// itself holds — on the development chain that is anvil's first unlocked
// account — so no private key exists in this repository or its configuration.
package evm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/sha3"
)

// DayLayout is how a leaf day is written into its bytes32: the ASCII date,
// right-padded with zero bytes. `cast format-bytes32-string 2026-08-07`
// produces the same value, so a day can be checked from the command line.
const DayLayout = "2006-01-02"

// ABI selectors and the event topic of RootAnchor. Each is keccak256 of the
// canonical signature; the golden vectors pin them to cast's output.
var (
	SelectorAnchor    = Selector("anchor(bytes32,bytes32)")
	SelectorRootOf    = Selector("rootOf(bytes32)")
	SelectorVersions  = Selector("versions(bytes32)")
	SelectorPublisher = Selector("publisher()")
	AnchoredTopic     = Keccak256([]byte("Anchored(bytes32,bytes32,uint256)"))
)

// Keccak256 is the Ethereum (pre-standard) Keccak-256 hash.
func Keccak256(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	return h.Sum(nil)
}

// Selector is the 4-byte function selector of a canonical signature.
func Selector(signature string) []byte {
	return Keccak256([]byte(signature))[:4]
}

// DayBytes32 encodes a calendar day as the contract expects it.
func DayBytes32(day time.Time) [32]byte {
	var out [32]byte
	copy(out[:], day.UTC().Format(DayLayout))
	return out
}

// DayString reverses DayBytes32.
func DayString(b [32]byte) string {
	return string(bytes.TrimRight(b[:], "\x00"))
}

// Root32 checks that a Merkle root is exactly 32 non-zero bytes — the only
// shape the contract accepts.
func Root32(root []byte) ([32]byte, error) {
	var out [32]byte
	if len(root) != len(out) {
		return out, fmt.Errorf("evm: root must be %d bytes, got %d", len(out), len(root))
	}
	copy(out[:], root)
	if out == [32]byte{} {
		return out, errors.New("evm: root must not be zero")
	}
	return out, nil
}

// PackAnchor builds the calldata for anchor(bytes32 day, bytes32 root).
func PackAnchor(day, root [32]byte) []byte {
	out := make([]byte, 0, 4+64)
	out = append(out, SelectorAnchor...)
	out = append(out, day[:]...)
	return append(out, root[:]...)
}

// PackRootOf builds the calldata for rootOf(bytes32 day).
func PackRootOf(day [32]byte) []byte {
	return append(append([]byte{}, SelectorRootOf...), day[:]...)
}

// PackVersions builds the calldata for versions(bytes32 day).
func PackVersions(day [32]byte) []byte {
	return append(append([]byte{}, SelectorVersions...), day[:]...)
}

// PackPublisher builds the calldata for publisher().
func PackPublisher() []byte {
	return append([]byte{}, SelectorPublisher...)
}

// UnpackBytes32 decodes a single bytes32 return value.
func UnpackBytes32(ret []byte) ([32]byte, error) {
	var out [32]byte
	if len(ret) != len(out) {
		return out, fmt.Errorf("evm: expected a 32-byte return value, got %d bytes", len(ret))
	}
	copy(out[:], ret)
	return out, nil
}

// UnpackUint64 decodes a uint256 return value that fits in 64 bits.
func UnpackUint64(ret []byte) (uint64, error) {
	if len(ret) != 32 {
		return 0, fmt.Errorf("evm: expected a 32-byte return value, got %d bytes", len(ret))
	}
	for _, b := range ret[:24] {
		if b != 0 {
			return 0, errors.New("evm: uint256 return value does not fit in 64 bits")
		}
	}
	return binary.BigEndian.Uint64(ret[24:]), nil
}

// UnpackAddress decodes an address return value as 0x-prefixed lowercase hex.
func UnpackAddress(ret []byte) (string, error) {
	if len(ret) != 32 {
		return "", fmt.Errorf("evm: expected a 32-byte return value, got %d bytes", len(ret))
	}
	for _, b := range ret[:12] {
		if b != 0 {
			return "", errors.New("evm: address return value has non-zero padding")
		}
	}
	return EncodeHex(ret[12:]), nil
}

// AnchoredEvent is one decoded Anchored(day, root, version) log.
type AnchoredEvent struct {
	Day     string
	Root    [32]byte
	Version uint64
}

// DecodeAnchored decodes an Anchored log: topic[1] is the indexed day, the
// data carries the root and the version.
func DecodeAnchored(l Log) (AnchoredEvent, error) {
	var ev AnchoredEvent
	if len(l.Topics) != 2 {
		return ev, fmt.Errorf("evm: Anchored log needs 2 topics, got %d", len(l.Topics))
	}
	topic0, err := DecodeHex(l.Topics[0])
	if err != nil || !bytes.Equal(topic0, AnchoredTopic) {
		return ev, errors.New("evm: log is not an Anchored event")
	}
	dayBytes, err := DecodeHex(l.Topics[1])
	if err != nil {
		return ev, fmt.Errorf("evm: Anchored day topic: %w", err)
	}
	day, err := UnpackBytes32(dayBytes)
	if err != nil {
		return ev, err
	}
	data, err := DecodeHex(l.Data)
	if err != nil {
		return ev, fmt.Errorf("evm: Anchored data: %w", err)
	}
	if len(data) != 64 {
		return ev, fmt.Errorf("evm: Anchored data must be 64 bytes, got %d", len(data))
	}
	root, err := UnpackBytes32(data[:32])
	if err != nil {
		return ev, err
	}
	version, err := UnpackUint64(data[32:])
	if err != nil {
		return ev, err
	}
	return AnchoredEvent{Day: DayString(day), Root: root, Version: version}, nil
}

// DevChainLabel is what the development chain must always be called. It is
// started by this stack, holds no value, and is not a public network; the
// wording exists so no surface can imply otherwise.
const DevChainLabel = "local development chain started by this stack — not a public network"

// Label names a chain by its id. The two ids that development nodes use by
// default get the honest sentence; anything else is named by number only,
// because this repository does not vouch for any chain it did not start.
func Label(chainID int64) string {
	switch chainID {
	case 31337, 1337:
		return DevChainLabel
	}
	return fmt.Sprintf("chain id %d", chainID)
}

// IsDevChain reports whether chainID is one of the development-node defaults.
func IsDevChain(chainID int64) bool {
	return chainID == 31337 || chainID == 1337
}
