-- +goose Up
-- MSL-52: each morning the assignee hears what is due today or tomorrow, or
-- went overdue yesterday.
ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
  CHECK (type IN ('assigned', 'comment', 'mention', 'status', 'job_done', 'due'));

-- +goose Down
DELETE FROM notifications WHERE type = 'due';
ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
  CHECK (type IN ('assigned', 'comment', 'mention', 'status', 'job_done'));
