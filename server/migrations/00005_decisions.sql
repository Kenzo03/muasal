-- +goose Up
-- Closing, decision records and ticket search (FSD §9, §16).
ALTER TABLE tickets ADD COLUMN closed_at timestamptz; -- set on close, cleared on reopen (FSD §8.1)
CREATE INDEX tickets_closed_idx ON tickets (project_id, closed_at DESC);

-- One record per ticket, written on close (DC-1). Client and menus come from
-- the ticket (R-DC-3). A draft has no confirmer; a confirmed record does (R-DC-4).
CREATE TABLE decision_records (
  ticket_id    bigint PRIMARY KEY REFERENCES tickets (id),
  what_changed text NOT NULL,
  why          text NOT NULL,
  alternatives text NOT NULL DEFAULT '',
  outcome      text NOT NULL CHECK (outcome IN ('implemented', 'rejected')),
  state        text NOT NULL CHECK (state IN ('draft', 'confirmed')),
  confirmed_by bigint REFERENCES users (id),
  confirmed_at timestamptz,
  CONSTRAINT decision_records_confirmed CHECK (state = 'draft' OR (confirmed_by IS NOT NULL AND confirmed_at IS NOT NULL))
);

-- Search (FSD §6.1): words anywhere in a ticket, and parts of titles.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX tickets_search_idx ON tickets USING gin (to_tsvector('simple', key || ' ' || title || ' ' || reason || ' ' || description));
CREATE INDEX tickets_title_trgm ON tickets USING gin (title gin_trgm_ops);

-- +goose Down
DROP INDEX tickets_title_trgm;
DROP INDEX tickets_search_idx;
DROP EXTENSION IF EXISTS pg_trgm;
DROP TABLE decision_records;
DROP INDEX tickets_closed_idx;
ALTER TABLE tickets DROP COLUMN closed_at;
