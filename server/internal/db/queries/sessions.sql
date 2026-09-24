-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at, ip, user_agent)
VALUES ($1, $2, $3, $4, $5);

-- name: GetSession :one
SELECT sqlc.embed(u), s.last_seen_at, s.expires_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now() WHERE token_hash = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: DeleteOtherSessions :exec
DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2;
