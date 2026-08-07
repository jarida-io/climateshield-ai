// SPDX-License-Identifier: Apache-2.0

package climate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jarida-io/climateshield/internal/store/db"
)

// UpsertForecast persists a forecast window. Keyed on (area_id,
// forecast_date, issued_at), so re-ingesting the same batch is a no-op update
// and a newly issued batch creates fresh rows alongside history.
func UpsertForecast(ctx context.Context, q *db.Queries, fc Forecast) (int, error) {
	for _, d := range fc.Days {
		err := q.UpsertClimateObservation(ctx, db.UpsertClimateObservationParams{
			AreaID:             fc.AreaID,
			ForecastDate:       pgtype.Date{Time: d.Date, Valid: true},
			IssuedAt:           pgtype.Timestamptz{Time: fc.IssuedAt, Valid: true},
			PrecipitationSumMm: d.PrecipitationSumMM,
			TempMaxC:           d.TempMaxC,
			TempMinC:           d.TempMinC,
			HumidityMaxPct:     d.HumidityMaxPct,
			Source:             fc.Source,
		})
		if err != nil {
			return 0, fmt.Errorf("climate: upsert %s/%s: %w", fc.AreaID, d.Date.Format("2006-01-02"), err)
		}
	}
	return len(fc.Days), nil
}
