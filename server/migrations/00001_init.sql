-- +goose Up
CREATE TABLE users (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  email          text NOT NULL,
  name           text NOT NULL,
  password_hash  text,                                  -- argon2id PHC string; NULL until set
  is_admin       boolean NOT NULL DEFAULT false,
  locale         text NOT NULL DEFAULT 'id' CHECK (locale IN ('id', 'en')),
  timezone       text NOT NULL DEFAULT 'Asia/Jakarta',
  failed_logins  int NOT NULL DEFAULT 0,               -- sign-in lockout window (FSD §15.1)
  failed_since   timestamptz,
  locked_until   timestamptz,
  disabled_at    timestamptz,
  last_login_at  timestamptz,
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_uq ON users (lower(email));

CREATE TABLE setup_tokens (
  token_hash bytea PRIMARY KEY,                         -- sha256 of the link token
  user_id    bigint NOT NULL REFERENCES users (id),
  expires_at timestamptz NOT NULL,
  used_at    timestamptz
);
CREATE INDEX setup_tokens_user_idx ON setup_tokens (user_id);

CREATE TABLE sessions (
  token_hash   bytea PRIMARY KEY,                       -- sha256 of the cookie value
  user_id      bigint NOT NULL REFERENCES users (id),
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz NOT NULL,
  ip           inet,
  user_agent   text
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE audit_events (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  actor_id    bigint REFERENCES users (id),             -- NULL = system
  via         text NOT NULL CHECK (via IN ('web', 'api', 'system', 'import', 'webhook')),
  entity      text NOT NULL,
  entity_id   bigint NOT NULL,
  project_id  bigint,
  action      text NOT NULL,
  changes     jsonb NOT NULL DEFAULT '{}',
  request_id  text,
  ip          inet
);
CREATE INDEX audit_entity_idx ON audit_events (entity, entity_id, occurred_at);

-- +goose Down
DROP TABLE audit_events;
DROP TABLE sessions;
DROP TABLE setup_tokens;
DROP TABLE users;
