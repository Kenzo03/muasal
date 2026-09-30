-- +goose Up
-- MSL-67: a project's releases, such as v1.0, and the release each ticket
-- ships in, so a summary can say what went into one.
CREATE TABLE releases (
  id          bigserial PRIMARY KEY,
  project_id  bigint NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 50),
  released_on date,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX releases_project_name ON releases (project_id, lower(name));
ALTER TABLE tickets ADD COLUMN release_id bigint REFERENCES releases (id);
CREATE INDEX tickets_release_idx ON tickets (release_id) WHERE release_id IS NOT NULL;

-- +goose Down
ALTER TABLE tickets DROP COLUMN release_id;
DROP TABLE releases;
