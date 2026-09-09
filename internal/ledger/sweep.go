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
// event, keyed per child), recomputes the daily root of every day, and makes
// sure the CURRENT root of every day has been published through every
// configured anchor. Idempotent: a second sweep with no new events and every
// root already anchored writes nothing.
//
// Anchoring is decided per (day, anchor type) from the anchors table, not
// from whether the root changed on this run — so a root whose anchoring
// failed once (chain down, timeout) is retried on the next sweep instead of
// being forgotten, and no anchor is ever recorded twice for the same root.
// Anchor failures are collected and returned together after every day has
// been attempted, so a chain outage never stops the local anchor of another
// day; the returned error makes River retry the job.
//
// Chain calls happen outside any database transaction: nothing here holds a
// transaction open while waiting for a block.
func Sweep(ctx context.Context, q *db.Queries, anchors anchor.Multi, log *slog.Logger) (leaves, roots int, err error) {
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
	var anchorErrs []error
	for _, day := range days {
		root, changed, err := recomputeRoot(ctx, q, day)
		if err != nil {
			return leaves, roots, err
		}
		if changed {
			roots++
		}
		if err := anchorDay(ctx, q, anchors, day, root); err != nil {
			anchorErrs = append(anchorErrs, err)
		}
	}
	if leaves > 0 || roots > 0 {
		log.Info("ledger sweep complete", "new_leaves", leaves, "roots_updated", roots)
	}
	return leaves, roots, errors.Join(anchorErrs...)
}

// recomputeRoot rebuilds one day's root from its stored leaves and upserts it
// when it differs from the stored root (or none exists). It returns the
// current root either way so the caller can check its anchors.
func recomputeRoot(ctx context.Context, q *db.Queries, day pgtype.Date) (root []byte, changed bool, err error) {
	rows, err := q.LeavesForDay(ctx, day)
	if err != nil {
		return nil, false, fmt.Errorf("ledger: leaves for day: %w", err)
	}
	hashes := make([][]byte, 0, len(rows))
	for _, r := range rows {
		hashes = append(hashes, r.LeafHash)
	}
	root = Root(hashes)

	existing, err := q.GetDailyRoot(ctx, day)
	if err == nil && bytes.Equal(existing.Root, root) {
		return root, false, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("ledger: get root: %w", err)
	}

	err = q.UpsertDailyRoot(ctx, db.UpsertDailyRootParams{
		LeafDay:   day,
		Root:      root,
		LeafCount: int32(len(hashes)),
	})
	if err != nil {
		return nil, false, fmt.Errorf("ledger: upsert root: %w", err)
	}
	return root, true, nil
}

// anchorDay publishes root through every anchor that has no row yet for this
// exact (day, type, root), recording one anchors row per receipt.
func anchorDay(ctx context.Context, q *db.Queries, anchors anchor.Multi, day pgtype.Date, root []byte) error {
	for _, a := range anchors {
		exists, err := q.AnchorExistsForRoot(ctx, db.AnchorExistsForRootParams{
			LeafDay: day, AnchorType: a.Type(), Root: root,
		})
		if err != nil {
			return fmt.Errorf("ledger: anchor lookup: %w", err)
		}
		if exists {
			continue
		}
		rcpt, err := a.AnchorRoot(ctx, day.Time, root)
		if err != nil {
			return fmt.Errorf("ledger: anchor %s for %s: %w", a.Type(), day.Time.Format("2006-01-02"), err)
		}
		if err := recordAnchor(ctx, q, day, root, rcpt); err != nil {
			return err
		}
	}
	return nil
}

// recordAnchor writes the anchors row for one receipt. The row carries the
// root that was published and, for anchors that read back, what came back.
func recordAnchor(ctx context.Context, q *db.Queries, day pgtype.Date, root []byte, rcpt anchor.Receipt) error {
	p := db.InsertAnchorParams{LeafDay: day, AnchorType: rcpt.Type, Root: root}
	if rcpt.Reference != "" {
		p.Reference = &rcpt.Reference
	}
	if rcpt.ChainID != 0 {
		p.ChainID = &rcpt.ChainID
	}
	if rcpt.ChainLabel != "" {
		p.ChainLabel = &rcpt.ChainLabel
	}
	if rcpt.ContractAddress != "" {
		p.ContractAddress = &rcpt.ContractAddress
	}
	if rcpt.TxHash != "" {
		p.TxHash = &rcpt.TxHash
	}
	if rcpt.BlockNumber != 0 {
		p.BlockNumber = &rcpt.BlockNumber
	}
	if rcpt.ReadBackOK {
		p.ReadbackRoot = rcpt.ReadBack
		p.VerifiedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	if err := q.InsertAnchor(ctx, p); err != nil {
		return fmt.Errorf("ledger: record %s anchor: %w", rcpt.Type, err)
	}
	return nil
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
