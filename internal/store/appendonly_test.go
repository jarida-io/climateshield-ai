// SPDX-License-Identifier: Apache-2.0

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

// seedOneEvent inserts guardian -> child -> one immunization event and
// returns the child and event IDs.
func seedOneEvent(t *testing.T, pool interface {
	db.DBTX
}) (pgtype.UUID, pgtype.UUID) {
	t.Helper()
	ctx := context.Background()
	q := db.New(pool)
	key, err := crypto.NewRandomKey()
	require.NoError(t, err)

	seal := func(v string) []byte {
		enc, err := crypto.Seal(key, v)
		require.NoError(t, err)
		return enc.Bytes()
	}

	gid, err := q.CreateGuardian(ctx, db.CreateGuardianParams{
		NameEnc: seal("Test Guardian"), PhoneEnc: seal("+254700000199"), Lang: "en",
	})
	require.NoError(t, err)

	cid, err := q.CreateChild(ctx, db.CreateChildParams{
		GuardianID: gid, AreaID: "kisumu", NameEnc: seal("Test Child"),
		DateOfBirth: pgtype.Date{Time: time.Now().AddDate(0, -5, 0), Valid: true},
	})
	require.NoError(t, err)

	row, err := q.InsertImmunizationEvent(ctx, db.InsertImmunizationEventParams{
		ChildID: cid, VaccineCode: "bcg",
		AdministeredAt: pgtype.Timestamptz{Time: time.Now().AddDate(0, -5, 0), Valid: true},
	})
	require.NoError(t, err)
	return cid, row.ID
}

func TestImmunizationEventsRejectUpdate(t *testing.T) {
	pool := testdb.Pool(t)
	_, eventID := seedOneEvent(t, pool)

	_, err := pool.Exec(context.Background(),
		`UPDATE immunization_events SET vaccine_code = 'opv0' WHERE id = $1`, eventID)
	require.ErrorContains(t, err, "append-only")
}

func TestImmunizationEventsRejectDelete(t *testing.T) {
	pool := testdb.Pool(t)
	_, eventID := seedOneEvent(t, pool)

	_, err := pool.Exec(context.Background(),
		`DELETE FROM immunization_events WHERE id = $1`, eventID)
	require.ErrorContains(t, err, "append-only")
}

func TestErasurePathAllowsDelete(t *testing.T) {
	// The one sanctioned exception: a WithErasure transaction (ForgetChild)
	// may delete a child's events. UPDATE stays forbidden even there.
	pool := testdb.Pool(t)
	childID, _ := seedOneEvent(t, pool)
	ctx := context.Background()

	err := store.WithErasure(ctx, pool, func(q *db.Queries) error {
		n, err := q.EraseChildEvents(ctx, childID)
		if err != nil {
			return err
		}
		require.Equal(t, int64(1), n)
		return nil
	})
	require.NoError(t, err)

	var remaining int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM immunization_events WHERE child_id = $1`, childID).Scan(&remaining))
	require.Zero(t, remaining)
}

func TestErasureFlagDoesNotPermitUpdate(t *testing.T) {
	pool := testdb.Pool(t)
	_, eventID := seedOneEvent(t, pool)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, "SET LOCAL climateshield.allow_erasure = 'on'")
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `UPDATE immunization_events SET vaccine_code = 'opv0' WHERE id = $1`, eventID)
	require.ErrorContains(t, err, "append-only")
}

func TestErasureFlagScopedToTransaction(t *testing.T) {
	// After a WithErasure transaction commits, plain DELETE must fail again —
	// the flag must not leak into the session.
	pool := testdb.Pool(t)
	_, eventID := seedOneEvent(t, pool)
	ctx := context.Background()

	require.NoError(t, store.WithErasure(ctx, pool, func(*db.Queries) error { return nil }))

	_, err := pool.Exec(ctx, `DELETE FROM immunization_events WHERE id = $1`, eventID)
	require.ErrorContains(t, err, "append-only")
}
