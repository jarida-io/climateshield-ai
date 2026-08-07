// SPDX-License-Identifier: Apache-2.0

package ledger_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/ledger"
	"github.com/jarida-io/climateshield/internal/ledger/anchor"
	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/seed"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

func TestSweepCommitsAllEventsAndAnchorsRoots(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	log := logging.New(io.Discard, "info")

	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	sum, err := seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)

	leaves, roots, err := ledger.Sweep(ctx, q, anchor.NewLocal(pool), log)
	require.NoError(t, err)
	require.Equal(t, sum.Events, leaves, "one leaf per immunization event")
	require.Positive(t, roots)

	// Every event now has a leaf; the day root covers them; an anchor exists.
	remaining, err := q.ListEventsWithoutLeaves(ctx)
	require.NoError(t, err)
	require.Empty(t, remaining)

	days, err := q.ListLeafDays(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, days)
	for _, day := range days {
		stored, err := q.GetDailyRoot(ctx, day)
		require.NoError(t, err)

		rows, err := q.LeavesForDay(ctx, day)
		require.NoError(t, err)
		require.EqualValues(t, stored.LeafCount, len(rows))

		hashes := make([][]byte, 0, len(rows))
		for _, r := range rows {
			hashes = append(hashes, r.LeafHash)
		}
		require.Equal(t, ledger.Root(hashes), stored.Root, "stored root must equal recomputed root")

		anchors, err := q.ListAnchorsForDay(ctx, day)
		require.NoError(t, err)
		require.NotEmpty(t, anchors)
		require.Equal(t, "local", anchors[0].AnchorType)
	}

	// Idempotence: nothing new on a second sweep, no duplicate anchors.
	leaves2, roots2, err := ledger.Sweep(ctx, q, anchor.NewLocal(pool), log)
	require.NoError(t, err)
	require.Zero(t, leaves2)
	require.Zero(t, roots2)
}

func TestSweepInclusionProofEndToEnd(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)
	_, _, err = ledger.Sweep(ctx, q, anchor.NewLocal(pool), logging.New(io.Discard, "info"))
	require.NoError(t, err)

	days, err := q.ListLeafDays(ctx)
	require.NoError(t, err)
	day := days[0]

	rows, err := q.LeavesForDay(ctx, day)
	require.NoError(t, err)
	hashes := make([][]byte, 0, len(rows))
	for _, r := range rows {
		hashes = append(hashes, r.LeafHash)
	}
	stored, err := q.GetDailyRoot(ctx, day)
	require.NoError(t, err)

	// Any stored leaf can prove inclusion under the stored root.
	for i := range hashes {
		proof, err := ledger.BuildProof(hashes, i)
		require.NoError(t, err)
		require.True(t, ledger.VerifyProof(hashes[i], proof, stored.Root))
	}
}

func TestForgetChildErasesAndPreservesStructure(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	log := logging.New(io.Discard, "info")

	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)
	_, _, err = ledger.Sweep(ctx, q, anchor.NewLocal(pool), log)
	require.NoError(t, err)

	children, err := q.ListChildren(ctx)
	require.NoError(t, err)
	victim := children[0]

	// Capture pre-erasure state.
	eventsBefore, err := q.ListEventsForChild(ctx, victim.ID)
	require.NoError(t, err)
	require.NotEmpty(t, eventsBefore)
	var leavesBefore int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM event_leaves`).Scan(&leavesBefore))

	require.NoError(t, ledger.ForgetChild(ctx, pool, victim.ID))

	// The child, their events, and their key are gone.
	_, err = q.GetChild(ctx, victim.ID)
	require.Error(t, err)
	events, err := q.ListEventsForChild(ctx, victim.ID)
	require.NoError(t, err)
	require.Empty(t, events)
	_, err = q.GetChildKey(ctx, victim.ID)
	require.Error(t, err, "HMAC key must be destroyed")

	// The child is no longer derivable from the ledger: no leaf references
	// them, and without the key the leaf hashes cannot be recomputed or
	// matched against any reconstruction of their data.
	var linked int64
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM event_leaves WHERE child_id = $1`, victim.ID).Scan(&linked))
	require.Zero(t, linked)

	// Structure is preserved: the anonymous leaves remain and every stored
	// daily root still verifies against its stored leaves.
	var leavesAfter int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM event_leaves`).Scan(&leavesAfter))
	require.Equal(t, leavesBefore, leavesAfter, "erasure must not remove ledger leaves")

	days, err := q.ListLeafDays(ctx)
	require.NoError(t, err)
	for _, day := range days {
		stored, err := q.GetDailyRoot(ctx, day)
		require.NoError(t, err)
		rows, err := q.LeavesForDay(ctx, day)
		require.NoError(t, err)
		hashes := make([][]byte, 0, len(rows))
		for _, r := range rows {
			hashes = append(hashes, r.LeafHash)
		}
		require.Equal(t, ledger.Root(hashes), stored.Root,
			"daily root must still verify structurally after erasure")
	}
}

func TestForgetChildKeepsGuardianAndSiblings(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)

	children, err := q.ListChildren(ctx)
	require.NoError(t, err)

	// Pick a child whose guardian has two children (seed guardians mostly do).
	var victim, sibling pgtype.UUID
	byGuardian := map[string][]pgtype.UUID{}
	for _, c := range children {
		gid := c.GuardianID
		k := string(gid.Bytes[:])
		byGuardian[k] = append(byGuardian[k], c.ID)
		if len(byGuardian[k]) == 2 {
			victim, sibling = byGuardian[k][0], byGuardian[k][1]
		}
	}
	require.True(t, victim.Valid)

	require.NoError(t, ledger.ForgetChild(ctx, pool, victim))

	_, err = q.GetChild(ctx, sibling)
	require.NoError(t, err, "sibling must survive")
}
