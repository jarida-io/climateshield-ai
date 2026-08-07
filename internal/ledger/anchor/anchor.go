// SPDX-License-Identifier: Apache-2.0

// Package anchor defines where daily Merkle roots are published. The walking
// skeleton ships LocalAnchor only (a database table). A blockchain anchor is
// a later phase — nothing in this repo calls any chain.
package anchor

import (
	"context"
	"time"
)

// Anchor publishes a daily root somewhere tamper-resistant and returns an
// implementation-specific reference (transaction ID, row ID, ...).
type Anchor interface {
	AnchorRoot(ctx context.Context, day time.Time, root []byte) (reference string, err error)
}
