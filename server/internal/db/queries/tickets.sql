-- name: NextTicketNumber :one
-- The row lock makes ticket numbers gapless and unique per project (FSD §8.1).
UPDATE projects SET ticket_seq = ticket_seq + 1 WHERE id = $1 RETURNING ticket_seq;

-- name: CreateTicket :one
INSERT INTO tickets (project_id, number, key, type, title, description, reason, status_id, client_id,
                     requester_contact_id, requester_user_id, reporter_id, assignee_id, priority, due_date, estimate_hours, labels)
VALUES (sqlc.arg('project_id'), sqlc.arg('number'), sqlc.arg('key'), sqlc.arg('type'), sqlc.arg('title'), sqlc.arg('description'),
        sqlc.arg('reason'), sqlc.arg('status_id'), sqlc.narg('client_id'), sqlc.narg('requester_contact_id'), sqlc.narg('requester_user_id'),
        sqlc.arg('reporter_id'), sqlc.narg('assignee_id'), sqlc.arg('priority'), sqlc.narg('due_date'), sqlc.narg('estimate_hours'),
        coalesce(sqlc.narg('labels')::text[], '{}'))
RETURNING *;

-- name: GetTicketByKey :one
SELECT sqlc.embed(t), sqlc.embed(s), p.key AS project_key, rp.name AS reporter_name, c.name AS client_name,
       rc.name AS requester_contact_name, rc.title AS requester_contact_title,
       ru.name AS requester_user_name, a.name AS assignee_name, ac.name AS accepted_contact_name
FROM tickets t
JOIN statuses s ON s.id = t.status_id
JOIN projects p ON p.id = t.project_id
JOIN users rp ON rp.id = t.reporter_id
LEFT JOIN clients c ON c.id = t.client_id
LEFT JOIN contacts rc ON rc.id = t.requester_contact_id
LEFT JOIN users ru ON ru.id = t.requester_user_id
LEFT JOIN users a ON a.id = t.assignee_id
LEFT JOIN contacts ac ON ac.id = t.accepted_contact_id
WHERE t.key = $1;

-- name: SetTicketAcceptance :one
-- MSL-66: records or clears the client's acceptance; like any edit it moves
-- the version on (FSD §8.6).
UPDATE tickets SET accepted_contact_id = sqlc.narg('contact_id'), accepted_on = sqlc.narg('accepted_on'),
  acceptance_note = sqlc.arg('note'), version = version + 1, updated_at = now()
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: UpdateTicket :one
-- Optimistic locking: no row comes back when the version moved on (FSD §8.6).
UPDATE tickets SET type = sqlc.arg('type'), title = sqlc.arg('title'), description = sqlc.arg('description'),
  reason = sqlc.arg('reason'), client_id = sqlc.narg('client_id'), requester_contact_id = sqlc.narg('requester_contact_id'),
  requester_user_id = sqlc.narg('requester_user_id'), assignee_id = sqlc.narg('assignee_id'), priority = sqlc.arg('priority'),
  due_date = sqlc.narg('due_date'), estimate_hours = sqlc.narg('estimate_hours'), labels = coalesce(sqlc.narg('labels')::text[], '{}'),
  version = version + 1, updated_at = now()
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
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
             WHERE tn.ticket_id = t.id ORDER BY lower(n.name), n.id)::text[] AS node_names,
       -- MSL-55: the description's task list, "- [ ] step" and "- [x] step".
       (SELECT count(*) FROM regexp_matches(t.description, '^[ \t]*(?:[-*+]|[0-9]+[.)])[ \t]+\[[ xX]\]', 'gn'))::int AS checklist_total,
       (SELECT count(*) FROM regexp_matches(t.description, '^[ \t]*(?:[-*+]|[0-9]+[.)])[ \t]+\[[xX]\]', 'gn'))::int AS checklist_done,
       t.labels, t.accepted_on
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
  AND (sqlc.narg('label')::text IS NULL OR sqlc.narg('label')::text = ANY (t.labels))
  AND (sqlc.narg('accepted')::boolean IS NULL OR (t.accepted_on IS NOT NULL) = sqlc.narg('accepted')::boolean)
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
  -- Tickets filed through the API or an import never show the form's
  -- weak-reason hint, so the list finds them (MSL-12, 00017).
  AND (NOT sqlc.arg('weak_reason')::boolean OR weak_reason(t.reason))
  AND (NOT sqlc.arg('missing_menus')::boolean OR NOT EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id))
  AND (sqlc.arg('q')::text = '' OR t.title ILIKE '%' || sqlc.arg('q')::text || '%' OR t.key = upper(sqlc.arg('q')::text))
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'priority' THEN array_position(ARRAY['urgent', 'high', 'medium', 'low'], t.priority) END,
  CASE WHEN sqlc.arg('sort')::text IN ('priority', 'due') THEN t.due_date END NULLS LAST,
  CASE WHEN sqlc.arg('sort')::text = 'updated' THEN t.updated_at END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'created' THEN t.number END DESC,
  t.number
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: ListProjectLabels :many
-- MSL-56: the labels a project's tickets use, most used first, for filters and the form.
SELECT l::text AS label, count(*) AS uses
FROM tickets t, unnest(t.labels) AS l
WHERE t.project_id = sqlc.arg('project_id')
GROUP BY l
ORDER BY count(*) DESC, l
LIMIT 200;
