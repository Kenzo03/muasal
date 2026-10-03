# MCP Server for Tickets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let AI agents list, read, create, update, transition and cancel tickets through an MCP endpoint at `/mcp`, after the user signs in once through a Muasal approval page.

**Architecture:** Muasal becomes a small OAuth 2.1 authorization server (discovery, dynamic client registration, authorization code with PKCE) whose issued token is an ordinary `msl_…` API token. `/mcp` is a stateless streamable-HTTP MCP server (official Go SDK) whose tools call the existing `/api/v1` handlers in-process with the caller's token, so every rule of the REST API applies unchanged.

**Tech Stack:** Go 1.27, net/http ServeMux, sqlc, goose, oapi-codegen, `github.com/modelcontextprotocol/go-sdk`, Next.js 16 with next-intl, Playwright.

**Spec:** `docs/superpowers/specs/2026-10-03-mcp-tickets-design.md`

## Global Constraints

- Branch `feat/mcp`, from `main`. One commit per task; messages use `feat:`, `test:`, `docs:` and end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Code comments: short, matching the surrounding style; cite the spec as "MCP spec" where a rule comes from it.
- New dependency allowed: only `github.com/modelcontextprotocol/go-sdk`. It must pass `go run ./cmd/licenses` (allowed: Apache-2.0, MIT, BSD-2/3-Clause, ISC, MPL-2.0). Only `mcp.go` and `mcp_tools.go` import it.
- Issued tokens: name `<client name> (MCP)` cut to 100 characters, no expiry, `read_only` from the approval page.
- Codes: 32 random bytes base64url (`auth.NewToken()`), stored as SHA-256, valid 10 minutes, single-use, PKCE `S256` only, challenge 43–128 characters.
- Redirect URIs: 1–5 per client; `https://` anywhere, or `http://` on `localhost`, `127.0.0.1` or `[::1]`; no fragment.
- Register and token endpoints share the sign-in limiter `s.ipLimit` (20 a minute per IP).
- `/mcp` accepts only `Authorization: Bearer msl_…`; a 401 carries `WWW-Authenticate: Bearer resource_metadata="<PUBLIC_URL>/.well-known/oauth-protected-resource/mcp"`.
- The resource is `<PUBLIC_URL>/mcp`; a `resource` parameter, when sent, must equal it.
- Every new user-visible string goes in both `web/messages/id.json` and `web/messages/en.json`.
- Go tests need `TEST_DATABASE_URL` pointing at a PostgreSQL 18 superuser with pgvector (CI provides one; locally `make testdb`, or ask the user for a URL to their local PostgreSQL 18 on port 5433). Run tests from `server/`.

## File Structure

| File | Responsibility |
| --- | --- |
| `server/migrations/00020_oauth.sql` (new) | `oauth_clients`, `oauth_codes` |
| `server/internal/db/queries/oauth.sql` (new) | sqlc queries for both tables |
| `server/internal/httpapi/oauth.go` (new) | Discovery, register, token, and the two `/api/v1/oauth` handlers |
| `server/internal/httpapi/mcp.go` (new) | `/mcp` auth wrapper, SDK handler, in-process API caller |
| `server/internal/httpapi/mcp_tools.go` (new) | The seven tools |
| `server/internal/httpapi/server.go` | Route registration in `Handler()` |
| `server/internal/indexer/purge.go` | Daily purge of old codes |
| `api/openapi.yaml` | `GET /oauth/clients/{id}`, `POST /oauth/approve` |
| `server/internal/httpapi/oauth_test.go`, `mcp_test.go` (new) | Go integration tests |
| `server/internal/httpapi/permission_test.go` | Rows for the new routes |
| `web/app/oauth/authorize/page.tsx`, `Approve.tsx` (new) | Approval page |
| `web/app/login/page.tsx`, `LoginForm.tsx`, `web/proxy.ts` | `?next=` after sign-in |
| `web/next.config.ts` | Dev rewrites |
| `web/messages/*.json` | `oauth` strings |
| `web/e2e/mcp-oauth.spec.ts` (new), `web/e2e/global-setup.ts` | Browser test |
| `deploy/Caddyfile`, `docs/mcp.md` (new), `mkdocs.yml`, `README.md` | Deploy and docs |

---

### Task 1: OAuth tables, discovery and client registration

**Files:**
- Create: `server/migrations/00020_oauth.sql`, `server/internal/db/queries/oauth.sql`, `server/internal/httpapi/oauth.go`, `server/internal/httpapi/oauth_test.go`
- Modify: `server/internal/httpapi/server.go` (`Handler()`, around line 94)

**Interfaces:**
- Produces: tables `oauth_clients(id, name, redirect_uris, created_at)` and `oauth_codes(code_hash, client_id, user_id, redirect_uri, code_challenge, read_only, expires_at, used_at)`; sqlc methods `CreateOAuthClient`, `GetOAuthClient`, `CreateOAuthCode`, `UseOAuthCode`, `PurgeOAuthCodes` (check the generated struct names in `internal/db/oauth.sql.go`; sqlc names the row type `OauthClient` and `OauthCode`); `func (s *Server) mcpResource() string`; `func validRedirectURI(raw string) bool`; `func writeOAuthError(w http.ResponseWriter, status int, code, desc string)`; routes `GET /.well-known/oauth-protected-resource`, `GET /.well-known/oauth-protected-resource/mcp`, `GET /.well-known/oauth-authorization-server`, `POST /oauth/register`.

- [ ] **Step 1: Write the migration**

`server/migrations/00020_oauth.sql`:

```sql
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
```

- [ ] **Step 2: Write the queries**

`server/internal/db/queries/oauth.sql`:

```sql
-- MCP sign-in: OAuth clients and authorization codes (MCP spec).

-- name: CreateOAuthClient :one
INSERT INTO oauth_clients (id, name, redirect_uris)
VALUES (sqlc.arg('id'), sqlc.arg('name'), sqlc.arg('redirect_uris'))
RETURNING *;

-- name: GetOAuthClient :one
SELECT * FROM oauth_clients WHERE id = $1;

-- name: CreateOAuthCode :exec
INSERT INTO oauth_codes (code_hash, client_id, user_id, redirect_uri, code_challenge, read_only, expires_at)
VALUES (sqlc.arg('code_hash'), sqlc.arg('client_id'), sqlc.arg('user_id'), sqlc.arg('redirect_uri'),
        sqlc.arg('code_challenge'), sqlc.arg('read_only'), sqlc.arg('expires_at'));

-- name: UseOAuthCode :one
-- Marks a live code used and returns it; a used or expired code returns no row.
UPDATE oauth_codes SET used_at = now()
WHERE code_hash = $1 AND used_at IS NULL AND expires_at > now()
RETURNING *;

-- name: PurgeOAuthCodes :execrows
DELETE FROM oauth_codes WHERE expires_at < now() - interval '1 day';
```

- [ ] **Step 3: Generate**

Run: `cd server && go generate ./internal/db/...`
Expected: `internal/db/oauth.sql.go` appears; `go build ./...` passes.

- [ ] **Step 4: Write the failing tests**

`server/internal/httpapi/oauth_test.go`:

```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// rawJSON posts JSON to a path outside /api/v1 and decodes the reply.
func (e *env) rawJSON(method, path string, body, out any) int {
	e.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, e.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

// MCP spec: discovery documents point clients at this server's endpoints.
func TestOAuthDiscovery(t *testing.T) {
	e := newEnv(t)
	var pr map[string]any
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		if code := e.rawJSON(http.MethodGet, path, nil, &pr); code != http.StatusOK || pr["resource"] != origin+"/mcp" {
			t.Fatalf("%s: %d %v", path, code, pr)
		}
	}
	var as map[string]any
	if code := e.rawJSON(http.MethodGet, "/.well-known/oauth-authorization-server", nil, &as); code != http.StatusOK ||
		as["issuer"] != origin || as["authorization_endpoint"] != origin+"/oauth/authorize" ||
		as["token_endpoint"] != origin+"/oauth/token" || as["registration_endpoint"] != origin+"/oauth/register" {
		t.Fatalf("metadata: %d %v", code, as)
	}
}

// MCP spec: dynamic registration takes public clients with safe redirect URIs.
func TestOAuthRegistration(t *testing.T) {
	e := newEnv(t)
	var c map[string]any
	if code := e.rawJSON(http.MethodPost, "/oauth/register", map[string]any{
		"client_name": "Claude Code", "redirect_uris": []string{"http://localhost:53682/callback", "https://agent.example.com/cb"},
		"grant_types": []string{"authorization_code"}, // extra fields are ignored
	}, &c); code != http.StatusCreated || c["client_id"] == "" || c["client_name"] != "Claude Code" || c["token_endpoint_auth_method"] != "none" {
		t.Fatalf("register: %d %v", code, c)
	}
	if code := e.rawJSON(http.MethodPost, "/oauth/register", map[string]any{"redirect_uris": []string{"http://127.0.0.1:9/cb"}}, &c); code != http.StatusCreated || c["client_name"] != "MCP client" {
		t.Fatalf("unnamed: %d %v", code, c)
	}
	for _, uris := range [][]string{
		{},
		{"http://evil.example.com/cb"},
		{"https://agent.example.com/cb#frag"},
		{"ftp://localhost/cb"},
		{"https://a.example/1", "https://a.example/2", "https://a.example/3", "https://a.example/4", "https://a.example/5", "https://a.example/6"},
	} {
		var bad map[string]any
		if code := e.rawJSON(http.MethodPost, "/oauth/register", map[string]any{"client_name": "X", "redirect_uris": uris}, &bad); code != http.StatusBadRequest || bad["error"] != "invalid_redirect_uri" {
			t.Errorf("%v: %d %v", uris, code, bad)
		}
	}
}
```

