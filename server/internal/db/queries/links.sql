-- Ticket links (FSD §8.8): stored once, shown on both tickets.

-- name: CreateLink :one
INSERT INTO ticket_links (from_id, to_id, type, created_by)
VALUES (sqlc.arg('from_id'), sqlc.arg('to_id'), sqlc.arg('type'), sqlc.arg('created_by'))
ON CONFLICT (from_id, to_id, type) DO NOTHING
RETURNING *;

-- name: GetLink :one
SELECT * FROM ticket_links WHERE id = $1;

-- name: DeleteLink :exec
DELETE FROM ticket_links WHERE id = $1;

-- name: ListTicketLinks :many
-- Both directions, each with the other ticket; the caller keeps the ones the
-- reader may see (R-TK-6).
SELECT l.id, l.type, (l.from_id = sqlc.arg('ticket_id'))::boolean AS outgoing,
       o.id AS other_id, o.key AS other_key, o.title AS other_title, o.project_id AS other_project_id,
       o.client_id AS other_client_id, o.closed_at AS other_closed_at, sqlc.embed(s), l.created_at
FROM ticket_links l
JOIN tickets o ON o.id = CASE WHEN l.from_id = sqlc.arg('ticket_id') THEN l.to_id ELSE l.from_id END
JOIN statuses s ON s.id = o.status_id
WHERE l.from_id = sqlc.arg('ticket_id') OR l.to_id = sqlc.arg('ticket_id')
ORDER BY l.created_at, l.id;

-- name: GetTicketByID :one
SELECT * FROM tickets WHERE id = $1;
