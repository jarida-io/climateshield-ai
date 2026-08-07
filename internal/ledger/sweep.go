// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jarida-io/climateshield/internal/ledger/anchor"
	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// Sweep commits every un-committed immunization event to the ledger (leaf per
// event, keyed per child) and recomputes + anchors the daily root of every
// day whose leaf set changed. Idempotent: a second sweep with no new events
// writes nothing.
func Sweep(ctx context.Context, q *db.Queries, anc anchor.Anchor, log *slog.Logger) (leaves, roots int, err error) {
	events, err := q.ListEventsWithoutLeaves(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("ledger: list events: %w", err)
	}
	for _, ev := range events {
		key, err := childKey(ctx, q, ev.ChildID)
		if err != nil {
			return leaves, roots, err
		}
		canonical, err := Canonicalize(CanonicalEvent{
			EventID:        uuidString(ev.ID),
			ChildID:        uuidString(ev.ChildID),
			VaccineCode:    ev.VaccineCode,
			AdministeredAt: ev.AdministeredAt.Time,
			RecordedAt:     ev.RecordedAt.Time,
		})
		if err != nil {
			return leaves, roots, err
		}
		rec := ev.RecordedAt.Time.UTC()
		leafDay := time.Date(rec.Year(), rec.Month(), rec.Day(), 0, 0, 0, 0, time.UTC)
		err = q.InsertLeaf(ctx, db.InsertLeafParams{
			EventID:  ev.ID,
			ChildID:  ev.ChildID,
			LeafDay:  pgtype.Date{Time: leafDay, Valid: true},
			LeafHash: Leaf(key, canonical),
		})
		if err != nil {
			return leaves, roots, fmt.Errorf("ledger: insert leaf: %w", err)
		}
		leaves++
	}

	days, err := q.ListLeafDays(ctx)
	if err != nil {
		return leaves, roots, fmt.Errorf("ledger: list days: %w", err)
	}
	for _, day := range days {
		changed, err := recomputeRoot(ctx, q, anc, day)
		if err != nil {
			return leaves, roots, err
		}
		if changed {
			roots++
		}
	}
	if leaves > 0 || roots > 0 {
		log.Info("ledger sweep complete", "new_leaves", leaves, "roots_updated", roots)
	}
	return leaves, roots, nil
}

// recomputeRoot rebuilds one day's root from its stored leaves; if it differs
// from the stored root (or none exists), it upserts and anchors it.
func recomputeRoot(ctx context.Context, q *db.Queries, anc anchor.Anchor, day pgtype.Date) (bool, error) {
	rows, err := q.LeavesForDay(ctx, day)
	if err != nil {
		return false, fmt.Errorf("ledger: leaves for day: %w", err)
	}
	hashes := make([][]byte, 0, len(rows))
	for _, r := range rows {
		hashes = append(hashes, r.LeafHash)
	}
	root := Root(hashes)

	existing, err := q.GetDailyRoot(ctx, day)
	if err == nil && bytes.Equal(existing.Root, root) {
		return false, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("ledger: get root: %w", err)
	}

	err = q.UpsertDailyRoot(ctx, db.UpsertDailyRootParams{
		LeafDay:   day,
		Root:      root,
		LeafCount: int32(len(hashes)),
	})
	if err != nil {
		return false, fmt.Errorf("ledger: upsert root: %w", err)
	}
	if _, err := anc.AnchorRoot(ctx, day.Time, root); err != nil {
		return false, err
	}
	return true, nil
}

// childKey fetches the child's HMAC key, creating one on first use.
func childKey(ctx context.Context, q *db.Queries, childID pgtype.UUID) ([]byte, error) {
	key, err := q.GetChildKey(ctx, childID)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("ledger: get child key: %w", err)
	}
	fresh, err := crypto.NewRandomKey()
	if err != nil {
		return nil, err
	}
	if err := q.InsertChildKey(ctx, db.InsertChildKeyParams{ChildID: childID, HmacKey: fresh[:]}); err != nil {
		return nil, fmt.Errorf("ledger: insert child key: %w", err)
	}
	// Re-read: ON CONFLICT DO NOTHING means a concurrent sweep may have won.
	key, err = q.GetChildKey(ctx, childID)
	if err != nil {
		return nil, fmt.Errorf("ledger: reread child key: %w", err)
	}
	return key, nil
}

func uuidString(u pgtype.UUID) string {
	v, err := u.Value()
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
