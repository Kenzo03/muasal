-- +goose Up
-- AI-drafted decision records (FSD §9.3) and change summaries (§12.1).
ALTER TABLE decision_records ADD COLUMN ai_drafted boolean NOT NULL DEFAULT false;

CREATE TABLE summaries (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects (id),
  created_by bigint NOT NULL REFERENCES users (id),
  title      text NOT NULL,
  params     jsonb NOT NULL,              -- node, client, dates, cancelled, language, audience
  items      jsonb NOT NULL,              -- the ticked tickets and notes, with the appendix fields
  markdown   text NOT NULL,               -- editable; editing never changes tickets
  model      text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX summaries_project_idx ON summaries (project_id, created_at DESC);

-- +goose Down
DROP TABLE summaries;
ALTER TABLE decision_records DROP COLUMN ai_drafted;
