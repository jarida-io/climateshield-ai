// SPDX-License-Identifier: Apache-2.0

// Command demo drives the walking skeleton end to end against a running
// `make up` stack: seed the fictional population, ingest the committed
// fixture scenario (or live Open-Meteo when CLIMATE_SOURCE=openmeteo), let
// the predictor and notifier work the queues, record one immunization event
// through the registry API, sweep it into the Merkle ledger, and print an
// HONEST summary of what happened — and, for the mock channel, what
// deliberately did not.
package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"google.golang.org/protobuf/types/known/timestamppb"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/gen/climateshield/v1/climateshieldv1connect"
	"github.com/jarida-io/climateshield/internal/jobs"
	"github.com/jarida-io/climateshield/internal/ledger"
	"github.com/jarida-io/climateshield/internal/notify"
	"github.com/jarida-io/climateshield/internal/platform/clock"
	"github.com/jarida-io/climateshield/internal/platform/config"
	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/seed"
)

type demoConfig struct {
	config.Common
	config.DB
	PIIKeyHex      string `env:"PII_KEY_HEX,required"`
	AllowDevPIIKey bool   `env:"PII_ALLOW_DEV_KEY" envDefault:"false"`
	Source         string `env:"CLIMATE_SOURCE" envDefault:"fixture"`
	RegistryURL    string `env:"REGISTRY_URL" envDefault:"http://localhost:8082"`
	PublicAPIURL   string `env:"PUBLICAPI_URL" envDefault:"http://localhost:8080"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "\ndemo: FAILED: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load[demoConfig]()
	if err != nil {
		return err
	}
	key, err := crypto.KeyFromHexChecked(cfg.PIIKeyHex, cfg.AllowDevPIIKey)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fmt.Println("============================================================")
	fmt.Println("ClimateShield — walking skeleton demo")
	if cfg.Source == "fixture" {
		fmt.Println("requesting ingest from: fixture (committed demo scenario, not live weather)")
	} else {
		fmt.Printf("requesting ingest from: %s (live data — results vary with the actual forecast)\n", cfg.Source)
	}
	fmt.Println("============================================================")

	pool, err := store.Connect(ctx, cfg.URL)
	if err != nil {
		return fmt.Errorf("database unreachable (%w) — run `make up` first", err)
	}
	defer pool.Close()
	q := db.New(pool)

	// 1. Seed the fictional demo population (once).
	children, err := q.ListChildren(ctx)
	if err != nil {
		return fmt.Errorf("schema missing (%w) — run `make up` first", err)
	}
	if len(children) == 0 {
		sum, err := seed.Demo(ctx, pool, key, time.Now())
		if err != nil {
			return err
		}
		fmt.Printf("seeded fictional population: %d guardians (%d opted out), %d children, %d immunization events\n",
			sum.Guardians, sum.OptedOutGuardians, sum.Children, sum.Events)
		children, err = q.ListChildren(ctx)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("demo population already present (%d children) — reusing\n", len(children))
	}

	// 2. Kick the pipeline: one ingest job; predictor and notifier fan out.
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return err
	}
	if _, err := riverClient.Insert(ctx, jobs.ClimateIngestArgs{Source: cfg.Source},
		&river.InsertOpts{Queue: jobs.QueueIngest}); err != nil {
		return fmt.Errorf("enqueue ingest: %w", err)
	}
	fmt.Println("enqueued climate_ingest -> risk_predict -> alert_dispatch")

	// 3. Record one immunization event through the registry API, then ask the
	// ledger to sweep it into today's Merkle tree.
	regClient := climateshieldv1connect.NewRegistryServiceClient(http.DefaultClient, cfg.RegistryURL)
	childID := uuidString(children[0].ID)
	rec, err := regClient.RecordImmunization(ctx, connect.NewRequest(&climateshieldv1.RecordImmunizationRequest{
		ChildId:        childID,
		VaccineCode:    "opv3",
		AdministeredAt: timestamppb.Now(),
		Facility:       "Demo Health Centre",
	}))
	if err != nil {
		return fmt.Errorf("registry API at %s: %w — run `make up` first", cfg.RegistryURL, err)
	}
	eventID := rec.Msg.GetEventId()
	fmt.Printf("recorded immunization event %s (opv3) via registry API\n", eventID)
	if _, err := riverClient.Insert(ctx, jobs.LedgerDailyRootArgs{},
		&river.InsertOpts{Queue: jobs.QueueLedger}); err != nil {
		return fmt.Errorf("enqueue ledger sweep: %w", err)
	}

	// 4. Wait for the pipeline.
	quiet := notify.InQuietHours(clock.Real{}.Now())
	if quiet {
		fmt.Println("NOTE: quiet hours (21:00-07:00 EAT) — alert dispatch is deferred to 07:00 EAT;")
		fmt.Println("      alert counts below will be zero, honestly.")
	}
	if err := waitFor(ctx, "risk scores for all 5 counties", 90*time.Second, func() (bool, error) {
		rows, err := q.CurrentRisk(ctx)
		if err != nil {
			return false, err
		}
		areas := map[string]bool{}
		for _, r := range rows {
			areas[r.AreaID] = true
		}
		return len(areas) >= 5, nil
	}); err != nil {
		return err
	}
	if !quiet {
		if err := waitFor(ctx, "alert dispatch", 60*time.Second, func() (bool, error) {
			rows, err := q.CountAlertsByStatus(ctx)
			if err != nil {
				return false, err
			}
			return len(rows) > 0, nil
		}); err != nil {
			return err
		}
	}
	if err := waitFor(ctx, "ledger sweep of the recorded event", 60*time.Second, func() (bool, error) {
		pending, err := q.ListEventsWithoutLeaves(ctx)
		if err != nil {
			return false, err
		}
		return len(pending) == 0, nil
	}); err != nil {
		return err
	}

	// 5. Report: risk grid. The source label is read back from the
	// observations these scores were actually computed from — never assumed
	// from configuration, so the output cannot claim fixture data while
	// showing live numbers (or the reverse).
	fmt.Println("\n--- Outbreak risk (latest per county x disease) ---")
	scoredFrom, err := observedSources(ctx, q)
	if err != nil {
		return err
	}
	fmt.Printf("scored from observations ingested via: %s\n", scoredFrom)
	rows, err := q.CurrentRisk(ctx)
	if err != nil {
		return err
	}
	elevated := 0
	for _, r := range rows {
		marker := " "
		if r.Level != "LOW" {
			marker = "⚠"
			elevated++
		}
		fmt.Printf("  %s %-8s %-11s %-6s (%s = %.1f, %s v%s)\n",
			marker, r.AreaName, r.Disease, r.Level, r.Driver, r.DriverValue, r.Predictor, r.PredictorVersion)
	}
	fmt.Printf("%d elevated (HIGH/MEDIUM) county-disease pairs\n", elevated)

	// 6. Report: alerts, with the honesty line.
	fmt.Println("\n--- Alerts ---")
	statusRows, err := q.CountAlertsByStatus(ctx)
	if err != nil {
		return err
	}
	var wouldSend, sent int64
	for _, r := range statusRows {
		fmt.Printf("  %-20s %d\n", r.Status, r.N)
		switch r.Status {
		case "would_send":
			wouldSend = r.N
		case "sent":
			sent = r.N
		}
	}
	if sent > 0 {
		fmt.Printf("sent %d alerts via a live channel\n", sent)
	} else {
		fmt.Printf("[mock] would send %d alerts\n", wouldSend)
		fmt.Println("(mock channel active: NO SMS was sent; see var/outbox.jsonl for the rendered messages)")
	}

	// 7. Report: ledger + inclusion proof for the event recorded above.
	fmt.Println("\n--- Tamper-evident ledger ---")
	days, err := q.ListLeafDays(ctx)
	if err != nil {
		return err
	}
	for _, day := range days {
		root, err := q.GetDailyRoot(ctx, day)
		if err != nil {
			return err
		}
		fmt.Printf("  %s: %d leaves, root %s…\n",
			day.Time.Format("2006-01-02"), root.LeafCount, hex.EncodeToString(root.Root)[:16])
	}
	if err := verifyInclusion(ctx, q, eventID); err != nil {
		return err
	}

	// 8. Report: public API liveness.
	fmt.Println("\n--- Public API ---")
	resp, err := http.Get(cfg.PublicAPIURL + "/v1/risk/current")
	if err != nil {
		return fmt.Errorf("public API at %s unreachable: %w", cfg.PublicAPIURL, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	fmt.Printf("  GET %s/v1/risk/current -> %d (%d bytes JSON)\n", cfg.PublicAPIURL, resp.StatusCode, len(body))
	fmt.Printf("  dashboard: http://localhost:8081\n")

	fmt.Println("\ndemo complete.")
	fmt.Println("============================================================")
	return nil
}

// observedSources reports which climate source produced the latest
// observation window for each area, as a human-readable summary.
func observedSources(ctx context.Context, q *db.Queries) (string, error) {
	areas, err := q.ListAreas(ctx)
	if err != nil {
		return "", err
	}
	counts := map[string]int{}
	for _, a := range areas {
		window, err := q.LatestObservationWindow(ctx, a.ID)
		if err != nil {
			return "", err
		}
		if len(window) > 0 {
			counts[window[0].Source]++
		}
	}
	if len(counts) == 0 {
		return "no observations", nil
	}
	parts := make([]string, 0, len(counts))
	for src, n := range counts {
		label := src
		if src == "fixture" {
			label = "fixture (committed demo scenario, not live weather)"
		}
		parts = append(parts, fmt.Sprintf("%s [%d counties]", label, n))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", "), nil
}

// verifyInclusion proves the recorded event is in its day's anchored root.
func verifyInclusion(ctx context.Context, q *db.Queries, eventID string) error {
	evID := uuidFromString(eventID)
	leaf, err := q.GetLeaf(ctx, evID)
	if err != nil {
		return fmt.Errorf("ledger leaf for event %s: %w", eventID, err)
	}
	rows, err := q.LeavesForDay(ctx, leaf.LeafDay)
	if err != nil {
		return err
	}
	hashes := make([][]byte, 0, len(rows))
	index := -1
	for i, r := range rows {
		hashes = append(hashes, r.LeafHash)
		if r.EventID == evID {
			index = i
		}
	}
	root, err := q.GetDailyRoot(ctx, leaf.LeafDay)
	if err != nil {
		return err
	}
	proof, err := ledger.BuildProof(hashes, index)
	if err != nil {
		return err
	}
	if !ledger.VerifyProof(hashes[index], proof, root.Root) {
		return fmt.Errorf("inclusion proof FAILED for event %s", eventID)
	}
	fmt.Printf("  inclusion proof for event %s…: OK (leaf %d of %d under root %s…)\n",
		eventID[:8], index+1, len(rows), hex.EncodeToString(root.Root)[:16])
	return nil
}

func waitFor(ctx context.Context, what string, timeout time.Duration, check func() (bool, error)) error {
	fmt.Printf("waiting for %s", what)
	deadline := time.Now().Add(timeout)
	for {
		ok, err := check()
		if err == nil && ok {
			fmt.Println(" ... done")
			return nil
		}
		if time.Now().After(deadline) {
			fmt.Println()
			if err != nil {
				return fmt.Errorf("timed out waiting for %s (last error: %w)", what, err)
			}
			return fmt.Errorf("timed out waiting for %s — check `docker compose logs`", what)
		}
		fmt.Print(".")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func uuidString(u pgtype.UUID) string {
	v, err := u.Value()
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func uuidFromString(s string) (u pgtype.UUID) {
	_ = u.Scan(s)
	return u
}
