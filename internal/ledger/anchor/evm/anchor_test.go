// SPDX-License-Identifier: Apache-2.0

package evm_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/ledger/anchor"
	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm"
	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm/evmtest"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// memStore is an in-memory ContractStore with the same not-found semantics as
// the sqlc-generated queries.
type memStore struct {
	mu   sync.Mutex
	rows map[int64]db.AnchorContract
	err  error
}

func newMemStore() *memStore { return &memStore{rows: map[int64]db.AnchorContract{}} }

func (m *memStore) GetAnchorContract(_ context.Context, chainID int64) (db.AnchorContract, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return db.AnchorContract{}, m.err
	}
	row, ok := m.rows[chainID]
	if !ok {
		return db.AnchorContract{}, pgx.ErrNoRows
	}
	return row, nil
}

func (m *memStore) UpsertAnchorContract(_ context.Context, arg db.UpsertAnchorContractParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.rows[arg.ChainID] = db.AnchorContract{ChainID: arg.ChainID, Address: arg.Address, DeployTx: arg.DeployTx}
	return nil
}

func newAnchor(t *testing.T, f *evmtest.Fake, store evm.ContractStore, mutate func(*evm.Config)) *evm.Anchor {
	t.Helper()
	cfg := evm.Config{RPCURL: f.URL(), ConfirmTimeout: 2 * time.Second, PollInterval: time.Millisecond}
	if mutate != nil {
		mutate(&cfg)
	}
	return evm.New(cfg, store, logging.New(io.Discard, "info"))
}

func TestEnsureDeploysOnceAndReusesTheRecordedContract(t *testing.T) {
	f := evmtest.NewFake(t)
	store := newMemStore()
	ctx := context.Background()

	a := newAnchor(t, f, store, nil)
	dep, err := a.Ensure(ctx)
	require.NoError(t, err)
	require.True(t, dep.Deployed)
	require.NotEmpty(t, dep.DeployTx)
	require.EqualValues(t, 31337, dep.ChainID)
	require.Equal(t, evm.DevChainLabel, dep.ChainLabel)
	require.Equal(t, evmtest.DefaultAccount, dep.From)
	require.Contains(t, f.Deployed(), dep.ContractAddress)

	// The address is remembered for this chain.
	row, err := store.GetAnchorContract(ctx, 31337)
	require.NoError(t, err)
	require.Equal(t, dep.ContractAddress, row.Address)
	require.Equal(t, dep.DeployTx, *row.DeployTx)

	// Same instance: cached, no second deployment.
	again, err := a.Ensure(ctx)
	require.NoError(t, err)
	require.Equal(t, dep, again)

	// A fresh process finds the row, verifies the code and does not redeploy.
	b := newAnchor(t, f, store, nil)
	reused, err := b.Ensure(ctx)
	require.NoError(t, err)
	require.False(t, reused.Deployed)
	require.Equal(t, dep.ContractAddress, reused.ContractAddress)
	require.Len(t, f.Deployed(), 1, "exactly one contract must exist")
	require.Equal(t, anchor.TypeEVM, b.Type())
	require.Equal(t, f.URL(), b.Client().URL())
}

func TestAnchorRootSendsExactCalldataAndReadsBack(t *testing.T) {
	v := loadVectors(t)
	day, day32, root := vectorDay(t, v)
	f := evmtest.NewFake(t)
	a := newAnchor(t, f, newMemStore(), nil)

	rcpt, err := a.AnchorRoot(context.Background(), day, root[:])
	require.NoError(t, err)
	require.Equal(t, anchor.TypeEVM, rcpt.Type)
	require.True(t, rcpt.ReadBackOK)
	require.Equal(t, root[:], rcpt.ReadBack)
	require.EqualValues(t, 31337, rcpt.ChainID)
	require.Equal(t, evm.DevChainLabel, rcpt.ChainLabel)
	require.NotEmpty(t, rcpt.TxHash)
	require.Equal(t, rcpt.TxHash, rcpt.Reference)
	require.Positive(t, rcpt.BlockNumber)
	require.Contains(t, f.Deployed(), rcpt.ContractAddress)

	// The anchor transaction carried exactly the calldata cast produces.
	sends := f.RequestsFor("eth_sendTransaction")
	require.Len(t, sends, 2, "one deployment, one anchor")
	require.Contains(t, string(sends[1].Params), `"data":"`+v.AnchorCalldata+`"`)
	require.Contains(t, string(sends[1].Params), `"to":"`+rcpt.ContractAddress+`"`)
	require.Contains(t, string(sends[1].Params), `"from":"`+evmtest.DefaultAccount+`"`)

	// And the read-back was rootOf(day) with cast's calldata.
	calls := f.RequestsFor("eth_call")
	require.Len(t, calls, 1)
	require.Contains(t, string(calls[0].Params), `"data":"`+v.RootOfCalldata+`"`)

	// The chain now holds the root, once.
	require.Equal(t, [][32]byte{root}, f.Roots(rcpt.ContractAddress, day32))

	// Anchoring the same root again is harmless: a new transaction, still
	// one version on the chain (the contract is idempotent).
	rcpt2, err := a.AnchorRoot(context.Background(), day, root[:])
	require.NoError(t, err)
	require.NotEqual(t, rcpt.TxHash, rcpt2.TxHash)
	require.Equal(t, [][32]byte{root}, f.Roots(rcpt.ContractAddress, day32))
}

