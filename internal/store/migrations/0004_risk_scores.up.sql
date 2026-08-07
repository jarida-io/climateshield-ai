-- SPDX-License-Identifier: Apache-2.0
-- One row per area x disease x forecast window start. Every row records which
-- predictor produced it (name + version) so scores are auditable after model
-- changes. Rescoring the same window upserts rather than duplicating.
CREATE TABLE risk_scores (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    area_id text NOT NULL REFERENCES areas (id),
    disease text NOT NULL CHECK (disease IN ('cholera', 'malaria', 'pneumonia', 'meningitis')),
    level text NOT NULL CHECK (level IN ('LOW', 'MEDIUM', 'HIGH')),
    driver text NOT NULL,
    driver_value double precision NOT NULL,
    forecast_date date NOT NULL,
    window_days int NOT NULL DEFAULT 14,
    predictor text NOT NULL,
    predictor_version text NOT NULL,
    scored_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (area_id, disease, forecast_date)
);

CREATE INDEX risk_scores_forecast_date_idx ON risk_scores (forecast_date DESC);
