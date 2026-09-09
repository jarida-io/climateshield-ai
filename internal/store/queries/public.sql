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
-- Only the NEWEST anchor per day is returned: the number of anchor versions
-- a day accumulated would track how many late immunizations were recorded,
-- which is a people-derived count.
SELECT dr.leaf_day, dr.root, dr.leaf_count, dr.computed_at,
       COALESCE(an.anchor_type, '')::text AS anchor_type,
       an.anchored_at,
       an.reference AS anchor_reference,
       an.chain_id, an.chain_label, an.contract_address, an.tx_hash,
       an.block_number, an.readback_root, an.verified_at
FROM daily_roots dr
LEFT JOIN LATERAL (
    SELECT a.anchor_type, a.anchored_at, a.reference, a.chain_id, a.chain_label,
           a.contract_address, a.tx_hash, a.block_number, a.readback_root, a.verified_at
    FROM anchors a
    WHERE a.leaf_day = dr.leaf_day
    ORDER BY a.anchored_at DESC, a.id DESC
    LIMIT 1
) an ON true
ORDER BY dr.leaf_day DESC;

-- name: CountRiskScores :one
SELECT count(*) FROM risk_scores;

-- name: LatestObservationIssuedAt :one
SELECT max(issued_at)::timestamptz FROM climate_observations;
