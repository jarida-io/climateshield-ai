// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"context"
	"errors"
)

// Recipient is one guardian endpoint. The phone number only ever exists in
// memory during dispatch — it is decrypted from the registry immediately
// before Send and never logged.
type Recipient struct {
	Phone string
	Lang  string
}

// Message is a rendered, validated SMS body.
type Message struct {
	Body string
}

// MessageID is the channel's reference for a dispatched message.
type MessageID string

// Channel is the messaging port. Implementations: mock (JSONL to disk,
// CI/demo default), smpp (wired, not live-tested), africastalking (stub).
type Channel interface {
	Send(ctx context.Context, r Recipient, m Message) (MessageID, error)
}

// ErrNotConfigured marks channels that exist as integration points but have
// no working configuration yet.
var ErrNotConfigured = errors.New("notify: channel not configured")
