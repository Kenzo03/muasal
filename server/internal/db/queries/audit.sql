-- name: InsertAuditEvent :exec
INSERT INTO audit_events (actor_id, via, entity, entity_id, action, changes, request_id, ip)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListAuditEvents :many
SELECT * FROM audit_events WHERE entity = $1 AND entity_id = $2 ORDER BY id;
