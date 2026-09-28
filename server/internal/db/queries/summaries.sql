-- name: CreateSummary :one
INSERT INTO summaries (project_id, created_by, title, params, items, markdown, model)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetSummary :one
SELECT sqlc.embed(s), u.name AS creator_name, p.key AS project_key
FROM summaries s
JOIN users u ON u.id = s.created_by
JOIN projects p ON p.id = s.project_id
WHERE s.id = $1;

-- name: UpdateSummary :one
UPDATE summaries SET title = sqlc.arg('title'), markdown = sqlc.arg('markdown'), updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ListSummaries :many
-- A summary is visible to its creator and to project admins (§12.1).
SELECT s.id, s.title, s.created_at, s.updated_at, u.name AS creator_name, p.key AS project_key
FROM summaries s
JOIN users u ON u.id = s.created_by
JOIN projects p ON p.id = s.project_id
WHERE (sqlc.narg('project_id')::bigint IS NULL OR s.project_id = sqlc.narg('project_id')::bigint)
  AND (sqlc.arg('is_admin')::boolean OR s.created_by = sqlc.arg('user_id')::bigint OR EXISTS (
        SELECT 1 FROM memberships m WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = s.project_id AND m.role = 'admin'))
ORDER BY s.created_at DESC
LIMIT 100;

-- name: CreateSummarySchedule :one
INSERT INTO summary_schedules (project_id, client_id, language, audience, weekday, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListSummarySchedules :many
SELECT sqlc.embed(ss), c.name AS client_name, u.name AS creator_name
FROM summary_schedules ss
LEFT JOIN clients c ON c.id = ss.client_id
JOIN users u ON u.id = ss.created_by
WHERE ss.project_id = $1
ORDER BY ss.weekday, lower(coalesce(c.name, '')), ss.id;

-- name: GetSummarySchedule :one
SELECT * FROM summary_schedules WHERE id = $1;

-- name: DeleteSummarySchedule :exec
DELETE FROM summary_schedules WHERE id = $1;

-- name: ListDueSummarySchedules :many
-- The weekly summaries due on this weekday that have not run today.
SELECT * FROM summary_schedules
WHERE weekday = sqlc.arg('weekday')::smallint AND (last_run_on IS NULL OR last_run_on < sqlc.arg('today')::date)
ORDER BY id;

-- name: MarkSummaryScheduleRun :exec
UPDATE summary_schedules SET last_run_on = sqlc.arg('today')::date WHERE id = sqlc.arg('id');

-- name: ListNodesOfTickets :many
SELECT tn.ticket_id, tn.node_id FROM ticket_nodes tn WHERE tn.ticket_id = ANY (sqlc.arg('ticket_ids')::bigint[]) ORDER BY tn.ticket_id, tn.node_id;

-- name: ListNodesOfNotes :many
SELECT dn.note_id, dn.node_id FROM decision_note_nodes dn WHERE dn.note_id = ANY (sqlc.arg('note_ids')::bigint[]) ORDER BY dn.note_id, dn.node_id;

-- name: ListCommentsOfTickets :many
-- Live comments for a summary's model input; client-facing summaries take
-- Client-safe comments only (AC-TK-10).
SELECT c.ticket_id, c.body, c.created_at
FROM comments c
WHERE c.ticket_id = ANY (sqlc.arg('ticket_ids')::bigint[]) AND c.deleted_at IS NULL
  AND (NOT sqlc.arg('client_safe')::boolean OR NOT c.internal)
ORDER BY c.ticket_id, c.created_at, c.id;

-- name: ListDraftComments :many
-- The latest 30 live comments, oldest first, for an AI draft (§9.3).
SELECT body, created_at, author_name FROM (
  SELECT c.body, c.created_at, c.id, coalesce(u.name, c.author_label, '')::text AS author_name
  FROM comments c LEFT JOIN users u ON u.id = c.author_id
  WHERE c.ticket_id = $1 AND c.deleted_at IS NULL
  ORDER BY c.created_at DESC, c.id DESC
  LIMIT 30
) x ORDER BY created_at, id;