func TestAnchorRootWaitsForAPendingReceipt(t *testing.T) {
	v := loadVectors(t)
	day, _, root := vectorDay(t, v)
	f := evmtest.NewFake(t)
	a := newAnchor(t, f, newMemStore(), nil)
	_, err := a.Ensure(context.Background())
	require.NoError(t, err)

	before := len(f.RequestsFor("eth_getTransactionReceipt"))
	f.PendingPolls = 3
	rcpt, err := a.AnchorRoot(context.Background(), day, root[:])
	require.NoError(t, err)
	require.True(t, rcpt.ReadBackOK)
	require.Equal(t, before+4, len(f.RequestsFor("eth_getTransactionReceipt")), "three nulls then the receipt")
}

func TestAnchorRootTimesOutWithoutAReceipt(t *testing.T) {
	v := loadVectors(t)
	day, _, root := vectorDay(t, v)
	f := evmtest.NewFake(t)
	a := newAnchor(t, f, newMemStore(), func(c *evm.Config) { c.ConfirmTimeout = 20 * time.Millisecond })
	_, err := a.Ensure(context.Background())
	require.NoError(t, err)

	f.PendingPolls = 1 << 20
	_, err = a.AnchorRoot(context.Background(), day, root[:])
	require.ErrorIs(t, err, evm.ErrConfirmTimeout)

	// A cancelled context ends the wait too.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = a.AnchorRoot(ctx, day, root[:])
	require.Error(t, err)
}

func TestAnchorRootTreatsARevertAsFailure(t *testing.T) {
	v := loadVectors(t)
	day, _, root := vectorDay(t, v)
	f := evmtest.NewFake(t)
	a := newAnchor(t, f, newMemStore(), nil)
	_, err := a.Ensure(context.Background())
	require.NoError(t, err)

	f.RevertNext = true
	_, err = a.AnchorRoot(context.Background(), day, root[:])
	require.ErrorIs(t, err, evm.ErrReverted)
}

func TestAnchorRootReadBackMismatchIsAnError(t *testing.T) {
	v := loadVectors(t)
	day, _, root := vectorDay(t, v)
	f := evmtest.NewFake(t)
	a := newAnchor(t, f, newMemStore(), nil)

	lie := [32]byte{0xbb}
	f.ReadBackOverride = &lie
	rcpt, err := a.AnchorRoot(context.Background(), day, root[:])
	require.ErrorIs(t, err, evm.ErrReadBackMismatch)
	// The partial receipt says what the chain answered, and that it did not match.
	require.False(t, rcpt.ReadBackOK)
	require.Equal(t, lie[:], rcpt.ReadBack)
}

func TestAnchorRootRejectsMalformedRoots(t *testing.T) {
	f := evmtest.NewFake(t)
	a := newAnchor(t, f, newMemStore(), nil)
	_, err := a.AnchorRoot(context.Background(), time.Now(), []byte{1, 2, 3})
	require.Error(t, err)
	_, err = a.AnchorRoot(context.Background(), time.Now(), make([]byte, 32))
	require.Error(t, err)
	require.Empty(t, f.Requests(), "a bad root must be refused before any chain call")
}

func TestAnchorRootSurfacesRPCFailures(t *testing.T) {
	v := loadVectors(t)
	day, _, root := vectorDay(t, v)
	f := evmtest.NewFake(t)
	a := newAnchor(t, f, newMemStore(), nil)
	_, err := a.Ensure(context.Background())
	require.NoError(t, err)

	f.FailNext("eth_sendTransaction", 1)
	_, err = a.AnchorRoot(context.Background(), day, root[:])
	require.ErrorContains(t, err, "injected failure")

	f.FailNext("eth_getTransactionReceipt", 1)
	_, err = a.AnchorRoot(context.Background(), day, root[:])
	require.ErrorContains(t, err, "injected failure")

	f.FailNext("eth_call", 1)
	_, err = a.AnchorRoot(context.Background(), day, root[:])
	require.ErrorContains(t, err, "injected failure")

	// After the failures clear, the same root anchors fine.
	rcpt, err := a.AnchorRoot(context.Background(), day, root[:])
	require.NoError(t, err)
	require.True(t, rcpt.ReadBackOK)
}

