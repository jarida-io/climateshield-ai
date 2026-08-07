-- SPDX-License-Identifier: Apache-2.0
-- Registry tables. Personal data (names, phones, national IDs) is stored only
-- as AES-256-GCM blobs sealed by internal/platform/crypto; the key lives in
-- the environment, never in this database. Date of birth stays plain because
-- KEPI due/overdue computation needs it; it never reaches a public surface.

CREATE TABLE guardians (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name_enc bytea NOT NULL,
    phone_enc bytea NOT NULL,
    national_id_enc bytea,
    lang text NOT NULL DEFAULT 'sw' CHECK (lang IN ('en', 'sw')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE children (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    guardian_id uuid NOT NULL REFERENCES guardians (id) ON DELETE CASCADE,
    area_id text NOT NULL REFERENCES areas (id),
    name_enc bytea NOT NULL,
    date_of_birth date NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX children_area_idx ON children (area_id);

-- Consent is an event log, never an updated flag: the latest row per guardian
-- decides. STOP replies append OPT_OUT.
CREATE TABLE consent_log (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    guardian_id uuid NOT NULL REFERENCES guardians (id) ON DELETE CASCADE,
    action text NOT NULL CHECK (action IN ('OPT_IN', 'OPT_OUT')),
    channel text NOT NULL DEFAULT 'sms',
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX consent_log_guardian_idx ON consent_log (guardian_id, occurred_at DESC);

CREATE TABLE immunization_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    child_id uuid NOT NULL REFERENCES children (id),
    vaccine_code text NOT NULL,
    administered_at timestamptz NOT NULL,
    facility text,
    recorded_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX immunization_events_child_idx ON immunization_events (child_id);

-- Append-only enforcement. Immunization history must be tamper-evident:
-- UPDATE is never allowed; DELETE is allowed only on the explicit erasure
-- path (ForgetChild), which sets the transaction-local flag below before
-- deleting. Nothing else may mutate a recorded event.
CREATE FUNCTION forbid_immunization_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF tg_op = 'DELETE'
        AND current_setting('climateshield.allow_erasure', true) = 'on' THEN
        RETURN old;
    END IF;
    RAISE EXCEPTION 'immunization_events is append-only (% blocked)', tg_op
        USING errcode = 'raise_exception';
END;
$$;

CREATE TRIGGER immunization_events_append_only
BEFORE UPDATE OR DELETE ON immunization_events
FOR EACH ROW EXECUTE FUNCTION forbid_immunization_mutation();
