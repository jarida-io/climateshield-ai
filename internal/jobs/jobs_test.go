// SPDX-License-Identifier: Apache-2.0

package jobs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Job kinds are persisted in the River tables: renaming one orphans every
// queued job of that kind, so the strings are pinned here.
func TestJobKindsArePinned(t *testing.T) {
	require.Equal(t, "climate_ingest", ClimateIngestArgs{}.Kind())
	require.Equal(t, "risk_predict", RiskPredictArgs{}.Kind())
	require.Equal(t, "alert_dispatch", AlertDispatchArgs{}.Kind())
	require.Equal(t, "ledger_daily_root", LedgerDailyRootArgs{}.Kind())
}

func TestQueueNamesAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, q := range []string{QueueIngest, QueuePredict, QueueNotify, QueueLedger} {
		require.NotEmpty(t, q)
		require.False(t, seen[q], "duplicate queue name %q", q)
		seen[q] = true
	}
}
