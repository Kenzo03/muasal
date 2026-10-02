-- Personal API tokens (FSD §14.3) and idempotency keys (§17.1).

-- name: CreateAPIToken :one
INSERT INTO api_tokens (user_id, name, token_hash, read_only, expires_at)
VALUES (sqlc.arg('user_id'), sqlc.arg('name'), sqlc.arg('token_hash'), sqlc.arg('read_only'), sqlc.narg('expires_at'))
RETURNING *;

-- name: ListAPITokens :many
SELECT * FROM api_tokens WHERE user_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC, id DESC;

-- name: RevokeAPIToken :one
UPDATE api_tokens SET revoked_at = now() WHERE id = sqlc.arg('id') AND user_id = sqlc.arg('user_id') AND revoked_at IS NULL
RETURNING *;

-- name: GetAPITokenUser :one
SELECT sqlc.embed(t), sqlc.embed(u)
FROM api_tokens t JOIN users u ON u.id = t.user_id
WHERE t.token_hash = $1;

-- name: TouchAPIToken :exec
UPDATE api_tokens SET last_used_at = now() WHERE id = $1;

-- name: GetIdempotentTicket :one
SELECT t.key FROM idempotency_keys k JOIN tickets t ON t.id = k.ticket_id
WHERE k.user_id = sqlc.arg('user_id') AND k.key = sqlc.arg('key') AND k.created_at > now() - interval '24 hours';

-- name: SaveIdempotencyKey :exec
-- A key older than 24 hours is free again.
INSERT INTO idempotency_keys (user_id, key, ticket_id) VALUES (sqlc.arg('user_id'), sqlc.arg('key'), sqlc.arg('ticket_id'))
ON CONFLICT (user_id, key) DO UPDATE SET ticket_id = excluded.ticket_id, created_at = now()
WHERE idempotency_keys.created_at <= now() - interval '24 hours';

-- name: PurgeIdempotencyKeys :execrows
DELETE FROM idempotency_keys WHERE created_at < now() - interval '24 hours';

-- name: LockIdempotencyKey :exec
SELECT pg_advisory_xact_lock(hashtextextended('idempotency:' || sqlc.arg('user_id')::bigint::text || ':' || sqlc.arg('key')::text, 0));

-- name: RevokeUserAPITokens :exec
-- A password reset or change ends every token the user holds.
UPDATE api_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL;
