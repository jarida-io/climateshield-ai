// SPDX-License-Identifier: Apache-2.0

// Package logging configures structured JSON logging with mandatory PII
// redaction. Services must obtain their logger from New so that every log
// line passes through the redacting handler — logging a phone number, name,
// or national ID in clear text is a funding-agreement violation.
package logging

import (
	"io"
	"log/slog"
	"strings"
)

// New returns a JSON logger at the given level ("debug", "info", "warn",
// "error") writing to w, wrapped in the PII-redacting handler.
func New(w io.Writer, level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl})
	return slog.New(NewRedactingHandler(h))
}
