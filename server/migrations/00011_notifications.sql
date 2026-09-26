-- +goose Up
-- In-app and browser notifications (FSD §8.10, §16).
ALTER TABLE users ADD COLUMN notify_prefs jsonb NOT NULL DEFAULT '{}'; -- per event on or off, and browser notifications

CREATE TABLE notifications (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id    bigint NOT NULL REFERENCES users (id),
  type       text NOT NULL CHECK (type IN ('assigned', 'comment', 'mention', 'status', 'job_done')),
  ticket_id  bigint REFERENCES tickets (id),
  actor_id   bigint REFERENCES users (id),
  payload    jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  read_at    timestamptz -- rows older than 90 days are purged
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;

-- +goose Down
DROP TABLE notifications;
ALTER TABLE users DROP COLUMN notify_prefs;
