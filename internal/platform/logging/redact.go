// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"context"
	"log/slog"
	"regexp"
)

// Typed PII wrappers. Code that must mention a person in a log line wraps the
// value, and the logger renders only a redaction marker. The raw value never
// reaches the log stream.

// Phone is a guardian phone number. It logs as "[redacted-phone]".
type Phone string

// LogValue implements slog.LogValuer.
func (Phone) LogValue() slog.Value { return slog.StringValue("[redacted-phone]") }

// PersonName is a child or guardian name. It logs as "[redacted-name]".
type PersonName string

// LogValue implements slog.LogValuer.
func (PersonName) LogValue() slog.Value { return slog.StringValue("[redacted-name]") }

// NationalID is a guardian national ID number. It logs as "[redacted-id]".
type NationalID string

// LogValue implements slog.LogValuer.
func (NationalID) LogValue() slog.Value { return slog.StringValue("[redacted-id]") }

// phonePattern matches phone-number-shaped candidates (Kenyan and generic
// E.164 forms, with optional separators), e.g. +254712345678, 0712 345 678.
var phonePattern = regexp.MustCompile(`\+?\d[\d\s\-]{7,14}\d`)

// RedactString is the defense-in-depth pass applied to every string attribute:
// anything phone-shaped is masked even if a caller forgot the typed wrappers.
// A candidate must carry at least 9 digits — ISO dates (2026-08-07, 8 digits)
// stay readable, while every Kenyan phone format (9+ digits) is masked.
func RedactString(s string) string {
	return phonePattern.ReplaceAllStringFunc(s, func(m string) string {
		if countDigits(m) >= 9 {
			return "[redacted-phone]"
		}
		return m
	})
}

func countDigits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

// RedactingHandler wraps a slog.Handler and masks phone-shaped substrings in
// all string attributes and message text.
type RedactingHandler struct {
	inner slog.Handler
}

// NewRedactingHandler wraps h with PII redaction.
func NewRedactingHandler(h slog.Handler) *RedactingHandler {
	return &RedactingHandler{inner: h}
}

// Enabled implements slog.Handler.
func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle implements slog.Handler.
func (h *RedactingHandler) Handle(ctx context.Context, rec slog.Record) error {
	clean := slog.NewRecord(rec.Time, rec.Level, RedactString(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, clean)
}

// WithAttrs implements slog.Handler.
func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = redactAttr(a)
	}
	return &RedactingHandler{inner: h.inner.WithAttrs(redacted)}
}

// WithGroup implements slog.Handler.
func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{inner: h.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, RedactString(v.String()))
	case slog.KindGroup:
		members := v.Group()
		clean := make([]any, 0, len(members))
		for _, m := range members {
			clean = append(clean, redactAttr(m))
		}
		return slog.Group(a.Key, clean...)
	default:
		return slog.Attr{Key: a.Key, Value: v}
	}
}
