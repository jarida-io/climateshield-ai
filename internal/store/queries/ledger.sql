-- SPDX-License-Identifier: Apache-2.0
-- The ONLY file allowed to reference the sealed schema (per-child HMAC keys).
-- scripts/contract-checks.sh enforces this.

-- name: InsertChildKey :exec
INSERT INTO sealed.child_keys (child_id, hmac_key)
VALUES ($1, $2)
ON CONFLICT (child_id) DO NOTHING;

-- name: GetChildKey :one
SELECT hmac_key FROM sealed.child_keys WHERE child_id = $1;

-- name: DestroyChildKey :execrows
DELETE FROM sealed.child_keys WHERE child_id = $1;

-- name: ListEventsWithoutLeaves :many
-- Immunization events the ledger has not yet committed to a Merkle leaf.
SELECT ie.id, ie.child_id, ie.vaccine_code, ie.administered_at, ie.recorded_at
FROM immunization_events ie
LEFT JOIN event_leaves el ON el.event_id = ie.id
WHERE el.event_id IS NULL
ORDER BY ie.recorded_at, ie.id;

-- name: ScrubChildFromLeaves :execrows
-- Erasure: sever child->leaf linkage; the anonymous hash stays so daily
-- roots remain structurally verifiable.
UPDATE event_leaves SET child_id = NULL WHERE child_id = $1;

-- name: InsertLeaf :exec
INSERT INTO event_leaves (event_id, child_id, leaf_day, leaf_hash)
VALUES ($1, $2, $3, $4)
ON CONFLICT (event_id) DO NOTHING;

-- name: LeavesForDay :many
SELECT event_id, leaf_hash
FROM event_leaves
WHERE leaf_day = $1
ORDER BY event_id;

-- name: ListLeafDays :many
SELECT DISTINCT leaf_day FROM event_leaves ORDER BY leaf_day;

-- name: UpsertDailyRoot :exec
INSERT INTO daily_roots (leaf_day, root, leaf_count)
VALUES ($1, $2, $3)
ON CONFLICT (leaf_day) DO UPDATE SET
    root = excluded.root,
    leaf_count = excluded.leaf_count,
    computed_at = now();

-- name: GetDailyRoot :one
SELECT * FROM daily_roots WHERE leaf_day = $1;

-- name: InsertAnchor :exec
INSERT INTO anchors (leaf_day, anchor_type, reference)
VALUES ($1, $2, $3);

-- name: ListAnchorsForDay :many
SELECT * FROM anchors WHERE leaf_day = $1 ORDER BY anchored_at, id;
