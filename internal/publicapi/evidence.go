// SPDX-License-Identifier: Apache-2.0

package publicapi

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/notify"
	"github.com/jarida-io/climateshield/internal/predict"
)

// This file backs the evidence views. Each response is built so that a
// reviewer can check a claim against live data, and so that nothing
// person-derived leaves the process. Two rules are applied throughout:
// counts that describe people go through Suppress, and anything the system
// has not actually done is described as not done.

// buildModelInfo reports which predictor is scoring, what the published
// thresholds are, and — the point of the view — whether each threshold is
// reachable at all in the reference climate record.
func (s *Server) buildModelInfo(_ context.Context) (*climateshieldv1.GetModelInfoResponse, error) {
	resp := &climateshieldv1.GetModelInfoResponse{
		ActivePredictor:     s.predictorName,
		ActiveVersion:       s.predictorVersion,
		AvailablePredictors: []string{"rules", "climatology"},
		Rules:               thresholdRules(s.climatology),
		HighExceedance:      predict.HighExceedance,
		MediumExceedance:    predict.MediumExceedance,
	}
	if s.climatology != nil {
		resp.ReferencePeriod = s.climatology.ReferencePeriod
		resp.ReferenceSource = s.climatology.Source
		resp.ReferenceLicence = s.climatology.SourceLicence
		resp.WindowDays = int32(s.climatology.WindowDays)
		resp.Interpretation = "Exceedance is how unusual the forecast window is for this county " +
			"and month against the reference record: 0.02 means the most extreme 2% of that decade. " +
			"It describes the weather. It is not a probability that an outbreak will occur — this " +
			"system holds no outbreak surveillance data and does not estimate that."
	}
	return resp, nil
}

// thresholdRules reports the published cutoffs together with whether the
// reference record ever came near them. Two of them cannot be reached in any
// monitored county, and this view is where that shows up rather than staying
// buried in a test.
func thresholdRules(clim *predict.Climatology) []*climateshieldv1.ThresholdRule {
	rules := []*climateshieldv1.ThresholdRule{
		{
			Disease: "cholera", Driver: predict.DriverPeakRainfall,
			High: predict.CholeraHighMM, Medium: predict.CholeraMediumMM, HigherIsWorse: true,
		},
		{
			Disease: "malaria", Driver: predict.DriverPeakRainfall,
			High: predict.MalariaHighMM, Medium: predict.MalariaMediumMM, HigherIsWorse: true,
		},
		{
			Disease: "pneumonia", Driver: predict.DriverMeanMaxTemp,
			High: predict.PneumoniaHighC, Medium: predict.PneumoniaMediumC, HigherIsWorse: false,
		},
		{
			Disease: "meningitis", Driver: predict.DriverMeanMaxTemp,
			High: predict.MeningitisHighC, Medium: predict.MeningitisMediumC, HigherIsWorse: true,
		},
	}
	for _, r := range rules {
		if clim == nil {
			r.ReachableInReferencePeriod = true
			continue
		}
		reachable, extreme := reachable(clim, r)
		r.ReachableInReferencePeriod = reachable
		if !reachable {
			dir := "hottest"
			if !r.GetHigherIsWorse() {
				dir = "coldest"
			}
			r.Note = fmt.Sprintf(
				"Not reachable: the %s 14-day mean in the reference record is %.1f°C, so this "+
					"HIGH cutoff of %.0f°C never fires in the monitored counties.",
				dir, extreme, r.GetHigh())
		}
	}
	return rules
}

// reachable reports whether any county-month in the reference record has a
// distribution that reaches the rule's HIGH cutoff, plus the closest value.
func reachable(clim *predict.Climatology, r *climateshieldv1.ThresholdRule) (bool, float64) {
	if r.GetDriver() != predict.DriverMeanMaxTemp {
		// Rainfall cutoffs are reached in the record; only the temperature
		// rules are in question.
		return true, 0
	}
	extreme := 0.0
	first := true
	for _, county := range clim.Counties {
		for _, month := range county.Months {
			q := month.Quantiles["mean_tmax_c"]
			if len(q) == 0 {
				continue
			}
			var candidate float64
			if r.GetHigherIsWorse() {
				candidate = q[len(q)-1]
			} else {
				candidate = q[0]
			}
			if first {
				extreme, first = candidate, false
				continue
			}
			if r.GetHigherIsWorse() == (candidate > extreme) {
				extreme = candidate
			}
		}
	}
	if first {
		return true, 0
	}
	if r.GetHigherIsWorse() {
		return extreme >= r.GetHigh(), extreme
	}
	return extreme <= r.GetHigh(), extreme
}

