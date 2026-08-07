-- SPDX-License-Identifier: Apache-2.0
-- One row per area x forecast day x forecast issue time. The ingestor
-- upserts on (area_id, forecast_date, issued_at), so re-running ingestion is
-- idempotent by construction. forecast_date is the Africa/Nairobi calendar
-- date as returned by the source; issued_at is UTC.
CREATE TABLE climate_observations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    area_id text NOT NULL REFERENCES areas (id),
    forecast_date date NOT NULL,
    issued_at timestamptz NOT NULL,
    precipitation_sum_mm double precision NOT NULL,
    temp_max_c double precision NOT NULL,
    temp_min_c double precision NOT NULL,
    humidity_max_pct double precision,
    source text NOT NULL,
    ingested_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (area_id, forecast_date, issued_at)
);

CREATE INDEX climate_observations_area_date_idx
    ON climate_observations (area_id, forecast_date);
