// SPDX-License-Identifier: Apache-2.0

// Package anchor defines where daily Merkle roots are published.
//
// Two implementations exist. Local records a root in the anchors table of the
// same database that holds the ledger. The evm subpackage writes a root to
// the RootAnchor contract on an EVM chain and reads it back before reporting
// success. The ledger sweep publishes every root through a Multi — the local
// table first, then the chain — and records one anchors row per Receipt, so
// that table is the single place that says what was published where.
//
// Only whole-day roots pass through here. No leaf, leaf hash, leaf count or
// anything else derived from a person is ever handed to an Anchor.
package anchor

import (
	"context"
	"time"
)

// Anchor types, as recorded in anchors.anchor_type.
const (
	TypeLocal = "local"
	TypeEVM   = "evm"
)

// Receipt describes one publication of one daily root. The ledger stores it
// as an anchors row; the public API republishes it (never the leaves).
type Receipt struct {
	// Type is the anchor_type recorded in the anchors table.
	Type string
	// Reference is implementation-specific: the hex root for Local, the
	// transaction hash for the EVM anchor.
	Reference string

	// Chain fields; zero for Local.
	ChainID         int64
	ChainLabel      string
	ContractAddress string
	TxHash          string
	BlockNumber     int64

	// ReadBack is the root the anchor read back from its destination right
	// after publishing, and ReadBackOK says whether it equalled the root that
	// was sent. Local sets neither: a database row is not an independent
	// place to read from, and the row must not pretend otherwise.
	ReadBack   []byte
	ReadBackOK bool
}

// Anchor publishes a daily root somewhere it can later be checked against.
type Anchor interface {
	// Type returns the anchor_type this implementation records.
	Type() string
	// AnchorRoot publishes root as the current root for day and returns
	// where it went. An error means nothing verifiable was published; the
	// sweep will try again on its next run.
	AnchorRoot(ctx context.Context, day time.Time, root []byte) (Receipt, error)
}

// Multi is the ordered set of anchors a root is published through: the local
// table first, so a chain outage never prevents the local record, then the
// chain. The sweep satisfies each member independently — whichever ones have
// no row yet for the current root are the ones it calls.
type Multi []Anchor

// Types lists the anchor types in publication order.
func (m Multi) Types() []string {
	out := make([]string, 0, len(m))
	for _, a := range m {
		out = append(out, a.Type())
	}
	return out
}
