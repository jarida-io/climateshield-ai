-- SPDX-License-Identifier: Apache-2.0
-- County risk briefings. Every row here is derived from aggregate facts only
-- (see migration 0011): no query in this file reads a person's row, and none
-- may be added that does.

-- name: GetBriefingForKey :one
-- Cache lookup. A row for this exact key means the facts have not changed
-- since that briefing was written, so nothing needs regenerating.
SELECT id, area_id, lang, facts_hash, facts_json, generator, model,
       prompt_version, body, grounded, grounding_notes, status, created_at
FROM briefings
WHERE area_id = $1 AND lang = $2 AND facts_hash = $3
  AND generator = $4 AND model = $5 AND prompt_version = $6;

-- name: LatestBriefing :one
-- What the public API serves: the newest briefing for one county and language.
SELECT id, area_id, lang, facts_hash, facts_json, generator, model,
       prompt_version, body, grounded, grounding_notes, status, created_at
FROM briefings
WHERE area_id = $1 AND lang = $2
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: ListBriefingAreas :many
-- Counties that have at least one briefing in this language, for the API to
-- tell a caller what it can ask for instead of guessing.
SELECT DISTINCT b.area_id, a.name AS area_name
FROM briefings b
JOIN areas a ON a.id = b.area_id
WHERE b.lang = $1
ORDER BY a.name;

-- name: UpsertBriefing :one
-- Re-running a generation for the same facts replaces the row rather than
-- accumulating duplicates: an 'unavailable' attempt becomes 'served' when the
-- generator comes back, and the served text is always the latest truth.
INSERT INTO briefings (
    area_id, lang, facts_hash, facts_json, generator, model, prompt_version,
    body, grounded, grounding_notes, status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (area_id, lang, facts_hash, generator, model, prompt_version)
DO UPDATE SET
    facts_json = excluded.facts_json,
    body = excluded.body,
    grounded = excluded.grounded,
    grounding_notes = excluded.grounding_notes,
    status = excluded.status,
    created_at = now()
RETURNING id;

-- name: CountBriefingsByStatus :many
SELECT status, count(*) AS n FROM briefings GROUP BY status ORDER BY status;