- [ ] **Step 5: Run the tests to see them fail**

Run: `cd server && go test ./internal/httpapi -run 'TestOAuthDiscovery|TestOAuthRegistration' -count=1`
Expected: FAIL (404 from the unknown routes).

- [ ] **Step 6: Write `oauth.go`**

```go
package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/kenzo03/muasal/server/internal/db"
)

// MCP sign-in (MCP spec): Muasal is the authorization server for its own
// /mcp endpoint. Clients register themselves, a signed-in user approves them on
// /oauth/authorize, and the code they get back buys an ordinary API token.

// mcpResource is the protected resource the tokens are for.
func (s *Server) mcpResource() string { return s.cfg.PublicURL + "/mcp" }

// protectedResource is RFC 9728's metadata.
func (s *Server) protectedResource(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 s.mcpResource(),
		"authorization_servers":    []string{s.cfg.PublicURL},
		"bearer_methods_supported": []string{"header"},
	})
}

// authServerMetadata is RFC 8414's metadata.
func (s *Server) authServerMetadata(w http.ResponseWriter, _ *http.Request) {
	u := s.cfg.PublicURL
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                u,
		"authorization_endpoint":                u + "/oauth/authorize",
		"token_endpoint":                        u + "/oauth/token",
		"registration_endpoint":                 u + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
	})
}

// writeOAuthError answers in RFC 6749's error format, which OAuth clients parse.
func writeOAuthError(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

// validRedirectURI allows https anywhere and http only on the loopback
// address, where native clients listen (RFC 8252). No fragments.
func validRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || strings.Contains(raw, "#") {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		h := u.Hostname()
		return h == "localhost" || h == "127.0.0.1" || h == "::1"
	}
	return false
}

// registerClient is RFC 7591 dynamic registration for public clients.
// Registering grants nothing: a signed-in user still has to approve.
func (s *Server) registerClient(w http.ResponseWriter, r *http.Request) {
	if !s.ipLimit.Allow(clientIP(r)) {
		writeOAuthError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests from this address; wait a minute")
		return
	}
	var in struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	// Clients send more metadata than this; unknown fields are ignored.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "Send the client metadata as JSON")
		return
	}
	name := strings.TrimSpace(in.ClientName)
	if name == "" {
		name = "MCP client"
	}
	if utf8.RuneCountInString(name) > 100 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "client_name takes at most 100 characters")
		return
	}
	if len(in.RedirectURIs) < 1 || len(in.RedirectURIs) > 5 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "Give 1 to 5 redirect URIs")
		return
	}
	for _, u := range in.RedirectURIs {
		if !validRedirectURI(u) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "Redirect URIs use https, or http on localhost, with no fragment")
			return
		}
	}
	c, err := s.q.CreateOAuthClient(r.Context(), db.CreateOAuthClientParams{ID: rand.Text(), Name: name, RedirectUris: in.RedirectURIs})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id": c.ID, "client_name": c.Name, "redirect_uris": c.RedirectUris,
		"token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code"}, "response_types": []string{"code"},
	})
}
```

In `server.go` `Handler()`, after the `/webhooks` line:

```go
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.protectedResource)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", s.protectedResource)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.authServerMetadata)
	mux.HandleFunc("POST /oauth/register", s.registerClient)
```

If the generated params field is named differently (e.g. `RedirectURIs`), use the generated name.

- [ ] **Step 7: Run the tests to see them pass**

Run: `cd server && go test ./internal/httpapi -run 'TestOAuthDiscovery|TestOAuthRegistration' -count=1`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add server/migrations/00020_oauth.sql server/internal/db/queries/oauth.sql server/internal/db/oauth.sql.go server/internal/db/models.go server/internal/httpapi/oauth.go server/internal/httpapi/oauth_test.go server/internal/httpapi/server.go
git commit -m "feat(oauth): discovery and client registration for MCP sign-in"
```

---

### Task 2: Approval API

**Files:**
- Modify: `api/openapi.yaml` (paths after `/me/tokens/{id}`, around line 213; schemas near `APITokenCreate`, around line 2939), `server/internal/httpapi/oauth.go`, `server/internal/httpapi/oauth_test.go`, `server/internal/httpapi/permission_test.go`
- Regenerate: `server/internal/httpapi/api.gen.go`, `web/lib/api-types.ts`

**Interfaces:**
- Consumes: `GetOAuthClient`, `CreateOAuthCode`, `mcpResource()`, `sessionOnly` (tokens.go), `auth.NewToken()`.
- Produces: `func (s *Server) GetOAuthClient(w, r, id string)` and `func (s *Server) ApproveOAuth(w, r)` (generated interface names — check `api.gen.go`); generated types `OAuthClient{Id, Name string; RedirectUris []string}`, `OAuthApprove{...}`, `OAuthRedirect{RedirectUrl string}`; const `codeTTL = 10 * time.Minute`; test helpers `registerTestClient(e *env, redirect string) string` and `approve(e *env, c *http.Client, body map[string]any) (int, string)`.

- [ ] **Step 1: Add the OpenAPI paths and schemas**

Paths:

```yaml
  /oauth/clients/{id}:
    parameters:
      - { name: id, in: path, required: true, schema: { type: string } }
    get:
      operationId: getOAuthClient
      tags: [oauth]
      description: A registered MCP client, for the approval page. Needs a signed-in browser session (MCP spec).
      responses:
        "200":
          description: The client.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/OAuthClient" }
        default: { $ref: "#/components/responses/Problem" }
  /oauth/approve:
    post:
      operationId: approveOAuth
      tags: [oauth]
      description: >
        The signed-in user allows or denies an MCP client. Allow stores a one-time
        code for 10 minutes. The answer is where the browser goes next. Needs a
        browser session: a token can't approve more tokens.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/OAuthApprove" }
      responses:
        "200":
          description: The client's redirect URI with a code, or with error=access_denied.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/OAuthRedirect" }
        default: { $ref: "#/components/responses/Problem" }
```

Schemas:

```yaml
    OAuthClient:
      type: object
      required: [id, name, redirect_uris]
      properties:
        id: { type: string }
        name: { type: string }
        redirect_uris: { type: array, items: { type: string } }
    OAuthApprove:
      type: object
      required: [client_id, redirect_uri, code_challenge, code_challenge_method, read_only, allow]
      properties:
        client_id: { type: string }
        redirect_uri: { type: string }
        code_challenge: { type: string }
        code_challenge_method: { type: string }
        state: { type: string }
        resource: { type: string }
        read_only: { type: boolean }
        allow: { type: boolean }
    OAuthRedirect:
      type: object
      required: [redirect_url]
      properties:
        redirect_url: { type: string }
```

- [ ] **Step 2: Generate**

Run: `cd server && go generate ./internal/httpapi/... && cd ../web && npm run gen:api`
Expected: `go build ./...` now fails with "Server does not implement ServerInterface (missing method ApproveOAuth)" — that is expected until Step 5.

- [ ] **Step 3: Write the failing tests**

Append to `oauth_test.go` (add imports `net/url`, `strings`, `github.com/kenzo03/muasal/server/internal/httpapi`):

```go
// A 43-character PKCE verifier and its S256 challenge (RFC 7636 appendix B).
const (
	verifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
)

func registerTestClient(e *env, redirect string) string {
	e.t.Helper()
	var c map[string]any
	if code := e.rawJSON(http.MethodPost, "/oauth/register", map[string]any{"client_name": "Test Agent", "redirect_uris": []string{redirect}}, &c); code != http.StatusCreated {
		e.t.Fatalf("register: %d %v", code, c)
	}
	return c["client_id"].(string)
}

// approve posts the approval page's request as a signed-in browser and returns the redirect URL.
func approve(e *env, c *http.Client, body map[string]any) (int, string) {
	e.t.Helper()
	var out httpapi.OAuthRedirect
	code := e.call(c, http.MethodPost, "/oauth/approve", body, &out)
	return code, out.RedirectUrl
}

func approveBody(clientID, redirect string, allow bool) map[string]any {
	return map[string]any{"client_id": clientID, "redirect_uri": redirect, "code_challenge": challenge,
		"code_challenge_method": "S256", "state": "st-1", "resource": origin + "/mcp", "read_only": false, "allow": allow}
}