// buildClimateSeries returns the forecast window currently driving the
// scores, labelled with the source it was actually ingested from.
func (s *Server) buildClimateSeries(ctx context.Context, area string) (*climateshieldv1.GetClimateSeriesResponse, error) {
	rows, err := s.q.LatestSeriesForAllAreas(ctx)
	if err != nil {
		return nil, fmt.Errorf("publicapi: climate series: %w", err)
	}
	resp := &climateshieldv1.GetClimateSeriesResponse{GeneratedAt: timestamppb.Now()}
	byArea := map[string]*climateshieldv1.CountySeries{}
	for _, r := range rows {
		if area != "" && r.AreaName != area {
			continue
		}
		cs, ok := byArea[r.AreaName]
		if !ok {
			cs = &climateshieldv1.CountySeries{
				Area:     r.AreaName,
				Source:   r.Source,
				IssuedAt: timestamppb.New(r.IssuedAt.Time),
			}
			byArea[r.AreaName] = cs
			resp.Series = append(resp.Series, cs)
		}
		cs.Days = append(cs.Days, &climateshieldv1.ClimateDay{
			Date:            r.ForecastDate.Time.Format("2006-01-02"),
			PrecipitationMm: r.PrecipitationSumMm,
			TempMaxC:        r.TempMaxC,
			TempMinC:        r.TempMinC,
		})
	}
	return resp, nil
}

// buildLedgerSummary publishes daily roots and their anchors — never a leaf.
// A leaf is a per-child HMAC; publishing one would put a per-child artifact
// on a public surface, which is forbidden regardless of how opaque it looks.
// The root is a commitment over a whole day and discloses nothing about an
// individual, so it is safe and is what makes tamper-evidence checkable.
func (s *Server) buildLedgerSummary(ctx context.Context) (*climateshieldv1.GetLedgerSummaryResponse, error) {
	rows, err := s.q.LedgerRootSummary(ctx)
	if err != nil {
		return nil, fmt.Errorf("publicapi: ledger summary: %w", err)
	}
	resp := &climateshieldv1.GetLedgerSummaryResponse{
		TotalDays:   int64(len(rows)),
		Algorithm:   "per-child HMAC-SHA256 leaves, RFC 6962 Merkle tree, daily root",
		AnchorMode:  s.anchorMode,
		AnchorNote:  anchorNote(s.anchorMode, rows),
		GeneratedAt: timestamppb.Now(),
	}
	for _, r := range rows {
		root := &climateshieldv1.DailyRoot{
			Day:     r.LeafDay.Time.Format("2006-01-02"),
			RootHex: hex.EncodeToString(r.Root),
		}
		// A day's leaf count is how many immunizations were recorded that
		// day, so it is people-derived and gets the same k treatment as any
		// other such count.
		root.LeafCount, root.LeafCountSuppressed = Suppress(int64(r.LeafCount))
		root.AnchorType = r.AnchorType
		if r.AnchoredAt.Valid {
			root.AnchoredAt = timestamppb.New(r.AnchoredAt.Time)
		}
		// The newest anchor's own record of where the root went. Only the
		// newest: how many times a day was re-anchored tracks how many late
		// immunizations were recorded, which is people-derived.
		if r.AnchorReference != nil {
			root.AnchorReference = *r.AnchorReference
		}
		if r.ChainID != nil {
			root.ChainId = *r.ChainID
		}
		if r.ChainLabel != nil {
			root.ChainLabel = *r.ChainLabel
		}
		if r.ContractAddress != nil {
			root.ContractAddress = *r.ContractAddress
		}
		if r.TxHash != nil {
			root.TxHash = *r.TxHash
		}
		if r.BlockNumber != nil {
			root.BlockNumber = *r.BlockNumber
		}
		// A local anchor reads nothing back — a row in this database is not
		// an independent place to read from — so it never claims a match.
		root.ReadbackMatches = len(r.ReadbackRoot) > 0 && bytes.Equal(r.ReadbackRoot, r.Root)
		if r.VerifiedAt.Valid {
			root.VerifiedAt = timestamppb.New(r.VerifiedAt.Time)
		}
		resp.Roots = append(resp.Roots, root)
	}
	return resp, nil
}

