-- +goose Up
-- Git links (FSD §14.1, §16): repositories, their commits and merge requests,
-- linked to the tickets their messages name.
CREATE TABLE git_repos (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects (id),
  provider   text NOT NULL CHECK (provider IN ('github', 'gitlab', 'gitea')),
  name       text NOT NULL,
  web_url    text NOT NULL,
  secret_enc bytea NOT NULL, -- AES-GCM with APP_SECRET_KEY
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE commits (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  repo_id      bigint NOT NULL REFERENCES git_repos (id) ON DELETE CASCADE,
  sha          text NOT NULL,
  message      text NOT NULL,
  author_name  text,
  author_email text,
  committed_at timestamptz,
  url          text,
  UNIQUE (repo_id, sha)
);
CREATE TABLE merge_requests (
  id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  repo_id   bigint NOT NULL REFERENCES git_repos (id) ON DELETE CASCADE,
  number    int NOT NULL,
  title     text NOT NULL,
  state     text NOT NULL,
  merged_at timestamptz,
  url       text,
  UNIQUE (repo_id, number)
);
CREATE TABLE ticket_commits (
  ticket_id bigint NOT NULL REFERENCES tickets (id),
  commit_id bigint NOT NULL REFERENCES commits (id) ON DELETE CASCADE,
  PRIMARY KEY (ticket_id, commit_id)
);
CREATE TABLE ticket_merge_requests (
  ticket_id bigint NOT NULL REFERENCES tickets (id),
  mr_id     bigint NOT NULL REFERENCES merge_requests (id) ON DELETE CASCADE,
  PRIMARY KEY (ticket_id, mr_id)
);
-- A delivery waits here for the job that processes it; the webhook answers at once.
CREATE TABLE webhook_deliveries (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  repo_id     bigint NOT NULL REFERENCES git_repos (id) ON DELETE CASCADE,
  event       text NOT NULL,
  payload     jsonb NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now()
);
-- A ticket's commits and merge requests are one more chunk of it (§13.1).
ALTER TABLE chunks DROP CONSTRAINT chunks_source_type_check;
ALTER TABLE chunks ADD CONSTRAINT chunks_source_type_check CHECK (source_type IN ('ticket', 'comment', 'decision', 'note', 'code'));

-- +goose Down
DELETE FROM chunks WHERE source_type = 'code';
ALTER TABLE chunks DROP CONSTRAINT chunks_source_type_check;
ALTER TABLE chunks ADD CONSTRAINT chunks_source_type_check CHECK (source_type IN ('ticket', 'comment', 'decision', 'note'));
DROP TABLE webhook_deliveries;
DROP TABLE ticket_merge_requests;
DROP TABLE ticket_commits;
DROP TABLE merge_requests;
DROP TABLE commits;
DROP TABLE git_repos;
