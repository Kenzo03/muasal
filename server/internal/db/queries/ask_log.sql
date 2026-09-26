-- name: ListAskLog :many
-- The Ask log for system admins, newest first (FSD §15.4).
SELECT q.id, q.created_at, q.user_id, u.name AS user_name, q.question, q.status, q.llm_called, q.latency_ms, q.model,
       jsonb_array_length(q.evidence)::int AS evidence_count,
       (SELECT count(DISTINCT c.key)
        FROM jsonb_array_elements(coalesce(q.answer, '[]'::jsonb)) a, jsonb_array_elements_text(a -> 'cites') AS c(key))::int AS citations,
       f.rating, f.reasons, f.comment
FROM ask_queries q
JOIN users u ON u.id = q.user_id
LEFT JOIN ask_feedback f ON f.query_id = q.id
WHERE (sqlc.narg('status')::text IS NULL OR q.status = sqlc.narg('status')::text)
  AND (NOT sqlc.arg('slow')::boolean OR q.latency_ms > 30000)
  AND (NOT sqlc.arg('down')::boolean OR f.rating = -1)
  AND (sqlc.narg('user_id')::bigint IS NULL OR q.user_id = sqlc.narg('user_id')::bigint)
  AND (sqlc.narg('before')::bigint IS NULL OR q.id < sqlc.narg('before')::bigint)
ORDER BY q.id DESC
LIMIT sqlc.arg('lim');

-- name: GetAskLogEntry :one
SELECT q.*, u.name AS user_name,
       jsonb_array_length(q.evidence)::int AS evidence_count,
       (SELECT count(DISTINCT c.key)
        FROM jsonb_array_elements(coalesce(q.answer, '[]'::jsonb)) a, jsonb_array_elements_text(a -> 'cites') AS c(key))::int AS citations,
       f.rating, f.reasons, f.comment
FROM ask_queries q
JOIN users u ON u.id = q.user_id
LEFT JOIN ask_feedback f ON f.query_id = q.id
WHERE q.id = $1;

-- name: PurgeAskQueries :execrows
-- Retention: questions older than the cutoff go (FSD §15.4, 365 days by default).
DELETE FROM ask_queries WHERE created_at < sqlc.arg('cutoff');

-- name: PurgeEmptyThreads :execrows
-- Then the threads that retention left empty.
DELETE FROM ask_threads t
WHERE t.created_at < sqlc.arg('cutoff')
  AND NOT EXISTS (SELECT 1 FROM ask_queries q WHERE q.thread_id = t.id);
