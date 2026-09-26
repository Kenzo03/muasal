-- +goose Up
-- Pilot (P1) part two: personal API tokens and idempotent ticket creation
-- (FSD §14.3, §17.1, §16).

-- Personal access tokens: shown once, stored as a SHA-256 hash (§14.3).
CREATE TABLE api_tokens (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id      bigint NOT NULL REFERENCES users (id),
  name         text NOT NULL,
  token_hash   bytea NOT NULL UNIQUE,
  read_only    boolean NOT NULL DEFAULT true,
  expires_at   timestamptz,
  last_used_at timestamptz,
  revoked_at   timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX api_tokens_user_idx ON api_tokens (user_id, created_at DESC);

-- POST /projects/{key}/tickets with an Idempotency-Key returns the ticket it
-- created before, for 24 hours (§17.1).
CREATE TABLE idempotency_keys (
  user_id    bigint NOT NULL REFERENCES users (id),
  key        text NOT NULL,
  ticket_id  bigint NOT NULL REFERENCES tickets (id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, key)
);

-- +goose Down
DROP TABLE idempotency_keys;
DROP TABLE api_tokens;
