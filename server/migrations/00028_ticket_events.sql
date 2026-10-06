-- +goose Up
-- Live ticket updates: every change to what a ticket page shows announces
-- '<project_id>:<ticket_id>' on zettra_tickets when its transaction commits.
-- The app fans these out to open tabs (GET /api/v1/events).
-- +goose StatementBegin
CREATE FUNCTION notify_ticket_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  r   jsonb := to_jsonb(CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END);
  ids bigint[];
BEGIN
  IF TG_TABLE_NAME = 'tickets' THEN
    PERFORM pg_notify('zettra_tickets', (r->>'project_id') || ':' || (r->>'id'));
    RETURN NULL;
  ELSIF TG_TABLE_NAME = 'ticket_links' THEN
    ids := ARRAY[(r->>'from_id')::bigint, (r->>'to_id')::bigint];
  ELSIF TG_TABLE_NAME = 'merge_requests' THEN -- a webhook marks it merged or closed
    ids := ARRAY(SELECT ticket_id FROM ticket_merge_requests WHERE mr_id = (r->>'id')::bigint);
  ELSIF TG_TABLE_NAME = 'commits' THEN
    ids := ARRAY(SELECT ticket_id FROM ticket_commits WHERE commit_id = (r->>'id')::bigint);
  ELSE
    ids := ARRAY[(r->>'ticket_id')::bigint]; -- NULL (an attachment elsewhere) matches no ticket
  END IF;
  PERFORM pg_notify('zettra_tickets', t.project_id || ':' || t.id) FROM tickets t WHERE t.id = ANY (ids);
  RETURN NULL;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER tickets_notify AFTER INSERT OR UPDATE OR DELETE ON tickets FOR EACH ROW EXECUTE FUNCTION notify_ticket_change();
CREATE TRIGGER comments_notify AFTER INSERT OR UPDATE OR DELETE ON comments FOR EACH ROW EXECUTE FUNCTION notify_ticket_change();
CREATE TRIGGER attachments_notify AFTER INSERT OR UPDATE OR DELETE ON attachments FOR EACH ROW EXECUTE FUNCTION notify_ticket_change();
CREATE TRIGGER ticket_nodes_notify AFTER INSERT OR UPDATE OR DELETE ON ticket_nodes FOR EACH ROW EXECUTE FUNCTION notify_ticket_change();
CREATE TRIGGER decision_records_notify AFTER INSERT OR UPDATE OR DELETE ON decision_records FOR EACH ROW EXECUTE FUNCTION notify_ticket_change();
CREATE TRIGGER ticket_commits_notify AFTER INSERT OR UPDATE OR DELETE ON ticket_commits FOR EACH ROW EXECUTE FUNCTION notify_ticket_change();
CREATE TRIGGER ticket_merge_requests_notify AFTER INSERT OR UPDATE OR DELETE ON ticket_merge_requests FOR EACH ROW EXECUTE FUNCTION notify_ticket_change();
CREATE TRIGGER ticket_links_notify AFTER INSERT OR UPDATE OR DELETE ON ticket_links FOR EACH ROW EXECUTE FUNCTION notify_ticket_change();
-- A merge request or commit row changes only when a webhook updates one that
-- tickets already link to; new links announce through the link tables above.
CREATE TRIGGER merge_requests_notify AFTER UPDATE ON merge_requests FOR EACH ROW EXECUTE FUNCTION notify_ticket_change();
CREATE TRIGGER commits_notify AFTER UPDATE ON commits FOR EACH ROW EXECUTE FUNCTION notify_ticket_change();

-- +goose Down
DROP TRIGGER commits_notify ON commits;
DROP TRIGGER merge_requests_notify ON merge_requests;
DROP TRIGGER ticket_links_notify ON ticket_links;
DROP TRIGGER ticket_merge_requests_notify ON ticket_merge_requests;
DROP TRIGGER ticket_commits_notify ON ticket_commits;
DROP TRIGGER decision_records_notify ON decision_records;
DROP TRIGGER ticket_nodes_notify ON ticket_nodes;
DROP TRIGGER attachments_notify ON attachments;
DROP TRIGGER comments_notify ON comments;
DROP TRIGGER tickets_notify ON tickets;
DROP FUNCTION notify_ticket_change();