// buildAlertSummary reports what the messaging path did, and shows the
// templates rendered with PLACEHOLDER values. Real message bodies are never
// published: they contain a child's first name.
func (s *Server) buildAlertSummary(ctx context.Context) (*climateshieldv1.GetAlertSummaryResponse, error) {
	rows, err := s.q.CountAlertsByStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("publicapi: alert summary: %w", err)
	}
	sends := s.channel != "mock" && s.channel != ""
	note := "The mock channel is active: alerts are rendered and recorded, and no SMS is sent."
	if sends {
		note = fmt.Sprintf("Channel %q is active and delivers messages.", s.channel)
	}
	resp := &climateshieldv1.GetAlertSummaryResponse{
		Channel:      s.channel,
		ChannelSends: sends,
		ChannelNote:  note,
		QuietHours:   "21:00–07:00 East Africa Time; alerts falling in the window are deferred to 07:00, not dropped",
		Templates:    templateSamples(),
		GeneratedAt:  timestamppb.Now(),
	}
	for _, r := range rows {
		c := &climateshieldv1.AlertStatusCount{Status: r.Status}
		c.Count, c.Suppressed = Suppress(r.N)
		resp.Statuses = append(resp.Statuses, c)
	}
	return resp, nil
}

// templateSamples renders each language template with obvious placeholder
// values so the shape and length can be reviewed with no real recipient.
func templateSamples() []*climateshieldv1.TemplateSample {
	var out []*climateshieldv1.TemplateSample
	for _, lang := range []string{"en", "sw"} {
		body, err := notify.Render(notify.TemplateData{
			Lang:      lang,
			RiskLevel: "HIGH",
			County:    "<COUNTY>",
			FirstName: "<CHILD>",
			// Longest name in the KEPI seed, so the length shown is worst case.
			VaccineName: "Measles-Rubella 1",
		})
		if err != nil {
			continue
		}
		n, err := notify.SeptetLength(body)
		if err != nil {
			continue
		}
		out = append(out, &climateshieldv1.TemplateSample{
			Lang: lang, Body: body,
			Septets: int32(n), MaxSeptets: notify.MaxSeptets,
		})
	}
	return out
}

// buildPipelineStatus proves the system runs unattended: job outcomes, how
// much data has landed, and when. System metadata only.
func (s *Server) buildPipelineStatus(ctx context.Context) (*climateshieldv1.GetPipelineStatusResponse, error) {
	resp := &climateshieldv1.GetPipelineStatusResponse{
		IngestInterval: s.ingestInterval,
		GeneratedAt:    timestamppb.Now(),
	}

	obs, err := s.q.CountObservations(ctx)
	if err != nil {
		return nil, fmt.Errorf("publicapi: pipeline status: %w", err)
	}
	resp.ClimateObservations = obs

	scores, err := s.q.CountRiskScores(ctx)
	if err != nil {
		return nil, fmt.Errorf("publicapi: pipeline status: %w", err)
	}
	resp.RiskScores = scores

	if latest, err := s.q.LatestObservationIssuedAt(ctx); err == nil && latest.Valid {
		resp.LatestObservationAt = timestamppb.New(latest.Time)
	}

	// River owns its own tables (created by rivermigrate, not our migrations),
	// so this is a direct query rather than a generated one.
	jobs, err := s.jobStatuses(ctx)
	if err != nil {
		return nil, err
	}
	resp.Jobs = jobs
	return resp, nil
}

