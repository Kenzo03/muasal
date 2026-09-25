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
SELECT id, question, lang, status, answer, model, created_at FROM ask_queries WHERE thread_id = $1 ORDER BY created_at, id;
