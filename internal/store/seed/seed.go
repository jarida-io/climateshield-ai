// SPDX-License-Identifier: Apache-2.0

// Package seed provides reference data (the five monitored counties) and the
// deterministic demo population used by `make demo` and the test suite.
// Every person below is FICTIONAL; phones use a fake +254 7000001xx range.
// The area counts are chosen deliberately so the public API's k>=10
// suppression has both visible (>=10) and suppressed (<10) counties.
package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// County is one monitored area with its centroid (canonical coordinates,
// carried over from the Python prototype).
type County struct {
	ID   string
	Name string
	Lat  float64
	Lon  float64
}

// Counties is the fixed monitoring set for the walking skeleton.
var Counties = []County{
	{ID: "nairobi", Name: "Nairobi", Lat: -1.2921, Lon: 36.8219},
	{ID: "kisumu", Name: "Kisumu", Lat: -0.1022, Lon: 34.7617},
	{ID: "mombasa", Name: "Mombasa", Lat: -4.0435, Lon: 39.6682},
	{ID: "nakuru", Name: "Nakuru", Lat: -0.3031, Lon: 36.0800},
	{ID: "eldoret", Name: "Eldoret", Lat: 0.5143, Lon: 35.2698},
}

// Areas upserts the five counties. Idempotent; run by cmd/migrate and the
// test harness.
func Areas(ctx context.Context, pool *pgxpool.Pool) error {
	q := db.New(pool)
	for _, c := range Counties {
		err := q.UpsertArea(ctx, db.UpsertAreaParams{
			ID:        c.ID,
			Name:      c.Name,
			Level:     "county",
			Longitude: c.Lon,
			Latitude:  c.Lat,
		})
		if err != nil {
			return fmt.Errorf("seed area %s: %w", c.ID, err)
		}
	}
	return nil
}

// Summary reports what Demo created.
type Summary struct {
	Guardians         int
	OptedOutGuardians int
	Children          int
	ChildrenByArea    map[string]int
	Events            int
}

type demoChild struct {
	firstName string
	surname   string
	ageDays   int
	// vaccinated lists KEPI codes already administered; the child is due (or
	// overdue) for everything scheduled at or before their age that is absent.
	vaccinated []string
}

type demoGuardian struct {
	name       string
	phone      string
	nationalID string
	lang       string
	area       string
	optOut     bool
	children   []demoChild
}

// standardCourse is the vaccination state of a 150-day-old who attended
// birth, 6-week and 10-week visits but missed the 14-week visit: opv3, dpt3,
// pcv3 and ipv (due at day 98) are now overdue.
var standardCourse = []string{
	"bcg", "opv0",
	"opv1", "dpt1", "pcv1", "rota1",
	"opv2", "dpt2", "pcv2", "rota2",
}

// vaccineDueDays mirrors the KEPI seed in migration 0006 for computing
// administered_at timestamps.
var vaccineDueDays = map[string]int{
	"bcg": 0, "opv0": 0,
	"opv1": 42, "dpt1": 42, "pcv1": 42, "rota1": 42,
	"opv2": 70, "dpt2": 70, "pcv2": 70, "rota2": 70,
	"opv3": 98, "dpt3": 98, "pcv3": 98, "ipv": 98,
	"mr1": 270, "mr2": 540,
}

