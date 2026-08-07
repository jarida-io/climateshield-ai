-- SPDX-License-Identifier: Apache-2.0

-- name: UpsertArea :exec
INSERT INTO areas (id, name, level, centroid)
VALUES (
    $1, $2, $3,
    ST_SetSRID(ST_MakePoint(sqlc.arg(longitude)::float8, sqlc.arg(latitude)::float8), 4326)
)
ON CONFLICT (id) DO UPDATE SET
    name = excluded.name,
    level = excluded.level,
    centroid = excluded.centroid;

-- name: ListAreas :many
SELECT
    id,
    name,
    level,
    ST_X(centroid)::float8 AS longitude,
    ST_Y(centroid)::float8 AS latitude
FROM areas
ORDER BY name;
