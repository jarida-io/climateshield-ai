// SPDX-License-Identifier: Apache-2.0

package at

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/notify"
)

func TestStubRefusesToSend(t *testing.T) {
	_, err := New().Send(context.Background(),
		notify.Recipient{Phone: "+254700000101"}, notify.Message{Body: "x"})
	require.ErrorIs(t, err, notify.ErrNotConfigured)
}
