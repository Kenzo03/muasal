# MCP server for tickets

**Date:** 2026-10-03
**Branch:** `feat/mcp`

## Problem

AI agents such as Claude Code can't work in Muasal today. The REST API and personal API tokens exist, but each agent needs a token copied by hand and its own client code. The Model Context Protocol (MCP) lets an agent discover tools and call them, and its authorization flow lets the user sign in through a web page once while the agent keeps the token.

## Goals

1. An MCP endpoint at `/mcp` with tools to list, read, create, update, transition and cancel tickets, plus one to read a project's statuses, menus and clients.
2. Sign-in through the browser: an MCP client that meets a 401 discovers Muasal's authorization server, the user approves on a Muasal page, and the client stores the token it receives.
3. The same visibility, permissions, validation, audit and rate limits as the REST API, because every tool goes through it.

## Non-goals

- **Deleting tickets.** Muasal keeps why every change happened; "delete" in the MCP tools means cancel, which needs a decision record like the web app does.
- **Refresh tokens and short-lived access tokens.** The issued token is a normal API token that lives until it is revoked. Add refresh tokens when a client needs them.
- **Scopes beyond read-only.** A token can write or is read-only, as API tokens are today.
- **Other MCP features:** resources, prompts, sampling, notifications, and tools for comments, notes, documents or Ask. Add them one by one when an agent needs them.
- **Browser-based MCP clients.** No CORS on the OAuth endpoints. Desktop and command-line clients don't need it.
- **A stdio transport** (`app mcp`). The HTTP endpoint serves local and remote agents alike.

## How an agent connects

1. The user adds `https://muasal.example.com/mcp` to the agent, e.g. `claude mcp add --transport http muasal https://muasal.example.com/mcp`.
2. The agent calls `/mcp` with no token. Muasal answers 401 with `WWW-Authenticate: Bearer resource_metadata="<PUBLIC_URL>/.well-known/oauth-protected-resource/mcp"`.
3. The agent reads the protected-resource metadata (RFC 9728), then the authorization server metadata (RFC 8414), and registers itself at `/oauth/register` (RFC 7591).
4. The agent opens the browser at `/oauth/authorize` with PKCE (S256), `state` and `resource`.
5. The page asks the user to sign in if needed, then shows the agent's name, the account it will act as, a **Read only** box and **Allow** / **Deny**.
6. **Allow** redirects to the agent's `redirect_uri` with a one-time code. The agent exchanges it at `/oauth/token` for an `msl_…` token, stores it, and sends it as `Authorization: Bearer` from then on.
7. The user sees the token in Settings › API tokens, named after the agent, and can revoke it there. A revoked token answers 401, and the agent starts again at step 2.

## Server

### Data

Migration `00020_oauth.sql`:

```sql
-- MCP sign-in (OAuth 2.1 authorization code with PKCE): registered clients
-- and their one-time codes. The tokens issued are api_tokens rows.
CREATE TABLE oauth_clients (
  id            text PRIMARY KEY,          -- random: crypto/rand.Text(), 26 base32 chars
  name          text NOT NULL,             -- client_name, at most 100 chars
  redirect_uris text[] NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_codes (
  code_hash      bytea PRIMARY KEY,        -- SHA-256 of the code, as for api_tokens
  client_id      text NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
  user_id        bigint NOT NULL REFERENCES users (id),
  redirect_uri   text NOT NULL,
  code_challenge text NOT NULL,            -- S256 only
  read_only      boolean NOT NULL,
  expires_at     timestamptz NOT NULL,     -- 10 minutes after approval
  used_at        timestamptz
);
```

The app role gets the usual grants through `migrate.Up`. No new job is needed: the token endpoint ignores expired codes, and the daily `PurgeAsk` job in `internal/indexer/purge.go`, which already drops expired idempotency keys, also deletes codes that expired more than a day ago.

### Endpoints outside `/api/v1`

These sit on the mux next to `/webhooks`, so they skip the session and Origin middleware. Each one is in `server/internal/httpapi/oauth.go` or `mcp.go`.

| Route | Purpose |
| --- | --- |
| `GET /.well-known/oauth-protected-resource` and `.../oauth-protected-resource/mcp` | `{"resource": "<PUBLIC_URL>/mcp", "authorization_servers": ["<PUBLIC_URL>"], "bearer_methods_supported": ["header"]}` |
| `GET /.well-known/oauth-authorization-server` | `issuer` = PUBLIC_URL; `authorization_endpoint`, `token_endpoint`, `registration_endpoint`; `response_types_supported: ["code"]`, `grant_types_supported: ["authorization_code"]`, `code_challenge_methods_supported: ["S256"]`, `token_endpoint_auth_methods_supported: ["none"]` |
| `POST /oauth/register` | Dynamic client registration. Public clients only. |
| `POST /oauth/token` | Exchanges a code for a token. |
| `POST /mcp` | The MCP endpoint (streamable HTTP). `GET` and `DELETE` answer 405: the server is stateless. |

