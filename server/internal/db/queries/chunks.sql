-- name: UpsertChunk :exec
-- A chunk whose text is unchanged keeps its vector and only takes the new
-- filter columns; changed text clears the vector for the embedder (§13.2).
INSERT INTO chunks (source_type, source_id, seq, ticket_id, project_id, client_id, node_ids, user_ids, contact_ids,
                    internal, occurred_at, content, content_hash)
VALUES (sqlc.arg('source_type'), sqlc.arg('source_id'), sqlc.arg('seq'), sqlc.arg('ticket_id'), sqlc.arg('project_id'),
        sqlc.narg('client_id'), sqlc.arg('node_ids')::bigint[], sqlc.arg('user_ids')::bigint[], sqlc.arg('contact_ids')::bigint[],
        sqlc.arg('internal'), sqlc.arg('occurred_at'), sqlc.arg('content'), sqlc.arg('content_hash'))
ON CONFLICT (source_type, source_id, seq) DO UPDATE SET
  ticket_id = excluded.ticket_id, project_id = excluded.project_id, client_id = excluded.client_id,
  node_ids = excluded.node_ids, user_ids = excluded.user_ids, contact_ids = excluded.contact_ids,
  internal = excluded.internal, occurred_at = excluded.occurred_at,
  embedding   = CASE WHEN chunks.content_hash = excluded.content_hash THEN chunks.embedding END,
  embed_model = CASE WHEN chunks.content_hash = excluded.content_hash THEN chunks.embed_model END,
  content = excluded.content, content_hash = excluded.content_hash, indexed_at = now();

-- name: DeleteChunksFrom :exec
-- Removes a source's parts from seq on: all of them with 0, or the tail that
-- a shorter text no longer has.
DELETE FROM chunks WHERE source_type = $1 AND source_id = $2 AND seq >= $3;

-- name: DeleteTicketChunks :exec
DELETE FROM chunks WHERE ticket_id = $1;

-- name: ListPendingChunks :many
-- Chunks without a vector from the current embedding model, newest first (§13.4).
SELECT id, content, content_hash FROM chunks
WHERE embedding IS NULL OR embed_model IS DISTINCT FROM sqlc.arg('model')::text
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg('lim');

-- name: SetChunkEmbedding :exec
-- The hash guard skips a vector whose text changed while it was embedded.
UPDATE chunks SET embedding = sqlc.arg('embedding')::halfvec, embed_model = sqlc.arg('model')::text, indexed_at = now()
WHERE id = sqlc.arg('id') AND content_hash = sqlc.arg('content_hash');

-- name: CountChunksByModel :many
SELECT coalesce(embed_model, '') AS model, count(*) AS chunks, max(indexed_at)::timestamptz AS latest
FROM chunks GROUP BY 1 ORDER BY 1;

-- name: CountPendingChunks :one
SELECT count(*) FROM chunks WHERE embedding IS NULL OR embed_model IS DISTINCT FROM sqlc.arg('model')::text;

-- name: ListTicketChunks :many
SELECT id, source_type, source_id, seq, content, content_hash, embed_model, (embedding IS NOT NULL)::boolean AS embedded
FROM chunks WHERE ticket_id = $1 ORDER BY source_type, source_id, seq;

-- name: CountEmbeddedChunks :one
SELECT count(*) FROM chunks WHERE embedding IS NOT NULL;
