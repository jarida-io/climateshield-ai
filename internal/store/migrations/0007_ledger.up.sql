-- SPDX-License-Identifier: Apache-2.0
-- Tamper-evident ledger. Each immunization event yields one HMAC-SHA256 leaf
-- keyed by a per-child key; a Merkle tree over each day's leaves produces a
-- daily root. Leaves are unlinkable pseudonymous hashes: without the child's
-- key nothing in this schema derives back to a person.

CREATE TABLE event_leaves (
    event_id uuid PRIMARY KEY,
    child_id uuid NOT NULL,
    leaf_day date NOT NULL,
    leaf_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX event_leaves_day_idx ON event_leaves (leaf_day, event_id);
CREATE INDEX event_leaves_child_idx ON event_leaves (child_id);

CREATE TABLE daily_roots (
    leaf_day date PRIMARY KEY,
    root bytea NOT NULL,
    leaf_count int NOT NULL,
    computed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE anchors (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    leaf_day date NOT NULL REFERENCES daily_roots (leaf_day),
    anchor_type text NOT NULL,
    reference text,
    anchored_at timestamptz NOT NULL DEFAULT now()
);

-- Per-child HMAC keys live in their own schema, physically separate from the
-- event data tables. Skeleton honesty note: same database instance and role;
-- production separates roles (schema-scoped GRANTs) and then external key
-- management. Only internal/store/queries/ledger.sql may reference sealed.*
-- (enforced by scripts/contract-checks.sh). ForgetChild deletes the key row —
-- that destruction is what makes erased children underivable.
CREATE SCHEMA sealed;

CREATE TABLE sealed.child_keys (
    child_id uuid PRIMARY KEY,
    hmac_key bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
