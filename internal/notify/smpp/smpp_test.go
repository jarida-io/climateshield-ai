// SPDX-License-Identifier: Apache-2.0

package smpp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/notify"
)

// The SMPP adapter is wired but deliberately not tested against a live
// carrier (walking-skeleton scope). What we do verify: construction is
// side-effect free and Send fails cleanly when no SMSC is reachable rather
// than pretending success.
func TestSendFailsCleanlyWithoutSMSC(t *testing.T) {
	ch := New("127.0.0.1:1", "test", "test") // nothing listens on port 1
	_, err := ch.Send(context.Background(),
		notify.Recipient{Phone: "+254700000101"}, notify.Message{Body: "x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "smpp")
}
