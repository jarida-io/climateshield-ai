-- SPDX-License-Identifier: Apache-2.0

-- name: UpsertClimateObservation :exec
INSERT INTO climate_observations (
    area_id, forecast_date, issued_at,
    precipitation_sum_mm, temp_max_c, temp_min_c, humidity_max_pct, source
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (area_id, forecast_date, issued_at) DO UPDATE SET
    precipitation_sum_mm = excluded.precipitation_sum_mm,
    temp_max_c = excluded.temp_max_c,
    temp_min_c = excluded.temp_min_c,
    humidity_max_pct = excluded.humidity_max_pct,
    source = excluded.source,
    ingested_at = now();

-- name: LatestObservationWindow :many
-- The most recently issued forecast batch for one area, in day order.
SELECT co.*
FROM climate_observations co
WHERE co.area_id = $1
  AND co.issued_at = (
      SELECT max(inner_co.issued_at)
      FROM climate_observations inner_co
      WHERE inner_co.area_id = $1
  )
ORDER BY co.forecast_date;

-- name: CountObservations :one
SELECT count(*) FROM climate_observations;
