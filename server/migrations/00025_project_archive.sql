-- +goose Up
-- MSL-64: a finished project is archived: read-only, out of pickers and Home,
-- still readable under All projects until a project admin restores it.
ALTER TABLE projects ADD COLUMN archived_at timestamptz;

-- +goose Down
ALTER TABLE projects DROP COLUMN archived_at;
