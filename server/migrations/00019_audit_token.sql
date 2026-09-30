-- +goose Up
-- MSL-27: the audit log names the API token an action came through.
ALTER TABLE audit_events ADD COLUMN token_id bigint REFERENCES api_tokens (id);

-- +goose Down
ALTER TABLE audit_events DROP COLUMN token_id;