// MCP spec: only a signed-in session approves, only to a registered URI.
func TestOAuthApprove(t *testing.T) {
	e := newEnv(t)
	redirect := "http://localhost:53682/callback"
	id := registerTestClient(e, redirect)
	pm, _ := e.signedIn("pm@example.com", false)

	var got httpapi.OAuthClient
	if code := e.call(pm, http.MethodGet, "/oauth/clients/"+id, nil, &got); code != http.StatusOK || got.Name != "Test Agent" {
		t.Fatalf("client: %d %+v", code, got)
	}
	if code := e.call(pm, http.MethodGet, "/oauth/clients/NOPE", nil, nil); code != http.StatusNotFound {
		t.Fatalf("unknown client: %d", code)
	}

	code, to := approve(e, pm, approveBody(id, redirect, true))
	u, _ := url.Parse(to)
	if code != http.StatusOK || !strings.HasPrefix(to, redirect+"?") || u.Query().Get("code") == "" || u.Query().Get("state") != "st-1" {
		t.Fatalf("allow: %d %q", code, to)
	}
	if code, to := approve(e, pm, approveBody(id, redirect, false)); code != http.StatusOK || to != redirect+"?error=access_denied&state=st-1" {
		t.Fatalf("deny: %d %q", code, to)
	}
	for name, change := range map[string]func(map[string]any){
		"other redirect": func(b map[string]any) { b["redirect_uri"] = "http://localhost:1/evil" },
		"plain method":   func(b map[string]any) { b["code_challenge_method"] = "plain" },
		"short challenge": func(b map[string]any) { b["code_challenge"] = "abc" },
		"other resource": func(b map[string]any) { b["resource"] = "https://elsewhere.example/mcp" },
		"unknown client": func(b map[string]any) { b["client_id"] = "NOPE" },
	} {
		b := approveBody(id, redirect, true)
		change(b)
		if code, to := approve(e, pm, b); code != http.StatusUnprocessableEntity || to != "" {
			t.Errorf("%s: %d %q", name, code, to)
		}
	}

	// A token can't approve more tokens.
	var tok httpapi.APITokenCreated
	e.call(pm, http.MethodPost, "/me/tokens", map[string]any{"name": "Script", "read_only": false}, &tok)
	var p httpapi.Problem
	if code := e.bearer(tok.Token, http.MethodPost, "/oauth/approve", approveBody(id, redirect, true), &p); code != http.StatusForbidden || p.Code != "session_required" {
		t.Fatalf("approve with a token: %d %+v", code, p)
	}
}
```

- [ ] **Step 4: Run to see it fail**

Run: `cd server && go test ./internal/httpapi -run TestOAuthApprove -count=1`
Expected: build failure (missing methods) — the test cannot pass before Step 5.

- [ ] **Step 5: Implement the handlers**

Append to `oauth.go` (add imports `errors`, `time`, `github.com/jackc/pgx/v5`, `github.com/kenzo03/muasal/server/internal/auth`):

```go
// codeTTL is how long an approval's code may be exchanged.
const codeTTL = 10 * time.Minute

// GetOAuthClient shows the approval page which client is asking.
func (s *Server) GetOAuthClient(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.sessionOnly(w, r); !ok {
		return
	}
	c, err := s.q.GetOAuthClient(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "This app is not registered")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, OAuthClient{Id: c.ID, Name: c.Name, RedirectUris: c.RedirectUris})
}

// ApproveOAuth records the signed-in user's answer. It redirects only to a
// URI the client registered, so a bad request never leaves Muasal.
func (s *Server) ApproveOAuth(w http.ResponseWriter, r *http.Request) {
	u, ok := s.sessionOnly(w, r)
	if !ok {
		return
	}
	var in OAuthApprove
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	c, err := s.q.GetOAuthClient(ctx, in.ClientId)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.fail(w, r, err)
		return
	}
	invalid := func(msg string) {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_authorization_request", msg)
	}
	switch {
	case err != nil:
		invalid("This app is not registered")
		return
	case !slices.Contains(c.RedirectUris, in.RedirectUri):
		invalid("The app asked to return to an address it did not register")
		return
	case in.CodeChallengeMethod != "S256" || len(in.CodeChallenge) < 43 || len(in.CodeChallenge) > 128:
		invalid("The app must use PKCE with S256")
		return
	case in.Resource != nil && *in.Resource != "" && *in.Resource != s.mcpResource():
		invalid("The app asked for access to another server")
		return
	}
	q := url.Values{}
	if in.State != nil && *in.State != "" {
		q.Set("state", *in.State)
	}
	action := "deny"
	if in.Allow {
		code, hash := auth.NewToken()
		if err := s.q.CreateOAuthCode(ctx, db.CreateOAuthCodeParams{
			CodeHash: hash, ClientID: c.ID, UserID: u.ID, RedirectUri: in.RedirectUri,
			CodeChallenge: in.CodeChallenge, ReadOnly: in.ReadOnly, ExpiresAt: s.now().Add(codeTTL),
		}); err != nil {
			s.fail(w, r, err)
			return
		}
		q.Set("code", code)
		action = "approve"
	} else {
		q.Set("error", "access_denied")
	}
	if err := audit(ctx, s.q, webMeta(r), &u.ID, "oauth_client", 0, action, map[string]any{"client_id": c.ID, "name": c.Name, "read_only": in.ReadOnly}); err != nil {
		s.fail(w, r, err)
		return
	}
	sep := "?"
	if strings.Contains(in.RedirectUri, "?") {
		sep = "&"
	}
	writeJSON(w, http.StatusOK, OAuthRedirect{RedirectUrl: in.RedirectUri + sep + encodeQuery(q)})
}

