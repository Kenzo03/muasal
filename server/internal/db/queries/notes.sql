-- Decision notes (FSD §9.4): decisions made outside tickets.

-- name: NextNoteNumber :one
UPDATE projects SET note_seq = note_seq + 1 WHERE id = $1 RETURNING note_seq;

-- name: CreateNote :one
INSERT INTO decision_notes (project_id, number, key, title, decided_on, client_id, attendees, body, created_by)
VALUES (sqlc.arg('project_id'), sqlc.arg('number'), sqlc.arg('key'), sqlc.arg('title'), sqlc.arg('decided_on'),
        sqlc.narg('client_id'), sqlc.arg('attendees'), sqlc.arg('body'), sqlc.arg('created_by'))
RETURNING *;

-- name: UpdateNote :one
UPDATE decision_notes SET title = sqlc.arg('title'), decided_on = sqlc.arg('decided_on'), client_id = sqlc.narg('client_id'),
  attendees = sqlc.arg('attendees'), body = sqlc.arg('body'), updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ArchiveNote :exec
UPDATE decision_notes SET archived_at = CASE WHEN sqlc.arg('archived')::boolean THEN coalesce(archived_at, now()) END, updated_at = now()
WHERE id = sqlc.arg('id');

-- name: GetNoteByKey :one
SELECT sqlc.embed(n), p.key AS project_key, c.name AS client_name, u.name AS author_name
FROM decision_notes n
JOIN projects p ON p.id = n.project_id
JOIN users u ON u.id = n.created_by
LEFT JOIN clients c ON c.id = n.client_id
WHERE n.key = $1;

-- name: GetNoteByID :one
SELECT sqlc.embed(n), p.key AS project_key, c.name AS client_name, u.name AS author_name
FROM decision_notes n
JOIN projects p ON p.id = n.project_id
JOIN users u ON u.id = n.created_by
LEFT JOIN clients c ON c.id = n.client_id
WHERE n.id = $1;

