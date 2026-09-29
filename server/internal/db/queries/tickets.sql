-- name: NextTicketNumber :one
-- The row lock makes ticket numbers gapless and unique per project (FSD §8.1).
UPDATE projects SET ticket_seq = ticket_seq + 1 WHERE id = $1 RETURNING ticket_seq;

-- name: CreateTicket :one
INSERT INTO tickets (project_id, number, key, type, title, description, reason, status_id, client_id,
                     requester_contact_id, requester_user_id, reporter_id, assignee_id, priority, due_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: GetTicketByKey :one
SELECT sqlc.embed(t), sqlc.embed(s), p.key AS project_key, rp.name AS reporter_name, c.name AS client_name,
       rc.name AS requester_contact_name, rc.title AS requester_contact_title,
       ru.name AS requester_user_name, a.name AS assignee_name
FROM tickets t
JOIN statuses s ON s.id = t.status_id
JOIN projects p ON p.id = t.project_id
JOIN users rp ON rp.id = t.reporter_id
LEFT JOIN clients c ON c.id = t.client_id
LEFT JOIN contacts rc ON rc.id = t.requester_contact_id
LEFT JOIN users ru ON ru.id = t.requester_user_id
LEFT JOIN users a ON a.id = t.assignee_id
WHERE t.key = $1;

-- name: UpdateTicket :one
-- Optimistic locking: no row comes back when the version moved on (FSD §8.6).
UPDATE tickets SET type = $3, title = $4, description = $5, reason = $6, client_id = $7,
  requester_contact_id = $8, requester_user_id = $9, assignee_id = $10, priority = $11, due_date = $12,
  version = version + 1, updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: SetTicketStatus :one
-- A close stamps closed_at and any other move clears it (FSD §8.1). With a
-- version, a stale one matches no row (If-Match).
UPDATE tickets SET status_id = sqlc.arg('status_id'),
  closed_at  = CASE WHEN sqlc.arg('closed')::boolean THEN now() END,
  version    = version + 1,
  updated_at = now()
WHERE id = sqlc.arg('id') AND (sqlc.narg('version')::int IS NULL OR version = sqlc.narg('version')::int)
RETURNING *;

-- name: SetTicketReason :exec
UPDATE tickets SET reason = $2 WHERE id = $1;

-- name: ListTicketNodes :many
SELECT n.id, n.name, (n.archived_at IS NOT NULL)::boolean AS archived
FROM ticket_nodes tn
JOIN nodes n ON n.id = tn.node_id
WHERE tn.ticket_id = $1
ORDER BY lower(n.name), n.id;

-- name: ClearTicketNodes :exec
DELETE FROM ticket_nodes WHERE ticket_id = $1;

-- name: AddTicketNodes :exec
INSERT INTO ticket_nodes (ticket_id, node_id)
SELECT sqlc.arg('ticket_id')::bigint, unnest(sqlc.arg('node_ids')::bigint[]);

-- name: ListTickets :many
-- One project's tickets that the scope may see (R-AC-2, R-AC-3), for the list
-- and the board. A filter is off when its argument is NULL or false.
SELECT t.id, t.key, t.title, t.type, t.priority, t.due_date, t.status_id, t.client_id, c.name AS client_name,
       t.assignee_id, a.name AS assignee_name, coalesce(rc.name, ru.name, '')::text AS requester_name,
       t.reason = '' AS missing_reason, t.updated_at,
       ARRAY(SELECT n.name FROM ticket_nodes tn JOIN nodes n ON n.id = tn.node_id
             WHERE tn.ticket_id = t.id ORDER BY lower(n.name), n.id)::text[] AS node_names
FROM tickets t
JOIN statuses s ON s.id = t.status_id
LEFT JOIN clients c ON c.id = t.client_id
LEFT JOIN users a ON a.id = t.assignee_id
LEFT JOIN contacts rc ON rc.id = t.requester_contact_id
LEFT JOIN users ru ON ru.id = t.requester_user_id
WHERE t.project_id = sqlc.arg('project_id')
  AND (sqlc.arg('all_clients')::boolean OR t.client_id IS NULL OR t.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
  AND (sqlc.narg('status_id')::bigint IS NULL OR t.status_id = sqlc.narg('status_id')::bigint)
  AND (sqlc.narg('category')::text IS NULL OR s.category = sqlc.narg('category')::text)
  AND (NOT sqlc.arg('open_only')::boolean OR s.category IN ('todo', 'in_progress'))
  AND (sqlc.narg('closed_days')::int IS NULL OR t.closed_at IS NULL OR t.closed_at >= now() - make_interval(days => sqlc.narg('closed_days')::int))
  AND (sqlc.narg('type')::text IS NULL OR t.type = sqlc.narg('type')::text)
  AND (sqlc.narg('client_id')::bigint IS NULL OR t.client_id = sqlc.narg('client_id')::bigint)
  AND (NOT sqlc.arg('core_only')::boolean OR t.client_id IS NULL)
  AND (sqlc.narg('assignee_id')::bigint IS NULL OR t.assignee_id = sqlc.narg('assignee_id')::bigint)
  AND (NOT sqlc.arg('unassigned')::boolean OR t.assignee_id IS NULL)
  -- due and stale_days count open tickets only, as Home and the workload page do;
  -- due counts from today on the caller's calendar.
  AND (sqlc.narg('due')::text IS NULL OR (s.category IN ('todo', 'in_progress') AND (
        (sqlc.narg('due')::text = 'overdue' AND t.due_date < sqlc.arg('today')::date)
        OR (sqlc.narg('due')::text = 'week' AND t.due_date BETWEEN sqlc.arg('today')::date AND sqlc.arg('today')::date + 7))))
  AND (sqlc.narg('stale_days')::int IS NULL OR (s.category IN ('todo', 'in_progress')
        AND t.updated_at < now() - make_interval(days => sqlc.narg('stale_days')::int)))
  AND (sqlc.narg('node_ids')::bigint[] IS NULL OR EXISTS (
        SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id AND tn.node_id = ANY (sqlc.narg('node_ids')::bigint[])))
  AND (NOT sqlc.arg('missing_reason')::boolean OR t.reason = '')
  AND (NOT sqlc.arg('missing_menus')::boolean OR NOT EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id))
  AND (sqlc.arg('q')::text = '' OR t.title ILIKE '%' || sqlc.arg('q')::text || '%' OR t.key = upper(sqlc.arg('q')::text))
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'priority' THEN array_position(ARRAY['urgent', 'high', 'medium', 'low'], t.priority) END,
  CASE WHEN sqlc.arg('sort')::text IN ('priority', 'due') THEN t.due_date END NULLS LAST,
  CASE WHEN sqlc.arg('sort')::text = 'updated' THEN t.updated_at END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'created' THEN t.number END DESC,
  t.number
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');
