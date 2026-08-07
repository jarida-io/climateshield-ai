-- SPDX-License-Identifier: Apache-2.0

-- name: InsertAlert :one
INSERT INTO alerts (
    risk_score_id, child_id, guardian_id, area_id, vaccine_code,
    lang, channel, status, message_hash, message_id, dispatched_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: CountAlertsByStatus :many
SELECT status, count(*) AS n FROM alerts GROUP BY status ORDER BY status;

-- name: ExistsAlertForChildRisk :one
-- Dedup guard: has this child already been alerted for this risk score?
SELECT EXISTS (
    SELECT 1 FROM alerts WHERE child_id = $1 AND risk_score_id = $2
);
