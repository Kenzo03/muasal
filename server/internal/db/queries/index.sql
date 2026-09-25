-- name: GetTicketSource :one
-- Everything a ticket's chunks say about it (FSD §13.1).
SELECT t.id, t.key, t.type, t.title, t.description, t.reason, t.project_id, t.client_id,
       t.requester_contact_id, t.requester_user_id, t.reporter_id, t.assignee_id, t.created_at, t.closed_at,
       s.name AS status_name, c.name AS client_name,
       rc.name AS contact_name, rc.title AS contact_title, rcc.name AS contact_client_name,
       ru.name AS requester_user_name
FROM tickets t
JOIN statuses s ON s.id = t.status_id
LEFT JOIN clients c ON c.id = t.client_id
LEFT JOIN contacts rc ON rc.id = t.requester_contact_id
LEFT JOIN clients rcc ON rcc.id = rc.client_id
LEFT JOIN users ru ON ru.id = t.requester_user_id
WHERE t.id = $1;

-- name: ListTicketNodePaths :many
-- The ticket's menus, each with the names from the top of the tree down.
WITH RECURSIVE up AS (
  SELECT tn.node_id AS id, n.parent_id, ARRAY[n.name]::text[] AS path
  FROM ticket_nodes tn
  JOIN nodes n ON n.id = tn.node_id
  WHERE tn.ticket_id = $1
  UNION ALL
  SELECT up.id, p.parent_id, p.name || up.path
  FROM up
  JOIN nodes p ON p.id = up.parent_id
)
SELECT id, path::text[] AS path FROM up WHERE parent_id IS NULL ORDER BY id;

-- name: ListCommentSources :many
SELECT c.id, u.name AS author, c.internal, c.body, c.created_at
FROM comments c
JOIN users u ON u.id = c.author_id
WHERE c.ticket_id = $1 AND c.deleted_at IS NULL
ORDER BY c.created_at, c.id;

-- name: LockTicketIndex :exec
-- One index job per ticket at a time (§13.2); the lock ends with the transaction.
SELECT pg_advisory_xact_lock(hashtextextended('index_ticket:' || sqlc.arg('ticket_id')::bigint::text, 0));

-- name: DeleteStaleChunks :exec
-- Drops the ticket's chunks that the rebuild no longer produced: a deleted
-- comment, a drafted decision, the tail of a shorter text.
-- keep holds "type:id:seq" of every chunk the rebuild wrote.
DELETE FROM chunks c
WHERE c.ticket_id = sqlc.arg('ticket_id')
  AND c.source_type || ':' || c.source_id || ':' || c.seq <> ALL (sqlc.arg('keep')::text[]);

-- name: ListPendingTicketChunks :many
SELECT id, content, content_hash FROM chunks
WHERE ticket_id = $1 AND (embedding IS NULL OR embed_model IS DISTINCT FROM sqlc.arg('model')::text)
ORDER BY id;

-- name: ListTicketIDsUnderNode :many
-- Tickets on a node or its sub-nodes, whose chunks name the node's path.
WITH RECURSIVE sub AS (
  SELECT n.id FROM nodes n WHERE n.id = $1
  UNION ALL
  SELECT n.id FROM nodes n JOIN sub ON n.parent_id = sub.id
)
SELECT DISTINCT tn.ticket_id FROM ticket_nodes tn JOIN sub ON sub.id = tn.node_id ORDER BY 1;

-- name: ListTicketIDsOfClient :many
SELECT id FROM tickets WHERE client_id = sqlc.arg('client_id')::bigint ORDER BY id;

-- name: ListAllTicketIDs :many
SELECT id FROM tickets ORDER BY id DESC;

-- name: GetNodeIDByPath :one
-- A node by its names from the top of the tree, for the eval's node chips.
WITH RECURSIVE walk AS (
  SELECT n.id, 1 AS depth
  FROM nodes n JOIN projects p ON p.id = n.project_id
  WHERE p.key = sqlc.arg('project_key') AND n.parent_id IS NULL AND n.name = (sqlc.arg('path')::text[])[1]
  UNION ALL
  SELECT n.id, w.depth + 1
  FROM nodes n JOIN walk w ON n.parent_id = w.id
  WHERE n.name = (sqlc.arg('path')::text[])[w.depth + 1]
)
SELECT id FROM walk WHERE depth = cardinality(sqlc.arg('path')::text[]);

-- name: GetClientByName :one
SELECT * FROM clients WHERE lower(name) = lower($1);

-- name: ListProjectTicketIDs :many
SELECT id FROM tickets WHERE project_id = $1 ORDER BY id;
