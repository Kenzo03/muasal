-- Ticket imports (FSD §14.2).

-- name: CreateImportRun :one
INSERT INTO import_runs (kind, project_id, file_name, file_path, mapping, status, created_by)
VALUES ('tickets', sqlc.arg('project_id'), sqlc.arg('file_name'), sqlc.arg('file_path'), sqlc.arg('mapping'), 'uploaded', sqlc.arg('created_by'))
RETURNING *;

-- name: GetImportRun :one
SELECT sqlc.embed(r), p.key AS project_key, u.name AS created_by_name
FROM import_runs r JOIN projects p ON p.id = r.project_id JOIN users u ON u.id = r.created_by
WHERE r.id = $1;

-- name: ListImportRuns :many
SELECT sqlc.embed(r), p.key AS project_key, u.name AS created_by_name
FROM import_runs r JOIN projects p ON p.id = r.project_id JOIN users u ON u.id = r.created_by
ORDER BY r.id DESC LIMIT 100;

-- name: SaveImportPlan :exec
UPDATE import_runs SET mapping = sqlc.arg('mapping'), status = sqlc.arg('status'), stats = sqlc.arg('stats'), errors = sqlc.arg('errors')
WHERE id = sqlc.arg('id');

-- name: SetImportStatus :exec
UPDATE import_runs SET status = sqlc.arg('status'), stats = sqlc.arg('stats'),
  finished_at = CASE WHEN sqlc.arg('status')::text IN ('done', 'failed') THEN now() END
WHERE id = sqlc.arg('id');

-- name: ListExternalRefs :many
SELECT upper(external_ref)::text FROM tickets WHERE project_id = $1 AND external_ref IS NOT NULL;

-- name: GetTicketByExternalRef :one
SELECT * FROM tickets WHERE project_id = sqlc.arg('project_id') AND upper(external_ref) = upper(sqlc.arg('ref'));

-- name: CreateImportedTicket :one
-- An imported ticket keeps its old key and dates; source = import (R-IN-2).
INSERT INTO tickets (project_id, number, key, type, title, description, reason, status_id, client_id,
                     requester_contact_id, requester_user_id, reporter_id, assignee_id, priority,
                     source, external_ref, external_meta, created_at, updated_at, closed_at)
VALUES (sqlc.arg('project_id'), sqlc.arg('number'), sqlc.arg('key'), sqlc.arg('type'), sqlc.arg('title'), sqlc.arg('description'),
        sqlc.arg('reason'), sqlc.arg('status_id'), sqlc.narg('client_id'), sqlc.narg('requester_contact_id'), sqlc.narg('requester_user_id'),
        sqlc.arg('reporter_id'), sqlc.narg('assignee_id'), sqlc.arg('priority'), 'import', sqlc.arg('external_ref'), sqlc.arg('external_meta'),
        sqlc.arg('created_at'), sqlc.arg('created_at'), sqlc.narg('closed_at'))
RETURNING *;

-- name: UpdateImportedTicket :one
-- A re-import updates what the old tool owns and leaves Muasal's own work
-- (reason, menus added by hand, decision records) alone.
UPDATE tickets SET type = sqlc.arg('type'), title = sqlc.arg('title'), description = sqlc.arg('description'),
  status_id = sqlc.arg('status_id'), priority = sqlc.arg('priority'), assignee_id = sqlc.narg('assignee_id'),
  external_meta = sqlc.arg('external_meta'), closed_at = sqlc.narg('closed_at'), version = version + 1, updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: InsertImportedComment :execrows
-- Comments match on a hash of the ticket's old key, date, author and body (R-IN-1).
INSERT INTO comments (ticket_id, author_id, author_label, internal, body, created_at, external_hash)
VALUES (sqlc.arg('ticket_id'), sqlc.narg('author_id'), sqlc.narg('author_label'), true, sqlc.arg('body'), sqlc.arg('created_at'), sqlc.arg('external_hash'))
ON CONFLICT (ticket_id, external_hash) WHERE external_hash IS NOT NULL DO NOTHING;

-- name: FindUserByNameOrEmail :one
SELECT id FROM users WHERE lower(name) = lower(sqlc.arg('who')) OR lower(email) = lower(sqlc.arg('who'))
ORDER BY (lower(email) = lower(sqlc.arg('who'))) DESC, id LIMIT 1;

-- name: FindInternalContact :one
SELECT id FROM contacts WHERE client_id IS NULL AND lower(name) = lower($1) ORDER BY id LIMIT 1;

-- name: CountImportedComments :one
SELECT count(*) FROM comments c JOIN tickets t ON t.id = c.ticket_id WHERE t.project_id = $1 AND c.external_hash IS NOT NULL;

-- name: LinkImportedNodes :exec
-- A re-import adds menus and keeps the ones already linked.
INSERT INTO ticket_nodes (ticket_id, node_id)
SELECT sqlc.arg('ticket_id')::bigint, unnest(sqlc.arg('node_ids')::bigint[])
ON CONFLICT DO NOTHING;
