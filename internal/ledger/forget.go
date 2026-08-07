// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// ForgetChild is the right-to-erasure path. In one transaction it:
//
//  1. deletes the child's immunization events (guarded erasure — the only
//     way past the append-only trigger),
//  2. scrubs the child linkage from ledger leaves (the anonymous hashes stay,
//     so published daily roots still verify structurally),
//  3. destroys the child's HMAC key — after which no party can ever again
//     link or verify those leaves against the child's data,
//  4. deletes the child record itself.
//
// The guardian record survives (they may have other children); alert rows
// keep a NULL child reference by FK design.
func ForgetChild(ctx context.Context, pool *pgxpool.Pool, childID pgtype.UUID) error {
	return store.WithErasure(ctx, pool, func(q *db.Queries) error {
		if _, err := q.EraseChildEvents(ctx, childID); err != nil {
			return fmt.Errorf("ledger: erase events: %w", err)
		}
		if _, err := q.ScrubChildFromLeaves(ctx, childID); err != nil {
			return fmt.Errorf("ledger: scrub leaves: %w", err)
		}
		if _, err := q.DestroyChildKey(ctx, childID); err != nil {
			return fmt.Errorf("ledger: destroy key: %w", err)
		}
		if err := q.DeleteChild(ctx, childID); err != nil {
			return fmt.Errorf("ledger: delete child: %w", err)
		}
		return nil
	})
}