func (s *Server) jobStatuses(ctx context.Context) ([]*climateshieldv1.JobKindStatus, error) {
	if s.pool == nil {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT kind, state::text, count(*), max(finalized_at)
		FROM river_job GROUP BY kind, state ORDER BY kind, state`)
	if err != nil {
		// A deployment that has not yet run a job has no River tables; that
		// is an empty pipeline, not a failure of the whole view.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("publicapi: job status: %w", err)
	}
	defer rows.Close()

	var out []*climateshieldv1.JobKindStatus
	for rows.Next() {
		var kind, state string
		var n int64
		var last *time.Time
		if err := rows.Scan(&kind, &state, &n, &last); err != nil {
			return nil, fmt.Errorf("publicapi: job status: %w", err)
		}
		js := &climateshieldv1.JobKindStatus{Kind: kind, State: state, Count: n}
		if last != nil {
			js.LastFinishedAt = timestamppb.New(*last)
		}
		out = append(out, js)
	}
	return out, rows.Err()
}

// buildClimatology returns the reference distribution for one county-month,
// with the current forecast window marked on it. This is what makes the
// climatology model legible: a reviewer sees the decade of history the score
// was measured against, not just the score.
func (s *Server) buildClimatology(ctx context.Context, area string, month int) (*climateshieldv1.GetClimatologyResponse, error) {
	if s.climatology == nil {
		return nil, errBadRequest{"no reference climatology is loaded"}
	}
	if month < 1 || month > 12 {
		return nil, errBadRequest{fmt.Sprintf("month %d out of range (want 1-12)", month)}
	}
	areaID, ok := s.areaIDFor(ctx, area)
	if !ok {
		return nil, errBadRequest{fmt.Sprintf("unknown county %q", area)}
	}

	resp := &climateshieldv1.GetClimatologyResponse{
		Area:            area,
		Month:           int32(month),
		Samples:         int32(s.climatology.Samples(areaID, month)),
		ReferencePeriod: s.climatology.ReferencePeriod,
	}
	if resp.GetSamples() == 0 {
		return nil, errBadRequest{fmt.Sprintf("no reference data for %s in month %d", area, month)}
	}

	// The current window, if one has been ingested, so the marker has meaning.
	observed := map[string]float64{}
	if rows, err := s.q.LatestSeriesForAllAreas(ctx); err == nil {
		var peak, sumMax, sumMin float64
		var n int
		for _, r := range rows {
			if r.AreaID != areaID {
				continue
			}
			if r.PrecipitationSumMm > peak {
				peak = r.PrecipitationSumMm
			}
			sumMax += r.TempMaxC
			sumMin += r.TempMinC
			n++
		}
		if n > 0 {
			observed["peak_rain_mm"] = peak
			observed["mean_tmax_c"] = sumMax / float64(n)
			observed["mean_tmin_c"] = sumMin / float64(n)
		}
	}

	for _, d := range []struct {
		key, label, unit string
		lowerTail        bool
	}{
		{"peak_rain_mm", "14-day peak rainfall", "mm", false},
		{"mean_tmax_c", "14-day mean maximum temperature", "°C", false},
		{"mean_tmin_c", "14-day mean minimum temperature", "°C", true},
	} {
		q := s.climatology.QuantileLadder(areaID, month, d.key)
		if len(q) == 0 {
			continue
		}
		dist := &climateshieldv1.ClimatologyDistribution{
			Driver:            d.label,
			Unit:              d.unit,
			Quantiles:         q,
			PercentileSteps:   int32Slice(s.climatology.QuantileStepsPct),
			LowerTailIsHazard: d.lowerTail,
		}
		if v, ok := observed[d.key]; ok {
			value := v
			dist.Observed = &value
			if exc, err := s.climatology.Exceedance(areaID, month, d.key, v, !d.lowerTail); err == nil {
				e := exc
				dist.ObservedExceedance = &e
			}
		}
		resp.Distributions = append(resp.Distributions, dist)
	}
	return resp, nil
}

// areaIDFor maps a display county name back to its identifier.
func (s *Server) areaIDFor(ctx context.Context, name string) (string, bool) {
	areas, err := s.q.ListAreas(ctx)
	if err != nil {
		return "", false
	}
	for _, a := range areas {
		if strings.EqualFold(a.Name, name) {
			return a.ID, true
		}
	}
	return "", false
}

func int32Slice(in []int) []int32 {
	out := make([]int32, len(in))
	for i, v := range in {
		out[i] = int32(v)
	}
	return out
}
