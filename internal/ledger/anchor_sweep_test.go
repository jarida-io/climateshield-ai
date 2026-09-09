// SPDX-License-Identifier: Apache-2.0

package ledger_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/ledger"
	"github.com/jarida-io/climateshield/internal/ledger/anchor"
	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm"
	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm/evmtest"
	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/seed"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

// flaky is an anchor whose first call fails, like a chain that was down for
// one sweep.
type flaky struct{ calls, failures int }

func (f *flaky) Type() string { return "flaky" }
func (f *flaky) AnchorRoot(_ context.Context, _ time.Time, root []byte) (anchor.Receipt, error) {
	f.calls++
	if f.calls == 1 {
		f.failures++
		return anchor.Receipt{}, errors.New("chain down")
	}
	return anchor.Receipt{Type: "flaky", Reference: "ok", ReadBack: root, ReadBackOK: true}, nil
}

func countByType(t *testing.T, q *db.Queries, day pgtype.Date) map[string]int {
	t.Helper()
	rows, err := q.ListAnchorsForDay(context.Background(), day)
	require.NoError(t, err)
	out := map[string]int{}
	for _, r := range rows {
		out[r.AnchorType]++
	}
	return out
}

// TestSweepRetriesAFailedAnchorExactlyOnce is the regression test for the
// old behaviour, where a root whose anchoring failed was never re-anchored
// because the root itself had not changed.
func TestSweepRetriesAFailedAnchorExactlyOnce(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	log := logging.New(io.Discard, "info")

	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)

	fl := &flaky{}
	anchors := anchor.Multi{anchor.NewLocal(), fl}

	// First sweep: the flaky anchor fails for the first day. The sweep still
	// finishes every other day and reports the failure.
	_, roots, err := ledger.Sweep(ctx, q, anchors, log)
	require.ErrorContains(t, err, "chain down")
	require.Positive(t, roots)
	days, err := q.ListLeafDays(ctx)
	require.NoError(t, err)
	flakyRows := 0
	for _, day := range days {
		by := countByType(t, q, day)
		require.Equal(t, 1, by["local"], "the local anchor must never be blocked by a failing chain")
		require.LessOrEqual(t, by["flaky"], 1)
		flakyRows += by["flaky"]
	}
	require.Equal(t, len(days)-1, flakyRows, "exactly the failed day is missing its anchor")

	// Second sweep: no new leaves, no changed roots — and yet the missing
	// anchor is written. Exactly one row, no duplicates anywhere.
	leaves2, roots2, err := ledger.Sweep(ctx, q, anchors, log)
	require.NoError(t, err)
	require.Zero(t, leaves2)
	require.Zero(t, roots2)
	for _, day := range days {
		by := countByType(t, q, day)
		require.Equal(t, 1, by["local"])
		require.Equal(t, 1, by["flaky"])
	}
	require.Equal(t, len(days)+1, fl.calls, "one call per day plus the retry")

	// Third sweep: nothing to do.
	_, _, err = ledger.Sweep(ctx, q, anchors, log)
	require.NoError(t, err)
	require.Equal(t, len(days)+1, fl.calls, "an anchored root must not be anchored again")

	// The recorded row carries what the anchor reported.
	rows, err := q.ListAnchorsForDay(ctx, days[0])
	require.NoError(t, err)
	for _, r := range rows {
		stored, err := q.GetDailyRoot(ctx, days[0])
		require.NoError(t, err)
		require.Equal(t, stored.Root, r.Root, "every anchor row records the root it published")
		if r.AnchorType == "flaky" {
			require.Equal(t, stored.Root, r.ReadbackRoot)
			require.True(t, r.VerifiedAt.Valid)
		} else {
			require.Nil(t, r.ReadbackRoot, "the local anchor reads nothing back")
			require.False(t, r.VerifiedAt.Valid)
		}
	}
}

