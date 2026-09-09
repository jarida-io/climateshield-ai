// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/gen/climateshield/v1/climateshieldv1connect"
	"github.com/jarida-io/climateshield/internal/jobs"
)

// reportBriefing asks the briefing service to write Kisumu's briefing from the
// facts this run just produced, then prints it with its provenance line.
//
// The provenance line is the point of this step. It states which generator
// wrote the words, which model (or none), which prompt version, the hash of
// the fact sheet, and whether the grounding check passed — all read back from
// the stored briefing, never from configuration. A demo that printed generated
// prose without saying who generated it would be the "SMS sent" lie again, in
// a nicer typeface.
func reportBriefing(
	ctx context.Context, riverClient *river.Client[pgx.Tx], publicAPIURL, area string,
) error {
	fmt.Println("\n--- County briefing ---")

	if _, err := riverClient.Insert(ctx, jobs.BriefingSweepArgs{}, &river.InsertOpts{
		Queue: jobs.QueueBriefing,
	}); err != nil {
		return fmt.Errorf("enqueue briefing sweep: %w", err)
	}

	client := climateshieldv1connect.NewPublicServiceClient(http.DefaultClient, publicAPIURL)
	var msg *climateshieldv1.GetBriefingResponse
	// Wait for a briefing that describes the scores printed above, not merely
	// for the first one the sweep happens to write. A briefing is generated
	// from a fact sheet and cached under its hash, so the sweep that runs
	// before the predictor has scored this county legitimately produces one
	// saying no scores exist yet. That is true when it is written and stale a
	// second later; printing it here would have the demo contradict its own
	// risk grid. The sweep regenerates as soon as the facts change, so this
	// waits for the briefing whose fact sheet has the scores in it.
	err := waitFor(ctx, "the briefing service to write "+area+"'s briefing", 90*time.Second,
		func() (bool, error) {
			resp, err := client.GetBriefing(ctx, connect.NewRequest(&climateshieldv1.GetBriefingRequest{
				Area: area, Lang: "en",
			}))
			if err != nil {
				return false, err
			}
			msg = resp.Msg
			if msg.GetStatus() == "none" {
				return false, nil
			}
			return len(msg.GetFacts().GetScores()) > 0, nil
		})
	if err != nil {
		return err
	}

	fmt.Printf("  %s\n", msg.GetProvenance())
	for _, line := range strings.Split(strings.TrimSpace(msg.GetBody()), "\n") {
		fmt.Printf("  | %s\n", line)
	}

	// The fact sheet is served with the text so every number above can be
	// checked against the numbers the generator was given.
	if f := msg.GetFacts(); f != nil {
		fmt.Printf("  facts behind it: %d scored diseases, window %s to %s (source %s), facts %s\n",
			len(f.GetScores()), f.GetWindowFrom(), f.GetWindowTo(), f.GetWindowSource(),
			short(msg.GetFactsHashHex()))
	}
	if notes := msg.GetGroundingNotes(); len(notes) > 0 {
		kinds := make([]string, 0, len(notes))
		for _, n := range notes {
			kinds = append(kinds, n.GetKind())
		}
		fmt.Printf("  grounding check refused the draft: %s\n", strings.Join(kinds, ", "))
	}
	fmt.Println("  read it yourself, in English or Kiswahili:")
	fmt.Printf("    curl -s \"%s/v1/briefings?area=%s&lang=sw\" | jq .\n", publicAPIURL, area)
	return nil
}

func short(hash string) string {
	if len(hash) > 12 {
		return hash[:12] + "…"
	}
	return hash
}