func TestEnsureFailsWithoutAnUnlockedAccount(t *testing.T) {
	f := evmtest.NewFake(t)
	f.SetAccounts(nil)
	_, err := newAnchor(t, f, newMemStore(), nil).Ensure(context.Background())
	require.ErrorIs(t, err, evm.ErrNoAccount)
}

func TestEnsureUsesAConfiguredSenderWithoutAskingTheNode(t *testing.T) {
	f := evmtest.NewFake(t)
	a := newAnchor(t, f, newMemStore(), func(c *evm.Config) { c.From = evmtest.DefaultAccount })
	dep, err := a.Ensure(context.Background())
	require.NoError(t, err)
	require.Equal(t, evmtest.DefaultAccount, dep.From)
	require.Empty(t, f.RequestsFor("eth_accounts"))
}

func TestEnsurePinnedAddressMustHoldRootAnchor(t *testing.T) {
	f := evmtest.NewFake(t)
	store := newMemStore()
	const pinned = "0x00000000000000000000000000000000000000ee"

	// Nothing there: fail closed, deploy nothing.
	a := newAnchor(t, f, store, func(c *evm.Config) { c.ContractAddress = pinned })
	_, err := a.Ensure(context.Background())
	require.ErrorIs(t, err, evm.ErrCodeMismatch)
	require.Empty(t, f.Deployed())

	// Wrong code there: still refused.
	f.InstallCode(pinned, []byte{0x60, 0x00})
	_, err = newAnchor(t, f, store, func(c *evm.Config) { c.ContractAddress = pinned }).Ensure(context.Background())
	require.ErrorIs(t, err, evm.ErrCodeMismatch)

	// The genuine runtime there: accepted, and the store is left alone.
	f.InstallCode(pinned, evm.RuntimeBytecode())
	dep, err := newAnchor(t, f, store, func(c *evm.Config) { c.ContractAddress = pinned }).Ensure(context.Background())
	require.NoError(t, err)
	require.Equal(t, pinned, dep.ContractAddress)
	require.False(t, dep.Deployed)
	_, err = store.GetAnchorContract(context.Background(), 31337)
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestEnsureRedeploysWhenTheRecordedContractIsGone(t *testing.T) {
	f := evmtest.NewFake(t)
	store := newMemStore()
	stale := "0x00000000000000000000000000000000000000dd"
	require.NoError(t, store.UpsertAnchorContract(context.Background(), db.UpsertAnchorContractParams{
		ChainID: 31337, Address: stale,
	}))

	dep, err := newAnchor(t, f, store, nil).Ensure(context.Background())
	require.NoError(t, err)
	require.True(t, dep.Deployed)
	require.NotEqual(t, stale, dep.ContractAddress)
	row, err := store.GetAnchorContract(context.Background(), 31337)
	require.NoError(t, err)
	require.Equal(t, dep.ContractAddress, row.Address)
}

func TestEnsurePropagatesNodeAndStoreFailures(t *testing.T) {
	ctx := context.Background()

	// Chain unreachable.
	dead := evm.New(evm.Config{RPCURL: "http://127.0.0.1:1"}, newMemStore(), nil)
	_, err := dead.Ensure(ctx)
	require.Error(t, err)

	// Store broken.
	f := evmtest.NewFake(t)
	broken := newMemStore()
	broken.err = errors.New("database on fire")
	_, err = newAnchor(t, f, broken, nil).Ensure(ctx)
	require.ErrorContains(t, err, "database on fire")

	// eth_accounts failing.
	f.FailNext("eth_accounts", 1)
	_, err = newAnchor(t, f, newMemStore(), nil).Ensure(ctx)
	require.ErrorContains(t, err, "injected failure")

	// Deployment reverting.
	f.RevertNext = true
	_, err = newAnchor(t, f, newMemStore(), nil).Ensure(ctx)
	require.ErrorIs(t, err, evm.ErrReverted)

	// Deployment receipt without a contract address (a non-deploy tx).
	f2 := evmtest.NewFake(t)
	f2.FailNext("eth_getCode", 1)
	_, err = newAnchor(t, f2, newMemStore(), nil).Ensure(ctx)
	require.ErrorContains(t, err, "injected failure")

	// Store that answers the lookup (no rows) but refuses to record the
	// deployment: the anchor must report that, not carry on with an address
	// it failed to remember.
	f3 := evmtest.NewFake(t)
	failUpsert := &upsertFailingStore{memStore: newMemStore()}
	a := evm.New(evm.Config{RPCURL: f3.URL(), PollInterval: time.Millisecond}, failUpsert, nil)
	_, err = a.Ensure(ctx)
	require.ErrorContains(t, err, "record contract")
	require.True(t, strings.HasPrefix(err.Error(), "evm:"))
}

type upsertFailingStore struct{ *memStore }

func (s *upsertFailingStore) UpsertAnchorContract(context.Context, db.UpsertAnchorContractParams) error {
	return errors.New("upsert refused")
}
