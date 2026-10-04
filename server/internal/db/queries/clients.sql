-- name: CreateClient :one
INSERT INTO clients (name, code, aliases)
VALUES (sqlc.arg('name'), sqlc.narg('code'), coalesce(sqlc.narg('aliases')::text[], '{}'))
RETURNING *;

-- name: GetClient :one
SELECT * FROM clients WHERE id = $1;

-- name: ListClients :many
-- The admin list (GET /clients): each client with the keys of the projects
-- linked to it, in key order; none gives an empty array. Unless the caller is
-- a system admin, only projects the caller belongs to are listed.
SELECT sqlc.embed(c),
       coalesce(array_agg(p.key ORDER BY p.key) FILTER (WHERE p.key IS NOT NULL), '{}')::text[] AS projects
FROM clients c
LEFT JOIN project_clients pc ON pc.client_id = c.id
LEFT JOIN projects p ON p.id = pc.project_id
  AND (sqlc.arg('is_admin')::bool
       OR EXISTS (SELECT 1 FROM memberships m WHERE m.project_id = p.id AND m.user_id = sqlc.arg('user_id')))
GROUP BY c.id
ORDER BY lower(c.name), c.id;

-- name: UpdateClient :one
-- NULL keeps a field; an empty code clears it.
UPDATE clients SET
  name        = coalesce(sqlc.narg('name'), name),
  code        = CASE WHEN sqlc.narg('code')::text IS NULL THEN code ELSE nullif(sqlc.narg('code')::text, '') END,
  aliases     = coalesce(sqlc.narg('aliases')::text[], aliases),
  archived_at = CASE WHEN sqlc.narg('archived')::boolean IS NULL THEN archived_at
                     WHEN sqlc.narg('archived')::boolean THEN coalesce(archived_at, now())
                     ELSE NULL END
WHERE id = sqlc.arg('id')
RETURNING *;
