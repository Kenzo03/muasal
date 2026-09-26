-- name: CreateThread :one
INSERT INTO ask_threads (user_id, title) VALUES ($1, $2) RETURNING *;

-- name: GetThread :one
-- A thread is private to its owner (§10.6); a hidden one is gone for them.
SELECT * FROM ask_threads WHERE id = $1 AND user_id = $2 AND hidden_at IS NULL;

-- name: ListThreads :many
SELECT * FROM ask_threads WHERE user_id = $1 AND hidden_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 100;

-- name: HideThread :execrows
-- The log keeps the thread until retention expires (§10.6).
UPDATE ask_threads SET hidden_at = now() WHERE id = $1 AND user_id = $2 AND hidden_at IS NULL;

-- name: InsertAskQuery :one
INSERT INTO ask_queries (thread_id, user_id, question, lang, scope, evidence, llm_called, status, answer, dropped, model, latency_ms, first_claim_ms)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id;

-- name: ListThreadQueries :many
SELECT q.id, q.question, q.lang, q.status, q.scope, q.answer, q.evidence, q.model, q.created_at, f.rating, f.reasons, f.comment
FROM ask_queries q
LEFT JOIN ask_feedback f ON f.query_id = q.id
WHERE q.thread_id = $1 ORDER BY q.created_at, q.id;

-- name: SaveAskFeedback :execrows
-- The asker's rating of one answer (§10.7); a later one replaces it. Zero
-- rows when the question is not theirs.
INSERT INTO ask_feedback (query_id, user_id, rating, reasons, comment)
SELECT q.id, q.user_id, sqlc.arg('rating'), sqlc.arg('reasons')::text[], sqlc.narg('comment')
FROM ask_queries q
WHERE q.id = sqlc.arg('query_id') AND q.user_id = sqlc.arg('user_id')
ON CONFLICT (query_id) DO UPDATE SET rating = excluded.rating, reasons = excluded.reasons, comment = excluded.comment, created_at = now();
