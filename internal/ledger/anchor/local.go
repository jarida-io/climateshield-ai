// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jarida-io/climateshield/internal/store/db"
)

// Local writes roots to the anchors table — the honest skeleton anchor.
type Local struct {
	q *db.Queries
}

// NewLocal builds a Local anchor over a database handle.
func NewLocal(dbtx db.DBTX) *Local { return &Local{q: db.New(dbtx)} }

// AnchorRoot implements Anchor.
func (l *Local) AnchorRoot(ctx context.Context, day time.Time, root []byte) (string, error) {
	ref := hex.EncodeToString(root)
	err := l.q.InsertAnchor(ctx, db.InsertAnchorParams{
		LeafDay:    pgtype.Date{Time: day, Valid: true},
		AnchorType: "local",
		Reference:  &ref,
	})
	if err != nil {
		return "", fmt.Errorf("anchor: %w", err)
	}
	return ref, nil
}
