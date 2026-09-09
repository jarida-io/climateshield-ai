// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	"context"
	"encoding/hex"
	"errors"
	"time"
)

// Local is the database-table anchor. It publishes nowhere outside the
// database: the sweep records its Receipt as an anchors row and that row is
// the anchor. It is always present, so every root has a local record even
// when no chain is configured or the chain is unreachable.
type Local struct{}

// NewLocal builds the local anchor.
func NewLocal() *Local { return &Local{} }

// Type implements Anchor.
func (*Local) Type() string { return TypeLocal }

// AnchorRoot implements Anchor. The reference is the hex root, which is what
// the row is for: a human-readable copy of the commitment next to its day.
func (*Local) AnchorRoot(_ context.Context, _ time.Time, root []byte) (Receipt, error) {
	if len(root) == 0 {
		return Receipt{}, errors.New("anchor: empty root")
	}
	return Receipt{Type: TypeLocal, Reference: hex.EncodeToString(root)}, nil
}
