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

-- name: PendingEmails :many
-- MSL-10: notifications still unread two minutes on, from the last day, not
-- yet emailed, for users who chose email; the sender groups them per user.
SELECT n.id, n.user_id, n.type, n.payload, n.created_at, u.email, u.name AS user_name, u.locale,
       t.key AS ticket_key, t.title AS ticket_title, a.name AS actor_name
FROM notifications n
JOIN users u ON u.id = n.user_id
LEFT JOIN tickets t ON t.id = n.ticket_id
LEFT JOIN users a ON a.id = n.actor_id
WHERE n.emailed_at IS NULL AND n.read_at IS NULL
  AND n.created_at > now() - interval '1 day' AND n.created_at < now() - interval '2 minutes'
  AND u.disabled_at IS NULL AND coalesce((u.notify_prefs ->> 'email')::boolean, false)
ORDER BY n.user_id, n.id
LIMIT 500;

-- name: MarkEmailed :exec
UPDATE notifications SET emailed_at = now() WHERE id = ANY (sqlc.arg('ids')::bigint[]);

-- name: NotifyDue :many
-- MSL-52: from 08:00 in the assignee's timezone, once a day per ticket, their
-- open tickets due today or tomorrow, or overdue since yesterday. Those who
-- turned "due" off, or can no longer see the ticket, hear nothing.
WITH ins AS (
  INSERT INTO notifications (user_id, type, ticket_id, payload)
  SELECT u.id, 'due', t.id,
         jsonb_build_object('when', CASE t.due_date - d.today WHEN 0 THEN 'today' WHEN 1 THEN 'tomorrow' ELSE 'overdue' END,
                            'due', t.due_date, 'on', d.today)
  FROM tickets t
  JOIN users u ON u.id = t.assignee_id AND u.disabled_at IS NULL
  CROSS JOIN LATERAL (
    SELECT (sqlc.arg('now')::timestamptz AT TIME ZONE u.timezone)::date AS today,
           extract(hour FROM sqlc.arg('now')::timestamptz AT TIME ZONE u.timezone) AS hour
  ) d
  WHERE t.closed_at IS NULL AND t.due_date - d.today IN (-1, 0, 1) AND d.hour >= 8
    AND coalesce((u.notify_prefs ->> 'due')::boolean, true)
    AND (u.is_admin OR EXISTS (
          SELECT 1 FROM memberships m
          WHERE m.user_id = u.id AND m.project_id = t.project_id
            AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                  SELECT 1 FROM membership_clients mc
                  WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))))
    AND NOT EXISTS (
          SELECT 1 FROM notifications n
          WHERE n.user_id = u.id AND n.ticket_id = t.id AND n.type = 'due' AND n.payload ->> 'on' = d.today::text)
  RETURNING id, user_id
)
SELECT ins.id, ins.user_id, pg_notify('muasal_notifications', ins.user_id || ':' || ins.id)::text AS sent FROM ins;
