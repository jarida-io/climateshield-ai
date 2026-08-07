// SPDX-License-Identifier: Apache-2.0

package jobs

import (
	"context"
	"log/slog"
	"time"
)

// Schedule runs insert immediately, then on every tick until ctx is done.
//
// Why not River's periodic jobs: River elects ONE leader per database, and
// only the leader fires its periodic jobs. With several services sharing a
// database (ingestor, predictor, notifier, ledger) whichever client wins
// leadership would be the only one whose schedule ever ran — the ingestor's
// sweep would silently never fire if the ledger held leadership. Each
// service therefore owns its own in-process schedule and relies on River's
// per-period job uniqueness (see InsertOpts in the callers) so that running
// multiple replicas cannot produce duplicate work.
func Schedule(ctx context.Context, log *slog.Logger, kind string, every time.Duration, insert func(context.Context) error) {
	fire := func() {
		if err := insert(ctx); err != nil && ctx.Err() == nil {
			log.Error("failed to schedule job", "kind", kind, "error", err.Error())
		}
	}
	fire()

	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fire()
		}
	}
}
