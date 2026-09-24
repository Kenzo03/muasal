-- +goose Up
-- Statuses, tickets, their menus, comments and attachments (FSD §8, §16).
ALTER TABLE projects ADD COLUMN ticket_seq bigint NOT NULL DEFAULT 0; -- incremented inside the ticket create transaction

CREATE TABLE statuses (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects (id),
  name       text NOT NULL,
  category   text NOT NULL CHECK (category IN ('todo', 'in_progress', 'done', 'cancelled')),
  position   int NOT NULL,
  color      text NOT NULL DEFAULT '#6B7280',
  is_default boolean NOT NULL DEFAULT false,
  UNIQUE (project_id, id), -- lets a ticket point at a status of its own project
  CONSTRAINT statuses_default_todo CHECK (NOT is_default OR category = 'todo')
);
CREATE UNIQUE INDEX statuses_name_uq ON statuses (project_id, lower(name));
CREATE UNIQUE INDEX statuses_default_uq ON statuses (project_id) WHERE is_default; -- one default at most (R-TK-2)

-- R-TK-2: every project starts with these statuses; the trigger adds them to new projects.
-- +goose StatementBegin
CREATE FUNCTION insert_default_statuses(pid bigint) RETURNS void LANGUAGE sql AS $$
  INSERT INTO statuses (project_id, name, category, position, color, is_default)
  VALUES (pid, 'To do', 'todo', 0, '#6B7280', true),
         (pid, 'In progress', 'in_progress', 1, '#2563EB', false),
         (pid, 'In review', 'in_progress', 2, '#7C3AED', false),
         (pid, 'Done', 'done', 3, '#16A34A', false),
         (pid, 'Cancelled', 'cancelled', 4, '#9CA3AF', false);
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION projects_default_statuses() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM insert_default_statuses(NEW.id);
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER projects_default_statuses AFTER INSERT ON projects
  FOR EACH ROW EXECUTE FUNCTION projects_default_statuses();

SELECT insert_default_statuses(id) FROM projects;

CREATE TABLE tickets (
  id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id           bigint NOT NULL REFERENCES projects (id),
  number               bigint NOT NULL,
  key                  text NOT NULL UNIQUE,                 -- 'HRIS-231'
  type                 text NOT NULL CHECK (type IN ('bug', 'change_request', 'feature')),
  title                text NOT NULL,
  description          text NOT NULL DEFAULT '',
  reason               text NOT NULL DEFAULT '',
  status_id            bigint NOT NULL,
  client_id            bigint,                               -- NULL = all clients (core work)
  requester_contact_id bigint REFERENCES contacts (id),
  requester_user_id    bigint REFERENCES users (id),
  reporter_id          bigint NOT NULL REFERENCES users (id),
  assignee_id          bigint REFERENCES users (id),
  priority             text NOT NULL DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high', 'urgent')),
  due_date             date,
  version              int NOT NULL DEFAULT 1,              -- optimistic locking (If-Match)
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, number),
  -- A status of the ticket's own project; a status in use cannot be deleted (R-TK-4).
  CONSTRAINT tickets_status_same_project FOREIGN KEY (project_id, status_id) REFERENCES statuses (project_id, id),
  -- A client linked to the project; a client in use cannot be unlinked (FSD §15.2).
  CONSTRAINT tickets_client_linked FOREIGN KEY (project_id, client_id) REFERENCES project_clients,
  CONSTRAINT tickets_one_requester CHECK (num_nonnulls(requester_contact_id, requester_user_id) = 1)
);
CREATE INDEX tickets_scope_idx ON tickets (project_id, client_id, status_id);

CREATE TABLE ticket_nodes (
  ticket_id bigint NOT NULL REFERENCES tickets (id),
  node_id   bigint NOT NULL CONSTRAINT ticket_nodes_node_fk REFERENCES nodes (id), -- linked nodes are archived, never deleted (R-MR-4)
  PRIMARY KEY (ticket_id, node_id)
);
CREATE INDEX ticket_nodes_node_idx ON ticket_nodes (node_id);

CREATE TABLE comments (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ticket_id  bigint NOT NULL REFERENCES tickets (id),
  author_id  bigint NOT NULL REFERENCES users (id),
  internal   boolean NOT NULL DEFAULT true,                 -- Internal by default; false = client-safe (FSD §8.7)
  body       text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  edited_at  timestamptz,
  deleted_at timestamptz
);
CREATE INDEX comments_ticket_idx ON comments (ticket_id, created_at);

CREATE TABLE attachments (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ticket_id    bigint NOT NULL REFERENCES tickets (id),
  uploader_id  bigint NOT NULL REFERENCES users (id),
  filename     text NOT NULL,
  content_type text NOT NULL,
  size_bytes   bigint NOT NULL,
  sha256       bytea NOT NULL,                              -- the file is <ATTACHMENTS_DIR>/<hex[:2]>/<hex>
  created_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz
);
CREATE INDEX attachments_ticket_idx ON attachments (ticket_id);

-- +goose Down
DROP TABLE attachments;
DROP TABLE comments;
DROP TABLE ticket_nodes;
DROP TABLE tickets;
DROP TRIGGER projects_default_statuses ON projects;
DROP FUNCTION projects_default_statuses();
DROP FUNCTION insert_default_statuses(bigint);
DROP TABLE statuses;
ALTER TABLE projects DROP COLUMN ticket_seq;
