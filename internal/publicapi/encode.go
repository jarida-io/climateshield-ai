// SPDX-License-Identifier: Apache-2.0

package publicapi

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
)

// Formats. JSON is canonical protojson over the same proto messages the
// Connect endpoint serves; CSV and GeoJSON are derived views for
// spreadsheets and maps.
const (
	formatJSON    = "json"
	formatCSV     = "csv"
	formatGeoJSON = "geojson"
)

// encode renders msg in the requested format. Unknown formats and
// format/endpoint mismatches are client errors (errBadRequest).
func encode(msg proto.Message, format string) (body []byte, contentType string, err error) {
	switch format {
	case "", formatJSON:
		b, err := protojson.MarshalOptions{}.Marshal(msg)
		return b, "application/json", err
	case formatCSV:
		return encodeCSV(msg)
	case formatGeoJSON:
		return encodeGeoJSON(msg)
	default:
		return nil, "", errBadRequest{fmt.Sprintf("unknown format %q (want json, csv or geojson)", format)}
	}
}

func encodeCSV(msg proto.Message) ([]byte, string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	switch m := msg.(type) {
	case *climateshieldv1.GetCurrentRiskResponse:
		writeRiskCSV(w, m.GetScores())
	case *climateshieldv1.GetRiskHistoryResponse:
		writeRiskCSV(w, m.GetScores())
	case *climateshieldv1.GetStatsResponse:
		writeStatsCSV(w, m.GetStats())
	default:
		return nil, "", errBadRequest{"csv is not available for this endpoint"}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "text/csv; charset=utf-8", nil
}

func writeRiskCSV(w *csv.Writer, scores []*climateshieldv1.RiskScore) {
	_ = w.Write([]string{
		"area", "disease", "level", "forecast_date", "driver", "driver_value",
		"predictor", "predictor_version", "scored_at", "latitude", "longitude",
	})
	for _, s := range scores {
		_ = w.Write([]string{
			s.GetArea(), shortDisease(s.GetDisease()), shortLevel(s.GetLevel()),
			s.GetForecastDate(), s.GetDriver(),
			strconv.FormatFloat(s.GetDriverValue(), 'f', -1, 64),
			s.GetPredictor(), s.GetPredictorVersion(),
			s.GetScoredAt().AsTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
			strconv.FormatFloat(s.GetLatitude(), 'f', -1, 64),
			strconv.FormatFloat(s.GetLongitude(), 'f', -1, 64),
		})
	}
}

func writeStatsCSV(w *csv.Writer, stats []*climateshieldv1.CountyStats) {
	_ = w.Write([]string{
		"area",
		"children_registered", "children_registered_suppressed",
		"children_due", "children_due_suppressed",
		"children_overdue", "children_overdue_suppressed",
		"alerts_generated", "alerts_generated_suppressed",
	})
	optional := func(v *int64) string {
		if v == nil {
			return ""
		}
		return strconv.FormatInt(*v, 10)
	}
	for _, s := range stats {
		_ = w.Write([]string{
			s.GetArea(),
			optional(s.ChildrenRegistered), strconv.FormatBool(s.GetChildrenRegisteredSuppressed()),
			optional(s.ChildrenDue), strconv.FormatBool(s.GetChildrenDueSuppressed()),
			optional(s.ChildrenOverdue), strconv.FormatBool(s.GetChildrenOverdueSuppressed()),
			optional(s.AlertsGenerated), strconv.FormatBool(s.GetAlertsGeneratedSuppressed()),
		})
	}
}

// geoJSON structures (spec: RFC 7946).
type geoFeature struct {
	Type       string         `json:"type"`
	Geometry   geoPoint       `json:"geometry"`
	Properties map[string]any `json:"properties"`
}

type geoPoint struct {
	Type        string     `json:"type"`
	Coordinates [2]float64 `json:"coordinates"` // lon, lat
}

func encodeGeoJSON(msg proto.Message) ([]byte, string, error) {
	var scores []*climateshieldv1.RiskScore
	switch m := msg.(type) {
	case *climateshieldv1.GetCurrentRiskResponse:
		scores = m.GetScores()
	case *climateshieldv1.GetRiskHistoryResponse:
		scores = m.GetScores()
	default:
		return nil, "", errBadRequest{"geojson is only available for risk endpoints"}
	}
	features := make([]geoFeature, 0, len(scores))
	for _, s := range scores {
		features = append(features, geoFeature{
			Type: "Feature",
			Geometry: geoPoint{
				Type:        "Point",
				Coordinates: [2]float64{s.GetLongitude(), s.GetLatitude()},
			},
			Properties: map[string]any{
				"area":              s.GetArea(),
				"disease":           shortDisease(s.GetDisease()),
				"level":             shortLevel(s.GetLevel()),
				"forecast_date":     s.GetForecastDate(),
				"driver":            s.GetDriver(),
				"driver_value":      s.GetDriverValue(),
				"predictor":         s.GetPredictor(),
				"predictor_version": s.GetPredictorVersion(),
			},
		})
	}
	body, err := json.Marshal(map[string]any{"type": "FeatureCollection", "features": features})
	return body, "application/geo+json", err
}

func shortDisease(d climateshieldv1.Disease) string {
	return strings.ToLower(strings.TrimPrefix(d.String(), "DISEASE_"))
}

func shortLevel(l climateshieldv1.RiskLevel) string {
	return strings.TrimPrefix(l.String(), "RISK_LEVEL_")
}
