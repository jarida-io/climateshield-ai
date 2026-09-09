// SPDX-License-Identifier: Apache-2.0

package publicapi

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/registry"
	"github.com/jarida-io/climateshield/internal/store/db"
)

var diseaseEnum = map[string]climateshieldv1.Disease{
	"cholera":    climateshieldv1.Disease_DISEASE_CHOLERA,
	"malaria":    climateshieldv1.Disease_DISEASE_MALARIA,
	"pneumonia":  climateshieldv1.Disease_DISEASE_PNEUMONIA,
	"meningitis": climateshieldv1.Disease_DISEASE_MENINGITIS,
}

var levelEnum = map[string]climateshieldv1.RiskLevel{
	"LOW":    climateshieldv1.RiskLevel_RISK_LEVEL_LOW,
	"MEDIUM": climateshieldv1.RiskLevel_RISK_LEVEL_MEDIUM,
	"HIGH":   climateshieldv1.RiskLevel_RISK_LEVEL_HIGH,
}

// buildCurrentRisk assembles the latest score per county x disease.
func (s *Server) buildCurrentRisk(ctx context.Context) (*climateshieldv1.GetCurrentRiskResponse, error) {
	rows, err := s.q.CurrentRisk(ctx)
	if err != nil {
		return nil, fmt.Errorf("publicapi: current risk: %w", err)
	}
	resp := &climateshieldv1.GetCurrentRiskResponse{GeneratedAt: timestamppb.Now()}
	for _, r := range rows {
		resp.Scores = append(resp.Scores, riskScoreMsg(
			r.AreaName, r.Latitude, r.Longitude, r.Disease, r.Level, r.Driver,
			r.DriverValue, r.ForecastDate, r.Predictor, r.PredictorVersion, r.ScoredAt,
			r.Exceedance, r.Explanation,
		))
	}
	return resp, nil
}

// historyParams are the validated GET /v1/risk/history filters.
type historyParams struct {
	Area     string
	Disease  string
	FromDate string
	ToDate   string
	Limit    int32
}

const historyMaxLimit = 1000

// buildRiskHistory assembles filtered history rows, newest first.
func (s *Server) buildRiskHistory(ctx context.Context, p historyParams) (*climateshieldv1.GetRiskHistoryResponse, error) {
	limit := p.Limit
	if limit <= 0 || limit > historyMaxLimit {
		limit = historyMaxLimit
	}
	params := db.RiskHistoryParams{RowLimit: limit}
	if p.Area != "" {
		params.AreaName = &p.Area
	}
	if p.Disease != "" {
		params.Disease = &p.Disease
	}
	var err error
	if params.FromDate, err = optionalDate(p.FromDate); err != nil {
		return nil, errBadRequest{fmt.Sprintf("bad from date %q (want YYYY-MM-DD)", p.FromDate)}
	}
	if params.ToDate, err = optionalDate(p.ToDate); err != nil {
		return nil, errBadRequest{fmt.Sprintf("bad to date %q (want YYYY-MM-DD)", p.ToDate)}
	}

	rows, err := s.q.RiskHistory(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("publicapi: risk history: %w", err)
	}
	resp := &climateshieldv1.GetRiskHistoryResponse{GeneratedAt: timestamppb.Now()}
	for _, r := range rows {
		resp.Scores = append(resp.Scores, riskScoreMsg(
			r.AreaName, r.Latitude, r.Longitude, r.Disease, r.Level, r.Driver,
			r.DriverValue, r.ForecastDate, r.Predictor, r.PredictorVersion, r.ScoredAt,
			r.Exceedance, r.Explanation,
		))
	}
	return resp, nil
}

