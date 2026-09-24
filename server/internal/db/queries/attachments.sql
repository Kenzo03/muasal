-- name: CreateAttachment :one
INSERT INTO attachments (ticket_id, uploader_id, filename, content_type, size_bytes, sha256)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAttachment :one
SELECT sqlc.embed(a), t.key AS ticket_key
FROM attachments a JOIN tickets t ON t.id = a.ticket_id
WHERE a.id = $1 AND a.deleted_at IS NULL;

-- name: ListAttachments :many
SELECT sqlc.embed(a), u.name AS uploader_name
FROM attachments a JOIN users u ON u.id = a.uploader_id
WHERE a.ticket_id = $1 AND a.deleted_at IS NULL
ORDER BY a.created_at, a.id;

-- name: DeleteAttachment :exec
UPDATE attachments SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;
