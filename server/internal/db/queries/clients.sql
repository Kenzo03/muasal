-- name: CreateClient :one
INSERT INTO clients (name, code, aliases)
VALUES (sqlc.arg('name'), sqlc.narg('code'), coalesce(sqlc.narg('aliases')::text[], '{}'))
RETURNING *;

-- name: GetClient :one
SELECT * FROM clients WHERE id = $1;

-- name: ListClients :many
SELECT * FROM clients ORDER BY lower(name), id;

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
