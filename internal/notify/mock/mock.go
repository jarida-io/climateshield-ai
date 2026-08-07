// SPDX-License-Identifier: Apache-2.0

// Package mock is the default Channel for CI and demos: it appends one JSON
// line per message to a local file and sends NOTHING. Its message IDs are
// prefixed "mock-" and the phone number is masked in the outbox — the file
// documents what WOULD be sent without duplicating registry PII on disk.
package mock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jarida-io/climateshield/internal/notify"
)

// ChannelName identifies this adapter in config and alert rows.
const ChannelName = "mock"

// Channel writes would-be messages to a JSONL file.
type Channel struct {
	mu   sync.Mutex
	path string
}

// New creates the mock channel writing to path (parent dirs are created).
func New(path string) *Channel { return &Channel{path: path} }

type outboxLine struct {
	MessageID   string    `json:"message_id"`
	PhoneMasked string    `json:"phone_masked"`
	Lang        string    `json:"lang"`
	Body        string    `json:"body"`
	WouldSendAt time.Time `json:"would_send_at"`
	Note        string    `json:"note"`
}

// Send implements notify.Channel. It never contacts any carrier.
func (c *Channel) Send(_ context.Context, r notify.Recipient, m notify.Message) (notify.MessageID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return "", fmt.Errorf("mock: %w", err)
	}
	f, err := os.OpenFile(c.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", fmt.Errorf("mock: %w", err)
	}
	defer func() { _ = f.Close() }()

	id, err := newID()
	if err != nil {
		return "", err
	}
	line := outboxLine{
		MessageID:   id,
		PhoneMasked: maskPhone(r.Phone),
		Lang:        r.Lang,
		Body:        m.Body,
		WouldSendAt: time.Now().UTC(),
		Note:        "mock channel: NOT sent",
	}
	enc, err := json.Marshal(line)
	if err != nil {
		return "", fmt.Errorf("mock: %w", err)
	}
	if _, err := f.Write(append(enc, '\n')); err != nil {
		return "", fmt.Errorf("mock: %w", err)
	}
	return notify.MessageID(id), nil
}

func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("mock: %w", err)
	}
	return "mock-" + hex.EncodeToString(b[:]), nil
}

// maskPhone keeps only the last 3 digits: enough to eyeball a demo, useless
// for contacting anyone.
func maskPhone(p string) string {
	if len(p) <= 3 {
		return "***"
	}
	return strings.Repeat("*", len(p)-3) + p[len(p)-3:]
}
