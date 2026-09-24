-- name: CreateUser :one
INSERT INTO users (email, name, is_admin, locale, timezone)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower(sqlc.arg('email'));

-- name: ListUsers :many
SELECT * FROM users ORDER BY lower(name), id;

-- name: UpdateUser :one
UPDATE users SET
  name     = coalesce(sqlc.narg('name'), name),
  is_admin = coalesce(sqlc.narg('is_admin'), is_admin),
  locale   = coalesce(sqlc.narg('locale'), locale),
  timezone = coalesce(sqlc.narg('timezone'), timezone)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetDisabled :one
UPDATE users
SET disabled_at = CASE WHEN sqlc.arg('disabled')::boolean THEN coalesce(disabled_at, now()) ELSE NULL END
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetPasswordHash :exec
UPDATE users
SET password_hash = sqlc.narg('password_hash'), failed_logins = 0, failed_since = NULL, locked_until = NULL
WHERE id = sqlc.arg('id');

-- name: RecordLoginFailure :exec
-- Counts failures inside a 15-minute window; the 5th locks the account for 15 minutes (FSD §15.1).
-- Every expression in an UPDATE reads the row's old values.
UPDATE users SET
  failed_logins = CASE WHEN failed_since IS NULL OR failed_since < now() - interval '15 minutes'
                       THEN 1 ELSE failed_logins + 1 END,
  failed_since  = CASE WHEN failed_since IS NULL OR failed_since < now() - interval '15 minutes'
                       THEN now() ELSE failed_since END,
  locked_until  = CASE WHEN failed_since IS NOT NULL AND failed_since >= now() - interval '15 minutes'
                            AND failed_logins + 1 >= 5
                       THEN now() + interval '15 minutes' ELSE locked_until END
WHERE id = $1;

-- name: RecordLoginSuccess :exec
UPDATE users SET failed_logins = 0, failed_since = NULL, locked_until = NULL, last_login_at = now()
WHERE id = $1;
