-- MCP sign-in: OAuth clients and authorization codes (MCP spec).

-- name: CreateOAuthClient :one
INSERT INTO oauth_clients (id, name, redirect_uris)
VALUES (sqlc.arg('id'), sqlc.arg('name'), sqlc.arg('redirect_uris'))
RETURNING *;

-- name: GetOAuthClient :one
SELECT * FROM oauth_clients WHERE id = $1;

-- name: CreateOAuthCode :exec
INSERT INTO oauth_codes (code_hash, client_id, user_id, redirect_uri, code_challenge, read_only, expires_at)
VALUES (sqlc.arg('code_hash'), sqlc.arg('client_id'), sqlc.arg('user_id'), sqlc.arg('redirect_uri'),
        sqlc.arg('code_challenge'), sqlc.arg('read_only'), sqlc.arg('expires_at'));

-- name: UseOAuthCode :one
-- Marks a live code used and returns it; a used or expired code returns no row.
UPDATE oauth_codes SET used_at = now()
WHERE code_hash = $1 AND used_at IS NULL AND expires_at > now()
RETURNING *;

-- name: PurgeOAuthCodes :execrows
DELETE FROM oauth_codes WHERE expires_at < now() - interval '1 day';
