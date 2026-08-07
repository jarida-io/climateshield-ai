// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/notify"
)

func TestSendAppendsJSONLAndMasksPhone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox", "outbox.jsonl")
	ch := New(path)

	id1, err := ch.Send(context.Background(),
		notify.Recipient{Phone: "+254700000101", Lang: "sw"},
		notify.Message{Body: "ClimateShield: test body"})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(id1), "mock-"))

	id2, err := ch.Send(context.Background(),
		notify.Recipient{Phone: "+254700000102", Lang: "en"},
		notify.Message{Body: "second"})
	require.NoError(t, err)
	require.NotEqual(t, id1, id2)

	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	var lines []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m map[string]any
		require.NoError(t, json.Unmarshal(sc.Bytes(), &m))
		lines = append(lines, m)
	}
	require.Len(t, lines, 2)

	require.Equal(t, "**********101", lines[0]["phone_masked"])
	require.Equal(t, "ClimateShield: test body", lines[0]["body"])
	require.Contains(t, lines[0]["note"], "NOT sent")

	// The full phone number must not appear anywhere in the file.
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "254700000101")
}
