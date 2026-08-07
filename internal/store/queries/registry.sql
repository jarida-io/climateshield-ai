-- SPDX-License-Identifier: Apache-2.0
-- immunization_events is append-only: INSERT and SELECT only in this file,
-- with one exception — EraseChildEvents, the guarded erasure path used by
-- ForgetChild. It only works inside a transaction that has set
-- climateshield.allow_erasure = 'on' (see the trigger in migration 0005);
-- otherwise the trigger raises. UPDATE is never allowed for any caller.

-- name: CreateGuardian :one
INSERT INTO guardians (name_enc, phone_enc, national_id_enc, lang)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: GetGuardian :one
SELECT * FROM guardians WHERE id = $1;

-- name: CreateChild :one
INSERT INTO children (guardian_id, area_id, name_enc, date_of_birth)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: GetChild :one
SELECT * FROM children WHERE id = $1;

-- name: ListChildren :many
SELECT * FROM children ORDER BY created_at, id;

-- name: DeleteChild :exec
DELETE FROM children WHERE id = $1;

-- name: AppendConsent :exec
INSERT INTO consent_log (guardian_id, action, channel)
VALUES ($1, $2, $3);

-- name: LatestConsent :one
SELECT action
FROM consent_log
WHERE guardian_id = $1
ORDER BY occurred_at DESC, id DESC
LIMIT 1;

-- name: InsertImmunizationEvent :one
INSERT INTO immunization_events (child_id, vaccine_code, administered_at, facility)
VALUES ($1, $2, $3, $4)
RETURNING id, recorded_at;

-- name: GetImmunizationEvent :one
SELECT * FROM immunization_events WHERE id = $1;

-- name: ListEventsForChild :many
SELECT * FROM immunization_events WHERE child_id = $1 ORDER BY administered_at, id;

-- name: ListImmunizationPairs :many
SELECT child_id, vaccine_code FROM immunization_events;

-- name: EraseChildEvents :execrows
DELETE FROM immunization_events WHERE child_id = $1;

-- name: ListVaccineSchedule :many
SELECT * FROM vaccine_schedule ORDER BY due_age_days, code;
