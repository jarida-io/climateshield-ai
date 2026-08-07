// SPDX-License-Identifier: Apache-2.0

// Package smpp adapts notify.Channel onto an SMPP 3.4 transmitter
// (fiorix/go-smpp). It is WIRED BUT NOT TESTED against a live carrier — the
// walking skeleton never sends real SMS. Binding happens lazily on first
// Send so that constructing the adapter needs no reachable SMSC.
package smpp

import (
	"context"
	"fmt"
	"sync"

	gosmpp "github.com/fiorix/go-smpp/smpp"
	"github.com/fiorix/go-smpp/smpp/pdu/pdutext"

	"github.com/jarida-io/climateshield/internal/notify"
)

// ChannelName identifies this adapter in config and alert rows.
const ChannelName = "smpp"

// Channel sends via an SMPP transmitter bind.
type Channel struct {
	mu    sync.Mutex
	tx    *gosmpp.Transmitter
	bound bool
}

// New configures (but does not yet bind) an SMPP transmitter.
func New(addr, systemID, password string) *Channel {
	return &Channel{tx: &gosmpp.Transmitter{Addr: addr, User: systemID, Passwd: password}}
}

// Send implements notify.Channel.
func (c *Channel) Send(_ context.Context, r notify.Recipient, m notify.Message) (notify.MessageID, error) {
	if err := c.ensureBound(); err != nil {
		return "", err
	}
	sm, err := c.tx.Submit(&gosmpp.ShortMessage{
		Dst:  r.Phone,
		Text: pdutext.GSM7(m.Body),
	})
	if err != nil {
		return "", fmt.Errorf("smpp: submit: %w", err)
	}
	return notify.MessageID(sm.RespID()), nil
}

func (c *Channel) ensureBound() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bound {
		return nil
	}
	status := c.tx.Bind()
	st := <-status // first status: Connected or a failure
	if st.Error() != nil {
		return fmt.Errorf("smpp: bind %s: %w", c.tx.Addr, st.Error())
	}
	c.bound = true
	return nil
}
