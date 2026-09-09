// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm/evmtest"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

func TestBuildAnchorsLocalModeNeedsNoChain(t *testing.T) {
	anchors, err := buildAnchors(context.Background(), ServiceConfig{AnchorMode: AnchorModeLocal}, nil, logging.New(io.Discard, "info"))
	require.NoError(t, err)
	require.Equal(t, []string{"local"}, anchors.Types())
}

func TestBuildAnchorsRejectsAnUnknownMode(t *testing.T) {
	_, err := buildAnchors(context.Background(), ServiceConfig{AnchorMode: "chain"}, nil, logging.New(io.Discard, "info"))
	require.ErrorContains(t, err, `ANCHOR_MODE "chain"`)
}

// A configured chain that does not answer must fail startup, never degrade
// to local-only in silence — the same rule as an ONNX path that cannot load.
func TestBuildAnchorsEVMFailsFastWhenTheChainIsUnreachable(t *testing.T) {
	cfg := ServiceConfig{AnchorMode: AnchorModeEVM, AnchorRPCURL: "http://127.0.0.1:1", AnchorConfirmTimeout: time.Second}
	_, err := buildAnchors(context.Background(), cfg, nil, logging.New(io.Discard, "info"))
	require.ErrorContains(t, err, "ANCHOR_MODE=evm at http://127.0.0.1:1")
}

func TestBuildAnchorsEVMDeploysOnceAtStartup(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	f := evmtest.NewFake(t)
	cfg := ServiceConfig{AnchorMode: AnchorModeEVM, AnchorRPCURL: f.URL(), AnchorConfirmTimeout: 5 * time.Second}

	anchors, err := buildAnchors(context.Background(), cfg, q, logging.New(io.Discard, "info"))
	require.NoError(t, err)
	require.Equal(t, []string{"local", "evm"}, anchors.Types())

	row, err := q.GetAnchorContract(context.Background(), 31337)
	require.NoError(t, err)
	require.Contains(t, f.Deployed(), row.Address)

	// A second start (a restart) reuses the contract.
	_, err = buildAnchors(context.Background(), cfg, q, logging.New(io.Discard, "info"))
	require.NoError(t, err)
	require.Len(t, f.Deployed(), 1)

	// An operator pin that does not hold RootAnchor is refused.
	cfg.AnchorContractAddress = "0x00000000000000000000000000000000000000ee"
	_, err = buildAnchors(context.Background(), cfg, q, logging.New(io.Discard, "info"))
	require.Error(t, err)
}
