-- +goose Up
-- Home (FSD §6.4). Comments, files and decision records are changes to their
-- ticket, so they touch its updated_at; "Recently updated" then reads tickets
-- by updated_at alone. My tickets read by assignee.
-- +goose StatementBegin
CREATE FUNCTION touch_ticket() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  UPDATE tickets SET updated_at = now() WHERE id = NEW.ticket_id;
  RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER comments_touch_ticket AFTER INSERT OR UPDATE ON comments FOR EACH ROW EXECUTE FUNCTION touch_ticket();
CREATE TRIGGER attachments_touch_ticket AFTER INSERT OR UPDATE ON attachments FOR EACH ROW EXECUTE FUNCTION touch_ticket();
CREATE TRIGGER decision_records_touch_ticket AFTER INSERT OR UPDATE ON decision_records FOR EACH ROW EXECUTE FUNCTION touch_ticket();
CREATE INDEX tickets_updated_idx ON tickets (updated_at DESC);
CREATE INDEX tickets_assignee_idx ON tickets (assignee_id) WHERE assignee_id IS NOT NULL;

-- +goose Down
DROP INDEX tickets_assignee_idx;
DROP INDEX tickets_updated_idx;
DROP TRIGGER decision_records_touch_ticket ON decision_records;
DROP TRIGGER attachments_touch_ticket ON attachments;
DROP TRIGGER comments_touch_ticket ON comments;
DROP FUNCTION touch_ticket();