**`POST /oauth/register`** takes JSON with `client_name` and `redirect_uris` and answers 201 with `client_id`, `client_name`, `redirect_uris`, `token_endpoint_auth_method: "none"`, `grant_types: ["authorization_code"]` and `response_types: ["code"]`. It rejects, with 400 `invalid_redirect_uri` or `invalid_client_metadata`:
- more than 5 redirect URIs, or none;
- a URI with a fragment, or one that is not `https://`, or `http://` on `localhost`, `127.0.0.1` or `[::1]` (any port, for native clients, RFC 8252);
- a name longer than 100 characters. A missing name becomes "MCP client".

It shares the sign-in limiter: 20 requests a minute per IP. Registering creates no access: nothing is issued until a signed-in user approves.

**`POST /oauth/token`** takes `application/x-www-form-urlencoded` with `grant_type=authorization_code`, `code`, `redirect_uri`, `client_id`, `code_verifier` and an optional `resource`. In one statement it marks the code used (`UPDATE … SET used_at = now() WHERE code_hash = $1 AND used_at IS NULL AND expires_at > now() RETURNING …`), then checks that the client, the redirect URI and `BASE64URL(SHA-256(code_verifier))` match, and that `resource`, when given, is `<PUBLIC_URL>/mcp`. On success it creates an API token for the code's user, named `<client name> (MCP)` and cut to the token-name limit of 100 characters, with the code's `read_only` and no expiry, audits `token create` with `via: "web"` (the user approved it in the browser; `audit_events.via` allows no new value without a migration) and the client ID, and answers `{"access_token": "msl_…", "token_type": "Bearer"}` with `Cache-Control: no-store`. Every failure answers 400 with an RFC 6749 error: `invalid_grant` (unknown, used, expired or mismatched code, or a bad verifier), `invalid_request` or `unsupported_grant_type`. It shares the sign-in limiter.

### Endpoints inside `/api/v1`

Added to `api/openapi.yaml`, tag `oauth`. Both need a signed-in browser session (`sessionOnly`), so a token can't approve more tokens.

- **`GET /oauth/clients/{id}`** returns `{id, name, redirect_uris}` for the approval page, or 404.
- **`POST /oauth/approve`** takes `{client_id, redirect_uri, code_challenge, code_challenge_method, state, resource?, read_only, allow}`.
  - It checks that the client exists, `redirect_uri` is one of its URIs, `code_challenge_method` is `S256` and `code_challenge` is 43–128 characters, and `resource`, when given, is `<PUBLIC_URL>/mcp`. Otherwise it answers 422 and the page shows the error. It never redirects to a URI it hasn't matched.
  - With `allow: true` it stores a code (32 random bytes, base64url, hashed) for 10 minutes and answers `{"redirect_url": "<redirect_uri>?code=…&state=…"}`.
  - With `allow: false` it answers `{"redirect_url": "<redirect_uri>?error=access_denied&state=…"}`.
  - It audits `oauth approve` or `oauth deny` with the client ID.

### The MCP endpoint

- **Library:** the official Go SDK, `github.com/modelcontextprotocol/go-sdk` (MIT). It must pass `go run ./cmd/licenses`. `mcp.NewStreamableHTTPHandler` serves it with `Stateless: true` and JSON responses, so there is no session to keep.
- **Auth:** `/mcp` accepts only a bearer token, not a session cookie, so a web page can't drive it. A missing or invalid token answers 401 with the `WWW-Authenticate` header above. A valid one goes through the existing token check, rate limit and `read_only` rule.
- **Tools call the REST API in-process.** Each tool builds an `http.Request` to `/api/v1/...` with the caller's `Authorization` header and serves it through `s.Handler()` into an `httptest.ResponseRecorder`. So permissions, validation, the `If-Match` version checks, idempotency, audit (`via: "api"`) and the per-token rate limit are those of the API, with no second copy of any rule. A 2xx answer becomes the tool's result as JSON text. A problem answer becomes a tool error whose text carries the problem's `code`, `detail` and field errors, so the agent can correct itself. A `read_only` token gets `token_read_only` from the write tools that way.

### Tools

Each tool has a JSON input schema with descriptions written for an agent. Enums (`type`, `priority`, `sort`) match the OpenAPI ones.

