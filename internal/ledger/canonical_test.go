// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func sampleEvent() CanonicalEvent {
	return CanonicalEvent{
		EventID:        "5a1e8c3d-0000-4000-8000-000000000001",
		ChildID:        "5a1e8c3d-0000-4000-8000-000000000002",
		VaccineCode:    "mr1",
		AdministeredAt: time.Date(2026, 8, 7, 12, 30, 0, 0, time.FixedZone("EAT", 3*3600)),
		RecordedAt:     time.Date(2026, 8, 7, 9, 31, 12, 500_000_000, time.UTC),
	}
}

// Golden test: the exact canonical bytes. If this changes, every historical
// leaf hash changes — treat any diff here as a breaking ledger change.
func TestCanonicalizeGolden(t *testing.T) {
	got, err := Canonicalize(sampleEvent())
	require.NoError(t, err)
	require.Equal(t,
		`{"event_id":"5a1e8c3d-0000-4000-8000-000000000001",`+
			`"child_id":"5a1e8c3d-0000-4000-8000-000000000002",`+
			`"vaccine_code":"mr1",`+
			`"administered_at":"2026-08-07T09:30:00Z",`+ // 12:30 EAT normalized to UTC
			`"recorded_at":"2026-08-07T09:31:12.5Z"}`,
		string(got))
}

func TestCanonicalizeDeterministic(t *testing.T) {
	a, err := Canonicalize(sampleEvent())
	require.NoError(t, err)
	b, err := Canonicalize(sampleEvent())
	require.NoError(t, err)
	require.Equal(t, a, b)
}

func TestCanonicalizeTimezoneNormalization(t *testing.T) {
	e1 := sampleEvent()
	e2 := sampleEvent()
	// Same instant expressed in a different zone must serialize identically.
	e2.AdministeredAt = e1.AdministeredAt.In(time.UTC)
	a, err := Canonicalize(e1)
	require.NoError(t, err)
	b, err := Canonicalize(e2)
	require.NoError(t, err)
	require.Equal(t, a, b)
}

func TestCanonicalizeFieldSensitivity(t *testing.T) {
	base, err := Canonicalize(sampleEvent())
	require.NoError(t, err)

	mutations := []func(*CanonicalEvent){
		func(e *CanonicalEvent) { e.EventID = "5a1e8c3d-0000-4000-8000-00000000000f" },
		func(e *CanonicalEvent) { e.ChildID = "5a1e8c3d-0000-4000-8000-00000000000f" },
		func(e *CanonicalEvent) { e.VaccineCode = "mr2" },
		func(e *CanonicalEvent) { e.AdministeredAt = e.AdministeredAt.Add(time.Second) },
		func(e *CanonicalEvent) { e.RecordedAt = e.RecordedAt.Add(time.Nanosecond) },
	}
	for i, mutate := range mutations {
		e := sampleEvent()
		mutate(&e)
		got, err := Canonicalize(e)
		require.NoError(t, err)
		require.NotEqual(t, base, got, "mutation %d did not change canonical bytes", i)
	}
}

func TestCanonicalizeRejectsIncompleteEvents(t *testing.T) {
	for name, mutate := range map[string]func(*CanonicalEvent){
		"no event id":     func(e *CanonicalEvent) { e.EventID = "" },
		"no child id":     func(e *CanonicalEvent) { e.ChildID = "" },
		"no vaccine":      func(e *CanonicalEvent) { e.VaccineCode = "" },
		"no administered": func(e *CanonicalEvent) { e.AdministeredAt = time.Time{} },
		"no recorded":     func(e *CanonicalEvent) { e.RecordedAt = time.Time{} },
	} {
		t.Run(name, func(t *testing.T) {
			e := sampleEvent()
			mutate(&e)
			_, err := Canonicalize(e)
			require.Error(t, err)
		})
	}
}
