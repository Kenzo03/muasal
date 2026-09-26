-- +goose Up
-- Pilot (P1) part one: ticket links, decision notes, Ask feedback (FSD §8.8,
-- §9.4, §10.7, §16).

-- A link is stored once and shown on both tickets with inverse wording (§8.8).
CREATE TABLE ticket_links (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  from_id    bigint NOT NULL REFERENCES tickets (id),
  to_id      bigint NOT NULL REFERENCES tickets (id),
  type       text NOT NULL CHECK (type IN ('reverses', 'extends', 'related_to')),
  created_by bigint NOT NULL REFERENCES users (id),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (from_id, to_id, type),
  CHECK (from_id <> to_id)
);
CREATE INDEX ticket_links_to_idx ON ticket_links (to_id);

-- Set while a reverses link points at the ticket (R-TK-5, R-TK-7).
ALTER TABLE decision_records ADD COLUMN superseded_by bigint REFERENCES tickets (id);

-- Decisions made outside tickets: meetings, calls, emails (§9.4).
ALTER TABLE projects ADD COLUMN note_seq bigint NOT NULL DEFAULT 0;
CREATE TABLE decision_notes (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id  bigint NOT NULL REFERENCES projects (id),
  number      bigint NOT NULL,
  key         text NOT NULL UNIQUE,              -- 'HRIS-DN7'
  title       text NOT NULL,
  decided_on  date NOT NULL,
  client_id   bigint REFERENCES clients (id),    -- NULL = all clients
  attendees   text NOT NULL DEFAULT '',
  body        text NOT NULL,
  created_by  bigint NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz,
  UNIQUE (project_id, number)
);
CREATE INDEX decision_notes_project_idx ON decision_notes (project_id, decided_on DESC);
CREATE TABLE decision_note_nodes (
  note_id bigint NOT NULL REFERENCES decision_notes (id),
  node_id bigint NOT NULL REFERENCES nodes (id),
  PRIMARY KEY (note_id, node_id)
);
CREATE INDEX decision_note_nodes_node_idx ON decision_note_nodes (node_id);
CREATE TABLE decision_note_tickets (
  note_id   bigint NOT NULL REFERENCES decision_notes (id),
  ticket_id bigint NOT NULL REFERENCES tickets (id),
  PRIMARY KEY (note_id, ticket_id)
);

-- Notes are indexed like tickets (§13.1): a chunk belongs to a ticket or a note.
ALTER TABLE chunks ALTER COLUMN ticket_id DROP NOT NULL;
ALTER TABLE chunks ADD COLUMN note_id bigint REFERENCES decision_notes (id);
ALTER TABLE chunks DROP CONSTRAINT chunks_source_type_check;
ALTER TABLE chunks ADD CONSTRAINT chunks_source_type_check CHECK (source_type IN ('ticket', 'comment', 'decision', 'note'));
ALTER TABLE chunks ADD CONSTRAINT chunks_owner CHECK (num_nonnulls(ticket_id, note_id) = 1);
CREATE INDEX chunks_note_idx ON chunks (note_id) WHERE note_id IS NOT NULL;

-- Thumbs up or down on an answer (§10.7); one per question, the latest wins.
CREATE TABLE ask_feedback (
  query_id   bigint PRIMARY KEY REFERENCES ask_queries (id) ON DELETE CASCADE,
  user_id    bigint NOT NULL REFERENCES users (id),
  rating     smallint NOT NULL CHECK (rating IN (-1, 1)),
  reasons    text[] NOT NULL DEFAULT '{}',
  comment    text,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE ask_feedback;
DROP INDEX chunks_note_idx;
DELETE FROM chunks WHERE note_id IS NOT NULL;
ALTER TABLE chunks DROP CONSTRAINT chunks_owner;
ALTER TABLE chunks DROP CONSTRAINT chunks_source_type_check;
ALTER TABLE chunks ADD CONSTRAINT chunks_source_type_check CHECK (source_type IN ('ticket', 'comment', 'decision'));
ALTER TABLE chunks DROP COLUMN note_id;
ALTER TABLE chunks ALTER COLUMN ticket_id SET NOT NULL;
DROP TABLE decision_note_tickets;
DROP TABLE decision_note_nodes;
DROP TABLE decision_notes;
ALTER TABLE projects DROP COLUMN note_seq;
ALTER TABLE decision_records DROP COLUMN superseded_by;
DROP TABLE ticket_links;
