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
