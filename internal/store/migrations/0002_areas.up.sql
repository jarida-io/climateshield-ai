-- SPDX-License-Identifier: Apache-2.0
-- Monitored administrative areas. The walking skeleton operates at county
-- level with 5 counties; the level column leaves room for sub-county
-- granularity without a schema break.
CREATE TABLE areas (
    id text PRIMARY KEY,
    name text NOT NULL UNIQUE,
    level text NOT NULL DEFAULT 'county' CHECK (level IN ('county', 'subcounty')),
    centroid geometry (Point, 4326) NOT NULL
);