// buildStats assembles per-county people-derived aggregates, every one of
// them k>=10 suppressed. All monitored counties appear, including zeros.
func (s *Server) buildStats(ctx context.Context) (*climateshieldv1.GetStatsResponse, error) {
	areas, err := s.q.ListAreas(ctx)
	if err != nil {
		return nil, fmt.Errorf("publicapi: areas: %w", err)
	}
	registered := map[string]int64{}
	rows, err := s.q.CountChildrenByArea(ctx)
	if err != nil {
		return nil, fmt.Errorf("publicapi: children counts: %w", err)
	}
	for _, r := range rows {
		registered[r.AreaID] = r.N
	}
	alertRows, err := s.q.CountDispatchedAlertsByArea(ctx)
	if err != nil {
		return nil, fmt.Errorf("publicapi: alert counts: %w", err)
	}
	alerts := map[string]int64{}
	for _, r := range alertRows {
		alerts[r.AreaID] = r.N
	}
	due, overdue, err := s.dueCounts(ctx)
	if err != nil {
		return nil, err
	}

	resp := &climateshieldv1.GetStatsResponse{GeneratedAt: timestamppb.Now()}
	for _, a := range areas {
		st := &climateshieldv1.CountyStats{Area: a.Name}
		st.ChildrenRegistered, st.ChildrenRegisteredSuppressed = Suppress(registered[a.ID])
		st.ChildrenDue, st.ChildrenDueSuppressed = Suppress(due[a.ID])
		st.ChildrenOverdue, st.ChildrenOverdueSuppressed = Suppress(overdue[a.ID])
		st.AlertsGenerated, st.AlertsGeneratedSuppressed = Suppress(alerts[a.ID])
		resp.Stats = append(resp.Stats, st)
	}
	return resp, nil
}

// dueCounts computes, per area, how many children have >=1 due vaccine and
// how many have >=1 overdue vaccine, reusing the registry's pure KEPI logic.
func (s *Server) dueCounts(ctx context.Context) (due, overdue map[string]int64, err error) {
	scheduleRows, err := s.q.ListVaccineSchedule(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("publicapi: schedule: %w", err)
	}
	schedule := make([]registry.ScheduleEntry, 0, len(scheduleRows))
	for _, r := range scheduleRows {
		schedule = append(schedule, registry.ScheduleEntry{
			Code: r.Code, Name: r.Name,
			DueAgeDays: int(r.DueAgeDays), GraceDays: int(r.OverdueGraceDays),
		})
	}
	children, err := s.q.ListChildrenForDueComputation(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("publicapi: children: %w", err)
	}
	pairs, err := s.q.ListImmunizationPairs(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("publicapi: events: %w", err)
	}
	given := map[pgtype.UUID]map[string]bool{}
	for _, p := range pairs {
		if given[p.ChildID] == nil {
			given[p.ChildID] = map[string]bool{}
		}
		given[p.ChildID][p.VaccineCode] = true
	}

	due, overdue = map[string]int64{}, map[string]int64{}
	now := time.Now()
	for _, c := range children {
		statuses := registry.DueVaccines(schedule, c.DateOfBirth.Time, given[c.ID], now)
		if len(statuses) == 0 {
			continue
		}
		due[c.AreaID]++
		for _, st := range statuses {
			if st.Overdue {
				overdue[c.AreaID]++
				break
			}
		}
	}
	return due, overdue, nil
}

func riskScoreMsg(
	areaName string, lat, lon float64, disease, level, driver string,
	driverValue float64, forecastDate pgtype.Date, predictor, version string,
	scoredAt pgtype.Timestamptz, exceedance *float64, explanation *string,
) *climateshieldv1.RiskScore {
	msg := &climateshieldv1.RiskScore{
		Area:             areaName,
		Latitude:         lat,
		Longitude:        lon,
		Disease:          diseaseEnum[disease],
		Level:            levelEnum[level],
		ForecastDate:     forecastDate.Time.Format("2006-01-02"),
		Driver:           driver,
		DriverValue:      driverValue,
		Predictor:        predictor,
		PredictorVersion: version,
		ScoredAt:         timestamppb.New(scoredAt.Time),
	}
	// The explainability columns, published as stored. Both describe WEATHER:
	// the exceedance is how unusual this driver value is for this county and
	// month against the reference climatology, and the explanation is the
	// sentence the predictor wrote about the cutoff it applied. Neither is
	// derived from a person, and publishing them is the difference between a
	// risk level a health officer can act on and one they must simply accept.
	if exceedance != nil {
		v := *exceedance
		msg.Exceedance = &v
	}
	if explanation != nil {
		msg.Explanation = *explanation
	}
	return msg
}

func optionalDate(s string) (pgtype.Date, error) {
	if s == "" {
		return pgtype.Date{}, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

// errBadRequest marks client errors (400), as opposed to backend failures
// that trigger the stale-cache path.
type errBadRequest struct{ msg string }

func (e errBadRequest) Error() string { return e.msg }
