// SPDX-License-Identifier: Apache-2.0

// Package ledger makes immunization history tamper-evident: each event is
// canonically serialized, HMAC-SHA256'd under a per-child key, and folded
// into a daily Merkle tree whose root is anchored. Nothing in the ledger
// tables is derivable back to a person without the child's key.
package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// CanonicalEvent is the exact field set committed to the ledger.
type CanonicalEvent struct {
	EventID        string
	ChildID        string
	VaccineCode    string
	AdministeredAt time.Time
	RecordedAt     time.Time
}

// Canonicalize produces the deterministic byte representation of an event:
// a JSON object with FIXED field order (event_id, child_id, vaccine_code,
// administered_at, recorded_at) and timestamps normalized to RFC3339
// nanosecond precision in UTC. Two calls with equal inputs yield identical
// bytes on any platform — the property every leaf hash depends on.
func Canonicalize(e CanonicalEvent) ([]byte, error) {
	if e.EventID == "" || e.ChildID == "" || e.VaccineCode == "" {
		return nil, errors.New("ledger: canonical event missing required fields")
	}
	if e.AdministeredAt.IsZero() || e.RecordedAt.IsZero() {
		return nil, errors.New("ledger: canonical event missing timestamps")
	}

	var buf bytes.Buffer
	buf.WriteByte('{')
	if err := writeStringField(&buf, "event_id", e.EventID, false); err != nil {
		return nil, err
	}
	if err := writeStringField(&buf, "child_id", e.ChildID, false); err != nil {
		return nil, err
	}
	if err := writeStringField(&buf, "vaccine_code", e.VaccineCode, false); err != nil {
		return nil, err
	}
	if err := writeStringField(&buf, "administered_at", e.AdministeredAt.UTC().Format(time.RFC3339Nano), false); err != nil {
		return nil, err
	}
	if err := writeStringField(&buf, "recorded_at", e.RecordedAt.UTC().Format(time.RFC3339Nano), true); err != nil {
		return nil, err
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func writeStringField(buf *bytes.Buffer, key, val string, last bool) error {
	k, err := json.Marshal(key)
	if err != nil {
		return fmt.Errorf("ledger: %w", err)
	}
	v, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("ledger: %w", err)
	}
	buf.Write(k)
	buf.WriteByte(':')
	buf.Write(v)
	if !last {
		buf.WriteByte(',')
	}
	return nil
}
