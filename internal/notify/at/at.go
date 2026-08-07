// SPDX-License-Identifier: Apache-2.0

// Package at is the Africa's Talking integration point. Interface stub only:
// the walking skeleton has no carrier account and must run with zero
// credentials, so every call reports ErrNotConfigured.
package at

import (
	"context"

	"github.com/jarida-io/climateshield/internal/notify"
)

// ChannelName identifies this adapter in config.
const ChannelName = "africastalking"

// Channel is the not-yet-configured Africa's Talking adapter.
type Channel struct{}

// New returns the stub adapter.
func New() *Channel { return &Channel{} }

// Send implements notify.Channel by refusing: no credentials exist.
func (*Channel) Send(context.Context, notify.Recipient, notify.Message) (notify.MessageID, error) {
	return "", notify.ErrNotConfigured
}
