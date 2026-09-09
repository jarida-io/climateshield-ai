-- SPDX-License-Identifier: Apache-2.0
-- County risk briefings: plain-language text generated from the AGGREGATE
-- fact sheet the public API already publishes.
--
-- Nothing person-derived may enter this table. facts_json is the exact fact
-- sheet a generator was given, and it is built from county x disease scores,
-- the forecast window and k>=10 suppressed alert-status counts only — there is
-- no child, guardian, phone or per-child hash anywhere in it. Storing the
-- facts next to the text is the point: every sentence served can be checked
-- against the numbers it was allowed to use.
--
-- status vocabulary, mirroring the alerts table's no-false-output rule:
--   served      — body came from `generator` and passed the grounding check.
--   rejected    — a language model produced a draft that FAILED the grounding
--                 check; the body stored here is the deterministic template,
--                 labelled as such, and grounding_notes says why the draft was
--                 refused. Model text is never stored or served.
--   unavailable — the configured generator could not be reached; the body is
--                 again the labelled template.
-- A row therefore always holds the text that was actually served, and never
-- text a model did not produce under a model's name.
CREATE TABLE briefings (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    area_id text NOT NULL REFERENCES areas (id),
    lang text NOT NULL CHECK (lang IN ('en', 'sw')),
    -- SHA-256 over the canonical fact sheet. Regeneration is keyed on this:
    -- unchanged facts mean the stored briefing still describes the world.
    facts_hash bytea NOT NULL,
    facts_json jsonb NOT NULL,
    generator text NOT NULL,
    model text NOT NULL,
    prompt_version text NOT NULL,
    body text NOT NULL,
    grounded boolean NOT NULL,
    grounding_notes jsonb NOT NULL DEFAULT '[]'::jsonb,
    status text NOT NULL CHECK (status IN ('served', 'rejected', 'unavailable')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (area_id, lang, facts_hash, generator, model, prompt_version)
);

CREATE INDEX briefings_latest_idx ON briefings (area_id, lang, created_at DESC);
