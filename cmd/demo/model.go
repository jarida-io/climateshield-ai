// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jarida-io/climateshield/internal/predict"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// DemoScorerCounty is the county the side-by-side comparison uses. Kisumu is
// the demo scenario's wettest county, so the two scorers have something to
// disagree about.
const DemoScorerCounty = "kisumu"

// reportBothScorers runs both predictors over ONE county's actual forecast
// window and prints what each makes of it.
//
// Only one of them scored the alerts above: whichever PREDICTOR the running
// deployment is configured with. The other column is computed here, in the
// demo process, purely for comparison — it wrote nothing to the database and
// sent nothing to anybody, and the output says so.
func reportBothScorers(ctx context.Context, q *db.Queries, areaID string, out io.Writer) error {
	window, err := q.LatestObservationWindow(ctx, areaID)
	if err != nil {
		return fmt.Errorf("demo: window for %s: %w", areaID, err)
	}
	if len(window) == 0 {
		say(out, "\n--- Same weather, both scorers ---\n")
		say(out, "no observation window for %s yet — nothing to compare\n", areaID)
		return nil
	}

	precip := make([]float64, 0, len(window))
	tmax := make([]float64, 0, len(window))
	tmin := make([]float64, 0, len(window))
	for _, row := range window {
		precip = append(precip, row.PrecipitationSumMm)
		tmax = append(tmax, row.TempMaxC)
		tmin = append(tmin, row.TempMinC)
	}
	forecastDate := window[0].ForecastDate.Time
	feats, err := predict.FeaturesFrom(areaID, int(forecastDate.Month()), precip, tmax, tmin)
	if err != nil {
		return err
	}

	clim, err := predict.NewClimatologyPredictor()
	if err != nil {
		return fmt.Errorf("demo: reference climatology: %w", err)
	}
	rules := predict.Annotate(predict.NewRulesPredictor(), clim.Reference())

	active, err := activePredictor(ctx, q, areaID)
	if err != nil {
		return err
	}

	say(out, "\n--- Same weather, both scorers (%s, %d-day window from %s) ---\n",
		areaID, len(window), forecastDate.Format("2006-01-02"))
	say(out, "the levels that produced the alerts above came from: %s\n", active)
	say(out, "  %-11s  %-38s  %s\n", "disease", "published thresholds", "reference climatology")

	byDisease := map[predict.Disease]predict.Prediction{}
	for _, p := range clim.Predict(feats) {
		byDisease[p.Disease] = p
	}
	for _, r := range rules.Predict(feats) {
		c := byDisease[r.Disease]
		say(out, "  %-11s  %-38s  %s\n",
			r.Disease, scorerCell(r), scorerCell(c))
	}
	sayln(out, "the two columns read the same weather; they do not always read the same variable —")
	sayln(out, "for pneumonia the published rule uses the 14-day mean MAXIMUM temperature and the")
	sayln(out, "climatology uses the mean MINIMUM (see docs/threshold-validation.md).")
	sayln(out, "neither column is validated against disease outcomes: this system holds none.")
	sayln(out, "only the active predictor above wrote scores or triggered alerts; the other column")
	sayln(out, "was computed by this demo for comparison and sent nothing.")
	return nil
}

// say and sayln write the demo's own report. A failed write to a console is
// not worth failing the demo over, so the error is dropped deliberately
// rather than by omission.
func say(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }

func sayln(w io.Writer, line string) { _, _ = fmt.Fprintln(w, line) }

// scorerCell renders one prediction as "LEVEL value unit [rarity]". A value
// at or past the end of the reference record reads as that, rather than as a
// "top 0.0%" that looks like a missing number.
func scorerCell(p predict.Prediction) string {
	cell := fmt.Sprintf("%-6s %.1f%s", p.Level, p.DriverValue, driverUnit(p.Driver))
	switch {
	case p.Exceedance == nil:
		return cell
	case *p.Exceedance <= 0:
		return cell + " [at the record extreme]"
	default:
		return cell + fmt.Sprintf(" [top %.1f%%]", *p.Exceedance*100)
	}
}

func driverUnit(driver string) string {
	if driver == predict.DriverPeakRainfall {
		return "mm"
	}
	return "C"
}

// activePredictor reports which predictor actually scored this county, read
// back from the rows it wrote rather than from configuration.
func activePredictor(ctx context.Context, q *db.Queries, areaID string) (string, error) {
	rows, err := q.CurrentRisk(ctx)
	if err != nil {
		return "", err
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if r.AreaID == areaID {
			seen[fmt.Sprintf("%s v%s", r.Predictor, r.PredictorVersion)] = true
		}
	}
	if len(seen) == 0 {
		return "no predictor has scored this county yet", nil
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", "), nil
}
