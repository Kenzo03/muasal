-- +goose Up
-- Project documents and AI tree drafts (FSD §7.7).
ALTER TABLE projects ADD COLUMN doc_seq bigint NOT NULL DEFAULT 0;
CREATE TABLE documents (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id    bigint NOT NULL REFERENCES projects (id),
  number        bigint NOT NULL,
  key           text NOT NULL UNIQUE,             -- 'HRIS-DOC1'
  title         text NOT NULL,
  client_id     bigint REFERENCES clients (id),   -- NULL = all clients
  filename      text NOT NULL,
  content_type  text NOT NULL,
  size_bytes    bigint NOT NULL,
  sha256        bytea NOT NULL,                   -- the original, stored like attachments
  markdown      text NOT NULL,                    -- converted in the browser
  superseded_by bigint REFERENCES documents (id),
  uploaded_by   bigint NOT NULL REFERENCES users (id),
  created_at    timestamptz NOT NULL DEFAULT now(),
  archived_at   timestamptz,
  UNIQUE (project_id, number)
);
CREATE TABLE document_sections (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  document_id bigint NOT NULL REFERENCES documents (id),
  number      text NOT NULL,                      -- '7.4', or a running index
  title       text NOT NULL,
  level       int NOT NULL,
  position    int NOT NULL,
  body        text NOT NULL,
  UNIQUE (document_id, number)
);
CREATE TABLE document_section_nodes (
  section_id bigint NOT NULL REFERENCES document_sections (id),
  node_id    bigint NOT NULL REFERENCES nodes (id),
  PRIMARY KEY (section_id, node_id)
);
CREATE INDEX document_section_nodes_node_idx ON document_section_nodes (node_id);
CREATE TABLE tree_drafts (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id  bigint NOT NULL REFERENCES projects (id),
  document_id bigint NOT NULL REFERENCES documents (id),
  status      text NOT NULL CHECK (status IN ('running', 'ready', 'applied', 'discarded', 'failed')),
  proposal    jsonb NOT NULL DEFAULT '[]',
  used_ai     boolean NOT NULL,
  done_parts  int NOT NULL DEFAULT 0,           -- progress: model calls finished
  total_parts int NOT NULL DEFAULT 0,
  error       text NOT NULL DEFAULT '',
  created_by  bigint NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  applied_at  timestamptz
);

-- Sections are Ask evidence (R-MR-13): a chunk belongs to a ticket, a note or a section.
ALTER TABLE chunks ADD COLUMN section_id bigint REFERENCES document_sections (id);
ALTER TABLE chunks DROP CONSTRAINT chunks_owner;
ALTER TABLE chunks ADD CONSTRAINT chunks_owner CHECK (num_nonnulls(ticket_id, note_id, section_id) = 1);
ALTER TABLE chunks DROP CONSTRAINT chunks_source_type_check;
ALTER TABLE chunks ADD CONSTRAINT chunks_source_type_check CHECK (source_type IN ('ticket', 'comment', 'decision', 'note', 'code', 'document'));
CREATE INDEX chunks_section_idx ON chunks (section_id) WHERE section_id IS NOT NULL;

-- +goose Down
DELETE FROM chunks WHERE section_id IS NOT NULL;
ALTER TABLE chunks DROP CONSTRAINT chunks_source_type_check;
ALTER TABLE chunks ADD CONSTRAINT chunks_source_type_check CHECK (source_type IN ('ticket', 'comment', 'decision', 'note', 'code'));
ALTER TABLE chunks DROP CONSTRAINT chunks_owner;
ALTER TABLE chunks ADD CONSTRAINT chunks_owner CHECK (num_nonnulls(ticket_id, note_id) = 1);
ALTER TABLE chunks DROP COLUMN section_id;
DROP TABLE tree_drafts;
DROP TABLE document_section_nodes;
DROP TABLE document_sections;
DROP TABLE documents;
ALTER TABLE projects DROP COLUMN doc_seq;
