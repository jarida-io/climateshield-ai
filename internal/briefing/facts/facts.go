// SPDX-License-Identifier: Apache-2.0

// Package facts holds the fact sheet a briefing generator is allowed to see,
// and nothing else.
//
// The type has no field for a child, a guardian, a phone number or an
// unsuppressed people-derived count, so none of those can reach a prompt: the
// guarantee is structural rather than a matter of remembering to strip fields
// before each call. The same sheet is published beside the briefing on the
// public API, so every number in a briefing can be traced to the numbers it
// was allowed to use.
//
// It is a leaf package on purpose. The public API decodes stored fact sheets
// with it and must not, by doing so, link a language-model client into the
// read-only tier.
package facts

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// K is the k-anonymity threshold for people-derived counts. It mirrors
// publicapi.K, which this package cannot import (publicapi imports this one).
// TestSuppressMatchesPublicAPI asserts the two agree for every count, so a
// change to the public rule cannot silently pass this package by.
const K = 10

// Suppress applies the k>=10 rule to one people-derived count, exactly as
// publicapi.Suppress does: 0 is shown, 1..K-1 is withheld, >=K is shown.
func Suppress(n int64) (value *int64, suppressed bool) {
	if n > 0 && n < K {
		return nil, true
	}
	return &n, false
}

// Languages the briefing is written in. Kiswahili is included because the
// alert path already is; the Kiswahili wording has not been reviewed by a
// Kiswahili speaker, and every surface that shows it says so.
const (
	LangEN = "en"
	LangSW = "sw"
)

// Languages lists the supported briefing languages in stable order.
var Languages = []string{LangEN, LangSW}

// ValidLanguage reports whether lang is one this package can write.
func ValidLanguage(lang string) bool {
	return lang == LangEN || lang == LangSW
}

// Window is the forecast window the scores were computed from, labelled with
// the source the observations were actually ingested from.
type Window struct {
	From   string `json:"from"` // YYYY-MM-DD
	To     string `json:"to"`   // YYYY-MM-DD
	Days   int    `json:"days"`
	Source string `json:"source"` // "fixture" or "openmeteo"
}

// Score is one county x disease assessment as the generator receives it.
type Score struct {
	Disease     string   `json:"disease"`
	Level       string   `json:"level"`
	Driver      string   `json:"driver"`
	DriverValue float64  `json:"driver_value"`
	Exceedance  *float64 `json:"exceedance,omitempty"`
	Explanation string   `json:"explanation"`
	Predictor   string   `json:"predictor"`
	Version     string   `json:"predictor_version"`
}

// AlertCount is one messaging outcome count, k>=10 suppressed. These are
// system-wide, not per county: a per-county alert count is people-derived and
// belongs on GET /v1/stats where it is suppressed county by county.
type AlertCount struct {
	Status     string `json:"status"`
	Count      *int64 `json:"count,omitempty"`
	Suppressed bool   `json:"suppressed"`
}

// FactSheet is everything a generator is allowed to know. It is built only
// from aggregate queries, and it is published beside the briefing so a reader
// can check every sentence against the numbers behind it.
type FactSheet struct {
	Area   string  `json:"area"`
	Window Window  `json:"window"`
	Scores []Score `json:"scores"`
	// AlertsAllCounties are messaging outcomes across every monitored county.
	AlertsAllCounties []AlertCount `json:"alerts_all_counties"`
	ChannelSends      bool         `json:"channel_sends"`
	ChannelNote       string       `json:"channel_note"`

	// GeneratedAt is when the sheet was assembled. It is deliberately EXCLUDED
	// from Canonical (and so from the hash): including it would make every
	// sweep look like changed facts and regenerate a briefing that already
	// describes the world correctly.
	GeneratedAt time.Time `json:"generated_at"`
}

// canonicalFacts is FactSheet minus the timestamp, in a fixed field order.
type canonicalFacts struct {
	Area              string       `json:"area"`
	Window            Window       `json:"window"`
	Scores            []Score      `json:"scores"`
	AlertsAllCounties []AlertCount `json:"alerts_all_counties"`
	ChannelSends      bool         `json:"channel_sends"`
	ChannelNote       string       `json:"channel_note"`
}

// Canonical returns the stable JSON encoding of the fact sheet and its
// SHA-256. The hash is the cache key and the regeneration trigger: identical
// facts must always produce an identical hash, on any machine, in any order.
func Canonical(f FactSheet) ([]byte, [32]byte, error) {
	c := canonicalFacts{
		Area:              f.Area,
		Window:            f.Window,
		Scores:            f.Scores,
		AlertsAllCounties: f.AlertsAllCounties,
		ChannelSends:      f.ChannelSends,
		ChannelNote:       f.ChannelNote,
	}
	if c.Scores == nil {
		c.Scores = []Score{}
	}
	if c.AlertsAllCounties == nil {
		c.AlertsAllCounties = []AlertCount{}
	}
	body, err := json.Marshal(c)
	if err != nil {
		return nil, [32]byte{}, fmt.Errorf("briefing: canonical facts: %w", err)
	}
	return body, sha256.Sum256(body), nil
}
