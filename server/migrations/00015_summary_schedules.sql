-- +goose Up
-- Weekly change summaries: a project admin schedules one for a client, or for
-- all clients; an hourly job writes each due one for the past seven days.
CREATE TABLE summary_schedules (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id  bigint NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  client_id   bigint REFERENCES clients (id) ON DELETE CASCADE,  -- NULL: all clients
  language    text NOT NULL CHECK (language IN ('id', 'en')),
  audience    text NOT NULL CHECK (audience IN ('internal', 'client')),
  weekday     smallint NOT NULL CHECK (weekday BETWEEN 1 AND 7),  -- ISO: 1 is Monday
  created_by  bigint NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  last_run_on date                                                -- the UTC day it last ran
);
CREATE INDEX summary_schedules_project_idx ON summary_schedules (project_id);
CREATE INDEX summary_schedules_weekday_idx ON summary_schedules (weekday);

-- +goose Down
DROP TABLE summary_schedules;
