-- name: CreateComment :one
INSERT INTO comments (ticket_id, author_id, internal, body) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetComment :one
SELECT sqlc.embed(c), t.key AS ticket_key
FROM comments c JOIN tickets t ON t.id = c.ticket_id
WHERE c.id = $1;

-- name: UpdateCommentBody :one
UPDATE comments SET body = $2, edited_at = now() WHERE id = $1 AND deleted_at IS NULL RETURNING *;

-- name: DeleteComment :exec
UPDATE comments SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;

-- name: ListComments :many
SELECT sqlc.embed(c), u.name AS author_name
FROM comments c JOIN users u ON u.id = c.author_id
WHERE c.ticket_id = $1
ORDER BY c.created_at, c.id;
