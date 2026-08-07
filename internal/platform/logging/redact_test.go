// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// containsPhoneShaped is the auditor's view: does anything phone-shaped (9+
// digits, allowing space/dash separators) survive into the log stream?
// RFC3339 timestamps and ISO dates carry at most 8 consecutive digits/dashes
// between colons, so they don't count.
func containsPhoneShaped(s string) bool {
	for _, m := range regexp.MustCompile(`\+?\d[\d\s\-]{7,14}\d`).FindAllString(s, -1) {
		digits := 0
		for _, r := range m {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits >= 9 {
			return true
		}
	}
	return false
}

func TestTypedWrappersNeverEmitValue(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "debug")

	log.Info("guardian contacted",
		"phone", Phone("+254712345678"),
		"name", PersonName("Wanjiku Kamau"),
		"national_id", NationalID("12345678"),
	)

	out := buf.String()
	require.NotContains(t, out, "254712345678")
	require.NotContains(t, out, "Wanjiku")
	require.NotContains(t, out, "Kamau")
	require.NotContains(t, out, "12345678")
	require.Contains(t, out, "[redacted-phone]")
	require.Contains(t, out, "[redacted-name]")
	require.Contains(t, out, "[redacted-id]")
}

func TestRawStringAttrsAreScrubbed(t *testing.T) {
	// Defense in depth: even when a caller logs a bare string, phone-shaped
	// content must not survive.
	var buf bytes.Buffer
	log := New(&buf, "info")

	log.Info("sms queued for 0712 345 678", "recipient", "+254 733 000 004")
	log.With("contact", "0722-123456").Info("with-attrs path")

	require.False(t, containsPhoneShaped(buf.String()),
		"log output contains a phone-shaped string: %s", buf.String())
}

func TestLogsAreJSONAndLeveled(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "warn")

	log.Info("dropped")
	log.Warn("kept")

	out := buf.String()
	require.NotContains(t, out, "dropped")
	require.Contains(t, out, `"level":"WARN"`)
	require.Contains(t, out, `"msg":"kept"`)
}
