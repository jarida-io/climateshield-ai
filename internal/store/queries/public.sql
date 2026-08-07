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
