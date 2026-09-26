-- +goose Up
-- Ticket import from CSV and Jira (FSD §14.2, §16).

-- Where a ticket came from, and its key in the old tool (R-IN-1, R-IN-2).
ALTER TABLE tickets ADD COLUMN source text NOT NULL DEFAULT 'web' CHECK (source IN ('web', 'api', 'import', 'email'));
ALTER TABLE tickets ADD COLUMN external_ref text;   -- e.g. Jira 'PAY-332'
ALTER TABLE tickets ADD COLUMN external_meta jsonb; -- original fields kept from the import
CREATE UNIQUE INDEX tickets_external_uq ON tickets (project_id, external_ref) WHERE external_ref IS NOT NULL;
CREATE INDEX tickets_external_ref_idx ON tickets (upper(external_ref)) WHERE external_ref IS NOT NULL;

-- Imported comments keep the old author's name without an account (R-IN-4),
-- and match on a hash on re-import (R-IN-1).
ALTER TABLE comments ALTER COLUMN author_id DROP NOT NULL;
ALTER TABLE comments ADD COLUMN author_label text;
ALTER TABLE comments ADD COLUMN external_hash bytea;
ALTER TABLE comments ADD CONSTRAINT comments_author CHECK (num_nonnulls(author_id, author_label) >= 1);
CREATE UNIQUE INDEX comments_external_uq ON comments (ticket_id, external_hash) WHERE external_hash IS NOT NULL;

-- One import: the uploaded file, its mapping, the dry run's counts and errors,
-- and progress while it runs (§14.2).
CREATE TABLE import_runs (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  kind        text NOT NULL CHECK (kind IN ('tickets', 'nodes')),
  project_id  bigint NOT NULL REFERENCES projects (id),
  file_name   text NOT NULL,
  file_path   text NOT NULL,
  mapping     jsonb NOT NULL DEFAULT '{}',
  status      text NOT NULL CHECK (status IN ('uploaded', 'dry_run', 'running', 'done', 'failed')),
  stats       jsonb NOT NULL DEFAULT '{}',
  errors      jsonb NOT NULL DEFAULT '[]',
  created_by  bigint NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz
);

-- +goose Down
DROP TABLE import_runs;
DROP INDEX comments_external_uq;
ALTER TABLE comments DROP CONSTRAINT comments_author;
ALTER TABLE comments DROP COLUMN external_hash;
ALTER TABLE comments DROP COLUMN author_label;
DELETE FROM comments WHERE author_id IS NULL;
ALTER TABLE comments ALTER COLUMN author_id SET NOT NULL;
DROP INDEX tickets_external_ref_idx;
DROP INDEX tickets_external_uq;
ALTER TABLE tickets DROP COLUMN external_meta;
ALTER TABLE tickets DROP COLUMN external_ref;
ALTER TABLE tickets DROP COLUMN source;
