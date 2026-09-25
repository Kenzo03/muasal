-- +goose Up
-- AI settings, the retrieval index and the Ask log (FSD §11, §13, §16).
CREATE EXTENSION IF NOT EXISTS vector; -- pgvector 0.8

-- Install-wide settings as JSON, one row per area ('ai', later 'sign_in', ...).
CREATE TABLE settings (
  key        text PRIMARY KEY,
  value      jsonb NOT NULL,
  updated_by bigint REFERENCES users (id),
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- One row per piece of a source: a ticket header, a comment or a decision
-- record (§13.1). The filter columns copy the source's scope, so every
-- retrieval filter runs in SQL (§11.3). A chunk without a vector still serves
-- keyword search (R-AI-5).
CREATE TABLE chunks (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_type  text NOT NULL CHECK (source_type IN ('ticket', 'comment', 'decision')),
  source_id    bigint NOT NULL,                -- ticket id for 'ticket' and 'decision', comment id for 'comment'
  seq          int NOT NULL DEFAULT 0,         -- part number of a split source
  ticket_id    bigint NOT NULL REFERENCES tickets (id),
  project_id   bigint NOT NULL,
  client_id    bigint,                         -- NULL = core work
  node_ids     bigint[] NOT NULL DEFAULT '{}', -- the ticket's menus, directly linked
  user_ids     bigint[] NOT NULL DEFAULT '{}', -- requester, reporter and assignee users
  contact_ids  bigint[] NOT NULL DEFAULT '{}', -- the requesting contact
  internal     boolean NOT NULL DEFAULT false, -- a chunk of an Internal comment
  occurred_at  timestamptz NOT NULL,           -- closed or created date; comment date; confirmed date
  content      text NOT NULL,
  content_hash bytea NOT NULL,
  tsv          tsvector GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED,
  embedding    halfvec(1024),                  -- NULL until embedded
  embed_model  text,
  UNIQUE (source_type, source_id, seq)
);
CREATE INDEX chunks_filter_idx ON chunks (project_id, client_id, occurred_at);
CREATE INDEX chunks_ticket_idx ON chunks (ticket_id);
CREATE INDEX chunks_nodes_idx  ON chunks USING gin (node_ids);
CREATE INDEX chunks_tsv_idx    ON chunks USING gin (tsv);
CREATE INDEX chunks_pending_idx ON chunks (occurred_at DESC) WHERE embedding IS NULL;
CREATE INDEX chunks_vec_idx    ON chunks USING hnsw (embedding halfvec_cosine_ops) WITH (m = 16, ef_construction = 64);

-- Ask threads and the query log (§10.6, §10.8). The log keeps every question,
-- its scope, evidence, answer and timings; only system admins read it.
CREATE TABLE ask_threads (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id    bigint NOT NULL REFERENCES users (id),
  title      text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  hidden_at  timestamptz
);
CREATE INDEX ask_threads_user_idx ON ask_threads (user_id, created_at DESC) WHERE hidden_at IS NULL;

CREATE TABLE ask_queries (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  thread_id      bigint REFERENCES ask_threads (id),
  user_id        bigint NOT NULL REFERENCES users (id),
  question       text NOT NULL,
  lang           text NOT NULL,
  scope          jsonb NOT NULL,                -- explicit and detected chips
  evidence       jsonb NOT NULL,                -- [{key, score}]
  llm_called     boolean NOT NULL,
  status         text NOT NULL CHECK (status IN ('answered', 'not_enough_info', 'ai_off', 'error')),
  answer         jsonb,                         -- validated claims
  dropped        jsonb,                         -- claims and citations removed by validation
  model          text,                          -- "Local · qwen3.5:4b" or "Cloud · provider · model"
  latency_ms     int,
  first_claim_ms int,
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ask_queries_thread_idx ON ask_queries (thread_id, created_at);
CREATE INDEX ask_queries_user_idx ON ask_queries (user_id, created_at);

-- +goose Down
DROP TABLE ask_queries;
DROP TABLE ask_threads;
DROP TABLE chunks;
DROP TABLE settings;
DROP EXTENSION IF EXISTS vector;
