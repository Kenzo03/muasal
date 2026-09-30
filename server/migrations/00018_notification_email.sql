-- +goose Up
-- Email delivery (MSL-10): a notification the user has not read after a couple
-- of minutes goes out once, in a batch per user, to those who chose email.
ALTER TABLE notifications ADD COLUMN emailed_at timestamptz;
CREATE INDEX notifications_unemailed_idx ON notifications (created_at) WHERE emailed_at IS NULL;

-- +goose Down
DROP INDEX notifications_unemailed_idx;
ALTER TABLE notifications DROP COLUMN emailed_at;
