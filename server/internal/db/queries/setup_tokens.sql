-- name: CreateSetupToken :exec
INSERT INTO setup_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: VoidSetupTokens :exec
UPDATE setup_tokens SET used_at = now() WHERE user_id = $1 AND used_at IS NULL;

-- name: UseSetupToken :one
UPDATE setup_tokens SET used_at = now()
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
RETURNING user_id;
