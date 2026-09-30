-- name: ListReleases :many
-- MSL-67: a project's releases, unreleased ones first, then the latest.
SELECT * FROM releases WHERE project_id = $1 ORDER BY released_on DESC NULLS FIRST, id DESC;

-- name: GetRelease :one
SELECT * FROM releases WHERE id = $1;

-- name: CreateRelease :one
INSERT INTO releases (project_id, name, released_on) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateRelease :one
UPDATE releases SET name = $2, released_on = $3 WHERE id = $1 RETURNING *;
