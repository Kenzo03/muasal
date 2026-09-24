-- name: CreateProject :one
INSERT INTO projects (key, name, description)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetProjectByKey :one
SELECT * FROM projects WHERE key = $1;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = $1;

-- name: LockProject :exec
-- Serializes tree moves inside one project, so two moves cannot build a cycle.
SELECT id FROM projects WHERE id = $1 FOR UPDATE;

-- name: ListProjects :many
-- System admins see every project; everyone else sees the projects they belong to.
SELECT sqlc.embed(p), m.role
FROM projects p
LEFT JOIN memberships m ON m.project_id = p.id AND m.user_id = sqlc.arg('user_id')
WHERE sqlc.arg('is_admin')::boolean OR m.user_id IS NOT NULL
ORDER BY p.key;

-- name: UpdateProject :one
UPDATE projects SET
  key         = coalesce(sqlc.narg('key'), key),
  name        = coalesce(sqlc.narg('name'), name),
  description = coalesce(sqlc.narg('description'), description)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ListProjectClients :many
SELECT c.* FROM clients c
JOIN project_clients pc ON pc.client_id = c.id
WHERE pc.project_id = $1
ORDER BY lower(c.name), c.id;

-- name: UnlinkClientsExcept :exec
DELETE FROM project_clients
WHERE project_id = sqlc.arg('project_id') AND NOT (client_id = ANY (sqlc.arg('client_ids')::bigint[]));

-- name: LinkClients :exec
INSERT INTO project_clients (project_id, client_id)
SELECT sqlc.arg('project_id')::bigint, unnest(sqlc.arg('client_ids')::bigint[])
ON CONFLICT DO NOTHING;