func guardians() []demoGuardian {
	firstNames := []string{
		"Amina", "Baraka", "Chebet", "Dalila", "Emeka", "Furaha", "Gathoni",
		"Hawa", "Imani", "Juma", "Kadzo", "Lengo", "Makena", "Neema",
		"Otieno", "Pendo", "Rehema", "Saidi", "Tabitha", "Upendo", "Wafula",
		"Zawadi", "Akinyi", "Barasa", "Cherop", "Dzame", "Ekali", "Fadhili",
	}
	surnames := []string{"Odhiambo", "Mwangi", "Kiprop", "Mwakidudu", "Njoroge", "Wekesa", "Achieng"}

	// area -> (guardian count, children per guardian); Kisumu 12 children,
	// Eldoret 11, Mombasa 3, Nakuru 2, Nairobi 0. One Kisumu guardian has
	// opted out (their 2 children must be skipped by the notifier).
	plans := []struct {
		area      string
		guardians int
		perG      []int
		optOutIdx int // index into this area's guardians; -1 none
	}{
		{area: "kisumu", guardians: 6, perG: []int{2, 2, 2, 2, 2, 2}, optOutIdx: 5},
		{area: "eldoret", guardians: 6, perG: []int{2, 2, 2, 2, 2, 1}, optOutIdx: -1},
		{area: "mombasa", guardians: 2, perG: []int{2, 1}, optOutIdx: -1},
		{area: "nakuru", guardians: 1, perG: []int{2}, optOutIdx: -1},
	}

	var out []demoGuardian
	childIdx := 0
	guardianIdx := 0
	for _, p := range plans {
		for g := 0; g < p.guardians; g++ {
			sn := surnames[guardianIdx%len(surnames)]
			guard := demoGuardian{
				name:       fmt.Sprintf("%s %s", firstNames[(guardianIdx+7)%len(firstNames)], sn),
				phone:      fmt.Sprintf("+2547000001%02d", guardianIdx),
				nationalID: fmt.Sprintf("300000%02d", guardianIdx),
				lang:       []string{"sw", "en"}[guardianIdx%2],
				area:       p.area,
				optOut:     g == p.optOutIdx,
			}
			for c := 0; c < p.perG[g]; c++ {
				child := demoChild{
					firstName:  firstNames[childIdx%len(firstNames)],
					surname:    sn,
					ageDays:    150,
					vaccinated: standardCourse,
				}
				// One Mombasa newborn adds variety: only birth doses, nothing
				// due yet at 10 days old.
				if p.area == "mombasa" && childIdx%3 == 2 {
					child.ageDays = 10
					child.vaccinated = []string{"bcg", "opv0"}
				}
				guard.children = append(guard.children, child)
				childIdx++
			}
			out = append(out, guard)
			guardianIdx++
		}
	}
	return out
}

// Demo inserts the fictional demo population. Idempotence note: Demo is not
// idempotent (fresh IDs each run); `make demo` runs it against a database it
// checks is empty first.
func Demo(ctx context.Context, pool *pgxpool.Pool, key crypto.Key, now time.Time) (Summary, error) {
	q := db.New(pool)
	sum := Summary{ChildrenByArea: map[string]int{}}

	for _, g := range guardians() {
		nameEnc, err := crypto.Seal(key, g.name)
		if err != nil {
			return sum, err
		}
		phoneEnc, err := crypto.Seal(key, g.phone)
		if err != nil {
			return sum, err
		}
		idEnc, err := crypto.Seal(key, g.nationalID)
		if err != nil {
			return sum, err
		}
		gid, err := q.CreateGuardian(ctx, db.CreateGuardianParams{
			NameEnc:       nameEnc.Bytes(),
			PhoneEnc:      phoneEnc.Bytes(),
			NationalIDEnc: idEnc.Bytes(),
			Lang:          g.lang,
		})
		if err != nil {
			return sum, fmt.Errorf("seed guardian: %w", err)
		}
		sum.Guardians++

		if err := q.AppendConsent(ctx, db.AppendConsentParams{
			GuardianID: gid, Action: "OPT_IN", Channel: "sms",
		}); err != nil {
			return sum, err
		}
		if g.optOut {
			if err := q.AppendConsent(ctx, db.AppendConsentParams{
				GuardianID: gid, Action: "OPT_OUT", Channel: "sms",
			}); err != nil {
				return sum, err
			}
			sum.OptedOutGuardians++
		}

		for _, c := range g.children {
			dob := now.AddDate(0, 0, -c.ageDays)
			childNameEnc, err := crypto.Seal(key, fmt.Sprintf("%s %s", c.firstName, c.surname))
			if err != nil {
				return sum, err
			}
			cid, err := q.CreateChild(ctx, db.CreateChildParams{
				GuardianID:  gid,
				AreaID:      g.area,
				NameEnc:     childNameEnc.Bytes(),
				DateOfBirth: pgtype.Date{Time: dob, Valid: true},
			})
			if err != nil {
				return sum, fmt.Errorf("seed child: %w", err)
			}
			sum.Children++
			sum.ChildrenByArea[g.area]++

			for _, code := range c.vaccinated {
				administered := dob.AddDate(0, 0, vaccineDueDays[code])
				_, err := q.InsertImmunizationEvent(ctx, db.InsertImmunizationEventParams{
					ChildID:        cid,
					VaccineCode:    code,
					AdministeredAt: pgtype.Timestamptz{Time: administered, Valid: true},
					Facility:       ptr("Demo Health Centre"),
				})
				if err != nil {
					return sum, fmt.Errorf("seed event: %w", err)
				}
				sum.Events++
			}
		}
	}
	return sum, nil
}

func ptr[T any](v T) *T { return &v }
