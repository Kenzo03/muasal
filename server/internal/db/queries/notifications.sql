-- Notifications (FSD §8.10). A row goes only to a user who can see the
-- ticket and has the event on; pg_notify tells the app's listener, which
-- pushes it to the user's open tabs. The notification fires on commit.

-- name: NotifyTicket :many
WITH ins AS (
  INSERT INTO notifications (user_id, type, ticket_id, actor_id, payload)
  SELECT u.id, sqlc.arg('type'), t.id, sqlc.narg('actor_id'), sqlc.arg('payload')
  FROM users u
  JOIN tickets t ON t.id = sqlc.arg('ticket_id')
  WHERE u.id = ANY (sqlc.arg('user_ids')::bigint[])
    AND u.id IS DISTINCT FROM sqlc.narg('actor_id')::bigint
    AND u.disabled_at IS NULL
    AND coalesce((u.notify_prefs ->> sqlc.arg('type')::text)::boolean, true)
    AND (u.is_admin OR EXISTS (
          SELECT 1 FROM memberships m
          WHERE m.user_id = u.id AND m.project_id = t.project_id
            AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                  SELECT 1 FROM membership_clients mc
                  WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))))
  RETURNING id, user_id
)
SELECT ins.id, ins.user_id, pg_notify('muasal_notifications', ins.user_id || ':' || ins.id)::text AS sent FROM ins;

-- name: NotifyJobDone :exec
-- An import (or a later tree draft or re-index) finished: its starter hears.
WITH ins AS (
  INSERT INTO notifications (user_id, type, payload)
  SELECT u.id, 'job_done', sqlc.arg('payload') FROM users u
  WHERE u.id = sqlc.arg('user_id') AND coalesce((u.notify_prefs ->> 'job_done')::boolean, true)
  RETURNING id, user_id
)
SELECT pg_notify('muasal_notifications', ins.user_id || ':' || ins.id) FROM ins;

-- name: ListNotifications :many
-- The latest 50, each checked against the ticket's visibility now (§8.10).
SELECT n.id, n.type, n.payload, n.created_at, n.read_at, t.key AS ticket_key, t.title AS ticket_title,
       n.actor_id, a.name AS actor_name
FROM notifications n
LEFT JOIN tickets t ON t.id = n.ticket_id
LEFT JOIN users a ON a.id = n.actor_id
WHERE n.user_id = sqlc.arg('user_id')
  AND (sqlc.narg('id')::bigint IS NULL OR n.id = sqlc.narg('id')::bigint)
  AND (t.id IS NULL OR sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = n.user_id AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))))
ORDER BY n.created_at DESC, n.id DESC
LIMIT 50;

-- name: CountUnread :one
SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkRead :exec
-- One notification, or all of them when id is null.
UPDATE notifications SET read_at = now()
WHERE user_id = sqlc.arg('user_id') AND read_at IS NULL AND (sqlc.narg('id')::bigint IS NULL OR id = sqlc.narg('id')::bigint);

-- name: PurgeNotifications :execrows
DELETE FROM notifications WHERE created_at < now() - interval '90 days';

-- name: SetNotifyPrefs :one
UPDATE users SET notify_prefs = sqlc.arg('prefs') WHERE id = sqlc.arg('id') RETURNING *;

-- name: ListCommenters :many
SELECT DISTINCT author_id::bigint FROM comments WHERE ticket_id = $1 AND author_id IS NOT NULL AND deleted_at IS NULL;

-- name: ListMentionable :many
-- Project members who can see the ticket, with the handle an @mention uses:
-- the email's part before the @ (§8.7).
SELECT u.id, u.name, lower(split_part(u.email, '@', 1))::text AS handle
FROM users u
JOIN tickets t ON t.id = sqlc.arg('ticket_id')
JOIN memberships m ON m.user_id = u.id AND m.project_id = t.project_id
WHERE u.disabled_at IS NULL
  AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
        SELECT 1 FROM membership_clients mc
        WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))
ORDER BY lower(u.name), u.id;
