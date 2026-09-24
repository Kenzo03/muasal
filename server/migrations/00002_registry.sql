-- +goose Up
-- Projects, clients, contacts, memberships and the module tree (FSD §7, §15.2, §15.3, §16).
CREATE TABLE projects (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  key         text NOT NULL UNIQUE CONSTRAINT projects_key_format CHECK (key ~ '^[A-Z][A-Z0-9]{1,9}$'),
  name        text NOT NULL,
  description text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE clients (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name        text NOT NULL,
  code        text,
  aliases     text[] NOT NULL DEFAULT '{}',
  archived_at timestamptz
);
CREATE UNIQUE INDEX clients_name_uq ON clients (lower(name));

CREATE TABLE project_clients (
  project_id bigint NOT NULL REFERENCES projects (id),
  client_id  bigint NOT NULL CONSTRAINT project_clients_client_fk REFERENCES clients (id),
  PRIMARY KEY (project_id, client_id)
);

CREATE TABLE contacts (
  id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  client_id bigint CONSTRAINT contacts_client_fk REFERENCES clients (id), -- NULL = internal person
  name      text NOT NULL,
  title     text,
  email     text,
  phone     text
);
CREATE INDEX contacts_client_idx ON contacts (client_id);

CREATE TABLE memberships (
  user_id     bigint NOT NULL REFERENCES users (id),
  project_id  bigint NOT NULL REFERENCES projects (id),
  role        text NOT NULL CHECK (role IN ('admin', 'member', 'viewer')),
  all_clients boolean NOT NULL DEFAULT true,
  PRIMARY KEY (user_id, project_id),
  -- Project admins manage the tree, members and clients, so they see every client (R-AC-11).
  CONSTRAINT memberships_admin_all_clients CHECK (role <> 'admin' OR all_clients)
);
CREATE INDEX memberships_project_idx ON memberships (project_id);

-- A scoped membership lists clients linked to its project (R-AC-1).
CREATE TABLE membership_clients (
  user_id    bigint NOT NULL,
  project_id bigint NOT NULL,
  client_id  bigint NOT NULL,
  PRIMARY KEY (user_id, project_id, client_id),
  FOREIGN KEY (user_id, project_id) REFERENCES memberships ON DELETE CASCADE,
  CONSTRAINT membership_clients_linked FOREIGN KEY (project_id, client_id) REFERENCES project_clients
);

CREATE TABLE nodes (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id      bigint NOT NULL REFERENCES projects (id),
  parent_id       bigint CONSTRAINT nodes_parent_fk REFERENCES nodes (id), -- NULL = top level
  type            text NOT NULL CHECK (type IN ('module', 'menu')),
  name            text NOT NULL,
  code            text,
  aliases         text[] NOT NULL DEFAULT '{}',
  description     text NOT NULL DEFAULT '',
  client_specific boolean NOT NULL DEFAULT false,
  source          text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'csv', 'ai_draft')),
  position        int NOT NULL DEFAULT 0,
  archived_at     timestamptz,
  CONSTRAINT nodes_specific_menu CHECK (type = 'menu' OR NOT client_specific) -- R-MR-7
);
CREATE UNIQUE INDEX nodes_sibling_uq ON nodes (project_id, coalesce(parent_id, 0), lower(name))
  WHERE archived_at IS NULL;
CREATE UNIQUE INDEX nodes_code_uq ON nodes (project_id, code) WHERE code IS NOT NULL;
CREATE INDEX nodes_parent_idx ON nodes (parent_id);

-- A client-specific menu names clients linked to its project (R-MR-7).
CREATE TABLE node_clients (
  node_id    bigint NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
  project_id bigint NOT NULL,
  client_id  bigint NOT NULL,
  PRIMARY KEY (node_id, client_id),
  CONSTRAINT node_clients_linked FOREIGN KEY (project_id, client_id) REFERENCES project_clients
);

-- +goose Down
DROP TABLE node_clients;
DROP TABLE nodes;
DROP TABLE membership_clients;
DROP TABLE memberships;
DROP TABLE contacts;
DROP TABLE project_clients;
DROP TABLE clients;
DROP TABLE projects;
