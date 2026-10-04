-- +goose Up
-- MSL-56: free-text labels group tickets across menus, such as pilot or UAT.
ALTER TABLE tickets ADD COLUMN labels text[] NOT NULL DEFAULT '{}';
CREATE INDEX tickets_labels_idx ON tickets USING gin (labels);

-- +goose Down
DROP INDEX tickets_labels_idx;
ALTER TABLE tickets DROP COLUMN labels;