-- name: ListProjectNotes :many
-- A project's notes the scope may see, newest decision first (R-AC-2, R-AC-3).
SELECT n.id, n.key, n.title, n.decided_on, n.client_id, c.name AS client_name, n.archived_at
FROM decision_notes n
LEFT JOIN clients c ON c.id = n.client_id
WHERE n.project_id = sqlc.arg('project_id')
  AND (sqlc.arg('archived')::boolean OR n.archived_at IS NULL)
  AND (sqlc.arg('all_clients')::boolean OR n.client_id IS NULL OR n.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
ORDER BY n.decided_on DESC, n.id DESC
LIMIT 500;

-- name: SetNoteNodes :exec
-- Replaces the note's menus.
WITH gone AS (
  DELETE FROM decision_note_nodes WHERE note_id = sqlc.arg('note_id') AND node_id <> ALL (sqlc.arg('node_ids')::bigint[])
)
INSERT INTO decision_note_nodes (note_id, node_id)
SELECT sqlc.arg('note_id'), unnest(sqlc.arg('node_ids')::bigint[])
ON CONFLICT DO NOTHING;

-- name: SetNoteTickets :exec
WITH gone AS (
  DELETE FROM decision_note_tickets WHERE note_id = sqlc.arg('note_id') AND ticket_id <> ALL (sqlc.arg('ticket_ids')::bigint[])
)
INSERT INTO decision_note_tickets (note_id, ticket_id)
SELECT sqlc.arg('note_id'), unnest(sqlc.arg('ticket_ids')::bigint[])
ON CONFLICT DO NOTHING;

-- name: ListNoteNodes :many
SELECT n.id, n.name, (n.archived_at IS NOT NULL)::boolean AS archived
FROM decision_note_nodes dn JOIN nodes n ON n.id = dn.node_id
WHERE dn.note_id = $1 ORDER BY lower(n.name), n.id;

-- name: ListNoteTickets :many
SELECT t.id, t.key, t.title, t.project_id, t.client_id
FROM decision_note_tickets dt JOIN tickets t ON t.id = dt.ticket_id
WHERE dt.note_id = $1 ORDER BY t.id;

-- name: ListNoteNodePaths :many
-- The note's menus, each with the names from the top of the tree down.
WITH RECURSIVE up AS (
  SELECT dn.node_id AS id, n.parent_id, ARRAY[n.name]::text[] AS path
  FROM decision_note_nodes dn
  JOIN nodes n ON n.id = dn.node_id
  WHERE dn.note_id = $1
  UNION ALL
  SELECT up.id, p.parent_id, p.name || up.path
  FROM up
  JOIN nodes p ON p.id = up.parent_id
)
SELECT id, path::text[] AS path FROM up WHERE parent_id IS NULL ORDER BY id;

-- name: ListNodeNotes :many
-- The visible notes on these nodes for a node page, newest decision first
-- (§9.4, AC-DC-8); from_date and to_date filter on the decision date.
SELECT n.id, n.key, n.title, n.decided_on, n.client_id, c.name AS client_name, n.attendees, n.body, u.name AS author_name
FROM decision_notes n
JOIN users u ON u.id = n.created_by
LEFT JOIN clients c ON c.id = n.client_id
WHERE n.project_id = sqlc.arg('project_id') AND n.archived_at IS NULL
  AND EXISTS (SELECT 1 FROM decision_note_nodes dn WHERE dn.note_id = n.id AND dn.node_id = ANY (sqlc.arg('node_ids')::bigint[]))
  AND (sqlc.arg('all_clients')::boolean OR n.client_id IS NULL OR n.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
  AND (sqlc.narg('client_id')::bigint IS NULL OR n.client_id = sqlc.narg('client_id')::bigint)
  AND (NOT sqlc.arg('core_only')::boolean OR n.client_id IS NULL)
  AND (sqlc.narg('from_date')::date IS NULL OR n.decided_on >= sqlc.narg('from_date')::date)
  AND (sqlc.narg('to_date')::date IS NULL OR n.decided_on <= sqlc.narg('to_date')::date)
ORDER BY n.decided_on DESC, n.id DESC
LIMIT 500;

-- name: LockNoteIndex :exec
SELECT pg_advisory_xact_lock(hashtextextended('index_note:' || sqlc.arg('note_id')::bigint::text, 0));

-- name: DeleteStaleNoteChunks :exec
DELETE FROM chunks c
WHERE c.note_id = sqlc.arg('note_id')::bigint
  AND c.source_type || ':' || c.source_id || ':' || c.seq <> ALL (sqlc.arg('keep')::text[]);

-- name: ListPendingNoteChunks :many
SELECT id, content, content_hash FROM chunks
WHERE note_id = sqlc.arg('note_id')::bigint AND (embedding IS NULL OR embed_model IS DISTINCT FROM sqlc.arg('model')::text)
ORDER BY id;

-- name: ListNoteIDsUnderNode :many
-- Notes on a node or its sub-nodes, whose chunks name the node's path.
WITH RECURSIVE sub AS (
  SELECT n.id FROM nodes n WHERE n.id = $1
  UNION ALL
  SELECT n.id FROM nodes n JOIN sub ON n.parent_id = sub.id
)
SELECT DISTINCT dn.note_id FROM decision_note_nodes dn JOIN sub ON sub.id = dn.node_id ORDER BY 1;

-- name: ListAllNoteIDs :many
SELECT id FROM decision_notes ORDER BY id DESC;

-- name: SearchNotes :many
-- Notes the user may see (R-AC-2, R-AC-3): a key, words in the title or body,
-- or part of the title (FSD §6.1). An exact key comes first.
SELECT n.key, n.title, p.key AS project_key, n.decided_on
FROM decision_notes n
JOIN projects p ON p.id = n.project_id
WHERE n.archived_at IS NULL
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = n.project_id
          AND (m.all_clients OR n.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = n.client_id))))
  AND (n.key = upper(sqlc.arg('q')::text)
       OR n.title ILIKE '%' || sqlc.arg('q')::text || '%'
       OR to_tsvector('simple', n.title || ' ' || n.attendees || ' ' || n.body) @@ websearch_to_tsquery('simple', sqlc.arg('q')::text))
ORDER BY n.key = upper(sqlc.arg('q')::text) DESC, n.decided_on DESC
LIMIT 20;

-- name: IsProjectClient :one
SELECT EXISTS (SELECT 1 FROM project_clients WHERE project_id = $1 AND client_id = $2)::boolean;
