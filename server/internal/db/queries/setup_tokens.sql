-- name: CreateSetupToken :exec
INSERT INTO setup_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: VoidSetupTokens :exec
UPDATE setup_tokens SET used_at = now() WHERE user_id = $1 AND used_at IS NULL;

-- name: UseSetupToken :one
UPDATE setup_tokens SET used_at = now()
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
  AND user_id IN (SELECT id FROM users WHERE disabled_at IS NULL)
RETURNING user_id;

-- name: GetSetupTokenUser :one
-- The account a live setup link belongs to, so the setup page can name it (MSL-20).
SELECT u.name, u.email
FROM setup_tokens st JOIN users u ON u.id = st.user_id
WHERE st.token_hash = $1 AND st.used_at IS NULL AND st.expires_at > now() AND u.disabled_at IS NULL;
