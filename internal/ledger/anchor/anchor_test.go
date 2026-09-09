// SPDX-License-Identifier: Apache-2.0

package anchor_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/ledger/anchor"
)

type stub struct{ typ string }

func (s stub) Type() string { return s.typ }
func (s stub) AnchorRoot(context.Context, time.Time, []byte) (anchor.Receipt, error) {
	return anchor.Receipt{Type: s.typ}, nil
}

func TestLocalRecordsTheRootAndClaimsNoReadBack(t *testing.T) {
	root := []byte{0xde, 0xad, 0xbe, 0xef}
	rcpt, err := anchor.NewLocal().AnchorRoot(context.Background(), time.Now(), root)
	require.NoError(t, err)
	require.Equal(t, anchor.TypeLocal, rcpt.Type)
	require.Equal(t, "deadbeef", rcpt.Reference)

	// A database row is not somewhere independent to read from; the receipt
	// must not pretend a read-back happened.
	require.False(t, rcpt.ReadBackOK)
	require.Nil(t, rcpt.ReadBack)
	require.Zero(t, rcpt.ChainID)
	require.Empty(t, rcpt.TxHash)
}

func TestLocalRefusesAnEmptyRoot(t *testing.T) {
	_, err := anchor.NewLocal().AnchorRoot(context.Background(), time.Now(), nil)
	require.Error(t, err)
}

func TestMultiListsTypesInPublicationOrder(t *testing.T) {
	m := anchor.Multi{anchor.NewLocal(), stub{"evm"}}
	require.Equal(t, []string{"local", "evm"}, m.Types())
	require.Empty(t, anchor.Multi{}.Types())
}
