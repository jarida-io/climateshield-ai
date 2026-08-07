-- SPDX-License-Identifier: Apache-2.0
-- Aggregate queries feeding the public tier. Everything returns counts by
-- county — never rows about individual people. k-anonymity suppression is
-- applied in internal/publicapi before any of these numbers leave the
-- process.

-- name: CountChildrenByArea :many
SELECT area_id, count(*) AS n FROM children GROUP BY area_id ORDER BY area_id;

-- name: CountDispatchedAlertsByArea :many
SELECT area_id, count(*) AS n
FROM alerts
WHERE status IN ('would_send', 'sent')
GROUP BY area_id
ORDER BY area_id;

-- name: ListChildrenForDueComputation :many
-- Feeds the KEPI due/overdue computation (pure Go, internal/registry). Only
-- opaque ids, area and DOB — no encrypted columns are read here.
SELECT c.id, c.area_id, c.date_of_birth FROM children c ORDER BY c.id;

-- name: LatestSeriesForAllAreas :many
-- The most recent forecast window per area, for the climate view. Weather
-- only: no person appears in this result.
SELECT co.area_id, a.name AS area_name, co.forecast_date, co.issued_at,
       co.precipitation_sum_mm, co.temp_max_c, co.temp_min_c, co.source
FROM climate_observations co
JOIN areas a ON a.id = co.area_id
WHERE co.issued_at = (
    SELECT max(inner_co.issued_at)
    FROM climate_observations inner_co
    WHERE inner_co.area_id = co.area_id
)
ORDER BY a.name, co.forecast_date;

-- name: LedgerRootSummary :many
-- Daily Merkle roots and their anchors. A root is a commitment over a whole
-- day; no individual leaf is selected here and none may ever be published.
SELECT dr.leaf_day, dr.root, dr.leaf_count, dr.computed_at,
       COALESCE((SELECT an.anchor_type FROM anchors an WHERE an.leaf_day = dr.leaf_day
        ORDER BY an.anchored_at DESC LIMIT 1), '')::text AS anchor_type,
       (SELECT an.anchored_at FROM anchors an WHERE an.leaf_day = dr.leaf_day
        ORDER BY an.anchored_at DESC LIMIT 1) AS anchored_at
FROM daily_roots dr
ORDER BY dr.leaf_day DESC;

-- name: CountRiskScores :one
SELECT count(*) FROM risk_scores;

-- name: LatestObservationIssuedAt :one
SELECT max(issued_at)::timestamptz FROM climate_observations;
