-- +goose Up
-- MCP sign-in (OAuth 2.1 authorization code with PKCE, MCP spec): registered
-- clients and their one-time codes. The tokens issued are api_tokens rows.
CREATE TABLE oauth_clients (
  id            text PRIMARY KEY,
  name          text NOT NULL,
  redirect_uris text[] NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now()
);

-- A code is stored as its SHA-256, lives 10 minutes and works once.
CREATE TABLE oauth_codes (
  code_hash      bytea PRIMARY KEY,
  client_id      text NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
  user_id        bigint NOT NULL REFERENCES users (id),
  redirect_uri   text NOT NULL,
  code_challenge text NOT NULL,
  read_only      boolean NOT NULL,
  expires_at     timestamptz NOT NULL,
  used_at        timestamptz
);

-- +goose Down
DROP TABLE oauth_codes;
DROP TABLE oauth_clients;
