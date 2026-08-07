-- SPDX-License-Identifier: Apache-2.0

-- name: UpsertRiskScore :one
INSERT INTO risk_scores (
    area_id, disease, level, driver, driver_value,
    forecast_date, window_days, predictor, predictor_version
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (area_id, disease, forecast_date) DO UPDATE SET
    level = excluded.level,
    driver = excluded.driver,
    driver_value = excluded.driver_value,
    window_days = excluded.window_days,
    predictor = excluded.predictor,
    predictor_version = excluded.predictor_version,
    scored_at = now()
RETURNING id;

-- name: CurrentRisk :many
-- Latest score per area x disease, joined with the county centroid for maps.
SELECT DISTINCT ON (rs.area_id, rs.disease)
    rs.id, rs.area_id, rs.disease, rs.level, rs.driver, rs.driver_value,
    rs.forecast_date, rs.window_days, rs.predictor, rs.predictor_version,
    rs.scored_at,
    a.name AS area_name,
    ST_X(a.centroid)::float8 AS longitude,
    ST_Y(a.centroid)::float8 AS latitude
FROM risk_scores rs
JOIN areas a ON a.id = rs.area_id
ORDER BY rs.area_id, rs.disease, rs.forecast_date DESC, rs.scored_at DESC;

-- name: RiskHistory :many
SELECT
    rs.id, rs.area_id, rs.disease, rs.level, rs.driver, rs.driver_value,
    rs.forecast_date, rs.window_days, rs.predictor, rs.predictor_version,
    rs.scored_at,
    a.name AS area_name,
    ST_X(a.centroid)::float8 AS longitude,
    ST_Y(a.centroid)::float8 AS latitude
FROM risk_scores rs
JOIN areas a ON a.id = rs.area_id
WHERE (sqlc.narg(area_name)::text IS NULL OR a.name = sqlc.narg(area_name))
  AND (sqlc.narg(disease)::text IS NULL OR rs.disease = sqlc.narg(disease))
  AND (sqlc.narg(from_date)::date IS NULL OR rs.forecast_date >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR rs.forecast_date <= sqlc.narg(to_date))
ORDER BY rs.forecast_date DESC, a.name, rs.disease
LIMIT sqlc.arg(row_limit);
