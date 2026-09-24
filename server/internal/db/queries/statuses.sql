-- name: ListStatuses :many
SELECT * FROM statuses WHERE project_id = $1 ORDER BY position, id;

-- name: GetStatus :one
SELECT * FROM statuses WHERE id = $1;

-- name: GetDefaultStatus :one
SELECT * FROM statuses WHERE project_id = $1 AND is_default;

-- name: ClearDefaultStatus :exec
-- Runs before a statuses update, so the new default never meets the old one.
UPDATE statuses SET is_default = false WHERE project_id = $1;

-- name: UpdateStatus :execrows
UPDATE statuses SET name = $3, category = $4, position = $5, color = $6, is_default = $7
WHERE id = $1 AND project_id = $2;

-- name: InsertStatus :one
INSERT INTO statuses (project_id, name, category, position, color, is_default)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: MoveTicketsToStatus :exec
UPDATE tickets SET status_id = sqlc.arg('to_id'), version = version + 1, updated_at = now()
WHERE project_id = sqlc.arg('project_id') AND status_id = sqlc.arg('from_id');

-- name: DeleteStatusesExcept :exec
DELETE FROM statuses
WHERE project_id = sqlc.arg('project_id') AND NOT (id = ANY (sqlc.arg('keep_ids')::bigint[]));
