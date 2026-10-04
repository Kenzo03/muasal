-- +goose Up
-- MSL-57: people who follow a ticket they're not on hear its comments and status.
CREATE TABLE ticket_followers (
  ticket_id  bigint NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
  user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (ticket_id, user_id)
);

-- +goose Down
DROP TABLE ticket_followers;