| Tool | Input | Calls |
| --- | --- | --- |
| `get_project` | `project` | `GET /projects/{key}`, `/statuses`, `/nodes`, `/clients`, `/assignees`. Returns the project, its statuses with their categories, the menu tree as the flat node list `/nodes` gives (id, parent, name, type), the clients, and the assignees. Agents need it for the IDs the other tools take. |
| `list_tickets` | `project`; optional `q`, `status_id`, `open`, `type`, `client_id`, `assignee_id`, `mine`, `node_id`, `sort`, `limit` (default 50, max 200), `cursor` | `GET /projects/{key}/tickets` |
| `get_ticket` | `key` | `GET /tickets/{key}`. Returns the ticket and its `version` from the ETag. |
| `create_ticket` | `project`, `type`, `title`, `node_ids`; optional `client_id`, `reason`, `description`, `assignee_id`, `priority`, `due_date`, `status_id`, `idempotency_key` | `POST /projects/{key}/tickets` |
| `update_ticket` | `key` and any of `type`, `title`, `node_ids`, `client_id`, `requester_contact_id`, `requester_user_id`, `reason`, `description`, `assignee_id`, `priority`, `due_date`; optional `version` | `GET /tickets/{key}`, merge the given fields, then `PUT` with `If-Match`. Uses `version` instead of the ETag when given, so an agent that read the ticket earlier gets 412 if someone changed it since. |
| `transition_ticket` | `key`, `status_id`; optional `reason`, `node_ids`, `decision` {`what_changed`, `why`, `alternatives`} | `POST /tickets/{key}/transition`. Closing (Done or Cancelled) needs the reason, menus and decision, as in the API. |
| `cancel_ticket` | `key`, `reason`, `why`; optional `what_changed` (default "Cancelled; nothing changed."), `alternatives`, `node_ids` (default: the ticket's menus) | Finds the project's first status in category `cancelled`, then calls the transition. Errors with `no_cancelled_status` if the project has none. |

## Web

- **`web/app/oauth/authorize/page.tsx` (new).** A server component, shown in `AuthCard` like the login page.
  - Not signed in: redirect to `/login?next=<this URL>`.
  - It reads `client_id`, `redirect_uri`, `response_type`, `code_challenge`, `code_challenge_method`, `state` and `resource`. It loads the client with `GET /api/v1/oauth/clients/{id}`. If the client is unknown, the redirect URI isn't one of its URIs, `response_type` isn't `code` or the method isn't `S256`, it shows an error card and no buttons.
  - Otherwise it shows the client name, "will act as <name> (<email>)", what the agent can do (read what you can see; create, edit and close tickets unless read-only), the redirect host, a **Read only** checkbox (unticked), **Deny** and **Allow**.
  - A small client component posts to `/api/v1/oauth/approve` and sets `window.location` to the returned `redirect_url`.
- **Login `next`.** `login/page.tsx` and `LoginForm.tsx` take `?next=`. They follow it only when it is a path on this site: it starts with `/` and not `//` or `/\`. Otherwise they go to `/` as today.
- **Strings** in `messages/id.json` and `messages/en.json`, under `oauth`.
- **Settings › API tokens** needs no change: OAuth tokens appear there with their names.
- **`next.config.ts`** dev rewrites also send `/mcp`, `/oauth/register`, `/oauth/token` and `/.well-known/:path*` to `:8080`. `/oauth/authorize` stays with Next.js.

## Deploy

`deploy/Caddyfile` gets one `handle` block for the app paths, before the catch-all:

```
@app path /mcp /oauth/register /oauth/token /.well-known/oauth-*
handle @app {
	reverse_proxy app:8080
}
```

## Docs

- `docs/mcp.md` (new, in `mkdocs.yml` nav): connecting Claude Code, Claude Desktop and other clients, the tools, read-only tokens, revoking, and why there is no delete.
- `README.md`: one line under Connections.

## Tests

**Go**, in `oauth_test.go` and `mcp_test.go`, against a real PostgreSQL as the other handler tests are:

- Discovery documents carry PUBLIC_URL.
- Registration accepts loopback and https URIs, and rejects http on another host, fragments, and too many URIs.
- The whole flow: register, approve as a session user, exchange, then call `tools/list` and `list_tickets` with the token.
- Token errors: a wrong verifier, a reused code, an expired code, another client's code, and a mismatched redirect URI each give `invalid_grant`.
- Approve with a token instead of a session gives 403 `session_required`. An unmatched redirect URI gives 422.
- `/mcp` with no token gives 401 with `resource_metadata`. With a session cookie and no token it also gives 401.
- A read-only token can call `list_tickets` and `get_ticket`. `create_ticket` returns a tool error with `token_read_only`.
- Visibility: a member scoped to one client doesn't see another client's ticket through `list_tickets` or `get_ticket`.
- `update_ticket` changes only the given fields, and a stale `version` gives a tool error for 412.
- `cancel_ticket` moves the ticket to Cancelled with a decision record. Without `why` it fails validation.
- `permission_test.go` gets the two new `/api/v1/oauth` routes.

**Web:** `web/e2e/mcp-oauth.spec.ts` registers a client through the API, opens the authorize URL signed in, checks the client name, clicks Allow and checks the redirect carries `code` and `state`. A second case checks that Deny carries `error=access_denied`. `npm run check:i18n` covers the new strings.

## Risks

- **Phishing through registration.** Anyone can register a client named "Claude Code". The approval page therefore shows the redirect host next to the name, and nothing is issued without a signed-in user's click.
- **Long-lived tokens.** An agent's token works until revoked, like a personal token today. The token list shows its last use.
- **SDK churn.** The Go SDK is young. It is pinned in `go.mod`, and only `mcp.go` imports it.
