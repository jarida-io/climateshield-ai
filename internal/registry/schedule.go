// SPDX-License-Identifier: Apache-2.0

// Package registry implements the immunization registry: KEPI due/overdue
// computation (pure, DB-free) and the internal Connect API. PII lives only in
// encrypted columns; this package's API surface carries opaque IDs.
package registry

import (
	"time"
)

// ScheduleEntry is one KEPI schedule row (mirrors the vaccine_schedule table).
type ScheduleEntry struct {
	Code       string
	Name       string
	DueAgeDays int
	GraceDays  int
}

// DueStatus is one vaccine a child still needs.
type DueStatus struct {
	Code    string
	DueDate time.Time
	Overdue bool
}

// DueVaccines returns, for a child born on dob with the given vaccines
// already administered, every schedule entry that is due on `today` —
// i.e. the child has reached the due age and the dose is absent. A dose
// becomes overdue once `today` is past due date + grace days.
//
// Dates are compared at day granularity in the caller's zone of record
// (Africa/Nairobi calendar dates throughout the system).
func DueVaccines(schedule []ScheduleEntry, dob time.Time, given map[string]bool, today time.Time) []DueStatus {
	dob = truncateDay(dob)
	today = truncateDay(today)

	var due []DueStatus
	for _, e := range schedule {
		if given[e.Code] {
			continue
		}
		dueDate := dob.AddDate(0, 0, e.DueAgeDays)
		if today.Before(dueDate) {
			continue
		}
		due = append(due, DueStatus{
			Code:    e.Code,
			DueDate: dueDate,
			Overdue: today.After(dueDate.AddDate(0, 0, e.GraceDays)),
		})
	}
	return due
}

func truncateDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