// encodeQuery keeps code (or error) before state, as clients log it.
func encodeQuery(q url.Values) string {
	var parts []string
	for _, k := range []string{"code", "error", "state"} {
		if v := q.Get(k); v != "" {
			parts = append(parts, k+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}
```

Add `"slices"` to the imports. Check the generated field types for `State` and `Resource` in `api.gen.go`: optional strings are `*string`. If the audit query rejects entity `oauth_client` (check `InsertAuditEvent` and any CHECK on `audit_events.entity` in `migrations/00001_init.sql`), use entity `token` with entityID 0 instead, and say so in the commit message.

- [ ] **Step 6: Add the permission rows**

In `permission_test.go`, `TestPermissionSuiteWrites`, add before the "Allowed, as controls" comment:

```go
		{"ani", http.MethodPost, "/oauth/approve", nil, map[string]any{"client_id": "NOPE", "redirect_uri": "http://localhost:1/cb",
			"code_challenge": strings.Repeat("a", 43), "code_challenge_method": "S256", "read_only": true, "allow": true}, 422},
```

and, in the same table (it takes any method), `{"ani", http.MethodGet, "/oauth/clients/NOPE", nil, nil, 404}`. Add `"strings"` to its imports if missing.

- [ ] **Step 7: Run the tests**

Run: `cd server && go test ./internal/httpapi -run 'TestOAuth|TestPermissionSuite' -count=1`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add api/openapi.yaml server/internal/httpapi/api.gen.go web/lib/api-types.ts server/internal/httpapi/oauth.go server/internal/httpapi/oauth_test.go server/internal/httpapi/permission_test.go
git commit -m "feat(oauth): approval API for MCP clients"
```

---

### Task 3: Token endpoint and code purge

**Files:**
- Modify: `server/internal/httpapi/oauth.go`, `server/internal/httpapi/server.go`, `server/internal/httpapi/oauth_test.go`, `server/internal/indexer/purge.go:52`

**Interfaces:**
- Consumes: `UseOAuthCode`, `GetOAuthClient`, `CreateAPIToken`, `tokenPrefix`, `approve`, `approveBody`, `registerTestClient`, `verifier`.
- Produces: route `POST /oauth/token`; test helper `exchange(e *env, form url.Values) (int, map[string]any)` and `mcpToken(e *env, c *http.Client, readOnly bool) string` (full sign-in, returns an `msl_…` token).

- [ ] **Step 1: Write the failing tests**

Append to `oauth_test.go`:

```go
func exchange(e *env, form url.Values) (int, map[string]any) {
	e.t.Helper()
	res, err := http.PostForm(e.url+"/oauth/token", form)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func codeFrom(e *env, to string) string {
	u, err := url.Parse(to)
	if err != nil || u.Query().Get("code") == "" {
		e.t.Fatalf("no code in %q", to)
	}
	return u.Query().Get("code")
}

func tokenForm(id, redirect, code string) url.Values {
	return url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect},
		"client_id": {id}, "code_verifier": {verifier}, "resource": {origin + "/mcp"}}
}

// mcpToken runs the whole sign-in as c's user and returns the token.
func mcpToken(e *env, c *http.Client, readOnly bool) string {
	e.t.Helper()
	redirect := "http://127.0.0.1:7777/cb"
	id := registerTestClient(e, redirect)
	b := approveBody(id, redirect, true)
	b["read_only"] = readOnly
	_, to := approve(e, c, b)
	code, out := exchange(e, tokenForm(id, redirect, codeFrom(e, to)))
	if code != http.StatusOK {
		e.t.Fatalf("exchange: %d %v", code, out)
	}
	return out["access_token"].(string)
}

// MCP spec: a code buys one API token, once, with the right verifier.
func TestOAuthTokenExchange(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	redirect := "http://localhost:53682/callback"
	id := registerTestClient(e, redirect)

	_, to := approve(e, w.pm, approveBody(id, redirect, true))
	code := codeFrom(e, to)
	status, out := exchange(e, tokenForm(id, redirect, code))
	tok, _ := out["access_token"].(string)
	if status != http.StatusOK || !strings.HasPrefix(tok, "msl_") || out["token_type"] != "Bearer" {
		t.Fatalf("exchange: %d %v", status, out)
	}
	var page httpapi.TicketPage
	if s := e.bearer(tok, http.MethodGet, "/projects/HRIS/tickets", nil, &page); s != http.StatusOK {
		t.Fatalf("token works: %d", s)
	}
	var list httpapi.APITokenList
	e.call(w.pm, http.MethodGet, "/me/tokens", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Name != "Test Agent (MCP)" || list.Items[0].ReadOnly {
		t.Fatalf("token list: %+v", list.Items)
	}
	if s, out := exchange(e, tokenForm(id, redirect, code)); s != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("reused code: %d %v", s, out)
	}

	fresh := func() string { _, to := approve(e, w.pm, approveBody(id, redirect, true)); return codeFrom(e, to) }
	other := registerTestClient(e, redirect)
	for name, form := range map[string]url.Values{
		"wrong verifier": func() url.Values { f := tokenForm(id, redirect, fresh()); f.Set("code_verifier", strings.Repeat("x", 43)); return f }(),
		"other client":   tokenForm(other, redirect, fresh()),
		"other redirect": tokenForm(id, "http://localhost:1/cb", fresh()),
		"unknown code":   tokenForm(id, redirect, "nope"),
	} {
		if s, out := exchange(e, form); s != http.StatusBadRequest || out["error"] != "invalid_grant" {
			t.Errorf("%s: %d %v", name, s, out)
		}
	}
	expired := fresh()
	e.exec("UPDATE oauth_codes SET expires_at = now() - interval '1 second' WHERE used_at IS NULL")
	if s, out := exchange(e, tokenForm(id, redirect, expired)); s != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("expired: %d %v", s, out)
	}
	if s, out := exchange(e, url.Values{"grant_type": {"client_credentials"}}); s != http.StatusBadRequest || out["error"] != "unsupported_grant_type" {
		t.Fatalf("grant type: %d %v", s, out)
	}
	if s, out := exchange(e, func() url.Values { f := tokenForm(id, redirect, fresh()); f.Set("resource", "https://x.example/mcp"); return f }()); s != http.StatusBadRequest || out["error"] != "invalid_target" {
		t.Fatalf("resource: %d %v", s, out)
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `cd server && go test ./internal/httpapi -run TestOAuthTokenExchange -count=1`
Expected: FAIL (`/oauth/token` 404/405).

- [ ] **Step 3: Implement**

Append to `oauth.go` (imports `crypto/sha256`, `crypto/subtle`, `encoding/base64`):

```go
var errInvalidGrant = errors.New("invalid grant")

// exchangeCode is the token endpoint. The code is spent before anything is
// checked, so a wrong verifier can't be retried against the same code.
func (s *Server) exchangeCode(w http.ResponseWriter, r *http.Request) {
	if !s.ipLimit.Allow(clientIP(r)) {
		writeOAuthError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests from this address; wait a minute")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Send a form body")
		return
	}
	f := r.PostForm
	if f.Get("grant_type") != "authorization_code" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "Only authorization_code is supported")
		return
	}
	code, verifier, clientID, redirect := f.Get("code"), f.Get("code_verifier"), f.Get("client_id"), f.Get("redirect_uri")
	if code == "" || verifier == "" || clientID == "" || redirect == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "code, code_verifier, client_id and redirect_uri are required")
		return
	}
	if res := f.Get("resource"); res != "" && res != s.mcpResource() {
		writeOAuthError(w, http.StatusBadRequest, "invalid_target", "Tokens are only for "+s.mcpResource())
		return
	}
	ctx := r.Context()
	c, err := s.q.UseOAuthCode(ctx, auth.HashToken(code))
	if errors.Is(err, pgx.ErrNoRows) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "The code is unknown, used or expired")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sum := sha256.Sum256([]byte(verifier))
	if c.ClientID != clientID || c.RedirectUri != redirect ||
		subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(c.CodeChallenge)) != 1 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "The code does not match this client, redirect URI or verifier")
		return
	}
	secret := tokenPrefix + rand.Text() + rand.Text()
	err = s.inTx(ctx, func(q *db.Queries) error {
		client, err := q.GetOAuthClient(ctx, c.ClientID)
		if err != nil {
			return err
		}
		name := client.Name + " (MCP)"
		if rs := []rune(name); len(rs) > 100 {
			name = string(rs[:100])
		}
		t, err := q.CreateAPIToken(ctx, db.CreateAPITokenParams{UserID: c.UserID, Name: name, TokenHash: auth.HashToken(secret), ReadOnly: c.ReadOnly})
		if err != nil {
			return err
		}
		// The user approved this in the browser; audit_events.via has no "oauth".
		m := auditMeta{via: "web", requestID: ptr(requestIDFrom(ctx)), ip: ipAddr(r)}
		return audit(ctx, q, m, &c.UserID, "token", t.ID, "create", map[string]any{"name": name, "read_only": c.ReadOnly, "client_id": client.ID})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"access_token": secret, "token_type": "Bearer"})
}
```

Delete `errInvalidGrant` if unused after writing (the code above answers inline). In `server.go` `Handler()` add `mux.HandleFunc("POST /oauth/token", s.exchangeCode)`. `requestContext` already sets `Cache-Control: no-store`.

In `server/internal/indexer/purge.go`, after the `PurgeIdempotencyKeys` call:

```go
	if _, err := q.PurgeOAuthCodes(ctx); err != nil { // MCP sign-in codes, a day past expiry
		return err
	}
```

Update the `PurgeAsk` doc comment to say it also drops old sign-in codes.

- [ ] **Step 4: Run the tests**

Run: `cd server && go test ./internal/httpapi -run TestOAuth -count=1 && go test ./internal/indexer -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/httpapi/oauth.go server/internal/httpapi/oauth_test.go server/internal/httpapi/server.go server/internal/indexer/purge.go
git commit -m "feat(oauth): exchange a code for an API token"
```

---

### Task 4: The `/mcp` endpoint and read tools

**Files:**
- Create: `server/internal/httpapi/mcp.go`, `server/internal/httpapi/mcp_tools.go`, `server/internal/httpapi/mcp_test.go`
- Modify: `server/go.mod`, `server/go.sum`, `server/internal/httpapi/server.go`

**Interfaces:**
- Consumes: `tokenUser`, `mcpToken` (test helper), `Handler()`.
- Produces: `type apiCaller struct{...}` with `func (c *apiCaller) call(ctx context.Context, method, path string, headers map[string]string, body, out any) (http.Header, error)`; `func jsonResult(v any) (*mcp.CallToolResult, any, error)`; `func (s *Server) mcpServer(c *apiCaller) *mcp.Server`; test helper `mcpCall(e *env, token, tool string, args map[string]any) (text string, isError bool)`.

- [ ] **Step 1: Add the SDK**

Run: `cd server && go get github.com/modelcontextprotocol/go-sdk@latest && go mod tidy && go run ./cmd/licenses`
Expected: the licence scan passes. If it fails on a transitive dependency, stop and report to the user.

- [ ] **Step 2: Write the failing tests**

`server/internal/httpapi/mcp_test.go`:

```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mcpPost sends one JSON-RPC message to /mcp.
func mcpPost(e *env, token string, msg map[string]any) (*http.Response, []byte) {
	e.t.Helper()
	b, _ := json.Marshal(msg)
	req, _ := http.NewRequest(http.MethodPost, e.url+"/mcp", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res, body
}

// mcpCall calls one tool and returns its text and whether it is an error.
func mcpCall(e *env, token, tool string, args map[string]any) (string, bool) {
	e.t.Helper()
	res, body := mcpPost(e, token, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args}})
	var out struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
			IsError bool                     `json:"isError"`
		} `json:"result"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil || res.StatusCode != http.StatusOK || out.Error != nil || len(out.Result.Content) == 0 {
		e.t.Fatalf("%s: %d %s", tool, res.StatusCode, body)
	}
	return out.Result.Content[0].Text, out.Result.IsError
}

// MCP spec: /mcp asks for sign-in, and takes only a token.
func TestMCPNeedsAToken(t *testing.T) {
	e := newEnv(t)
	hello := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}}
	res, _ := mcpPost(e, "", hello)
	if res.StatusCode != http.StatusUnauthorized ||
		res.Header.Get("WWW-Authenticate") != `Bearer resource_metadata="`+origin+`/.well-known/oauth-protected-resource/mcp"` {
		t.Fatalf("no token: %d %q", res.StatusCode, res.Header.Get("WWW-Authenticate"))
	}
	if res, _ := mcpPost(e, "msl_nope", hello); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", res.StatusCode)
	}
	// A browser session is not enough: a web page must not drive the tools.
	pm, _ := e.signedIn("pm@example.com", false)
	b, _ := json.Marshal(hello)
	req, _ := http.NewRequest(http.MethodPost, e.url+"/mcp", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if r, err := pm.Do(req); err != nil || r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("session: %v %d", err, r.StatusCode)
	}
}

// The read tools see what the token's user sees, no more.
func TestMCPReadTools(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	e.seedTicket(w.p, w.pmUser, "Client A request", &w.a, w.ot)
	e.seedTicket(w.p, w.pmUser, "Client B request", &w.b, w.secret)
	tok := mcpToken(e, w.pm, true)

	res, body := mcpPost(e, tok, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	for _, name := range []string{"get_project", "list_tickets", "get_ticket"} {
		if res.StatusCode != http.StatusOK || !strings.Contains(string(body), `"`+name+`"`) {
			t.Fatalf("tools/list lacks %s: %d %s", name, res.StatusCode, body)
		}
	}
	text, isErr := mcpCall(e, tok, "list_tickets", map[string]any{"project": "HRIS"})
	if isErr || !strings.Contains(text, "Client A request") || strings.Contains(text, "Client B request") {
		t.Fatalf("list_tickets: %v %s", isErr, text)
	}
	if text, isErr := mcpCall(e, tok, "get_ticket", map[string]any{"key": "HRIS-1"}); isErr || !strings.Contains(text, `"version":1`) {
		t.Fatalf("get_ticket: %v %s", isErr, text)
	}
	if text, isErr := mcpCall(e, tok, "get_ticket", map[string]any{"key": "HRIS-2"}); !isErr || !strings.Contains(text, "404") {
		t.Fatalf("hidden ticket: %v %s", isErr, text)
	}
	text, isErr = mcpCall(e, tok, "get_project", map[string]any{"project": "HRIS"})
	if isErr || !strings.Contains(text, `"statuses"`) || !strings.Contains(text, "Overtime Approval") || strings.Contains(text, "Client B Report") || !strings.Contains(text, `"cancelled"`) {
		t.Fatalf("get_project: %v %s", isErr, text)
	}
}
```

