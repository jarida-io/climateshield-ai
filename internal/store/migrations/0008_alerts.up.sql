-- SPDX-License-Identifier: Apache-2.0
-- Alert outbox. The rendered message text is NOT stored (it contains a child
-- first name) — only a SHA-256 of it, enough to audit what was rendered.
-- Status vocabulary encodes the no-false-output rule: the mock channel
-- records would_send, never sent. Only a real carrier adapter may write sent.
CREATE TABLE alerts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    risk_score_id bigint REFERENCES risk_scores (id),
    child_id uuid REFERENCES children (id) ON DELETE SET NULL,
    guardian_id uuid REFERENCES guardians (id) ON DELETE SET NULL,
    area_id text NOT NULL REFERENCES areas (id),
    vaccine_code text NOT NULL,
    lang text NOT NULL CHECK (lang IN ('en', 'sw')),
    channel text NOT NULL,
    status text NOT NULL CHECK (
        status IN ('pending', 'would_send', 'sent', 'failed', 'skipped_consent', 'skipped_quiet_hours')
    ),
    message_hash bytea,
    message_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    dispatched_at timestamptz
);

CREATE INDEX alerts_area_idx ON alerts (area_id, created_at DESC);
CREATE INDEX alerts_status_idx ON alerts (status);
