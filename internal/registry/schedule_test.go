// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var testSchedule = []ScheduleEntry{
	{Code: "bcg", Name: "BCG", DueAgeDays: 0, GraceDays: 14},
	{Code: "opv1", Name: "OPV 1", DueAgeDays: 42, GraceDays: 14},
	{Code: "mr1", Name: "Measles-Rubella 1", DueAgeDays: 270, GraceDays: 14},
}

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestDueVaccinesBoundaries(t *testing.T) {
	dob := day(2026, 1, 1)
	none := map[string]bool{}

	find := func(due []DueStatus, code string) (DueStatus, bool) {
		for _, d := range due {
			if d.Code == code {
				return d, true
			}
		}
		return DueStatus{}, false
	}

	t.Run("not due the day before the due age", func(t *testing.T) {
		due := DueVaccines(testSchedule, dob, none, day(2026, 2, 11)) // age 41
		_, ok := find(due, "opv1")
		require.False(t, ok)
	})

	t.Run("due exactly at the due age", func(t *testing.T) {
		due := DueVaccines(testSchedule, dob, none, day(2026, 2, 12)) // age 42
		d, ok := find(due, "opv1")
		require.True(t, ok)
		require.Equal(t, day(2026, 2, 12), d.DueDate)
		require.False(t, d.Overdue)
	})

	t.Run("still within grace on the last grace day", func(t *testing.T) {
		due := DueVaccines(testSchedule, dob, none, day(2026, 2, 26)) // age 56 = 42+14
		d, ok := find(due, "opv1")
		require.True(t, ok)
		require.False(t, d.Overdue)
	})

	t.Run("overdue the day after grace expires", func(t *testing.T) {
		due := DueVaccines(testSchedule, dob, none, day(2026, 2, 27)) // age 57
		d, ok := find(due, "opv1")
		require.True(t, ok)
		require.True(t, d.Overdue)
	})

	t.Run("birth-dose due on the day of birth", func(t *testing.T) {
		due := DueVaccines(testSchedule, dob, none, dob)
		d, ok := find(due, "bcg")
		require.True(t, ok)
		require.False(t, d.Overdue)
	})
}

func TestDueVaccinesSkipsGivenAndFuture(t *testing.T) {
	dob := day(2026, 1, 1)
	given := map[string]bool{"bcg": true}

	due := DueVaccines(testSchedule, dob, given, day(2026, 3, 1)) // age 59
	codes := make([]string, 0, len(due))
	for _, d := range due {
		codes = append(codes, d.Code)
	}
	require.Equal(t, []string{"opv1"}, codes, "bcg given, mr1 not yet due")
}

func TestDueVaccinesFullyVaccinated(t *testing.T) {
	dob := day(2025, 1, 1)
	given := map[string]bool{"bcg": true, "opv1": true, "mr1": true}
	require.Empty(t, DueVaccines(testSchedule, dob, given, day(2026, 8, 7)))
}

func TestDueVaccinesIgnoresTimeOfDay(t *testing.T) {
	dob := time.Date(2026, 1, 1, 23, 55, 0, 0, time.UTC)
	due := DueVaccines(testSchedule, dob, nil, time.Date(2026, 2, 12, 0, 5, 0, 0, time.UTC))
	var found bool
	for _, d := range due {
		if d.Code == "opv1" {
			found = true
		}
	}
	require.True(t, found, "day-granularity comparison must ignore clock time")
}
