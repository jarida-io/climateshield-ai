// SPDX-License-Identifier: Apache-2.0

package facts

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jarida-io/climateshield/internal/predict"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// FactQuerier is the read surface a fact sheet is built from. Every method on
// it returns aggregates; there is deliberately no query here that can return a
// child, a guardian or an unsuppressed people-derived count, so the type
// itself documents what a generator can ever be told.
type FactQuerier interface {
	ListAreas(ctx context.Context) ([]db.ListAreasRow, error)
	CurrentRisk(ctx context.Context) ([]db.CurrentRiskRow, error)
	LatestSeriesForAllAreas(ctx context.Context) ([]db.LatestSeriesForAllAreasRow, error)
	CountAlertsByStatus(ctx context.Context) ([]db.CountAlertsByStatusRow, error)
}

// Area is one monitored county: its identifier and the name a reader sees.
type Area struct {
	ID   string
	Name string
}

// Areas lists the monitored counties in display order.
func Areas(ctx context.Context, q FactQuerier) ([]Area, error) {
	rows, err := q.ListAreas(ctx)
	if err != nil {
		return nil, fmt.Errorf("briefing: areas: %w", err)
	}
	out := make([]Area, 0, len(rows))
	for _, r := range rows {
		out = append(out, Area{ID: r.ID, Name: r.Name})
	}
	return out, nil
}

// ChannelNote states what the messaging channel does, in the same words the
// public alert summary uses. A briefing that mentioned alerts without this
// would imply a delivery the mock channel never makes.
func ChannelNote(channel string) (sends bool, note string) {
	sends = channel != "mock" && channel != ""
	if sends {
		return true, fmt.Sprintf("Channel %q is active and delivers messages.", channel)
	}
	return false, "The mock channel is active: alerts are rendered and recorded, and no SMS is sent."
}

// BuildFactSheet assembles one county's fact sheet from aggregate queries.
// It returns an error when the county is unknown; a county with no scores yet
// yields a sheet with no scores, which the generators report honestly rather
// than filling in.
func BuildFactSheet(
	ctx context.Context, q FactQuerier, area Area, channel string, now time.Time,
) (FactSheet, error) {
	f := FactSheet{Area: area.Name, GeneratedAt: now.UTC()}
	f.ChannelSends, f.ChannelNote = ChannelNote(channel)

	series, err := q.LatestSeriesForAllAreas(ctx)
	if err != nil {
		return FactSheet{}, fmt.Errorf("briefing: climate window: %w", err)
	}
	for _, r := range series {
		if r.AreaID != area.ID {
			continue
		}
		day := r.ForecastDate.Time.Format("2006-01-02")
		if f.Window.From == "" || day < f.Window.From {
			f.Window.From = day
		}
		if day > f.Window.To {
			f.Window.To = day
		}
		f.Window.Days++
		f.Window.Source = r.Source
	}

	scores, err := q.CurrentRisk(ctx)
	if err != nil {
		return FactSheet{}, fmt.Errorf("briefing: current risk: %w", err)
	}
	for _, r := range scores {
		if r.AreaID != area.ID {
			continue
		}
		s := Score{
			Disease: r.Disease, Level: r.Level, Driver: r.Driver,
			DriverValue: r.DriverValue, Exceedance: r.Exceedance,
			Predictor: r.Predictor, Version: r.PredictorVersion,
		}
		if r.Explanation != nil {
			s.Explanation = *r.Explanation
		}
		f.Scores = append(f.Scores, s)
	}
	sortScores(f.Scores)

	alerts, err := q.CountAlertsByStatus(ctx)
	if err != nil {
		return FactSheet{}, fmt.Errorf("briefing: alert counts: %w", err)
	}
	for _, r := range alerts {
		c := AlertCount{Status: r.Status}
		c.Count, c.Suppressed = Suppress(r.N)
		f.AlertsAllCounties = append(f.AlertsAllCounties, c)
	}
	return f, nil
}

// diseaseOrder is the stable disease ordering, taken from the predictor so a
// briefing lists diseases in the same order the scores are produced in.
var diseaseOrder = func() map[string]int {
	m := make(map[string]int, len(predict.Diseases))
	for i, d := range predict.Diseases {
		m[string(d)] = i
	}
	return m
}()

func sortScores(scores []Score) {
	sort.SliceStable(scores, func(i, j int) bool {
		oi, oki := diseaseOrder[scores[i].Disease]
		oj, okj := diseaseOrder[scores[j].Disease]
		switch {
		case oki && okj:
			return oi < oj
		case oki:
			return true
		case okj:
			return false
		default:
			return scores[i].Disease < scores[j].Disease
		}
	})
}