- [ ] **Step 3: Run to see it fail**

Run: `cd server && go test ./internal/httpapi -run TestMCP -count=1`
Expected: FAIL (`/mcp` 404).

- [ ] **Step 4: Write `mcp.go`**

```go
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpHandler serves POST /mcp (MCP spec): a stateless MCP server whose tools
// call the REST API through root with the caller's token, so every API rule
// (visibility, validation, versions, audit, rate limit) applies unchanged.
func (s *Server) mcpHandler(root http.Handler) http.Handler {
	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer(&apiCaller{root: root, auth: r.Header.Get("Authorization"), remote: r.RemoteAddr, xff: r.Header.Get("X-Forwarded-For")})
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only a token: a session cookie would let any web page drive the tools.
		bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			s.mcpUnauthorized(w)
			return
		}
		if _, u := s.tokenUser(r.Context(), strings.TrimSpace(bearer)); u == nil {
			s.mcpUnauthorized(w)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// mcpUnauthorized points the client at the sign-in metadata (RFC 9728).
func (s *Server) mcpUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+s.cfg.PublicURL+`/.well-known/oauth-protected-resource/mcp"`)
	writeProblem(w, http.StatusUnauthorized, "invalid_token", "Sign in to Muasal to use this endpoint")
}

// apiCaller sends a tool's requests to /api/v1 in-process, as the MCP caller.
type apiCaller struct {
	root              http.Handler
	auth, remote, xff string
}

// call decodes a 2xx JSON answer into out. Any other answer becomes an error
// carrying the problem's status, code, title and field errors, which the SDK
// returns to the agent as a tool error.
func (c *apiCaller) call(ctx context.Context, method, path string, headers map[string]string, body, out any) (http.Header, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://muasal.internal/api/v1"+path, rd)
	if err != nil {
		return nil, err
	}
	req.RemoteAddr = c.remote
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Content-Type", "application/json")
	if c.xff != "" {
		req.Header.Set("X-Forwarded-For", c.xff)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	c.root.ServeHTTP(rec, req)
	if rec.Code < 200 || rec.Code > 299 {
		var p Problem
		_ = json.Unmarshal(rec.Body.Bytes(), &p)
		msg := fmt.Sprintf("%d %s: %s", rec.Code, p.Code, p.Title)
		if p.Errors != nil {
			for _, f := range *p.Errors {
				msg += fmt.Sprintf("; %s: %s", f.Field, f.Message)
			}
		}
		return nil, errors.New(msg)
	}
	if out != nil && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			return nil, err
		}
	}
	return rec.Header(), nil
}

