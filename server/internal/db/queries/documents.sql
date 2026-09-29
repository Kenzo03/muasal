-- name: NextDocNumber :one
UPDATE projects SET doc_seq = doc_seq + 1 WHERE id = $1 RETURNING doc_seq;

-- name: CreateDocument :one
INSERT INTO documents (project_id, number, key, title, client_id, filename, content_type, size_bytes, sha256, markdown, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: SupersedeDocument :exec
UPDATE documents SET superseded_by = sqlc.arg('by') WHERE id = sqlc.arg('id') AND superseded_by IS NULL;

-- name: SetDocumentSupersededBy :exec
-- Marks a document replaced after its upload, or current again with NULL (§7.7).
UPDATE documents SET superseded_by = sqlc.narg('by') WHERE id = sqlc.arg('id');

-- name: CreateSection :one
INSERT INTO document_sections (document_id, number, title, level, position, body)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: GetDocumentByKey :one
SELECT sqlc.embed(d), c.name AS client_name, u.name AS uploader_name, sk.key AS superseded_by_key
FROM documents d
JOIN users u ON u.id = d.uploaded_by
LEFT JOIN clients c ON c.id = d.client_id
LEFT JOIN documents sk ON sk.id = d.superseded_by
WHERE d.key = $1;

-- name: GetDocumentByID :one
SELECT sqlc.embed(d), c.name AS client_name, u.name AS uploader_name, sk.key AS superseded_by_key
FROM documents d
JOIN users u ON u.id = d.uploaded_by
LEFT JOIN clients c ON c.id = d.client_id
LEFT JOIN documents sk ON sk.id = d.superseded_by
WHERE d.id = $1;

-- name: ListProjectDocuments :many
-- The project's live documents the scope may see (R-MR-15), newest first.
SELECT d.id, d.key, d.title, d.client_id, c.name AS client_name, d.filename, d.created_at, u.name AS uploader_name, sk.key AS superseded_by_key
FROM documents d
JOIN users u ON u.id = d.uploaded_by
LEFT JOIN clients c ON c.id = d.client_id
LEFT JOIN documents sk ON sk.id = d.superseded_by
WHERE d.project_id = sqlc.arg('project_id') AND d.archived_at IS NULL
  AND (sqlc.arg('all_clients')::boolean OR d.client_id IS NULL OR d.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
ORDER BY d.number DESC;

-- name: ListSections :many
SELECT * FROM document_sections WHERE document_id = $1 ORDER BY position;

-- name: GetSection :one
SELECT s.*, d.key AS document_key, d.title AS document_title, d.project_id, d.client_id, c.name AS client_name,
       d.created_at AS document_created_at, d.archived_at AS document_archived_at, sk.key AS superseded_by_key
FROM document_sections s
JOIN documents d ON d.id = s.document_id
LEFT JOIN clients c ON c.id = d.client_id
LEFT JOIN documents sk ON sk.id = d.superseded_by
WHERE s.id = $1;

-- name: ListSectionNodePaths :many
-- The nodes a section produced, each with its path from the top.
WITH RECURSIVE up AS (
  SELECT sn.node_id AS id, n.parent_id, ARRAY[n.name]::text[] AS path, sn.node_id AS origin
  FROM document_section_nodes sn JOIN nodes n ON n.id = sn.node_id
  WHERE sn.section_id = $1
  UNION ALL
  SELECT up.id, p.parent_id, p.name || up.path, up.origin
  FROM up JOIN nodes p ON p.id = up.parent_id
)
SELECT origin AS id, path::text[] AS path FROM up WHERE parent_id IS NULL ORDER BY origin;

-- name: ListSectionNodes :many
SELECT sn.section_id, sn.node_id FROM document_section_nodes sn
JOIN document_sections s ON s.id = sn.section_id
WHERE s.document_id = $1 ORDER BY sn.section_id, sn.node_id;

-- name: LinkSectionNode :exec
INSERT INTO document_section_nodes (section_id, node_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ListDocumentSectionIDs :many
SELECT s.id FROM document_sections s WHERE s.document_id = $1 ORDER BY s.position;

-- name: ListAllSectionIDs :many
SELECT s.id FROM document_sections s JOIN documents d ON d.id = s.document_id WHERE d.archived_at IS NULL ORDER BY s.id;

-- name: CreateTreeDraft :one
INSERT INTO tree_drafts (project_id, document_id, status, proposal, used_ai, total_parts, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetTreeDraft :one
SELECT sqlc.embed(t), d.key AS document_key, d.title AS document_title
FROM tree_drafts t JOIN documents d ON d.id = t.document_id
WHERE t.id = $1;

-- name: ListDocumentDrafts :many
SELECT id, status, used_ai, created_at FROM tree_drafts WHERE document_id = $1 ORDER BY id DESC;

-- name: SetDraftProgress :exec
UPDATE tree_drafts SET done_parts = $2 WHERE id = $1;

-- name: FinishTreeDraft :exec
UPDATE tree_drafts SET status = sqlc.arg('status'), proposal = sqlc.arg('proposal'), error = sqlc.arg('error')
WHERE id = sqlc.arg('id') AND status = 'running';

-- name: SaveTreeDraft :exec
UPDATE tree_drafts SET proposal = $2 WHERE id = $1 AND status = 'ready';

-- name: SetTreeDraftStatus :one
-- Applies or discards a ready draft once.
UPDATE tree_drafts SET status = sqlc.arg('status'), applied_at = CASE WHEN sqlc.arg('status') = 'applied' THEN now() END
WHERE id = sqlc.arg('id') AND status = 'ready'
RETURNING id;

-- name: LockSectionIndex :exec
SELECT pg_advisory_xact_lock(hashtextextended('index_section:' || sqlc.arg('section_id')::bigint::text, 0));

-- name: DeleteSectionChunks :exec
DELETE FROM chunks WHERE section_id = $1;

-- name: DeleteStaleSectionChunks :exec
DELETE FROM chunks c
WHERE c.section_id = sqlc.arg('section_id')::bigint
  AND c.source_type || ':' || c.source_id || ':' || c.seq <> ALL (sqlc.arg('keep')::text[]);

-- name: ListPendingSectionChunks :many
SELECT id, content, content_hash FROM chunks
WHERE section_id = sqlc.arg('section_id')::bigint AND (embedding IS NULL OR embed_model IS DISTINCT FROM sqlc.arg('model')::text)
ORDER BY id;

-- name: VisibleSectionIDs :many
-- Of the given sections, those the asker may open now (R-MR-15).
SELECT s.id
FROM document_sections s JOIN documents d ON d.id = s.document_id
WHERE s.id = ANY (sqlc.arg('ids')::bigint[]) AND d.archived_at IS NULL
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = d.project_id
          AND (m.all_clients OR d.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = d.client_id))));

-- name: ListNodeSections :many
-- The visible document sections these nodes came from, for a node page's
-- timeline (§7.7): where a menu's history starts. Archived documents never;
-- replaced ones come after current ones and say so. from_date and to_date
-- filter on the upload day, like a note's decision day.
SELECT s.number, s.title, s.body, d.key AS document_key, d.title AS document_title, d.client_id, c.name AS client_name,
       d.created_at AS uploaded_at, sk.key AS superseded_by_key
FROM document_sections s
JOIN documents d ON d.id = s.document_id
LEFT JOIN clients c ON c.id = d.client_id
LEFT JOIN documents sk ON sk.id = d.superseded_by
WHERE d.project_id = sqlc.arg('project_id') AND d.archived_at IS NULL
  AND EXISTS (SELECT 1 FROM document_section_nodes sn WHERE sn.section_id = s.id AND sn.node_id = ANY (sqlc.arg('node_ids')::bigint[]))
  AND (sqlc.arg('all_clients')::boolean OR d.client_id IS NULL OR d.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
  AND (sqlc.narg('client_id')::bigint IS NULL OR d.client_id = sqlc.narg('client_id')::bigint)
  AND (NOT sqlc.arg('core_only')::boolean OR d.client_id IS NULL)
  AND (sqlc.narg('from_date')::date IS NULL OR d.created_at >= sqlc.narg('from_date')::date)
  AND (sqlc.narg('to_date')::date IS NULL OR d.created_at < sqlc.narg('to_date')::date + 1)
ORDER BY d.superseded_by IS NOT NULL, d.created_at DESC, d.id DESC, s.position
LIMIT 100;