// TestSweepAnchorsEveryRootOnTheChainAndReadsItBack runs the real EVM anchor
// against the in-process fake node: deploy-once, one transaction per root,
// read-back recorded, and a new version when a day's root changes.
func TestSweepAnchorsEveryRootOnTheChainAndReadsItBack(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	log := logging.New(io.Discard, "info")

	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)

	f := evmtest.NewFake(t)
	chain := evm.New(evm.Config{RPCURL: f.URL(), PollInterval: time.Millisecond}, q, log)
	anchors := anchor.Multi{anchor.NewLocal(), chain}
	require.Equal(t, []string{"local", "evm"}, anchors.Types())

	_, _, err = ledger.Sweep(ctx, q, anchors, log)
	require.NoError(t, err)

	// One contract for this chain, remembered.
	require.Len(t, f.Deployed(), 1)
	contract, err := q.GetAnchorContract(ctx, 31337)
	require.NoError(t, err)
	require.Equal(t, f.Deployed()[0], contract.Address)
	require.NotNil(t, contract.DeployTx)

	days, err := q.ListLeafDays(ctx)
	require.NoError(t, err)
	for _, day := range days {
		stored, err := q.GetDailyRoot(ctx, day)
		require.NoError(t, err)
		row, err := q.LatestAnchorForDay(ctx, db.LatestAnchorForDayParams{LeafDay: day, AnchorType: "evm"})
		require.NoError(t, err)
		require.EqualValues(t, 31337, *row.ChainID)
		require.Equal(t, evm.DevChainLabel, *row.ChainLabel)
		require.Equal(t, contract.Address, *row.ContractAddress)
		require.NotEmpty(t, *row.TxHash)
		require.Equal(t, *row.TxHash, *row.Reference)
		require.Positive(t, *row.BlockNumber)
		require.Equal(t, stored.Root, row.Root)
		require.Equal(t, stored.Root, row.ReadbackRoot, "the row records what the chain gave back")
		require.True(t, row.VerifiedAt.Valid)

		var root32 [32]byte
		copy(root32[:], stored.Root)
		require.Equal(t, [][32]byte{root32}, f.Roots(contract.Address, evm.DayBytes32(day.Time)))
		require.Equal(t, map[string]int{"local": 1, "evm": 1}, countByType(t, q, day))
	}

	// Idempotent: nothing more is sent.
	sends := len(f.RequestsFor("eth_sendTransaction"))
	_, _, err = ledger.Sweep(ctx, q, anchors, log)
	require.NoError(t, err)
	require.Equal(t, sends, len(f.RequestsFor("eth_sendTransaction")))

	// A late event changes today's root: a second version goes on the chain
	// and the day now has two anchor rows per type, each for its own root.
	children, err := q.ListChildren(ctx)
	require.NoError(t, err)
	_, err = q.InsertImmunizationEvent(ctx, db.InsertImmunizationEventParams{
		ChildID: children[0].ID, VaccineCode: "opv3",
		AdministeredAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)
	leaves, roots, err := ledger.Sweep(ctx, q, anchors, log)
	require.NoError(t, err)
	require.Equal(t, 1, leaves)
	require.Equal(t, 1, roots)
	today := pgtype.Date{Time: time.Now().UTC().Truncate(24 * time.Hour), Valid: true}
	require.Equal(t, map[string]int{"local": 2, "evm": 2}, countByType(t, q, today))
	require.Len(t, f.Roots(contract.Address, evm.DayBytes32(today.Time)), 2)

	// A chain outage fails the sweep loudly, leaves the local record intact,
	// and the next sweep catches up without duplicating anything.
	_, err = q.InsertImmunizationEvent(ctx, db.InsertImmunizationEventParams{
		ChildID: children[1].ID, VaccineCode: "opv3",
		AdministeredAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)
	f.FailNext("eth_sendTransaction", 1)
	_, _, err = ledger.Sweep(ctx, q, anchors, log)
	require.ErrorContains(t, err, "injected failure")
	require.Equal(t, map[string]int{"local": 3, "evm": 2}, countByType(t, q, today))
	_, _, err = ledger.Sweep(ctx, q, anchors, log)
	require.NoError(t, err)
	require.Equal(t, map[string]int{"local": 3, "evm": 3}, countByType(t, q, today))
}