// jsonResult returns v to the agent as JSON text.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}
```

Check `Problem.Errors` type in `api.gen.go` (`*[]FieldError`) and `FieldError` field names; adjust if different.

- [ ] **Step 5: Write the read tools in `mcp_tools.go`**

```go
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpServer builds the tool set for one request. ponytail: schemas are
// inferred per request; cache the server per token if profiles show it.
func (s *Server) mcpServer(c *apiCaller) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "muasal", Version: "0.1"}, nil)

	mcp.AddTool(srv, &mcp.Tool{Name: "get_project", Description: "A project's statuses (with category todo, in_progress, done or cancelled), menu tree (flat node list with parent ids; tickets attach to menus by id), clients and assignees. Call it first for the ids the other tools take."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, any, error) {
			p := "/projects/" + url.PathEscape(in.Project)
			out := map[string]json.RawMessage{}
			for name, path := range map[string]string{"project": p, "statuses": p + "/statuses", "menus": p + "/nodes", "clients": p + "/clients", "assignees": p + "/assignees"} {
				var raw json.RawMessage
				if _, err := c.call(ctx, http.MethodGet, path, nil, nil, &raw); err != nil {
					return nil, nil, err
				}
				out[name] = raw
			}
			return jsonResult(out)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "list_tickets", Description: "Tickets in a project that you may see, newest change first unless sort says otherwise. Pass next_cursor back as cursor for the next page."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			setStr := func(k string, v *string) {
				if v != nil && *v != "" {
					q.Set(k, *v)
				}
			}
			setInt := func(k string, v *int64) {
				if v != nil {
					q.Set(k, strconv.FormatInt(*v, 10))
				}
			}
			setBool := func(k string, v *bool) {
				if v != nil {
					q.Set(k, strconv.FormatBool(*v))
				}
			}
			setStr("q", in.Q)
			setStr("type", in.Type)
			setStr("sort", in.Sort)
			setStr("cursor", in.Cursor)
			setInt("status_id", in.StatusID)
			setInt("client_id", in.ClientID)
			setInt("assignee_id", in.AssigneeID)
			setInt("node_id", in.NodeID)
			setBool("open", in.Open)
			setBool("mine", in.Mine)
			limit := 50
			if in.Limit != nil && *in.Limit > 0 {
				limit = min(*in.Limit, 200)
			}
			q.Set("limit", strconv.Itoa(limit))
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodGet, "/projects/"+url.PathEscape(in.Project)+"/tickets?"+q.Encode(), nil, nil, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "get_ticket", Description: "One ticket by key, e.g. HRIS-12, with its decision record and version. Pass version to update_ticket to avoid overwriting someone else's change."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in keyIn) (*mcp.CallToolResult, any, error) {
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodGet, "/tickets/"+url.PathEscape(in.Key), nil, nil, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	return srv
}

type projectIn struct {
	Project string `json:"project" jsonschema:"the project key, e.g. HRIS"`
}

type keyIn struct {
	Key string `json:"key" jsonschema:"the ticket key, e.g. HRIS-12"`
}

type listIn struct {
	Project    string  `json:"project" jsonschema:"the project key, e.g. HRIS"`
	Q          *string `json:"q,omitempty" jsonschema:"title words or a ticket key"`
	StatusID   *int64  `json:"status_id,omitempty" jsonschema:"only this status (ids from get_project)"`
	Open       *bool   `json:"open,omitempty" jsonschema:"true for open tickets only"`
	Type       *string `json:"type,omitempty" jsonschema:"bug, change_request or feature"`
	ClientID   *int64  `json:"client_id,omitempty" jsonschema:"only this client's tickets"`
	AssigneeID *int64  `json:"assignee_id,omitempty" jsonschema:"only tickets assigned to this user"`
	Mine       *bool   `json:"mine,omitempty" jsonschema:"true for tickets assigned to you"`
	NodeID     *int64  `json:"node_id,omitempty" jsonschema:"only tickets on this menu"`
	Sort       *string `json:"sort,omitempty" jsonschema:"updated, created, key, priority or due"`
	Limit      *int    `json:"limit,omitempty" jsonschema:"page size, 1 to 200, default 50"`
	Cursor     *string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
}
```

Check the list endpoint's real query names for `open` and `mine` in `api/openapi.yaml` (lines 853–895) and rename the `q.Set` keys to match.

- [ ] **Step 6: Register `/mcp`**

In `server.go` `Handler()`, replace the final `return securityHeaders(s.requestContext(mux))` with:

```go
	root := securityHeaders(s.requestContext(mux))
	// The MCP tools call the API through root, so they get its middleware too.
	mux.Handle("POST /mcp", s.mcpHandler(root))
	return root
```

- [ ] **Step 7: Run the tests**

Run: `cd server && go test ./internal/httpapi -run 'TestMCP|TestOAuth' -count=1`
Expected: PASS. If the SDK answers 403 because of its localhost DNS-rebinding check, set the option that disables it in `StreamableHTTPOptions` (see the SDK's `streamable.go`): Muasal sits behind Caddy and checks the token itself.

- [ ] **Step 8: Commit**

```bash
git add server/go.mod server/go.sum server/internal/httpapi/mcp.go server/internal/httpapi/mcp_tools.go server/internal/httpapi/mcp_test.go server/internal/httpapi/server.go
git commit -m "feat(mcp): /mcp endpoint with project and ticket read tools"
```

---

### Task 5: Write tools

**Files:**
- Modify: `server/internal/httpapi/mcp_tools.go`, `server/internal/httpapi/mcp_test.go`

**Interfaces:**
- Consumes: `apiCaller.call`, `jsonResult`, generated `Ticket`, `TicketUpdate`, `TransitionRequest`, `DecisionInput`, `StatusList`, `TicketRequesterKindContact`/`TicketRequesterKindUser` (check the generated constant names).
- Produces: tools `create_ticket`, `update_ticket`, `transition_ticket`, `cancel_ticket`; `func bodyOf(v any, drop ...string) (map[string]any, error)`.

- [ ] **Step 1: Write the failing tests**

Append to `mcp_test.go`:

```go
// The write tools go through the API's rules: read-only tokens, versions, close validation.
func TestMCPWriteTools(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tok := mcpToken(e, w.pm, false)

	text, isErr := mcpCall(e, tok, "create_ticket", map[string]any{"project": "HRIS", "type": "bug", "title": "Filed by an agent",
		"node_ids": []int64{w.ot.ID}, "client_id": w.a.ID, "reason": "The agent found it."})
	var tk struct {
		Key     string
		Title   string
		Reason  string
		Version int
		Status  struct{ Category string }
	}
	if isErr || json.Unmarshal([]byte(text), &tk) != nil || tk.Key != "HRIS-1" {
		t.Fatalf("create: %v %s", isErr, text)
	}

	// Only the given field changes.
	text, isErr = mcpCall(e, tok, "update_ticket", map[string]any{"key": "HRIS-1", "title": "Renamed by an agent"})
	if isErr || json.Unmarshal([]byte(text), &tk) != nil || tk.Title != "Renamed by an agent" || tk.Reason != "The agent found it." {
		t.Fatalf("update: %v %s", isErr, text)
	}
	if text, isErr := mcpCall(e, tok, "update_ticket", map[string]any{"key": "HRIS-1", "title": "Stale", "version": 1}); !isErr || !strings.Contains(text, "412") {
		t.Fatalf("stale version: %v %s", isErr, text)
	}

	// Cancel needs why; with it the ticket closes as Cancelled with a decision record.
	if text, isErr := mcpCall(e, tok, "cancel_ticket", map[string]any{"key": "HRIS-1", "reason": "Duplicate of another ticket.", "why": ""}); !isErr || !strings.Contains(text, "422") {
		t.Fatalf("cancel without why: %v %s", isErr, text)
	}
	text, isErr = mcpCall(e, tok, "cancel_ticket", map[string]any{"key": "HRIS-1", "reason": "Duplicate of another ticket.", "why": "The same request was filed twice."})
	if isErr || json.Unmarshal([]byte(text), &tk) != nil || tk.Status.Category != "cancelled" {
		t.Fatalf("cancel: %v %s", isErr, text)
	}
	if text, _ := mcpCall(e, tok, "get_ticket", map[string]any{"key": "HRIS-1"}); !strings.Contains(text, "The same request was filed twice.") {
		t.Fatalf("decision record: %s", text)
	}

	// transition_ticket reopens it.
	var statuses struct{ Items []struct{ Id int64; Category string } }
	e.call(w.pm, http.MethodGet, "/projects/HRIS/statuses", nil, &statuses)
	var todo int64
	for _, s := range statuses.Items {
		if s.Category == "todo" {
			todo = s.Id
		}
	}
	if text, isErr := mcpCall(e, tok, "transition_ticket", map[string]any{"key": "HRIS-1", "status_id": todo}); isErr || !strings.Contains(text, `"category":"todo"`) {
		t.Fatalf("reopen: %v %s", isErr, text)
	}

	// A read-only token can't write.
	ro := mcpToken(e, w.pm, true)
	if text, isErr := mcpCall(e, ro, "create_ticket", map[string]any{"project": "HRIS", "type": "bug", "title": "No", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID}); !isErr || !strings.Contains(text, "token_read_only") {
		t.Fatalf("read-only create: %v %s", isErr, text)
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `cd server && go test ./internal/httpapi -run TestMCPWriteTools -count=1`
Expected: FAIL (unknown tool `create_ticket`).

- [ ] **Step 3: Implement the write tools**

In `mcp_tools.go`, before `return srv` in `mcpServer`:

```go
	mcp.AddTool(srv, &mcp.Tool{Name: "create_ticket", Description: "File a ticket. node_ids are menu ids from get_project. Leave client_id out for core work that serves every client. Send the same idempotency_key when retrying."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in createIn) (*mcp.CallToolResult, any, error) {
			body, err := bodyOf(in, "project", "idempotency_key")
			if err != nil {
				return nil, nil, err
			}
			var h map[string]string
			if in.IdempotencyKey != nil && *in.IdempotencyKey != "" {
				h = map[string]string{"Idempotency-Key": *in.IdempotencyKey}
			}
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodPost, "/projects/"+url.PathEscape(in.Project)+"/tickets", h, body, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "update_ticket", Description: "Change a ticket's fields. Only the fields you pass change. Pass version from get_ticket to fail instead of overwriting a newer change. Status changes go through transition_ticket."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in updateIn) (*mcp.CallToolResult, any, error) {
			path := "/tickets/" + url.PathEscape(in.Key)
			var cur Ticket
			if _, err := c.call(ctx, http.MethodGet, path, nil, nil, &cur); err != nil {
				return nil, nil, err
			}
			up := TicketUpdate{Type: cur.Type, Title: cur.Title, Reason: &cur.Reason, Description: &cur.Description,
				Priority: &cur.Priority, DueDate: cur.DueDate, NodeIds: make([]int64, len(cur.Nodes))}
			for i, n := range cur.Nodes {
				up.NodeIds[i] = n.Id
			}
			if cur.Client != nil {
				up.ClientId = &cur.Client.Id
			}
			if cur.Assignee != nil {
				up.AssigneeId = &cur.Assignee.Id
			}
			if cur.Requester.Kind == TicketRequesterKindContact {
				up.RequesterContactId = &cur.Requester.Id
			} else {
				up.RequesterUserId = &cur.Requester.Id
			}
			if in.Type != nil {
				up.Type = TicketType(*in.Type)
			}
			if in.Title != nil {
				up.Title = *in.Title
			}
			if in.NodeIDs != nil {
				up.NodeIds = *in.NodeIDs
			}
			if in.ClientID != nil {
				up.ClientId = in.ClientID
			}
			if in.RequesterContactID != nil {
				up.RequesterContactId, up.RequesterUserId = in.RequesterContactID, nil
			}
			if in.RequesterUserID != nil {
				up.RequesterUserId, up.RequesterContactId = in.RequesterUserID, nil
			}
			if in.Reason != nil {
				up.Reason = in.Reason
			}
			if in.Description != nil {
				up.Description = in.Description
			}
			if in.AssigneeID != nil {
				up.AssigneeId = in.AssigneeID
			}
			if in.Priority != nil {
				p := Priority(*in.Priority)
				up.Priority = &p
			}
			if in.DueDate != nil {
				var d openapi_types.Date
				if err := d.UnmarshalText([]byte(*in.DueDate)); err != nil {
					return nil, nil, fmt.Errorf("due_date must be YYYY-MM-DD: %w", err)
				}
				up.DueDate = &d
			}
			version := cur.Version
			if in.Version != nil {
				version = *in.Version
			}
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodPut, path, map[string]string{"If-Match": fmt.Sprintf(`"%d"`, version)}, up, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "transition_ticket", Description: "Move a ticket to a status (ids from get_project). Moving to a done or cancelled status closes it and needs reason, at least one menu and decision {what_changed, why}. Leaving a closed status reopens it."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in transitionIn) (*mcp.CallToolResult, any, error) {
			req := TransitionRequest{StatusId: in.StatusID, Reason: in.Reason, NodeIds: in.NodeIDs}
			if in.Decision != nil {
				req.Decision = &DecisionInput{WhatChanged: in.Decision.WhatChanged, Why: in.Decision.Why, Alternatives: in.Decision.Alternatives}
			}
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodPost, "/tickets/"+url.PathEscape(in.Key)+"/transition", nil, req, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "cancel_ticket", Description: "Muasal never deletes tickets: this closes one as Cancelled and records why, so the history stays. reason and why are required."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in cancelIn) (*mcp.CallToolResult, any, error) {
			path := "/tickets/" + url.PathEscape(in.Key)
			var cur Ticket
			if _, err := c.call(ctx, http.MethodGet, path, nil, nil, &cur); err != nil {
				return nil, nil, err
			}
			var statuses StatusList
			if _, err := c.call(ctx, http.MethodGet, "/projects/"+url.PathEscape(cur.ProjectKey)+"/statuses", nil, nil, &statuses); err != nil {
				return nil, nil, err
			}
			var cancelled *Status
			for i := range statuses.Items {
				if statuses.Items[i].Category == StatusCategoryCancelled {
					cancelled = &statuses.Items[i]
					break
				}
			}
			if cancelled == nil {
				return nil, nil, errors.New("no_cancelled_status: this project has no status in the cancelled category")
			}
			what := "Cancelled; nothing changed."
			if in.WhatChanged != nil && *in.WhatChanged != "" {
				what = *in.WhatChanged
			}
			nodes := in.NodeIDs
			if nodes == nil {
				ids := make([]int64, len(cur.Nodes))
				for i, n := range cur.Nodes {
					ids[i] = n.Id
				}
				nodes = &ids
			}
			req := TransitionRequest{StatusId: cancelled.Id, Reason: &in.Reason, NodeIds: nodes,
				Decision: &DecisionInput{WhatChanged: what, Why: in.Why, Alternatives: in.Alternatives}}
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodPost, path+"/transition", nil, req, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})
```

Add the input types and `bodyOf` at the end of the file:

```go
type createIn struct {
	Project        string  `json:"project" jsonschema:"the project key, e.g. HRIS"`
	Type           string  `json:"type" jsonschema:"bug, change_request or feature"`
	Title          string  `json:"title" jsonschema:"at most 200 characters"`
	NodeIDs        []int64 `json:"node_ids" jsonschema:"menu ids from get_project"`
	ClientID       *int64  `json:"client_id,omitempty" jsonschema:"the client asking; leave out for core work"`
	Reason         *string `json:"reason,omitempty" jsonschema:"why the change is needed"`
	Description    *string `json:"description,omitempty" jsonschema:"Markdown details"`
	AssigneeID     *int64  `json:"assignee_id,omitempty"`
	Priority       *string `json:"priority,omitempty" jsonschema:"low, medium, high or urgent"`
	DueDate        *string `json:"due_date,omitempty" jsonschema:"YYYY-MM-DD"`
	StatusID       *int64  `json:"status_id,omitempty" jsonschema:"an open status; the project default if left out"`
	IdempotencyKey *string `json:"idempotency_key,omitempty" jsonschema:"repeat it when retrying to avoid a duplicate ticket"`
}

type updateIn struct {
	Key                string   `json:"key" jsonschema:"the ticket key, e.g. HRIS-12"`
	Type               *string  `json:"type,omitempty" jsonschema:"bug, change_request or feature"`
	Title              *string  `json:"title,omitempty"`
	NodeIDs            *[]int64 `json:"node_ids,omitempty" jsonschema:"replaces the menus"`
	ClientID           *int64   `json:"client_id,omitempty"`
	RequesterContactID *int64   `json:"requester_contact_id,omitempty"`
	RequesterUserID    *int64   `json:"requester_user_id,omitempty"`
	Reason             *string  `json:"reason,omitempty"`
	Description        *string  `json:"description,omitempty"`
	AssigneeID         *int64   `json:"assignee_id,omitempty"`
	Priority           *string  `json:"priority,omitempty" jsonschema:"low, medium, high or urgent"`
	DueDate            *string  `json:"due_date,omitempty" jsonschema:"YYYY-MM-DD"`
	Version            *int32   `json:"version,omitempty" jsonschema:"the version you read; the update fails if the ticket changed since"`
}

type decisionIn struct {
	WhatChanged  string  `json:"what_changed" jsonschema:"what changed in the product"`
	Why          string  `json:"why" jsonschema:"why it changed"`
	Alternatives *string `json:"alternatives,omitempty" jsonschema:"what was considered and rejected"`
}

type transitionIn struct {
	Key      string      `json:"key" jsonschema:"the ticket key, e.g. HRIS-12"`
	StatusID int64       `json:"status_id" jsonschema:"the target status id from get_project"`
	Reason   *string     `json:"reason,omitempty" jsonschema:"closing only: replaces the ticket's reason"`
	NodeIDs  *[]int64    `json:"node_ids,omitempty" jsonschema:"closing only: replaces the menus"`
	Decision *decisionIn `json:"decision,omitempty" jsonschema:"closing only: the decision record"`
}

type cancelIn struct {
	Key          string   `json:"key" jsonschema:"the ticket key, e.g. HRIS-12"`
	Reason       string   `json:"reason" jsonschema:"why the ticket existed"`
	Why          string   `json:"why" jsonschema:"why it is cancelled"`
	WhatChanged  *string  `json:"what_changed,omitempty" jsonschema:"default: Cancelled; nothing changed."`
	Alternatives *string  `json:"alternatives,omitempty"`
	NodeIDs      *[]int64 `json:"node_ids,omitempty" jsonschema:"default: the ticket's menus"`
}

// bodyOf turns a tool input into an API body without the tool-only fields.
func bodyOf(v any, drop ...string) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for _, k := range drop {
		delete(m, k)
	}
	return m, nil
}
```

Add imports `errors`, `fmt` and `openapi_types "github.com/oapi-codegen/runtime/types"`. Check the generated names `TicketRequesterKindContact`, `StatusCategoryCancelled`, `NodeRef.Id`, `Ref.Id` in `api.gen.go` and use the real ones.

- [ ] **Step 4: Run all server tests**

Run: `cd server && go vet ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/httpapi/mcp_tools.go server/internal/httpapi/mcp_test.go
git commit -m "feat(mcp): create, update, transition and cancel tools"
```

---

### Task 6: Approval page and sign-in return

**Files:**
- Create: `web/app/oauth/authorize/page.tsx`, `web/app/oauth/authorize/Approve.tsx`, `web/e2e/mcp-oauth.spec.ts`
- Modify: `web/proxy.ts`, `web/app/login/page.tsx`, `web/app/login/LoginForm.tsx`, `web/next.config.ts`, `web/messages/id.json`, `web/messages/en.json`, `web/e2e/global-setup.ts`

**Interfaces:**
- Consumes: `GET /api/v1/oauth/clients/{id}`, `POST /api/v1/oauth/approve` (types in `web/lib/api-types.ts`), `AuthCard`, `api` (`web/lib/api.ts`), `button`, `field`, `cx` (`web/lib/ui.ts`), `getMe`, `serverApi`.
- Produces: page `/oauth/authorize`; `safeNext(next?: string): string` exported from `web/lib/format.ts` (or a new `web/lib/next.ts` if format.ts is the wrong home); env `E2E_MCP_ADMIN_EMAIL`, `E2E_MCP_ADMIN_LINK`.

- [ ] **Step 1: Write the failing unit test for `safeNext`**

`web/lib/next.test.ts`:

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { safeNext } from "./next.ts";

test("safeNext follows only paths on this site", () => {
  assert.equal(safeNext("/oauth/authorize?client_id=X"), "/oauth/authorize?client_id=X");
  assert.equal(safeNext(undefined), "/");
  assert.equal(safeNext("https://evil.example"), "/");
  assert.equal(safeNext("//evil.example"), "/");
  assert.equal(safeNext("/\\evil.example"), "/");
});
```

Run: `cd web && node --test lib/next.test.ts`
Expected: FAIL (module not found).

- [ ] **Step 2: Implement `safeNext`**

`web/lib/next.ts`:

```ts
/** Where to go after sign-in: a path on this site, or home. `//x` and `/\x` would leave the site. */
export function safeNext(next?: string): string {
  return next && next.startsWith("/") && !next.startsWith("//") && !next.startsWith("/\\") ? next : "/";
}
```

Run: `cd web && npm test`
Expected: PASS.

- [ ] **Step 3: Return to `next` after sign-in**

`web/proxy.ts`: replace the redirect line so the approval page comes back after sign-in:

```ts
  if (!open.test(request.nextUrl.pathname) && !request.cookies.has("sid")) {
    const login = new URL("/login", request.url);
    // MCP spec: an agent's sign-in returns to its approval page.
    if (request.nextUrl.pathname.startsWith("/oauth/")) login.searchParams.set("next", request.nextUrl.pathname + request.nextUrl.search);
    return NextResponse.redirect(login);
  }
```

`web/app/login/page.tsx`: accept `next`, use it when already signed in, and pass it to the form:

```tsx
export default async function LoginPage({ searchParams }: { searchParams: Promise<{ email?: string; next?: string }> }) {
  const { email, next } = await searchParams;
  // Already signed in, as in a tab left here that reloads: go on. Switching
  // accounts starts with signing out.
  if (await getMe()) redirect(safeNext(next));
  const t = await getTranslations("login");
  return (
    <AuthCard title={t("title")}>
      <LoginForm email={email} next={next} />
    </AuthCard>
  );
}
```

`web/app/login/LoginForm.tsx`: take `next?: string` in the props and replace `router.push("/")` with `router.push(safeNext(next))`. Import `safeNext` from `@/lib/next` in both files.

- [ ] **Step 4: Add the strings**

`web/messages/en.json`, a new top-level `"oauth"` object:

```json
  "oauth": {
    "title": "Connect an AI agent",
    "asks": "{client} wants to use Muasal as you.",
    "account": "Signed in as {name} ({email})",
    "can": "It can read the tickets you can see, and create, edit and close tickets unless you tick Read only.",
    "returnsTo": "After you answer, your browser returns to {host}.",
    "readOnly": "Read only",
    "allow": "Allow",
    "deny": "Deny",
    "revoke": "You can disconnect it later in Settings › API tokens.",
    "invalid": "This connection request is not valid. Start again from your agent.",
    "failed": "Could not save your answer. Try again."
  },
```

`web/messages/id.json`:

```json
  "oauth": {
    "title": "Hubungkan agen AI",
    "asks": "{client} ingin memakai Muasal atas nama Anda.",
    "account": "Masuk sebagai {name} ({email})",
    "can": "Agen dapat membaca tiket yang Anda lihat, serta membuat, mengubah, dan menutup tiket, kecuali Anda mencentang Hanya baca.",
    "returnsTo": "Setelah Anda menjawab, browser kembali ke {host}.",
    "readOnly": "Hanya baca",
    "allow": "Izinkan",
    "deny": "Tolak",
    "revoke": "Anda dapat memutuskannya nanti di Pengaturan › Token API.",
    "invalid": "Permintaan koneksi ini tidak sah. Mulai lagi dari agen Anda.",
    "failed": "Jawaban Anda tidak tersimpan. Coba lagi."
  },
```

Run: `cd web && npm run check:i18n`
Expected: PASS.

- [ ] **Step 5: Write the page**

`web/app/oauth/authorize/page.tsx`:

```tsx
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import AuthCard from "@/components/AuthCard";
import { getMe, serverApi } from "@/lib/server-api";
import Approve from "./Approve";

type Params = Partial<Record<"client_id" | "redirect_uri" | "response_type" | "code_challenge" | "code_challenge_method" | "state" | "resource", string>>;

// MCP spec: an AI agent asks to act as the signed-in user. Nothing is issued
// until they click Allow; the API checks the request again then.
export default async function AuthorizePage({ searchParams }: { searchParams: Promise<Params> }) {
  const p = await searchParams;
  const me = await getMe();
  if (!me) redirect(`/login?next=${encodeURIComponent(`/oauth/authorize?${new URLSearchParams(p as Record<string, string>)}`)}`);
  const t = await getTranslations("oauth");
  const { data: client } = p.client_id
    ? await (await serverApi()).GET("/oauth/clients/{id}", { params: { path: { id: p.client_id } } })
    : { data: undefined };
  const valid = client && p.redirect_uri && client.redirect_uris.includes(p.redirect_uri) &&
    p.response_type === "code" && p.code_challenge_method === "S256" && p.code_challenge;
  return (
    <AuthCard title={t("title")}>
      {valid ? (
        <Approve
          client={client.name}
          host={new URL(p.redirect_uri!).host}
          me={{ name: me.name, email: me.email }}
          request={{ client_id: p.client_id!, redirect_uri: p.redirect_uri!, code_challenge: p.code_challenge!, code_challenge_method: "S256", state: p.state, resource: p.resource }}
        />
      ) : (
        <p role="alert">{t("invalid")}</p>
      )}
    </AuthCard>
  );
}
```

`web/app/oauth/authorize/Approve.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { button, cx, field } from "@/lib/ui";

type Request = { client_id: string; redirect_uri: string; code_challenge: string; code_challenge_method: string; state?: string; resource?: string };

export default function Approve({ client, host, me, request }: { client: string; host: string; me: { name: string; email: string }; request: Request }) {
  const t = useTranslations("oauth");
  const [readOnly, setReadOnly] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function answer(allow: boolean) {
    setBusy(true);
    const { data } = await api.POST("/oauth/approve", { body: { ...request, read_only: readOnly, allow } });
    if (!data) {
      setBusy(false);
      setError(t("failed"));
      return;
    }
    window.location.assign(data.redirect_url);
  }

  return (
    <div className="flex flex-col gap-3.5">
      <p className="font-medium">{t("asks", { client })}</p>
      <p className="text-sm">{t("account", me)}</p>
      <p className="text-sm">{t("can")}</p>
      <p className="text-sm">{t("returnsTo", { host })}</p>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={readOnly} onChange={(e) => setReadOnly(e.target.checked)} />
        {t("readOnly")}
      </label>
      {error && <p role="alert" className={field.error}>{error}</p>}
      <div className="flex gap-2">
        <button type="button" disabled={busy} onClick={() => answer(false)} className={cx(button.secondary, "h-9 flex-1")}>{t("deny")}</button>
        <button type="button" disabled={busy} onClick={() => answer(true)} className={cx(button.primary, "h-9 flex-1")}>{t("allow")}</button>
      </div>
      <p className="text-xs text-muted">{t("revoke")}</p>
    </div>
  );
}
```

Check `button.secondary` and `text-muted` exist in `web/lib/ui.ts` and `globals.css`; use the names the codebase has (e.g. `button.default`, `text-ink-3`).

- [ ] **Step 6: Dev rewrites**

`web/next.config.ts`, in the development branch of `rewrites()`:

```ts
      ? [
          { source: "/api/:path*", destination: "http://localhost:8080/api/:path*" },
          // MCP sign-in and the MCP endpoint; /oauth/authorize stays here.
          { source: "/mcp", destination: "http://localhost:8080/mcp" },
          { source: "/oauth/register", destination: "http://localhost:8080/oauth/register" },
          { source: "/oauth/token", destination: "http://localhost:8080/oauth/token" },
          { source: "/.well-known/:path*", destination: "http://localhost:8080/.well-known/:path*" },
        ]
```

Note `proxy.ts`'s matcher excludes only `api|_next/...`; rewrites run before the page lookup, but the proxy still runs for `/mcp`, `/oauth/register` and `/oauth/token` in dev and would redirect them to `/login` (no cookie). Add them to `open`: `const open = /^\/(login|setup|mcp|oauth\/(register|token)|\.well-known)(\/|$)/;`.

- [ ] **Step 7: Write the browser test**

`web/e2e/global-setup.ts`, with the other admins:

```ts
  const mcp = createAdmin("MCP Admin");
  process.env.E2E_MCP_ADMIN_EMAIL = mcp.email;
  process.env.E2E_MCP_ADMIN_LINK = mcp.link;
```

`web/e2e/mcp-oauth.spec.ts`:

```ts
import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// MCP spec: an agent registers, the user signs in and allows it on the
// approval page, and the browser returns to the agent with a code.
test("an AI agent is allowed, then denied, through the approval page", async ({ page }) => {
  const password = "e2e-mcp-admin-passphrase-8";
  const redirect = "http://127.0.0.1:9/callback";
  const reg = await page.request.post("/oauth/register", { data: { client_name: "E2E Agent", redirect_uris: [redirect] } });
  expect(reg.status()).toBe(201);
  const { client_id } = await reg.json();
  const authorize = `/oauth/authorize?${new URLSearchParams({
    client_id, redirect_uri: redirect, response_type: "code", state: "s1",
    code_challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", code_challenge_method: "S256",
  })}`;

  await setPassword(page, process.env.E2E_MCP_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_MCP_ADMIN_EMAIL!, password);

  await page.goto(authorize);
  await expect(page.getByText("E2E Agent ingin memakai Muasal atas nama Anda.")).toBeVisible();
  await expect(page.getByText("127.0.0.1:9")).toBeVisible();
  // The redirect target has no server; catch the navigation instead of loading it.
  await page.route(`${redirect}**`, (route) => route.fulfill({ status: 200, body: "ok" }));
  await page.getByRole("button", { name: "Izinkan" }).click();
  await page.waitForURL(`${redirect}**`);
  const allowed = new URL(page.url());
  expect(allowed.searchParams.get("code")).toBeTruthy();
  expect(allowed.searchParams.get("state")).toBe("s1");

  await page.goto(authorize);
  await page.getByRole("button", { name: "Tolak" }).click();
  await page.waitForURL(`${redirect}**`);
  expect(new URL(page.url()).searchParams.get("error")).toBe("access_denied");
});
```

- [ ] **Step 8: Check the web**

Run: `cd web && npm test && npm run check:i18n && npx tsc --noEmit`
Expected: PASS. The e2e spec runs in CI against the full stack (`make e2e` locally needs Docker).

- [ ] **Step 9: Commit**

```bash
git add web/lib/next.ts web/lib/next.test.ts web/proxy.ts web/app/login web/app/oauth web/next.config.ts web/messages web/e2e/global-setup.ts web/e2e/mcp-oauth.spec.ts
git commit -m "feat(web): approval page for AI agents signing in through MCP"
```

---

### Task 7: Caddy route and docs

**Files:**
- Modify: `deploy/Caddyfile`, `mkdocs.yml`, `README.md`
- Create: `docs/mcp.md`

- [ ] **Step 1: Route the app paths in Caddy**

In `deploy/Caddyfile`, after the `/webhooks/*` block and before the catch-all `handle {`:

```
	# MCP endpoint and its sign-in (docs/mcp.md); /oauth/authorize is a web page.
	@mcp path /mcp /oauth/register /oauth/token /.well-known/oauth-*
	handle @mcp {
		reverse_proxy app:8080
	}
```

- [ ] **Step 2: Write `docs/mcp.md`**

Sections, in the docs' existing voice (short sentences, second person):
1. **What it is:** AI agents work with tickets through `/mcp`; you approve each agent once in the browser.
2. **Connect Claude Code:** `claude mcp add --transport http muasal https://muasal.example.com/mcp`, then run `/mcp` in Claude Code and pick muasal to sign in.
3. **Other clients:** any MCP client that supports streamable HTTP and OAuth sign-in; the URL is `<PUBLIC_URL>/mcp`.
4. **Tools:** the table of the seven tools from the spec, one line each.
5. **Read-only access:** tick Read only on the approval page.
6. **Disconnect an agent:** revoke its token in Settings › API tokens; it is named after the agent with "(MCP)".
7. **Why there is no delete:** Muasal keeps why every change happened; `cancel_ticket` closes a ticket as Cancelled with its decision record.
8. **Behind your own proxy:** route `/mcp`, `/oauth/register`, `/oauth/token` and `/.well-known/oauth-*` to the app, like `/api`.

Add `- MCP for AI agents: mcp.md` to the `nav` in `mkdocs.yml`, next to the API page (match the file's existing nav format).

- [ ] **Step 3: README**

In `README.md` "Also included", extend the **Connections** line: `…, API tokens for scripts, and an MCP endpoint so AI agents such as Claude Code can work with tickets ([how](docs/mcp.md)).`

- [ ] **Step 4: Check the docs build**

Run: `mkdocs build --strict` if MkDocs is installed; otherwise rely on CI's docs job.
Expected: no warnings.

- [ ] **Step 5: Commit**

```bash
git add deploy/Caddyfile docs/mcp.md mkdocs.yml README.md
git commit -m "docs(mcp): connect AI agents, and route MCP paths in Caddy"
```

---

## Self-review notes

- Spec coverage: data (T1), discovery and register (T1), approve and client lookup (T2), token exchange and purge (T3), `/mcp` auth and read tools (T4), write tools including cancel (T5), web page, login `next`, dev rewrites, strings and e2e (T6), Caddy, docs and README (T7), permission rows (T2).
- Deviations from the spec, all recorded in the spec on 2026-10-03: client IDs are `rand.Text()` (26 characters); a missing client name becomes "MCP client"; the token's audit uses `via: "web"`; tool results are JSON text.
