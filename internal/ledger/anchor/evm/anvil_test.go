// SPDX-License-Identifier: Apache-2.0

package evm_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm"
	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm/evmtest"
	"github.com/jarida-io/climateshield/internal/platform/logging"
)

// TestRootAnchorOnARealDevelopmentChain executes the committed bytecode on
// anvil: deploy, verify eth_getCode, anchor, read back, fetch the event, and
// prove the contract's append-only and idempotent behaviour with real EVM
// execution rather than the fake's imitation of it. Skipped under -short.
func TestRootAnchorOnARealDevelopmentChain(t *testing.T) {
	rpcURL := evmtest.Anvil(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	store := newMemStore()
	log := logging.New(io.Discard, "info")

	a := evm.New(evm.Config{RPCURL: rpcURL, ConfirmTimeout: 30 * time.Second}, store, log)
	dep, err := a.Ensure(ctx)
	require.NoError(t, err)
	require.True(t, dep.Deployed)
	require.Equal(t, evmtest.ChainID, dep.ChainID)
	require.Equal(t, evm.DevChainLabel, dep.ChainLabel)

	client := a.Client()
	code, err := client.GetCode(ctx, dep.ContractAddress)
	require.NoError(t, err)
	require.Equal(t, evm.RuntimeBytecode(), code, "deployed code must equal the committed runtime byte for byte")

	// The deployer is the publisher.
	ret, err := client.EthCall(ctx, dep.ContractAddress, evm.PackPublisher())
	require.NoError(t, err)
	publisher, err := evm.UnpackAddress(ret)
	require.NoError(t, err)
	require.Equal(t, dep.From, publisher)

	day := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	day32 := evm.DayBytes32(day)
	root1 := [32]byte{0x9c, 0x11, 0x85, 0xa5}
	root2 := [32]byte{0x11, 0x22, 0x33, 0x44}

	// Nothing there yet.
	ret, err = client.EthCall(ctx, dep.ContractAddress, evm.PackRootOf(day32))
	require.NoError(t, err)
	require.Equal(t, make([]byte, 32), ret)

	// Anchor, read back, and check the versions counter.
	rcpt, err := a.AnchorRoot(ctx, day, root1[:])
	require.NoError(t, err)
	require.True(t, rcpt.ReadBackOK)
	require.Equal(t, root1[:], rcpt.ReadBack)
	require.Positive(t, rcpt.BlockNumber)
	versions := func() uint64 {
		ret, err := client.EthCall(ctx, dep.ContractAddress, evm.PackVersions(day32))
		require.NoError(t, err)
		n, err := evm.UnpackUint64(ret)
		require.NoError(t, err)
		return n
	}
	require.EqualValues(t, 1, versions())

	// Re-anchoring the same root is idempotent on the real EVM too.
	_, err = a.AnchorRoot(ctx, day, root1[:])
	require.NoError(t, err)
	require.EqualValues(t, 1, versions())

	// A changed root appends a version; the newest wins on read-back.
	rcpt2, err := a.AnchorRoot(ctx, day, root2[:])
	require.NoError(t, err)
	require.Equal(t, root2[:], rcpt2.ReadBack)
	require.EqualValues(t, 2, versions())

	// The events tell the same story, from the chain's own log index.
	logs, err := client.GetLogs(ctx, evm.LogFilter{
		FromBlock: "0x0", ToBlock: "latest",
		Address: dep.ContractAddress,
		Topics:  [][]string{{evm.EncodeHex(evm.AnchoredTopic)}},
	})
	require.NoError(t, err)
	require.Len(t, logs, 2, "one Anchored event per new root, none for the idempotent repeat")
	first, err := evm.DecodeAnchored(logs[0])
	require.NoError(t, err)
	require.Equal(t, evm.AnchoredEvent{Day: "2026-08-07", Root: root1, Version: 1}, first)
	second, err := evm.DecodeAnchored(logs[1])
	require.NoError(t, err)
	require.Equal(t, evm.AnchoredEvent{Day: "2026-08-07", Root: root2, Version: 2}, second)
	require.Equal(t, rcpt2.TxHash, logs[1].TransactionHash)

	// A second process reuses the recorded contract instead of deploying.
	b := evm.New(evm.Config{RPCURL: rpcURL}, store, log)
	reused, err := b.Ensure(ctx)
	require.NoError(t, err)
	require.False(t, reused.Deployed)
	require.Equal(t, dep.ContractAddress, reused.ContractAddress)
}
