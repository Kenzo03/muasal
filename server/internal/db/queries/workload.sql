-- name: ProjectWorkload :many
-- Open tickets per assignee (NULL: nobody) that the scope may see (R-AC-2,
-- R-AC-3), counted as Home counts them, from today on the caller's calendar.
-- Comments, files and decision records touch updated_at (00006), so stale
-- means no change of any kind.
SELECT t.assignee_id, a.name AS assignee_name,
       count(*) AS open,
       count(*) FILTER (WHERE s.category = 'in_progress') AS in_progress,
       count(*) FILTER (WHERE t.due_date < sqlc.arg('today')::date) AS overdue,
       count(*) FILTER (WHERE t.due_date BETWEEN sqlc.arg('today')::date AND sqlc.arg('today')::date + 7) AS due_week,
       count(*) FILTER (WHERE t.updated_at < now() - make_interval(days => sqlc.arg('stale_days')::int)) AS stale,
       count(*) FILTER (WHERE t.priority IN ('urgent', 'high')) AS high,
       coalesce(sum(t.estimate_hours), 0)::double precision AS open_hours,
       count(t.estimate_hours) AS estimated
FROM tickets t
JOIN statuses s ON s.id = t.status_id
LEFT JOIN users a ON a.id = t.assignee_id
WHERE t.project_id = sqlc.arg('project_id')
  AND s.category IN ('todo', 'in_progress')
  AND (sqlc.arg('all_clients')::boolean OR t.client_id IS NULL OR t.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
GROUP BY t.assignee_id, a.name;
