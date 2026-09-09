// SPDX-License-Identifier: Apache-2.0

// Package jobs defines the River job types that link the pipeline:
// ingest -> predict -> alert. Each service works its own queue so a service
// never fetches a job kind it has no worker for.
package jobs

// Queue names, one per working service.
const (
	QueueIngest  = "ingest"
	QueuePredict = "predict"
	QueueNotify  = "notify"
	QueueLedger  = "ledger"
	// QueueBriefing carries county briefing regeneration. Briefings are never
	// generated in a request path: a language model on a laptop CPU takes tens
	// of seconds, and a public read must not wait on one.
	QueueBriefing = "briefing"
)

// ClimateIngestArgs triggers a full ingestion sweep over all areas.
type ClimateIngestArgs struct {
	// Source optionally overrides the service's configured climate source for
	// this one job ("openmeteo" or "fixture"). The demo uses "fixture".
	Source string `json:"source,omitempty"`
}

// Kind implements river.JobArgs.
func (ClimateIngestArgs) Kind() string { return "climate_ingest" }

// RiskPredictArgs scores one area from its latest observation window.
type RiskPredictArgs struct {
	AreaID string `json:"area_id"`
}

// Kind implements river.JobArgs.
func (RiskPredictArgs) Kind() string { return "risk_predict" }

// AlertDispatchArgs fans one elevated risk score out to eligible guardians.
type AlertDispatchArgs struct {
	RiskScoreID int64  `json:"risk_score_id"`
	AreaID      string `json:"area_id"`
	Disease     string `json:"disease"`
	Level       string `json:"level"`
}

// Kind implements river.JobArgs.
func (AlertDispatchArgs) Kind() string { return "alert_dispatch" }

// BriefingSweepArgs regenerates county briefings whose facts have changed.
// An empty AreaID sweeps every monitored county. The job is idempotent: a
// county whose fact sheet still hashes the same is skipped, so running the
// sweep more often costs a hash comparison and nothing else.
type BriefingSweepArgs struct {
	AreaID string `json:"area_id,omitempty"`
}

// Kind implements river.JobArgs.
func (BriefingSweepArgs) Kind() string { return "briefing_sweep" }

// LedgerDailyRootArgs recomputes Merkle roots for all days with leaves.
type LedgerDailyRootArgs struct{}

// Kind implements river.JobArgs.
func (LedgerDailyRootArgs) Kind() string { return "ledger_daily_root" }
