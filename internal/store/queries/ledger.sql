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

-- name: GetLeaf :one
SELECT * FROM event_leaves WHERE event_id = $1;

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
-- One row per publication of one daily root. `root` is the root that was
-- published; `readback_root` is what the anchor read back from wherever it
-- published, so a row is checkable on its own. Only whole-day roots are ever
-- written here or anywhere an anchor points to.
INSERT INTO anchors (
    leaf_day, anchor_type, reference, root,
    chain_id, chain_label, contract_address, tx_hash, block_number,
    readback_root, verified_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: ListAnchorsForDay :many
SELECT * FROM anchors WHERE leaf_day = $1 ORDER BY anchored_at, id;

-- name: AnchorExistsForRoot :one
-- Whether THIS root of THIS day has already been published through the given
-- anchor type. The sweep anchors whenever this is false, so a root whose
-- anchoring failed once is retried on the next sweep instead of being
-- forgotten.
SELECT EXISTS (
    SELECT 1 FROM anchors
    WHERE leaf_day = $1 AND anchor_type = $2 AND root = $3
) AS exists;

-- name: LatestAnchorForDay :one
SELECT * FROM anchors
WHERE leaf_day = $1 AND anchor_type = $2
ORDER BY anchored_at DESC, id DESC
LIMIT 1;

-- name: GetAnchorContract :one
SELECT * FROM anchor_contracts WHERE chain_id = $1;

-- name: UpsertAnchorContract :exec
INSERT INTO anchor_contracts (chain_id, address, deploy_tx)
VALUES ($1, $2, $3)
ON CONFLICT (chain_id) DO UPDATE SET
    address = excluded.address,
    deploy_tx = excluded.deploy_tx,
    deployed_at = now();
