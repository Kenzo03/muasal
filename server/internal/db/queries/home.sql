-- name: ListMyTickets :many
-- Home's My tickets (FSD §6.4): the open tickets assigned to the user that
-- they may see in any project (R-AC-2, R-AC-3), by due date (none last), then
-- priority and key. view narrows them: overdue, week (due today through 7 days
-- ahead) or incomplete (no reason or no menu). today is the date on the user's
-- calendar, in their profile's timezone. menu is the first menu's parent and
-- name, or '' without one.
SELECT t.id, t.key, t.title, t.type, t.priority, t.due_date, t.client_id, c.name AS client_name, sqlc.embed(s),
       t.reason = '' AS missing_reason,
       NOT EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id) AS missing_menus,
       coalesce((SELECT coalesce(pn.name || ' › ', '') || n.name
                 FROM ticket_nodes tn JOIN nodes n ON n.id = tn.node_id LEFT JOIN nodes pn ON pn.id = n.parent_id
                 WHERE tn.ticket_id = t.id ORDER BY lower(n.name), n.id LIMIT 1), '')::text AS menu
FROM tickets t
JOIN statuses s ON s.id = t.status_id
LEFT JOIN clients c ON c.id = t.client_id
WHERE t.assignee_id = sqlc.arg('user_id')::bigint
  AND s.category IN ('todo', 'in_progress')
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))))
  AND (sqlc.arg('view')::text = 'all'
       OR (sqlc.arg('view')::text = 'overdue' AND t.due_date < sqlc.arg('today')::date)
       OR (sqlc.arg('view')::text = 'week' AND t.due_date BETWEEN sqlc.arg('today')::date AND sqlc.arg('today')::date + 7)
       OR (sqlc.arg('view')::text = 'incomplete' AND (t.reason = '' OR NOT EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id))))
ORDER BY t.due_date NULLS LAST, array_position(ARRAY['urgent', 'high', 'medium', 'low'], t.priority), t.project_id, t.number
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountMyTickets :one
-- The counts of Home's tabs, over all of the user's open tickets, with the
-- same today as ListMyTickets.
SELECT count(*) AS all_open,
       count(*) FILTER (WHERE t.due_date < sqlc.arg('today')::date) AS overdue,
       count(*) FILTER (WHERE t.due_date BETWEEN sqlc.arg('today')::date AND sqlc.arg('today')::date + 7) AS week,
       count(*) FILTER (WHERE t.reason = '' OR NOT EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id)) AS incomplete
FROM tickets t
JOIN statuses s ON s.id = t.status_id
WHERE t.assignee_id = sqlc.arg('user_id')::bigint
  AND s.category IN ('todo', 'in_progress')
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))));

-- name: CountMyTicketsByProject :many
-- The user's open tickets per project, for Home's My projects.
SELECT p.key, count(*) AS open
FROM tickets t
JOIN statuses s ON s.id = t.status_id
JOIN projects p ON p.id = t.project_id
WHERE t.assignee_id = sqlc.arg('user_id')::bigint
  AND s.category IN ('todo', 'in_progress')
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))))
GROUP BY p.key
ORDER BY p.key;

-- name: ListRecentTickets :many
-- Home's Recently updated (FSD §6.4): the tickets the user may see that changed
-- last. Comments, files and decision records touch updated_at (00006).
SELECT t.id, t.key, t.title, t.type, t.updated_at, sqlc.embed(s)
FROM tickets t
JOIN statuses s ON s.id = t.status_id
WHERE (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))))
ORDER BY t.updated_at DESC, t.id DESC
LIMIT 10;

-- name: LatestTicketChanges :many
-- The latest change on each of these tickets: a history event or a comment.
-- Events of one transaction share a time, so the later row wins. actor_id is 0
-- for a change without an actor.
SELECT DISTINCT ON (x.ticket_id) x.ticket_id::bigint AS ticket_id, x.at::timestamptz AS at, x.id::bigint AS id,
       coalesce(x.actor_id, 0)::bigint AS actor_id, u.name AS actor_name, x.action::text AS action, x.changes::jsonb AS changes
FROM (
  SELECT e.entity_id AS ticket_id, e.occurred_at AS at, e.id, e.actor_id, e.action, e.changes
  FROM audit_events e
  WHERE e.entity = 'ticket' AND e.entity_id = ANY (sqlc.arg('ticket_ids')::bigint[])
  UNION ALL
  SELECT c.ticket_id, c.created_at, c.id, c.author_id, 'comment', '{}'::jsonb
  FROM comments c
  WHERE c.ticket_id = ANY (sqlc.arg('ticket_ids')::bigint[]) AND c.deleted_at IS NULL
) x
LEFT JOIN users u ON u.id = x.actor_id
ORDER BY x.ticket_id, x.at DESC, x.id DESC;
