-- name: InsertAuditEvent :exec
INSERT INTO audit_events (actor_id, via, entity, entity_id, project_id, action, changes, request_id, ip)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListAuditEvents :many
SELECT * FROM audit_events WHERE entity = $1 AND entity_id = $2 ORDER BY id;

-- name: ListTicketEvents :many
-- A ticket's history: every event recorded against it, oldest first (FSD §8.7).
SELECT e.id, e.occurred_at, e.actor_id, u.name AS actor_name, e.action, e.changes
FROM audit_events e
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.entity = 'ticket' AND e.entity_id = $1
ORDER BY e.id;

-- name: ListAudit :many
-- The audit log for system admins, newest first, filtered (FSD §15.4). Dates
-- are whole days in the admin's timezone, passed as bounds.
SELECT e.id, e.occurred_at, e.actor_id, u.name AS actor_name, e.via, e.entity, e.entity_id, p.key AS project_key, e.action, e.changes
FROM audit_events e
LEFT JOIN users u ON u.id = e.actor_id
LEFT JOIN projects p ON p.id = e.project_id
WHERE (sqlc.narg('actor_id')::bigint IS NULL OR e.actor_id = sqlc.narg('actor_id')::bigint)
  AND (sqlc.narg('entity')::text IS NULL OR e.entity = sqlc.narg('entity')::text)
  AND (sqlc.narg('action')::text IS NULL OR e.action = sqlc.narg('action')::text)
  AND (sqlc.narg('since')::timestamptz IS NULL OR e.occurred_at >= sqlc.narg('since')::timestamptz)
  AND (sqlc.narg('until')::timestamptz IS NULL OR e.occurred_at < sqlc.narg('until')::timestamptz)
  AND (sqlc.narg('before')::bigint IS NULL OR e.id < sqlc.narg('before')::bigint)
ORDER BY e.id DESC
LIMIT sqlc.arg('lim');
