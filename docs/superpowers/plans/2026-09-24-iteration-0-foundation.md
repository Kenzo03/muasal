# Muasal Iteration 0 — Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A new open-source monorepo in which an admin creates a user and that user signs in to Muasal on the Docker Compose stack. This is the exit check for FSD §21 Iteration 0.

**Architecture:** `api/openapi.yaml` is the contract. oapi-codegen generates the Go std-http routing and models from it, and openapi-typescript generates the web types. One Go 1.27 binary (`serve`, `migrate up`, `admin create-admin`, `healthcheck`) talks to PostgreSQL 18 through pgx and sqlc, with goose migrations embedded. Sessions live in PostgreSQL as SHA-256 hashes of 256-bit cookie tokens. Next.js 16 renders pages and reads through the Go API on the server, forwarding the session cookie. The browser sends every write to `/api/v1`, which Caddy routes to Go on the same origin.

**Tech Stack:** Go 1.27.1, pgx v5.11.0, goose v3.28.0, sqlc v1.31.1, oapi-codegen v2.8.0 with runtime v1.7.0, golang.org/x/crypto v0.57.0; Node 24 (22+ works locally), Next.js 16.3.6, React 19.3.0, next-intl 4.14.6, openapi-fetch 0.17.0, openapi-typescript 7.13.0, Tailwind CSS 4.3.3, TypeScript 5.9.3, Playwright 1.63.0; PostgreSQL 18 (`pgvector/pgvector:0.8.6-pg18-trixie`), Caddy 2, Docker Compose, GitHub Actions (checkout, setup-go, setup-node v7).

**Spec:** Claude Docs "FSD — Muasal": §3 architecture, §4 stack and code rules, §5.1 roles, §6 screens, §15.1 users, §16 data model, §17 API, §18.2 security, §19 deployment, §21 delivery plan.

## Global Constraints

- Product name Muasal; license Apache-2.0; repo root `muasal/`; images `muasal-web` and `muasal-app`; database `muasal`.
- Go module path `github.com/muasal/muasal/server`.
- API base `/api/v1`; JSON; snake_case fields; RFC 3339 timestamps in UTC.
- Errors are RFC 9457 `application/problem+json` with a stable `code` and optional `errors: [{field, code, message}]`; the UI translates the codes.
- Session cookie `sid`: HttpOnly, SameSite=Lax, Secure when `PUBLIC_URL` is https; 12 h idle timeout, 7 d absolute; a 256-bit random token stored only as its SHA-256; a new token at every sign-in; all sessions revoked on disable, password reset and setup; other sessions revoked on password change.
- CSRF: cookie-authenticated POST, PUT, PATCH and DELETE must send `Origin` equal to `PUBLIC_URL`.
- Passwords: argon2id at the OWASP minimum (19 MiB, t=2, p=1); 12 or more characters; not in SecLists `xato-net-10-million-passwords-100000.txt` (MIT).
- Lockout: 5 failed sign-ins on one account within 15 minutes lock it for 15 minutes (HTTP 429, code `account_locked`); more than 20 sign-in attempts per IP per minute get HTTP 429, code `rate_limited`.
- Setup links are one-time and expire after 72 hours; a new link voids the older ones.
- Users are never hard-deleted.
- Every mutation runs in one transaction together with its `audit_events` row; the app's database role cannot UPDATE or DELETE `audit_events`.
- Logs use `log/slog` JSON with a request ID and never contain passwords, tokens or cookie values.
- Next.js never writes data or decides access; its server code only reads (GET) from the API.
- Every UI string exists in `id` and `en`; the default locale is `id` and the default timezone `Asia/Jakarta`.
- No ORM, no dependency-injection container, no interface with a single implementation.
- `NEXT_TELEMETRY_DISABLED=1` everywhere.
- Generated code (Go and TypeScript) is committed; CI regenerates it and fails on any diff.

## Deliberate Deviations from the FSD

- oapi-codegen generates the plain std-http `ServerInterface`, not the strict server. Handlers decode and encode through two small helpers, and method names follow the operationIds.
- TypeScript 5.9.3, not 7: `next build` type-checks through the compiler API, which the native TypeScript 7 does not offer. (Execution found that 6.0.3, the first choice, breaks `npm install`: openapi-typescript 7.13 declares a peer range of ^5.x.)
- shadcn/ui arrives in Iteration 1; Iteration 0 screens use plain Tailwind classes.
- Production TLS and the offline installer belong to Iteration 5; Iteration 0 serves `http://localhost`.
- `users` gains `failed_logins`, `failed_since` and `locked_until` for the lockout rule; `sso_subject` and `notify_prefs` come with their P1 features.
- HSTS and a nonce-based Content-Security-Policy (FSD §18.2) land with the Iteration 5 hardening; Iteration 0 sets `nosniff` and `no-store` on API responses.
- `GET /admin/users` is not paginated yet; cursor pagination (FSD §17.1) arrives with the ticket list in Iteration 2.

## Prerequisites

- Go 1.27 or newer. An older Go downloads 1.27.1 by itself through `GOTOOLCHAIN=auto`.
- A C compiler for sqlc (`xcode-select --install` on macOS; `build-essential` on Debian/Ubuntu).
- Node 22 or newer, and Docker with Compose.

## File Structure

```text
muasal/
├── .github/workflows/ci.yml            CI: Go tests + codegen drift, web build + types drift, end-to-end
├── .gitignore  LICENSE  NOTICE  SECURITY.md  README.md  Makefile
├── api/openapi.yaml                    the API contract (source of truth for both sides)
├── deploy/
│   ├── compose.yaml                    caddy, web, app, db
│   ├── Caddyfile                       /api/* → app, everything else → web
│   └── .env.example                    copied to deploy/.env (git-ignored)
├── server/                             Go module github.com/muasal/muasal/server
│   ├── Dockerfile  go.mod  go.sum  sqlc.yaml
│   ├── cmd/app/main.go (+ main_test.go)          serve | migrate up | admin create-admin | healthcheck
│   ├── migrations/embed.go  00001_init.sql       goose SQL, embedded in the binary
│   └── internal/
│       ├── config/config.go (+ config_test.go)    settings from the environment
│       ├── migrate/migrate.go (+ migrate_test.go) goose runner + least-privilege app role
│       ├── testdb/testdb.go                       throwaway migrated database per test
│       ├── db/generate.go  queries/*.sql          sqlc input; the other *.go files are generated
│       ├── auth/password.go policy.go token.go limiter.go (+ tests), common-passwords.txt, common-passwords.LICENSE
│       └── httpapi/generate.go oapi-codegen.yaml api.gen.go (generated) problem.go server.go
│                   middleware.go auth_handlers.go users.go (+ problem_test.go auth_test.go users_test.go)
└── web/                                Next.js app
    ├── Dockerfile  .dockerignore  package.json  package-lock.json  tsconfig.json
    ├── next.config.ts  postcss.config.mjs  playwright.config.ts  proxy.ts
    ├── i18n/request.ts  messages/id.json  messages/en.json
    ├── lib/api-types.ts (generated)  lib/api.ts  lib/server-api.ts  lib/problem.ts
    ├── app/layout.tsx  app/globals.css  app/page.tsx  app/SignOutButton.tsx
    ├── app/login/page.tsx  app/login/LoginForm.tsx
    ├── app/setup/[token]/page.tsx  app/setup/[token]/SetupForm.tsx
    ├── app/settings/profile/page.tsx  app/settings/profile/ProfileForm.tsx
    ├── app/admin/users/page.tsx  app/admin/users/UsersAdmin.tsx
    └── e2e/global-setup.ts  e2e/signin.spec.ts
```

---

### Task 1: Repository skeleton and configuration

**Files:**
- Create: `.gitignore`, `README.md`, `NOTICE`, `SECURITY.md`, `Makefile`, `LICENSE` (downloaded)
- Create: `server/go.mod` (via `go mod init`)
- Create: `server/internal/config/config.go`
- Test: `server/internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `config.Config{DatabaseURL, MigrateDatabaseURL, PublicURL, ListenAddr string}`, `config.Load(getenv func(string) string) (config.Config, error)`, `(config.Config).SecureCookies() bool`. Makefile targets `generate`, `testdb`, `test`, `up`, `down`, `logs`, `admin`, `e2e`.

- [ ] **Step 1: Initialise the repository and fetch the license**

Run from `~/Project/muasal` (the folder already holds `docs/`):

```bash
git init -b main
curl -fsSL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE
head -3 LICENSE
```

Expected: the first lines read `Apache License` / `Version 2.0, January 2004`.

- [ ] **Step 2: Write the top-level files**

`.gitignore`:

```gitignore
# Go
/server/bin/
# Node
node_modules/
/web/.next/
/web/next-env.d.ts
/web/test-results/
/web/playwright-report/
# Local environment
/deploy/.env
.DS_Store
```

`README.md`:

```markdown
# Muasal

Know why every screen is the way it is. Muasal is a self-hosted ticketing tool that remembers the reason behind every change to every menu, and answers "why does this screen work like this?" with links to the tickets behind it.

Status: early development (Iteration 0: sign-in and user management).

## Run it

    cp deploy/.env.example deploy/.env      # then change both passwords
    make up                                  # builds and starts Caddy, web, app and PostgreSQL
    make admin EMAIL=you@example.com NAME="Your Name"

Open the printed setup link, choose a password, and sign in at http://localhost.

## Develop

    make testdb      # throwaway PostgreSQL on localhost:55432 for the Go tests
    make test        # Go unit and integration tests
    make generate    # regenerate Go stubs, sqlc code and TypeScript API types
    make e2e         # browser test against a running `make up` stack

## License

Apache-2.0. See LICENSE and NOTICE.
```

`NOTICE`:

```text
Muasal
Copyright 2026 The Muasal Authors

This product includes server/internal/auth/common-passwords.txt, derived from
SecLists (https://github.com/danielmiessler/SecLists), which is licensed under
the MIT License. See server/internal/auth/common-passwords.LICENSE.
```

`SECURITY.md`:

```markdown
# Security policy

Please report vulnerabilities privately through GitHub: open this repository's **Security** tab and choose **Report a vulnerability**. Do not open a public issue.

We aim to acknowledge a report within three working days and to share a fix timeline after triage. Only the latest release receives security fixes.
```

`Makefile` (recipe lines start with a tab):

```make
TEST_DATABASE_URL ?= postgres://owner:owner@localhost:55432/postgres?sslmode=disable
COMPOSE = docker compose -f deploy/compose.yaml --env-file deploy/.env

.PHONY: generate testdb test up down logs admin e2e

generate: ## regenerate Go stubs, sqlc code and TypeScript API types
	cd server && go generate ./...
	cd web && npm run gen:api

testdb: ## start a throwaway PostgreSQL for the Go tests
	docker run -d --rm --name muasal-testdb -e POSTGRES_USER=owner -e POSTGRES_PASSWORD=owner \
		-p 55432:5432 pgvector/pgvector:0.8.6-pg18-trixie

test:
	cd server && TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test ./...

up:
	$(COMPOSE) up -d --build --wait

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

admin: ## make admin EMAIL=you@example.com NAME="Your Name"
	$(COMPOSE) exec app /app admin create-admin --email "$(EMAIL)" --name "$(NAME)"

e2e:
	cd web && npx playwright test
```

- [ ] **Step 3: Create the Go module**

```bash
mkdir -p server && cd server
go mod init github.com/muasal/muasal/server
go mod edit -go=1.27.1
cat go.mod
```

Expected: `module github.com/muasal/muasal/server` and `go 1.27.1`.

- [ ] **Step 4: Write the failing config test**

`server/internal/config/config_test.go`:

```go
package config

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x", "PUBLIC_URL": "https://muasal.test/"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":8080" || c.PublicURL != "https://muasal.test" || !c.SecureCookies() {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestLoadRejectsMissingAndInvalidValues(t *testing.T) {
	for _, publicURL := range []string{"", "muasal.test", "https://muasal.test/app"} {
		if _, err := Load(env(map[string]string{"PUBLIC_URL": publicURL})); err == nil {
			t.Errorf("PUBLIC_URL %q with no DATABASE_URL: want an error", publicURL)
		}
	}
}

func TestHTTPPublicURLMeansInsecureCookies(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x", "PUBLIC_URL": "http://localhost"}))
	if err != nil || c.SecureCookies() {
		t.Fatalf("want insecure cookies for http, got secure=%v err=%v", c.SecureCookies(), err)
	}
}
```

- [ ] **Step 5: Run it to verify it fails**

Run: `cd server && go test ./internal/config/`
Expected: FAIL with `undefined: Load`.

- [ ] **Step 6: Implement the config package**

`server/internal/config/config.go`:

```go
// Package config reads the app's settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Config holds every setting the binary reads at start.
type Config struct {
	DatabaseURL        string // app role (least privilege)
	MigrateDatabaseURL string // owner role; only migrations use it
	PublicURL          string // the origin users open, e.g. https://muasal.example.com
	ListenAddr         string // default ":8080"
}

// Load reads settings through getenv (os.Getenv in production). Invalid
// values fail at start rather than at the first request.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		DatabaseURL:        getenv("DATABASE_URL"),
		MigrateDatabaseURL: getenv("MIGRATE_DATABASE_URL"),
		PublicURL:          strings.TrimRight(getenv("PUBLIC_URL"), "/"),
		ListenAddr:         getenv("LISTEN_ADDR"),
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if u, err := url.Parse(c.PublicURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" {
		errs = append(errs, fmt.Errorf("PUBLIC_URL must be an origin such as https://muasal.example.com, got %q", c.PublicURL))
	}
	return c, errors.Join(errs...)
}

// SecureCookies reports whether session cookies need the Secure flag.
func (c Config) SecureCookies() bool { return strings.HasPrefix(c.PublicURL, "https://") }
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `cd server && go test ./internal/config/`
Expected: `ok  github.com/muasal/muasal/server/internal/config`

- [ ] **Step 8: Commit**

```bash
git add .gitignore README.md NOTICE SECURITY.md Makefile LICENSE server/go.mod server/internal/config docs
git commit -m "chore: repository skeleton, license and config"
```

---

### Task 2: API contract and Go code generation

**Files:**
- Create: `api/openapi.yaml`
- Create: `server/internal/httpapi/oapi-codegen.yaml`
- Create: `server/internal/httpapi/generate.go`
- Generate: `server/internal/httpapi/api.gen.go`
- Create: `server/internal/httpapi/problem.go`
- Test: `server/internal/httpapi/problem_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces (generated, package `httpapi`): `ServerInterface` with methods `Login`, `Logout`, `SetupPassword`, `GetMe`, `UpdateMe`, `ListUsers`, `CreateUser` (all `(w http.ResponseWriter, r *http.Request)`) plus `UpdateUser` and `CreateSetupLink` (`(w, r, id int64)`); `StdHTTPServerOptions`, `MiddlewareFunc`, `HandlerWithOptions`; models `User`, `UserList`, `Locale`, `LoginRequest`, `SetupRequest`, `MeUpdate`, `UserCreate`, `UserUpdate`, `SetupLink`, `CreatedUser`, `Problem`, `FieldError`. Handwritten: `writeJSON(w, status int, v any)`, `writeProblem(w, status int, code, title string, fields ...FieldError)`, `decodeJSON(w, r, dst any) bool`.

- [ ] **Step 1: Write the API contract**

`api/openapi.yaml`:

```yaml
openapi: 3.0.3
info:
  title: Muasal API
  version: 0.1.0
  license:
    name: Apache-2.0
    url: https://www.apache.org/licenses/LICENSE-2.0
servers:
  - url: /api/v1
paths:
  /auth/login:
    post:
      operationId: login
      tags: [auth]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/LoginRequest" }
      responses:
        "200":
          description: Signed in; the `sid` session cookie is set.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/User" }
        default: { $ref: "#/components/responses/Problem" }
  /auth/logout:
    post:
      operationId: logout
      tags: [auth]
      responses:
        "204": { description: Signed out; the session is revoked. }
        default: { $ref: "#/components/responses/Problem" }
  /auth/setup:
    post:
      operationId: setupPassword
      tags: [auth]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/SetupRequest" }
      responses:
        "204": { description: Password set; every session of the user is revoked. }
        default: { $ref: "#/components/responses/Problem" }
  /me:
    get:
      operationId: getMe
      tags: [me]
      responses:
        "200":
          description: The signed-in user.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/User" }
        default: { $ref: "#/components/responses/Problem" }
    patch:
      operationId: updateMe
      tags: [me]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/MeUpdate" }
      responses:
        "200":
          description: The updated profile.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/User" }
        default: { $ref: "#/components/responses/Problem" }
  /admin/users:
    get:
      operationId: listUsers
      tags: [admin]
      responses:
        "200":
          description: Every user.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/UserList" }
        default: { $ref: "#/components/responses/Problem" }
    post:
      operationId: createUser
      tags: [admin]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/UserCreate" }
      responses:
        "201":
          description: The user, with a one-time setup link.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/CreatedUser" }
        default: { $ref: "#/components/responses/Problem" }
  /admin/users/{id}:
    patch:
      operationId: updateUser
      tags: [admin]
      parameters:
        - name: id
          in: path
          required: true
          schema: { type: integer, format: int64 }
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/UserUpdate" }
      responses:
        "200":
          description: The updated user.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/User" }
        default: { $ref: "#/components/responses/Problem" }
  /admin/users/{id}/setup-link:
    post:
      operationId: createSetupLink
      tags: [admin]
      description: Resets the password; the old one stops working and every session ends.
      parameters:
        - name: id
          in: path
          required: true
          schema: { type: integer, format: int64 }
      responses:
        "201":
          description: A new one-time setup link.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/SetupLink" }
        default: { $ref: "#/components/responses/Problem" }
components:
  responses:
    Problem:
      description: An error, as RFC 9457 problem details.
      content:
        application/problem+json:
          schema: { $ref: "#/components/schemas/Problem" }
  schemas:
    Locale:
      type: string
      enum: [id, en]
    User:
      type: object
      required: [id, email, name, is_admin, locale, timezone, disabled, has_password, created_at, last_login_at]
      properties:
        id: { type: integer, format: int64 }
        email: { type: string }
        name: { type: string }
        is_admin: { type: boolean }
        locale: { $ref: "#/components/schemas/Locale" }
        timezone: { type: string, example: Asia/Jakarta }
        disabled: { type: boolean }
        has_password: { type: boolean }
        created_at: { type: string, format: date-time }
        last_login_at: { type: string, format: date-time, nullable: true }
    UserList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/User" }
    LoginRequest:
      type: object
      required: [email, password]
      properties:
        email: { type: string, maxLength: 320 }
        password: { type: string, maxLength: 1024 }
    SetupRequest:
      type: object
      required: [token, password]
      properties:
        token: { type: string }
        password: { type: string, maxLength: 1024 }
    MeUpdate:
      type: object
      properties:
        name: { type: string, maxLength: 200 }
        locale: { $ref: "#/components/schemas/Locale" }
        timezone: { type: string }
        current_password: { type: string, maxLength: 1024 }
        new_password: { type: string, maxLength: 1024 }
    UserCreate:
      type: object
      required: [email, name]
      properties:
        email: { type: string, maxLength: 320 }
        name: { type: string, maxLength: 200 }
        is_admin: { type: boolean }
        locale: { $ref: "#/components/schemas/Locale" }
        timezone: { type: string }
    UserUpdate:
      type: object
      properties:
        name: { type: string, maxLength: 200 }
        is_admin: { type: boolean }
        locale: { $ref: "#/components/schemas/Locale" }
        timezone: { type: string }
        disabled: { type: boolean }
    SetupLink:
      type: object
      required: [url, expires_at]
      properties:
        url: { type: string }
        expires_at: { type: string, format: date-time }
    CreatedUser:
      type: object
      required: [user, setup_link]
      properties:
        user: { $ref: "#/components/schemas/User" }
        setup_link: { $ref: "#/components/schemas/SetupLink" }
    Problem:
      type: object
      required: [type, title, status, code]
      properties:
        type: { type: string }
        title: { type: string }
        status: { type: integer }
        detail: { type: string }
        code: { type: string }
        errors:
          type: array
          items: { $ref: "#/components/schemas/FieldError" }
    FieldError:
      type: object
      required: [field, code, message]
      properties:
        field: { type: string }
        code: { type: string }
        message: { type: string }
```

- [ ] **Step 2: Configure and run the Go generator**

`server/internal/httpapi/oapi-codegen.yaml`:

```yaml
package: httpapi
output: api.gen.go
generate:
  std-http-server: true
  models: true
```

`server/internal/httpapi/generate.go`:

```go
// Package httpapi serves the /api/v1 REST API defined in api/openapi.yaml.
// api.gen.go is generated from that file; run `go generate ./...` after editing it.
package httpapi

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config oapi-codegen.yaml ../../../api/openapi.yaml
```

```bash
cd server
go get github.com/oapi-codegen/runtime@v1.7.0
go generate ./internal/httpapi/
grep -n "UpdateUser(w http.ResponseWriter, r \*http.Request, id int64)" internal/httpapi/api.gen.go
```

Expected: the grep prints one line from the `ServerInterface` declaration.

- [ ] **Step 3: Write the failing helper tests**

`server/internal/httpapi/problem_test.go`:

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteProblem(t *testing.T) {
	rec := httptest.NewRecorder()
	writeProblem(rec, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
		FieldError{Field: "email", Code: "invalid", Message: "Enter a valid email address"})
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content type %q", got)
	}
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Status != 422 || p.Code != "validation_failed" || p.Type != "/problems/validation_failed" ||
		p.Errors == nil || (*p.Errors)[0].Field != "email" {
		t.Fatalf("unexpected problem: %+v", p)
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"a@b.c","password":"x","is_admin":true}`))
	var in LoginRequest
	if decodeJSON(rec, req, &in) {
		t.Fatal("an unknown field must be rejected")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}
```

- [ ] **Step 4: Run them to verify they fail**

Run: `cd server && go test ./internal/httpapi/`
Expected: FAIL with `undefined: writeProblem` and `undefined: decodeJSON`.

- [ ] **Step 5: Implement the helpers**

`server/internal/httpapi/problem.go`:

```go
package httpapi

import (
	"encoding/json"
	"net/http"
)

// writeJSON writes v as a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeProblem writes an RFC 9457 problem (FSD §17.1). code is stable; the UI translates it.
func writeProblem(w http.ResponseWriter, status int, code, title string, fields ...FieldError) {
	p := Problem{Type: "/problems/" + code, Title: title, Status: status, Code: code}
	if len(fields) > 0 {
		p.Errors = &fields
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

// decodeJSON reads a JSON body of at most 1 MiB into dst and rejects unknown
// fields, so a client cannot slip in fields such as is_admin.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "The request body is not valid JSON for this endpoint")
		return false
	}
	return true
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd server && go mod tidy && go test ./internal/httpapi/`
Expected: `ok  github.com/muasal/muasal/server/internal/httpapi`

- [ ] **Step 7: Commit**

```bash
git add api server/go.mod server/go.sum server/internal/httpapi
git commit -m "feat(api): OpenAPI contract, generated Go server types, problem helpers"
```

---

### Task 3: Database schema, migrations and queries

**Files:**
- Create: `server/migrations/00001_init.sql`, `server/migrations/embed.go`
- Create: `server/sqlc.yaml`, `server/internal/db/generate.go`
- Create: `server/internal/db/queries/users.sql`, `sessions.sql`, `setup_tokens.sql`, `audit.sql`
- Generate: `server/internal/db/db.go`, `models.go`, `*.sql.go`
- Create: `server/internal/migrate/migrate.go`, `server/internal/testdb/testdb.go`
- Test: `server/internal/migrate/migrate_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `migrate.Up(ctx, ownerURL, appURL string) error`; `testdb.New(t) testdb.DB` with fields `OwnerURL`, `AppURL string` and `Pool *pgxpool.Pool`; sqlc package `db` with `db.New(DBTX) *db.Queries`, `(*db.Queries).WithTx(pgx.Tx)`, model `db.User{ID int64; Email, Name string; PasswordHash *string; IsAdmin bool; Locale, Timezone string; FailedLogins int32; FailedSince, LockedUntil, DisabledAt, LastLoginAt *time.Time; CreatedAt time.Time}` and methods `CreateUser(ctx, CreateUserParams{Email, Name string; IsAdmin bool; Locale, Timezone string}) (User, error)`, `GetUserByID(ctx, int64) (User, error)`, `GetUserByEmail(ctx, email string) (User, error)`, `ListUsers(ctx) ([]User, error)`, `UpdateUser(ctx, UpdateUserParams{Name *string; IsAdmin *bool; Locale, Timezone *string; ID int64}) (User, error)`, `SetDisabled(ctx, SetDisabledParams{Disabled bool; ID int64}) (User, error)`, `SetPasswordHash(ctx, SetPasswordHashParams{PasswordHash *string; ID int64}) error`, `RecordLoginFailure(ctx, int64) error`, `RecordLoginSuccess(ctx, int64) error`, `CreateSession(ctx, CreateSessionParams{TokenHash []byte; UserID int64; ExpiresAt time.Time; Ip *netip.Addr; UserAgent *string}) error`, `GetSession(ctx, []byte) (GetSessionRow{User User; LastSeenAt, ExpiresAt time.Time}, error)`, `TouchSession`, `DeleteSession` (`[]byte`), `DeleteUserSessions(ctx, int64)`, `DeleteOtherSessions(ctx, DeleteOtherSessionsParams{UserID int64; TokenHash []byte})`, `CreateSetupToken(ctx, CreateSetupTokenParams{TokenHash []byte; UserID int64; ExpiresAt time.Time}) error`, `VoidSetupTokens(ctx, int64) error`, `UseSetupToken(ctx, []byte) (int64, error)`, `InsertAuditEvent(ctx, InsertAuditEventParams{ActorID *int64; Via, Entity string; EntityID int64; Action string; Changes []byte; RequestID *string; Ip *netip.Addr}) error`, `ListAuditEvents(ctx, ListAuditEventsParams{Entity string; EntityID int64}) ([]AuditEvent, error)`.

- [ ] **Step 1: Write the migration**

`server/migrations/00001_init.sql`:

```sql
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
```

`server/migrations/embed.go`:

```go
// Package migrations embeds the goose SQL migrations into the binary.
package migrations

import "embed"

// FS holds every *.sql migration; goose applies them in file-name order.
//
//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 2: Write the queries and the sqlc configuration**

`server/sqlc.yaml`:

```yaml
version: "2"
sql:
  - engine: "postgresql"
    schema: "migrations"
    queries: "internal/db/queries"
    gen:
      go:
        package: "db"
        out: "internal/db"
        sql_package: "pgx/v5"
        emit_pointers_for_null_types: true
        overrides:
          - db_type: "timestamptz"
            go_type: "time.Time"
          - db_type: "timestamptz"
            go_type:
              type: "time.Time"
              pointer: true
            nullable: true
```

`server/internal/db/generate.go`:

```go
// Package db holds the sqlc-generated queries. Edit queries/*.sql, then run `go generate ./...`.
package db

//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate -f ../../sqlc.yaml
```

`server/internal/db/queries/users.sql`:

```sql
-- name: CreateUser :one
INSERT INTO users (email, name, is_admin, locale, timezone)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower(sqlc.arg('email'));

-- name: ListUsers :many
SELECT * FROM users ORDER BY lower(name), id;

-- name: UpdateUser :one
UPDATE users SET
  name     = coalesce(sqlc.narg('name'), name),
  is_admin = coalesce(sqlc.narg('is_admin'), is_admin),
  locale   = coalesce(sqlc.narg('locale'), locale),
  timezone = coalesce(sqlc.narg('timezone'), timezone)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetDisabled :one
UPDATE users
SET disabled_at = CASE WHEN sqlc.arg('disabled')::boolean THEN coalesce(disabled_at, now()) ELSE NULL END
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetPasswordHash :exec
UPDATE users
SET password_hash = sqlc.narg('password_hash'), failed_logins = 0, failed_since = NULL, locked_until = NULL
WHERE id = sqlc.arg('id');

-- name: RecordLoginFailure :exec
-- Counts failures inside a 15-minute window; the 5th locks the account for 15 minutes (FSD §15.1).
-- Every expression in an UPDATE reads the row's old values.
UPDATE users SET
  failed_logins = CASE WHEN failed_since IS NULL OR failed_since < now() - interval '15 minutes'
                       THEN 1 ELSE failed_logins + 1 END,
  failed_since  = CASE WHEN failed_since IS NULL OR failed_since < now() - interval '15 minutes'
                       THEN now() ELSE failed_since END,
  locked_until  = CASE WHEN failed_since IS NOT NULL AND failed_since >= now() - interval '15 minutes'
                            AND failed_logins + 1 >= 5
                       THEN now() + interval '15 minutes' ELSE locked_until END
WHERE id = $1;

-- name: RecordLoginSuccess :exec
UPDATE users SET failed_logins = 0, failed_since = NULL, locked_until = NULL, last_login_at = now()
WHERE id = $1;
```

`server/internal/db/queries/sessions.sql`:

```sql
-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at, ip, user_agent)
VALUES ($1, $2, $3, $4, $5);

-- name: GetSession :one
SELECT sqlc.embed(u), s.last_seen_at, s.expires_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now() WHERE token_hash = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: DeleteOtherSessions :exec
DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2;
```

`server/internal/db/queries/setup_tokens.sql`:

```sql
-- name: CreateSetupToken :exec
INSERT INTO setup_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: VoidSetupTokens :exec
UPDATE setup_tokens SET used_at = now() WHERE user_id = $1 AND used_at IS NULL;

-- name: UseSetupToken :one
UPDATE setup_tokens SET used_at = now()
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
RETURNING user_id;
```

`server/internal/db/queries/audit.sql`:

```sql
-- name: InsertAuditEvent :exec
INSERT INTO audit_events (actor_id, via, entity, entity_id, action, changes, request_id, ip)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListAuditEvents :many
SELECT * FROM audit_events WHERE entity = $1 AND entity_id = $2 ORDER BY id;
```

- [ ] **Step 3: Generate the query code**

```bash
cd server
go get github.com/jackc/pgx/v5@v5.11.0
go generate ./internal/db/
grep -n "func (q \*Queries) GetSession(ctx context.Context, tokenHash \[\]byte) (GetSessionRow, error)" internal/db/sessions.sql.go
```

Expected: the grep prints one line. If sqlc names a field differently from the Interfaces block above, use the generated name everywhere later and note it in the commit message.

- [ ] **Step 4: Write the failing migration test**

`server/internal/migrate/migrate_test.go`:

```go
package migrate_test

import (
	"context"
	"testing"

	"github.com/muasal/muasal/server/internal/migrate"
	"github.com/muasal/muasal/server/internal/testdb"
)

func TestUpIsIdempotentAndTheAuditLogIsAppendOnly(t *testing.T) {
	d := testdb.New(t) // runs migrate.Up once
	ctx := context.Background()
	if err := migrate.Up(ctx, d.OwnerURL, d.AppURL); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO audit_events (via, entity, entity_id, action) VALUES ('system', 'test', 1, 'create')`); err != nil {
		t.Fatalf("the app role must insert audit events: %v", err)
	}
	if _, err := d.Pool.Exec(ctx, `DELETE FROM audit_events`); err == nil {
		t.Fatal("the app role must not delete audit events")
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE audit_events SET action = 'changed'`); err == nil {
		t.Fatal("the app role must not update audit events")
	}
}
```

- [ ] **Step 5: Run it to verify it fails**

Run: `cd server && go test ./internal/migrate/`
Expected: FAIL to compile: `package github.com/muasal/muasal/server/internal/testdb is not in std` (or "no required module provides package").

- [ ] **Step 6: Implement the migration runner and the test-database helper**

`server/internal/migrate/migrate.go`:

```go
// Package migrate applies the embedded schema and prepares the least-privilege
// role that the app connects as (FSD §16, §19.1).
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver for goose
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/muasal/muasal/server/migrations"
)

// Up applies pending migrations as the owner role under an advisory lock, then
// creates or updates the app role from appURL's credentials and grants it data
// access. Running it again is harmless.
func Up(ctx context.Context, ownerURL, appURL string) error {
	sqlDB, err := sql.Open("pgx", ownerURL)
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS, goose.WithSessionLocker(locker))
	if err != nil {
		return err
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	conn, err := pgx.Connect(ctx, ownerURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	return ensureAppRole(ctx, conn, appURL)
}

func ensureAppRole(ctx context.Context, conn *pgx.Conn, appURL string) error {
	u, err := url.Parse(appURL)
	if err != nil {
		return err
	}
	role := u.User.Username()
	password, _ := u.User.Password()
	if role == "" || password == "" {
		return errors.New("DATABASE_URL must carry the app role's user name and password")
	}
	ident := pgx.Identifier{role}.Sanitize()
	literal := "'" + strings.ReplaceAll(password, "'", "''") + "'"
	_, err = conn.Exec(ctx, "CREATE ROLE "+ident+" LOGIN PASSWORD "+literal)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42710" { // duplicate_object: the role exists already
		_, err = conn.Exec(ctx, "ALTER ROLE "+ident+" LOGIN PASSWORD "+literal)
	}
	if err != nil {
		return fmt.Errorf("app role: %w", err)
	}
	for _, stmt := range []string{
		"GRANT USAGE ON SCHEMA public TO " + ident,
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO " + ident,
		"GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO " + ident,
		"REVOKE UPDATE, DELETE, TRUNCATE ON audit_events FROM " + ident, // append-only (FSD §8.7)
		"REVOKE ALL ON goose_db_version FROM " + ident,
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}
```

`server/internal/testdb/testdb.go`:

```go
// Package testdb gives integration tests a fresh, migrated PostgreSQL database.
package testdb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/muasal/muasal/server/internal/migrate"
)

// DB is a throwaway database that is dropped when the test ends.
type DB struct {
	OwnerURL string        // owner role: runs migrations
	AppURL   string        // least-privilege app role
	Pool     *pgxpool.Pool // connected as the app role
}

// New creates a database on the server named by TEST_DATABASE_URL (a role that
// may CREATE DATABASE), migrates it and connects as the app role. Without
// TEST_DATABASE_URL the test is skipped; `make testdb` starts a server.
func New(t *testing.T) DB {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set; run `make testdb`")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	name := fmt.Sprintf("muasal_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	d := DB{OwnerURL: withDatabase(base, name)}
	d.AppURL = withUser(d.OwnerURL, "app", "app-test-password")
	t.Cleanup(func() {
		if d.Pool != nil {
			d.Pool.Close()
		}
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		_ = admin.Close(context.Background())
	})
	if err := migrate.Up(ctx, d.OwnerURL, d.AppURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if d.Pool, err = pgxpool.New(ctx, d.AppURL); err != nil {
		t.Fatal(err)
	}
	return d
}

func withDatabase(raw, name string) string {
	u, _ := url.Parse(raw)
	u.Path = "/" + name
	return u.String()
}

func withUser(raw, user, password string) string {
	u, _ := url.Parse(raw)
	u.User = url.UserPassword(user, password)
	return u.String()
}
```

- [ ] **Step 7: Start a test database and run the test**

```bash
make testdb
cd server && go get github.com/pressly/goose/v3@v3.28.0 && go mod tidy
TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./internal/migrate/ -v
```

Expected: `--- PASS: TestUpIsIdempotentAndTheAuditLogIsAppendOnly` and `ok`. If `make testdb` fails because the container already runs, skip it.

- [ ] **Step 8: Commit**

```bash
git add server
git commit -m "feat(db): schema, embedded migrations, app role and sqlc queries"
```

---

### Task 4: Password, token and rate-limit primitives

**Files:**
- Create: `server/internal/auth/password.go`, `policy.go`, `token.go`, `limiter.go`
- Create (downloaded): `server/internal/auth/common-passwords.txt`, `server/internal/auth/common-passwords.LICENSE`
- Test: `server/internal/auth/password_test.go`, `policy_test.go`, `token_test.go`, `limiter_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `auth.HashPassword(password string) string`, `auth.CheckPassword(hash, password string) bool`, `auth.CheckPolicy(password string) error` returning `auth.ErrPasswordTooShort` (text `password_too_short`) or `auth.ErrPasswordTooCommon` (text `password_too_common`), `auth.NewToken() (token string, hash []byte)`, `auth.HashToken(token string) []byte`, `auth.NewLimiter(limit int, window time.Duration) *auth.Limiter`, `(*auth.Limiter).Allow(key string) bool`.

- [ ] **Step 1: Fetch the common-password list**

Keep only entries of 12 or more characters, since shorter ones already fail the length rule:

```bash
curl -fsSL https://raw.githubusercontent.com/danielmiessler/SecLists/master/Passwords/Common-Credentials/xato-net-10-million-passwords-100000.txt \
  | tr -d '\r' | awk 'length($0) >= 12' > server/internal/auth/common-passwords.txt
curl -fsSL https://raw.githubusercontent.com/danielmiessler/SecLists/master/LICENSE -o server/internal/auth/common-passwords.LICENSE
wc -l < server/internal/auth/common-passwords.txt
grep -x '1qaz2wsx3edc' server/internal/auth/common-passwords.txt
```

Expected: about 488 lines, and the grep prints `1qaz2wsx3edc`.

- [ ] **Step 2: Write the failing tests**

`server/internal/auth/password_test.go`:

```go
package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	h := HashPassword("correct horse battery staple")
	if !CheckPassword(h, "correct horse battery staple") {
		t.Fatal("the right password was rejected")
	}
	if CheckPassword(h, "Correct horse battery staple") {
		t.Fatal("a wrong password was accepted")
	}
	if HashPassword("same") == HashPassword("same") {
		t.Fatal("hashes must be salted")
	}
	for _, bad := range []string{"", "plain", "$argon2id$v=19$m=1,t=1,p=1$$", "$bcrypt$x$y$z$w"} {
		if CheckPassword(bad, "x") {
			t.Fatalf("malformed hash %q accepted", bad)
		}
	}
}
```

`server/internal/auth/policy_test.go`:

```go
package auth

import (
	"errors"
	"testing"
)

func TestCheckPolicy(t *testing.T) {
	cases := []struct {
		password string
		want     error
	}{
		{"short", ErrPasswordTooShort},
		{"elevenchars", ErrPasswordTooShort},   // 11 characters
		{"1qaz2wsx3edc", ErrPasswordTooCommon}, // in the SecLists top 100,000
		{"1QAZ2WSX3EDC", ErrPasswordTooCommon}, // changing case does not help
		{"kopi-susu-di-kantor-7", nil},
	}
	for _, c := range cases {
		if got := CheckPolicy(c.password); !errors.Is(got, c.want) {
			t.Errorf("CheckPolicy(%q) = %v, want %v", c.password, got, c.want)
		}
	}
	if len(common) < 400 {
		t.Fatalf("the common list looks truncated: %d entries", len(common))
	}
}
```

`server/internal/auth/token_test.go`:

```go
package auth

import (
	"bytes"
	"testing"
)

func TestNewToken(t *testing.T) {
	a, hashA := NewToken()
	b, _ := NewToken()
	if a == b || len(a) != 43 {
		t.Fatalf("tokens %q and %q", a, b)
	}
	if !bytes.Equal(hashA, HashToken(a)) || len(hashA) != 32 {
		t.Fatal("the hash must be the SHA-256 of the token")
	}
}
```

`server/internal/auth/limiter_test.go`:

```go
package auth

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	clock := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	l := NewLimiter(20, time.Minute)
	l.now = func() time.Time { return clock }
	for i := 1; i <= 20; i++ {
		if !l.Allow("10.0.0.1") {
			t.Fatalf("attempt %d blocked", i)
		}
	}
	if l.Allow("10.0.0.1") {
		t.Fatal("the 21st attempt in one minute was allowed")
	}
	if !l.Allow("10.0.0.2") {
		t.Fatal("other addresses must not be affected")
	}
	clock = clock.Add(time.Minute)
	if !l.Allow("10.0.0.1") {
		t.Fatal("a new window must reset the count")
	}
}
```

- [ ] **Step 3: Run them to verify they fail**

Run: `cd server && go test ./internal/auth/`
Expected: FAIL with `undefined: HashPassword`, `undefined: CheckPolicy`, `undefined: NewToken`, `undefined: NewLimiter`.

- [ ] **Step 4: Implement the primitives**

`server/internal/auth/password.go`:

```go
// Package auth holds the sign-in building blocks: password hashing and policy,
// random tokens and the per-IP rate limiter (FSD §15.1, §18.2).
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id at the OWASP minimum: 19 MiB of memory, 2 passes, 1 lane.
const (
	argonMemory  = 19 * 1024
	argonPasses  = 2
	argonThreads = 1
	argonKeyLen  = 32
)

// HashPassword returns an argon2id hash in PHC string format.
func HashPassword(password string) string {
	salt := make([]byte, 16)
	rand.Read(salt) // never fails since Go 1.24
	key := argon2.IDKey([]byte(password), salt, argonPasses, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonPasses, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key))
}

// CheckPassword reports whether password matches hash. The parameters come
// from the hash itself, so raising the constants later keeps old hashes valid.
func CheckPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	var memory, passes uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil {
		return false
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, passes, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
```

`server/internal/auth/policy.go`:

```go
package auth

import (
	_ "embed"
	"errors"
	"strings"
	"unicode/utf8"
)

// MinPasswordLength is the FSD §15.1 minimum.
const MinPasswordLength = 12

// Policy errors; their text is the API error code the UI translates.
var (
	ErrPasswordTooShort  = errors.New("password_too_short")
	ErrPasswordTooCommon = errors.New("password_too_common")
)

// commonPasswords holds the 12+ character entries of SecLists'
// xato-net-10-million-passwords-100000.txt (MIT; see common-passwords.LICENSE).
//
//go:embed common-passwords.txt
var commonPasswords string

var common = func() map[string]bool {
	m := map[string]bool{}
	for _, line := range strings.Split(commonPasswords, "\n") {
		if s := strings.ToLower(strings.TrimSpace(line)); s != "" {
			m[s] = true
		}
	}
	return m
}()

// CheckPolicy returns nil for an acceptable password.
func CheckPolicy(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if common[strings.ToLower(password)] {
		return ErrPasswordTooCommon
	}
	return nil
}
```

`server/internal/auth/token.go`:

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// NewToken returns a 256-bit random token for cookies and links, plus its
// SHA-256 hash, which is the only form the database stores.
func NewToken() (token string, hash []byte) {
	b := make([]byte, 32)
	rand.Read(b) // never fails since Go 1.24
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken maps a presented token to its stored hash.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
```

`server/internal/auth/limiter.go`:

```go
package auth

import (
	"sync"
	"time"
)

// Limiter allows at most limit events per key within each fixed window.
// ponytail: in memory and per process; move it to PostgreSQL if the API ever runs as several processes.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	start  time.Time
	counts map[string]int
	now    func() time.Time
}

// NewLimiter returns a limiter on the wall clock.
func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, counts: map[string]int{}, now: time.Now}
}

// Allow records one event for key and reports whether it is within the limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now := l.now(); now.Sub(l.start) >= l.window {
		l.start, l.counts = now, map[string]int{}
	}
	l.counts[key]++
	return l.counts[key] <= l.limit
}
```

- [ ] **Step 5: Run the tests to verify they pass**

```bash
cd server && go get golang.org/x/crypto@v0.57.0 && go mod tidy && go test ./internal/auth/ -v
```

Expected: `PASS` for `TestHashAndCheckPassword`, `TestCheckPolicy`, `TestNewToken` and `TestLimiter`.

- [ ] **Step 6: Commit**

```bash
git add server
git commit -m "feat(auth): argon2id hashing, password policy, tokens, IP limiter"
```

---

### Task 5: API server core: sessions, sign-in, sign-out, /me

**Files:**
- Create: `server/internal/httpapi/server.go`, `middleware.go`, `auth_handlers.go`
- Create: `server/internal/httpapi/users.go` (answers 501 until Task 6 replaces it)
- Test: `server/internal/httpapi/auth_test.go`

**Interfaces:**
- Consumes: `config.Config` (Task 1); generated `ServerInterface`, models and `writeJSON`/`writeProblem`/`decodeJSON` (Task 2); package `db` and `testdb` (Task 3); package `auth` (Task 4).
- Produces: `httpapi.New(cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) *httpapi.Server`, `(*Server).Handler() http.Handler` (serves `/api/v1/*`, `GET /healthz`, `GET /readyz`); unexported helpers used by Task 6: `(s *Server) inTx(ctx, func(q *db.Queries) error) error`, `audit(ctx, q, auditMeta, actorID *int64, entity string, entityID int64, action string, changes any) error`, `webMeta(r) auditMeta`, `systemMeta`, `(s *Server) requireUser(w, r) *db.User`, `(s *Server) requireAdmin(w, r) *db.User`, `currentSessionHash(r) []byte`, `toAPIUser(db.User) User`, `(s *Server) fail(w, r, err)`, `ptr[T](v T) *T`. Test helpers in package `httpapi_test`: `newEnv(t) *env` with fields `url string`, `q *db.Queries`, `d testdb.DB`, `api *httpapi.Server` and methods `client()`, `call(c, method, path string, body, out any) int`, `seedUser(email, password string, admin bool) db.User`; function `login(e, c, email, password) (int, httpapi.Problem)`; constants `origin` and `pw`.

- [ ] **Step 1: Write the failing integration tests**

`server/internal/httpapi/auth_test.go`:

```go
package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/muasal/muasal/server/internal/auth"
	"github.com/muasal/muasal/server/internal/config"
	"github.com/muasal/muasal/server/internal/db"
	"github.com/muasal/muasal/server/internal/httpapi"
	"github.com/muasal/muasal/server/internal/testdb"
)

const (
	origin = "http://muasal.test"
	pw     = "kopi-susu-di-kantor-7"
)

type env struct {
	t   *testing.T
	url string
	q   *db.Queries
	d   testdb.DB
	api *httpapi.Server
}

func newEnv(t *testing.T) *env {
	d := testdb.New(t)
	cfg := config.Config{DatabaseURL: d.AppURL, PublicURL: origin, ListenAddr: ":0"}
	api := httpapi.New(cfg, d.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return &env{t: t, url: srv.URL, q: db.New(d.Pool), d: d, api: api}
}

// client is a separate browser with its own cookie jar.
func (e *env) client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

// call sends JSON from the app's origin and decodes the JSON reply into out when out is not nil.
func (e *env) call(c *http.Client, method, path string, body, out any) int {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.url+"/api/v1"+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

// seedUser creates a user who already has a password.
func (e *env) seedUser(email, password string, admin bool) db.User {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.q.CreateUser(ctx, db.CreateUserParams{Email: email, Name: email, IsAdmin: admin, Locale: "id", Timezone: "Asia/Jakarta"})
	if err != nil {
		e.t.Fatal(err)
	}
	h := auth.HashPassword(password)
	if err := e.q.SetPasswordHash(ctx, db.SetPasswordHashParams{ID: u.ID, PasswordHash: &h}); err != nil {
		e.t.Fatal(err)
	}
	return u
}

func login(e *env, c *http.Client, email, password string) (int, httpapi.Problem) {
	var p httpapi.Problem
	code := e.call(c, http.MethodPost, "/auth/login", map[string]string{"email": email, "password": password}, &p)
	return code, p
}

func TestLoginStartsASessionAndIsAudited(t *testing.T) {
	e := newEnv(t)
	u := e.seedUser("budi@example.com", pw, false)
	c := e.client()
	if code, _ := login(e, c, "Budi@Example.com", pw); code != http.StatusOK {
		t.Fatalf("login status %d", code)
	}
	var me httpapi.User
	if code := e.call(c, http.MethodGet, "/me", nil, &me); code != http.StatusOK || me.Email != "budi@example.com" || me.LastLoginAt == nil {
		t.Fatalf("me: %d %+v", code, me)
	}
	events, err := e.q.ListAuditEvents(context.Background(), db.ListAuditEventsParams{Entity: "user", EntityID: u.ID})
	if err != nil || len(events) != 1 || events[0].Action != "login" {
		t.Fatalf("audit: %+v %v", events, err)
	}
}

func TestSessionCookieFlags(t *testing.T) {
	e := newEnv(t)
	e.seedUser("rina@example.com", pw, false)
	b, _ := json.Marshal(map[string]string{"email": "rina@example.com", "password": pw})
	req, _ := http.NewRequest(http.MethodPost, e.url+"/api/v1/auth/login", bytes.NewReader(b))
	req.Header.Set("Origin", origin)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	var sid *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "sid" {
			sid = c
		}
	}
	if sid == nil || !sid.HttpOnly || sid.SameSite != http.SameSiteLaxMode || len(sid.Value) != 43 {
		t.Fatalf("bad session cookie: %+v", sid)
	}
}

func TestWrongPasswordIsRejected(t *testing.T) {
	e := newEnv(t)
	e.seedUser("budi@example.com", pw, false)
	code, p := login(e, e.client(), "budi@example.com", "wrong-password-123")
	if code != http.StatusUnauthorized || p.Code != "invalid_credentials" {
		t.Fatalf("got %d %s", code, p.Code)
	}
}

// AC-AD-2: after five wrong passwords, even the right one is refused.
func TestFiveFailuresLockTheAccount(t *testing.T) {
	e := newEnv(t)
	e.seedUser("budi@example.com", pw, false)
	c := e.client()
	for i := 1; i <= 5; i++ {
		if code, _ := login(e, c, "budi@example.com", "wrong-password-123"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i, code)
		}
	}
	if code, p := login(e, c, "budi@example.com", pw); code != http.StatusTooManyRequests || p.Code != "account_locked" {
		t.Fatalf("got %d %s", code, p.Code)
	}
}

func TestTwentyOneAttemptsFromOneIPAreRateLimited(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	for i := 0; i < 20; i++ {
		login(e, c, "nobody@example.com", "whatever-password")
	}
	if code, p := login(e, c, "nobody@example.com", "whatever-password"); code != http.StatusTooManyRequests || p.Code != "rate_limited" {
		t.Fatalf("got %d %s", code, p.Code)
	}
}

func TestWritesNeedTheAppOrigin(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest(http.MethodPost, e.url+"/api/v1/auth/login", strings.NewReader(`{"email":"a@b.c","password":"x"}`))
	req.Header.Set("Origin", "https://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("got %d", res.StatusCode)
	}
}

func TestLogoutRevokesTheSession(t *testing.T) {
	e := newEnv(t)
	e.seedUser("budi@example.com", pw, false)
	c := e.client()
	login(e, c, "budi@example.com", pw)
	u, _ := url.Parse(e.url)
	stolen := c.Jar.Cookies(u)
	if code := e.call(c, http.MethodPost, "/auth/logout", nil, nil); code != http.StatusNoContent {
		t.Fatalf("logout %d", code)
	}
	replay := e.client()
	replay.Jar.SetCookies(u, stolen)
	if code := e.call(replay, http.MethodGet, "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("the old cookie still works: %d", code)
	}
}

func TestIdleSessionExpires(t *testing.T) {
	e := newEnv(t)
	e.seedUser("budi@example.com", pw, false)
	c := e.client()
	login(e, c, "budi@example.com", pw)
	if _, err := e.d.Pool.Exec(context.Background(), `UPDATE sessions SET last_seen_at = now() - interval '13 hours'`); err != nil {
		t.Fatal(err)
	}
	if code := e.call(c, http.MethodGet, "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("got %d", code)
	}
}

func TestHealthChecks(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		res, err := http.Get(e.url + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("%s: %d", path, res.StatusCode)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd server && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./internal/httpapi/`
Expected: FAIL to compile with `undefined: httpapi.New`.

- [ ] **Step 3: Implement the server core**

`server/internal/httpapi/server.go`:

```go
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/muasal/muasal/server/internal/auth"
	"github.com/muasal/muasal/server/internal/config"
	"github.com/muasal/muasal/server/internal/db"
)

// Server implements the generated ServerInterface.
type Server struct {
	cfg     config.Config
	pool    *pgxpool.Pool
	q       *db.Queries
	ipLimit *auth.Limiter
	log     *slog.Logger
	now     func() time.Time
}

// New wires a Server; it opens no connections of its own.
func New(cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) *Server {
	return &Server{
		cfg:     cfg,
		pool:    pool,
		q:       db.New(pool),
		ipLimit: auth.NewLimiter(20, time.Minute), // FSD §15.1: 20 sign-in attempts per IP per minute
		log:     log,
		now:     time.Now,
	}
}

// Handler serves the API under /api/v1 plus unauthenticated health checks.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /readyz", s.readyz)
	HandlerWithOptions(s, StdHTTPServerOptions{
		BaseURL:    "/api/v1",
		BaseRouter: mux,
		// The last middleware runs first: the Origin check, then the session lookup.
		Middlewares: []MiddlewareFunc{s.authenticate, s.requireOrigin},
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeProblem(w, http.StatusBadRequest, "invalid_parameter", err.Error())
		},
	})
	return s.requestContext(mux)
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if err := s.pool.Ping(r.Context()); err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "database_unavailable", "The database is not reachable")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// inTx runs fn in one transaction, so a change and its audit event commit together (FSD §4.2).
func (s *Server) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op after Commit
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// auditMeta records where a change came from.
type auditMeta struct {
	via       string
	requestID *string
	ip        *netip.Addr
}

var systemMeta = auditMeta{via: "system"}

func webMeta(r *http.Request) auditMeta {
	return auditMeta{via: "web", requestID: ptr(requestIDFrom(r.Context())), ip: ipAddr(r)}
}

// audit appends one event. changes must never contain secrets.
func audit(ctx context.Context, q *db.Queries, m auditMeta, actorID *int64, entity string, entityID int64, action string, changes any) error {
	if changes == nil {
		changes = map[string]any{}
	}
	b, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	return q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		ActorID: actorID, Via: m.via, Entity: entity, EntityID: entityID,
		Action: action, Changes: b, RequestID: m.requestID, Ip: m.ip,
	})
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("request failed", "request_id", requestIDFrom(r.Context()), "err", err)
	writeProblem(w, http.StatusInternalServerError, "internal", "Something went wrong")
}

func ptr[T any](v T) *T { return &v }
```

`server/internal/httpapi/middleware.go`:

```go
package httpapi

import (
	"context"
	"crypto/rand"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/muasal/muasal/server/internal/auth"
	"github.com/muasal/muasal/server/internal/db"
)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	userKey
	sessionHashKey
)

const (
	sessionCookie   = "sid"
	idleTimeout     = 12 * time.Hour     // FSD §17.1
	absoluteTimeout = 7 * 24 * time.Hour // FSD §17.1
)

// statusWriter records the status code for the access log.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach Flush for streaming responses later.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// requestContext gives every request an ID, logs it, recovers panics and sets
// headers that every API response needs.
func (s *Server) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := rand.Text()
		w.Header().Set("X-Request-Id", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		defer func() {
			if p := recover(); p != nil {
				s.log.Error("panic", "request_id", id, "panic", p)
				writeProblem(sw, http.StatusInternalServerError, "internal", "Something went wrong")
			}
			s.log.Info("request", "request_id", id, "method", r.Method, "path", r.URL.Path,
				"status", sw.status, "ms", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// authenticate attaches the signed-in user when the sid cookie names a live
// session. Handlers decide whether a user is required.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil {
			hash := auth.HashToken(c.Value)
			if u := s.sessionUser(r.Context(), hash); u != nil {
				ctx := context.WithValue(r.Context(), userKey, u)
				r = r.WithContext(context.WithValue(ctx, sessionHashKey, hash))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// sessionUser returns the session's user, or nil when the session is unknown,
// idle for 12 hours, past its 7-day limit or owned by a disabled user.
func (s *Server) sessionUser(ctx context.Context, hash []byte) *db.User {
	row, err := s.q.GetSession(ctx, hash)
	if err != nil {
		return nil
	}
	now := s.now()
	if now.After(row.ExpiresAt) || now.Sub(row.LastSeenAt) > idleTimeout || row.User.DisabledAt != nil {
		_ = s.q.DeleteSession(ctx, hash)
		return nil
	}
	if now.Sub(row.LastSeenAt) > time.Minute { // ponytail: one write per session per minute, not per request
		_ = s.q.TouchSession(ctx, hash)
	}
	return &row.User
}

func currentUser(r *http.Request) *db.User {
	u, _ := r.Context().Value(userKey).(*db.User)
	return u
}

func currentSessionHash(r *http.Request) []byte {
	h, _ := r.Context().Value(sessionHashKey).([]byte)
	return h
}

// requireOrigin blocks cross-site writes (FSD §17.1): unsafe methods must come
// from PUBLIC_URL. Next.js server-side calls only read, so they never hit this.
func (s *Server) requireOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get("Origin") != s.cfg.PublicURL {
				writeProblem(w, http.StatusForbidden, "bad_origin", "The request origin is not allowed")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP is the address Caddy saw. Caddy replaces X-Forwarded-For sent by
// untrusted clients, and the app is reachable only through Caddy.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func ipAddr(r *http.Request) *netip.Addr {
	a, err := netip.ParseAddr(clientIP(r))
	if err != nil {
		return nil
	}
	return &a
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) *db.User {
	u := currentUser(r)
	if u == nil {
		writeProblem(w, http.StatusUnauthorized, "unauthenticated", "Sign in first")
	}
	return u
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) *db.User {
	u := s.requireUser(w, r)
	if u != nil && !u.IsAdmin {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only admins can do this")
		return nil
	}
	return u
}
```

`server/internal/httpapi/auth_handlers.go`:

```go
package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muasal/muasal/server/internal/auth"
	"github.com/muasal/muasal/server/internal/db"
)

// dummyHash makes sign-in for an unknown email as slow as for a known one.
var dummyHash = auth.HashPassword("muasal-timing-equalizer")

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var in LoginRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if !s.ipLimit.Allow(clientIP(r)) {
		writeProblem(w, http.StatusTooManyRequests, "rate_limited", "Too many sign-in attempts from this address; wait a minute")
		return
	}
	ctx := r.Context()
	u, err := s.q.GetUserByEmail(ctx, strings.TrimSpace(in.Email))
	if errors.Is(err, pgx.ErrNoRows) {
		auth.CheckPassword(dummyHash, in.Password)
		writeProblem(w, http.StatusUnauthorized, "invalid_credentials", "Email or password is wrong")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if u.LockedUntil != nil && u.LockedUntil.After(s.now()) {
		writeProblem(w, http.StatusTooManyRequests, "account_locked", "Too many attempts, try again in 15 minutes")
		return
	}
	if u.DisabledAt != nil || u.PasswordHash == nil || !auth.CheckPassword(*u.PasswordHash, in.Password) {
		err := s.inTx(ctx, func(q *db.Queries) error {
			if err := q.RecordLoginFailure(ctx, u.ID); err != nil {
				return err
			}
			return audit(ctx, q, webMeta(r), nil, "user", u.ID, "login_failed", nil)
		})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeProblem(w, http.StatusUnauthorized, "invalid_credentials", "Email or password is wrong")
		return
	}
	token, hash := auth.NewToken()
	expires := s.now().Add(absoluteTimeout)
	err = s.inTx(ctx, func(q *db.Queries) error {
		if old, err := r.Cookie(sessionCookie); err == nil { // the new session replaces this browser's old one
			if err := q.DeleteSession(ctx, auth.HashToken(old.Value)); err != nil {
				return err
			}
		}
		if err := q.RecordLoginSuccess(ctx, u.ID); err != nil {
			return err
		}
		if err := q.CreateSession(ctx, db.CreateSessionParams{
			TokenHash: hash, UserID: u.ID, ExpiresAt: expires, Ip: ipAddr(r), UserAgent: ptr(r.UserAgent()),
		}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &u.ID, "user", u.ID, "login", nil)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setSessionCookie(w, token, expires)
	u.LastLoginAt = ptr(s.now())
	writeJSON(w, http.StatusOK, toAPIUser(u))
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	if hash := currentSessionHash(r); hash != nil {
		u := currentUser(r)
		ctx := r.Context()
		err := s.inTx(ctx, func(q *db.Queries) error {
			if err := q.DeleteSession(ctx, hash); err != nil {
				return err
			}
			return audit(ctx, q, webMeta(r), &u.ID, "user", u.ID, "logout", nil)
		})
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.setSessionCookie(w, "", time.Unix(0, 0))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GetMe(w http.ResponseWriter, r *http.Request) {
	if u := s.requireUser(w, r); u != nil {
		writeJSON(w, http.StatusOK, toAPIUser(*u))
	}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode,
	})
}

func toAPIUser(u db.User) User {
	return User{
		Id: u.ID, Email: u.Email, Name: u.Name, IsAdmin: u.IsAdmin,
		Locale: Locale(u.Locale), Timezone: u.Timezone,
		Disabled: u.DisabledAt != nil, HasPassword: u.PasswordHash != nil,
		CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt,
	}
}
```

`server/internal/httpapi/users.go` (Task 6 replaces this whole file):

```go
package httpapi

import "net/http"

// The generated ServerInterface needs every operation; user administration lands in the next task.
func (s *Server) SetupPassword(w http.ResponseWriter, r *http.Request)             { notImplemented(w) }
func (s *Server) UpdateMe(w http.ResponseWriter, r *http.Request)                  { notImplemented(w) }
func (s *Server) ListUsers(w http.ResponseWriter, r *http.Request)                 { notImplemented(w) }
func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request)                { notImplemented(w) }
func (s *Server) UpdateUser(w http.ResponseWriter, r *http.Request, id int64)      { notImplemented(w) }
func (s *Server) CreateSetupLink(w http.ResponseWriter, r *http.Request, id int64) { notImplemented(w) }

func notImplemented(w http.ResponseWriter) {
	writeProblem(w, http.StatusNotImplemented, "not_implemented", "Not implemented yet")
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd server && go mod tidy && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./internal/httpapi/ -v`
Expected: every test in `auth_test.go` and `problem_test.go` passes.

- [ ] **Step 5: Commit**

```bash
git add server
git commit -m "feat(api): sessions, sign-in with lockout, sign-out, /me"
```

---

### Task 6: User administration, setup links and profile

**Files:**
- Replace: `server/internal/httpapi/users.go`
- Test: `server/internal/httpapi/users_test.go`

**Interfaces:**
- Consumes: the helpers and test harness from Task 5; `db` queries from Task 3; `auth` from Task 4.
- Produces: working `ListUsers`, `CreateUser`, `UpdateUser`, `CreateSetupLink`, `SetupPassword`, `UpdateMe`; `(*Server).CreateAdmin(ctx context.Context, email, name string) (setupURL string, err error)` for the CLI in Task 7. API error codes the web translates: `validation_failed` (with field codes `required`, `invalid`, `password_too_short`, `password_too_common`, `wrong_password`, `email_taken`), `email_taken`, `setup_link_invalid`, `cannot_change_self`, `not_found`, `forbidden`, `unauthenticated`.

- [ ] **Step 1: Write the failing tests**

`server/internal/httpapi/users_test.go`:

```go
package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/muasal/muasal/server/internal/httpapi"
)

func setupToken(t *testing.T, link string) string {
	t.Helper()
	i := strings.LastIndex(link, "/setup/")
	if i < 0 {
		t.Fatalf("not a setup link: %q", link)
	}
	return link[i+len("/setup/"):]
}

// The Iteration 0 exit check at API level.
func TestAdminCreatesAUserWhoSetsAPasswordAndSignsIn(t *testing.T) {
	e := newEnv(t)
	e.seedUser("admin@example.com", pw, true)
	admin := e.client()
	if code, _ := login(e, admin, "admin@example.com", pw); code != http.StatusOK {
		t.Fatalf("admin login %d", code)
	}
	var created httpapi.CreatedUser
	code := e.call(admin, http.MethodPost, "/admin/users", map[string]any{"email": "budi@example.com", "name": "Budi"}, &created)
	if code != http.StatusCreated || created.User.HasPassword || !strings.HasPrefix(created.SetupLink.Url, origin+"/setup/") {
		t.Fatalf("create: %d %+v", code, created)
	}
	token := setupToken(t, created.SetupLink.Url)
	budi := e.client()
	if code := e.call(budi, http.MethodPost, "/auth/setup", map[string]string{"token": token, "password": "nasi-goreng-pedas-99"}, nil); code != http.StatusNoContent {
		t.Fatalf("setup %d", code)
	}
	if code, _ := login(e, budi, "budi@example.com", "nasi-goreng-pedas-99"); code != http.StatusOK {
		t.Fatalf("budi login %d", code)
	}
	var p httpapi.Problem
	if code := e.call(budi, http.MethodPost, "/auth/setup", map[string]string{"token": token, "password": "nasi-goreng-pedas-99"}, &p); code != http.StatusGone || p.Code != "setup_link_invalid" {
		t.Fatalf("second use: %d %s", code, p.Code)
	}
	var list httpapi.UserList
	if code := e.call(admin, http.MethodGet, "/admin/users", nil, &list); code != http.StatusOK || len(list.Items) != 2 {
		t.Fatalf("list: %d %+v", code, list)
	}
}

func TestDuplicateEmailIsRefused(t *testing.T) {
	e := newEnv(t)
	e.seedUser("admin@example.com", pw, true)
	admin := e.client()
	login(e, admin, "admin@example.com", pw)
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPost, "/admin/users", map[string]any{"email": "ADMIN@example.com", "name": "Twin"}, &p); code != http.StatusConflict || p.Code != "email_taken" {
		t.Fatalf("got %d %s", code, p.Code)
	}
}

// AC-AD-3: a setup link older than 72 hours no longer works.
func TestExpiredSetupLinkIsRefused(t *testing.T) {
	e := newEnv(t)
	link, err := e.api.CreateAdmin(context.Background(), "admin@example.com", "Admin")
	if err != nil || !strings.HasPrefix(link, origin+"/setup/") {
		t.Fatalf("create admin: %q %v", link, err)
	}
	if _, err := e.d.Pool.Exec(context.Background(), `UPDATE setup_tokens SET expires_at = now() - interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	var p httpapi.Problem
	code := e.call(e.client(), http.MethodPost, "/auth/setup", map[string]string{"token": setupToken(t, link), "password": "nasi-goreng-pedas-99"}, &p)
	if code != http.StatusGone || p.Code != "setup_link_invalid" {
		t.Fatalf("got %d %s", code, p.Code)
	}
}

func TestSetupRejectsWeakPasswords(t *testing.T) {
	e := newEnv(t)
	link, err := e.api.CreateAdmin(context.Background(), "admin@example.com", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	for password, want := range map[string]string{"short": "password_too_short", "1qaz2wsx3edc": "password_too_common"} {
		var p httpapi.Problem
		code := e.call(e.client(), http.MethodPost, "/auth/setup", map[string]string{"token": setupToken(t, link), "password": password}, &p)
		if code != http.StatusUnprocessableEntity || p.Errors == nil || (*p.Errors)[0].Code != want {
			t.Errorf("%q: got %d %+v", password, code, p)
		}
	}
}

func TestNonAdminsCannotManageUsers(t *testing.T) {
	e := newEnv(t)
	e.seedUser("budi@example.com", pw, false)
	c := e.client()
	login(e, c, "budi@example.com", pw)
	if code := e.call(c, http.MethodGet, "/admin/users", nil, nil); code != http.StatusForbidden {
		t.Fatalf("got %d", code)
	}
}

// AC-AD-1: disabling a user ends their session and blocks sign-in.
func TestDisablingAUserSignsThemOut(t *testing.T) {
	e := newEnv(t)
	adminUser := e.seedUser("admin@example.com", pw, true)
	budiUser := e.seedUser("budi@example.com", pw, false)
	admin, budi := e.client(), e.client()
	login(e, admin, "admin@example.com", pw)
	login(e, budi, "budi@example.com", pw)
	var updated httpapi.User
	if code := e.call(admin, http.MethodPatch, fmt.Sprintf("/admin/users/%d", budiUser.ID), map[string]bool{"disabled": true}, &updated); code != http.StatusOK || !updated.Disabled {
		t.Fatalf("disable: %d %+v", code, updated)
	}
	if code := e.call(budi, http.MethodGet, "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("the disabled user is still signed in: %d", code)
	}
	if code, _ := login(e, budi, "budi@example.com", pw); code != http.StatusUnauthorized {
		t.Fatalf("the disabled user signed in again: %d", code)
	}
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPatch, fmt.Sprintf("/admin/users/%d", adminUser.ID), map[string]bool{"disabled": true}, &p); code != http.StatusUnprocessableEntity || p.Code != "cannot_change_self" {
		t.Fatalf("self-disable: %d %s", code, p.Code)
	}
}

func TestResetPasswordEndsSessionsAndIssuesANewLink(t *testing.T) {
	e := newEnv(t)
	e.seedUser("admin@example.com", pw, true)
	budiUser := e.seedUser("budi@example.com", pw, false)
	admin, budi := e.client(), e.client()
	login(e, admin, "admin@example.com", pw)
	login(e, budi, "budi@example.com", pw)
	var link httpapi.SetupLink
	if code := e.call(admin, http.MethodPost, fmt.Sprintf("/admin/users/%d/setup-link", budiUser.ID), nil, &link); code != http.StatusCreated || link.Url == "" {
		t.Fatalf("reset: %d %+v", code, link)
	}
	if code := e.call(budi, http.MethodGet, "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("session survived the reset: %d", code)
	}
	if code, _ := login(e, e.client(), "budi@example.com", pw); code != http.StatusUnauthorized {
		t.Fatalf("the old password still works: %d", code)
	}
}

func TestChangingPasswordNeedsTheCurrentOne(t *testing.T) {
	e := newEnv(t)
	e.seedUser("budi@example.com", pw, false)
	c := e.client()
	login(e, c, "budi@example.com", pw)
	var p httpapi.Problem
	if code := e.call(c, http.MethodPatch, "/me", map[string]string{"current_password": "not-the-password", "new_password": "teh-manis-dingin-42"}, &p); code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong current password: %d", code)
	}
	var me httpapi.User
	if code := e.call(c, http.MethodPatch, "/me", map[string]string{"current_password": pw, "new_password": "teh-manis-dingin-42", "locale": "en"}, &me); code != http.StatusOK || me.Locale != "en" {
		t.Fatalf("change: %d %+v", code, me)
	}
	if code := e.call(c, http.MethodGet, "/me", nil, nil); code != http.StatusOK {
		t.Fatalf("the current session must survive: %d", code)
	}
	if code, _ := login(e, e.client(), "budi@example.com", "teh-manis-dingin-42"); code != http.StatusOK {
		t.Fatalf("the new password is rejected: %d", code)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd server && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./internal/httpapi/`
Expected: FAIL to compile with `e.api.CreateAdmin undefined`.

- [ ] **Step 3: Implement user administration**

Replace `server/internal/httpapi/users.go` with:

```go
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/muasal/muasal/server/internal/auth"
	"github.com/muasal/muasal/server/internal/db"
)

const setupLinkTTL = 72 * time.Hour // FSD §15.1

func (s *Server) ListUsers(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	rows, err := s.q.ListUsers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]User, len(rows))
	for i, u := range rows {
		items[i] = toAPIUser(u)
	}
	writeJSON(w, http.StatusOK, UserList{Items: items})
}

func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}
	var in UserCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	email := strings.TrimSpace(in.Email)
	fields := validateProfile(&in.Name, in.Locale, in.Timezone)
	if !validEmail(email) {
		fields = append(fields, FieldError{Field: "email", Code: "invalid", Message: "Enter a valid email address"})
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	params := db.CreateUserParams{
		Email: email, Name: strings.TrimSpace(in.Name), IsAdmin: in.IsAdmin != nil && *in.IsAdmin,
		Locale: "id", Timezone: "Asia/Jakarta",
	}
	if in.Locale != nil {
		params.Locale = string(*in.Locale)
	}
	if in.Timezone != nil {
		params.Timezone = *in.Timezone
	}
	ctx := r.Context()
	var out CreatedUser
	err := s.inTx(ctx, func(q *db.Queries) error {
		u, err := q.CreateUser(ctx, params)
		if err != nil {
			return err
		}
		link, err := s.issueSetupLink(ctx, q, u.ID)
		if err != nil {
			return err
		}
		out = CreatedUser{User: toAPIUser(u), SetupLink: link}
		return audit(ctx, q, webMeta(r), &admin.ID, "user", u.ID, "create",
			map[string]any{"email": params.Email, "name": params.Name, "is_admin": params.IsAdmin})
	})
	if isUniqueViolation(err) {
		writeProblem(w, http.StatusConflict, "email_taken", "A user with this email already exists",
			FieldError{Field: "email", Code: "email_taken", Message: "A user with this email already exists"})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) UpdateUser(w http.ResponseWriter, r *http.Request, id int64) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}
	var in UserUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	if id == admin.ID && ((in.Disabled != nil && *in.Disabled) || (in.IsAdmin != nil && !*in.IsAdmin)) {
		writeProblem(w, http.StatusUnprocessableEntity, "cannot_change_self", "You cannot disable yourself or remove your own admin role")
		return
	}
	if fields := validateProfile(in.Name, in.Locale, in.Timezone); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	ctx := r.Context()
	var updated db.User
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if updated, err = q.UpdateUser(ctx, db.UpdateUserParams{
			ID: id, Name: trimmed(in.Name), IsAdmin: in.IsAdmin, Locale: localeString(in.Locale), Timezone: in.Timezone,
		}); err != nil {
			return err
		}
		if in.Disabled != nil {
			if updated, err = q.SetDisabled(ctx, db.SetDisabledParams{ID: id, Disabled: *in.Disabled}); err != nil {
				return err
			}
			if *in.Disabled { // AC-AD-1: a disabled user is signed out everywhere at once
				if err := q.DeleteUserSessions(ctx, id); err != nil {
					return err
				}
			}
		}
		return audit(ctx, q, webMeta(r), &admin.ID, "user", id, "update", in)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIUser(updated))
}

// CreateSetupLink resets a password: the old one stops working, every session
// ends, and a new one-time link is returned (FSD §15.1).
func (s *Server) CreateSetupLink(w http.ResponseWriter, r *http.Request, id int64) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}
	ctx := r.Context()
	var link SetupLink
	err := s.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.GetUserByID(ctx, id); err != nil {
			return err
		}
		if err := q.SetPasswordHash(ctx, db.SetPasswordHashParams{ID: id, PasswordHash: nil}); err != nil {
			return err
		}
		if err := q.DeleteUserSessions(ctx, id); err != nil {
			return err
		}
		var err error
		if link, err = s.issueSetupLink(ctx, q, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &admin.ID, "user", id, "reset_password", nil)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

// SetupPassword redeems a one-time setup link.
func (s *Server) SetupPassword(w http.ResponseWriter, r *http.Request) {
	var in SetupRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := auth.CheckPolicy(in.Password); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", passwordField("password", err))
		return
	}
	hash := auth.HashPassword(in.Password)
	ctx := r.Context()
	err := s.inTx(ctx, func(q *db.Queries) error {
		userID, err := q.UseSetupToken(ctx, auth.HashToken(in.Token))
		if err != nil {
			return err
		}
		if err := q.SetPasswordHash(ctx, db.SetPasswordHashParams{ID: userID, PasswordHash: &hash}); err != nil {
			return err
		}
		if err := q.DeleteUserSessions(ctx, userID); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &userID, "user", userID, "set_password", nil)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusGone, "setup_link_invalid", "This link has expired or was already used. Ask your admin for a new one.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateMe edits the signed-in user's own profile and password.
func (s *Server) UpdateMe(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	var in MeUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	fields := validateProfile(in.Name, in.Locale, in.Timezone)
	var newHash *string
	if in.NewPassword != nil {
		policyErr := auth.CheckPolicy(*in.NewPassword)
		switch {
		case in.CurrentPassword == nil || u.PasswordHash == nil || !auth.CheckPassword(*u.PasswordHash, *in.CurrentPassword):
			fields = append(fields, FieldError{Field: "current_password", Code: "wrong_password", Message: "The current password is wrong"})
		case policyErr != nil:
			fields = append(fields, passwordField("new_password", policyErr))
		default:
			newHash = ptr(auth.HashPassword(*in.NewPassword))
		}
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	ctx := r.Context()
	var updated db.User
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if updated, err = q.UpdateUser(ctx, db.UpdateUserParams{
			ID: u.ID, Name: trimmed(in.Name), Locale: localeString(in.Locale), Timezone: in.Timezone,
		}); err != nil {
			return err
		}
		changes := map[string]any{"name": in.Name, "locale": in.Locale, "timezone": in.Timezone}
		if newHash != nil {
			if err := q.SetPasswordHash(ctx, db.SetPasswordHashParams{ID: u.ID, PasswordHash: newHash}); err != nil {
				return err
			}
			// Other devices sign out; this one stays signed in (FSD §18.2).
			if err := q.DeleteOtherSessions(ctx, db.DeleteOtherSessionsParams{UserID: u.ID, TokenHash: currentSessionHash(r)}); err != nil {
				return err
			}
			changes["password"] = "changed"
		}
		return audit(ctx, q, webMeta(r), &u.ID, "user", u.ID, "update_profile", changes)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIUser(updated))
}

// CreateAdmin makes an admin from the CLI and returns their setup link.
func (s *Server) CreateAdmin(ctx context.Context, email, name string) (string, error) {
	email, name = strings.TrimSpace(email), strings.TrimSpace(name)
	if !validEmail(email) || name == "" {
		return "", errors.New("a valid --email and a non-empty --name are required")
	}
	var link SetupLink
	err := s.inTx(ctx, func(q *db.Queries) error {
		u, err := q.CreateUser(ctx, db.CreateUserParams{Email: email, Name: name, IsAdmin: true, Locale: "id", Timezone: "Asia/Jakarta"})
		if err != nil {
			return err
		}
		if link, err = s.issueSetupLink(ctx, q, u.ID); err != nil {
			return err
		}
		return audit(ctx, q, systemMeta, nil, "user", u.ID, "create", map[string]any{"email": email, "name": name, "is_admin": true})
	})
	if isUniqueViolation(err) {
		return "", fmt.Errorf("a user with email %s already exists", email)
	}
	return link.Url, err
}

// issueSetupLink voids the user's older links and returns a new one.
func (s *Server) issueSetupLink(ctx context.Context, q *db.Queries, userID int64) (SetupLink, error) {
	token, hash := auth.NewToken()
	expires := s.now().Add(setupLinkTTL)
	if err := q.VoidSetupTokens(ctx, userID); err != nil {
		return SetupLink{}, err
	}
	if err := q.CreateSetupToken(ctx, db.CreateSetupTokenParams{TokenHash: hash, UserID: userID, ExpiresAt: expires}); err != nil {
		return SetupLink{}, err
	}
	return SetupLink{Url: s.cfg.PublicURL + "/setup/" + token, ExpiresAt: expires}, nil
}

func validateProfile(name *string, locale *Locale, timezone *string) []FieldError {
	var f []FieldError
	if name != nil {
		if n := strings.TrimSpace(*name); n == "" || len(n) > 200 {
			f = append(f, FieldError{Field: "name", Code: "required", Message: "Enter a name of at most 200 characters"})
		}
	}
	if locale != nil && *locale != "id" && *locale != "en" {
		f = append(f, FieldError{Field: "locale", Code: "invalid", Message: "Choose id or en"})
	}
	if timezone != nil {
		if _, err := time.LoadLocation(*timezone); err != nil || *timezone == "" {
			f = append(f, FieldError{Field: "timezone", Code: "invalid", Message: "Unknown timezone"})
		}
	}
	return f
}

func validEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && len(s) <= 320
}

func passwordField(field string, err error) FieldError {
	msg := "Use at least 12 characters"
	if errors.Is(err, auth.ErrPasswordTooCommon) {
		msg = "This password is too common; choose another"
	}
	return FieldError{Field: field, Code: err.Error(), Message: msg}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func trimmed(p *string) *string {
	if p == nil {
		return nil
	}
	return ptr(strings.TrimSpace(*p))
}

func localeString(l *Locale) *string {
	if l == nil {
		return nil
	}
	return ptr(string(*l))
}
```

- [ ] **Step 4: Run all Go tests to verify they pass**

Run: `make test`
Expected: `ok` for `internal/auth`, `internal/config`, `internal/httpapi` and `internal/migrate`.

- [ ] **Step 5: Commit**

```bash
git add server
git commit -m "feat(api): user administration, setup links, profile and password change"
```

---

### Task 7: The `app` binary and its image

**Files:**
- Create: `server/cmd/app/main.go`
- Test: `server/cmd/app/main_test.go`
- Create: `server/Dockerfile`

**Interfaces:**
- Consumes: `config.Load` (Task 1), `migrate.Up` (Task 3), `httpapi.New`, `(*Server).Handler`, `(*Server).CreateAdmin` (Tasks 5–6).
- Produces: the commands `app serve`, `app migrate up`, `app admin create-admin --email E --name N` (prints the setup URL on its own line) and `app healthcheck` (exit 0 when `GET /readyz` on `LISTEN_ADDR` returns 204); image entrypoint `/app` with default command `serve`.

- [ ] **Step 1: Write the failing tests**

`server/cmd/app/main_test.go`:

```go
package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muasal/muasal/server/internal/config"
)

func TestRunWithoutACommandPrintsUsage(t *testing.T) {
	err := run(context.Background(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("got %v", err)
	}
}

func TestHealthcheck(t *testing.T) {
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/readyz" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ready.Close()
	if err := healthcheck(config.Config{ListenAddr: strings.TrimPrefix(ready.URL, "http://")}); err != nil {
		t.Fatal(err)
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer down.Close()
	if err := healthcheck(config.Config{ListenAddr: strings.TrimPrefix(down.URL, "http://")}); err == nil {
		t.Fatal("want an error when the API is not ready")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd server && go test ./cmd/app/`
Expected: FAIL with `undefined: run` and `undefined: healthcheck`.

- [ ] **Step 3: Implement the command**

`server/cmd/app/main.go`:

```go
// Command app is Muasal's single server binary (FSD §4).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // timezone names also work in the distroless image

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/muasal/muasal/server/internal/config"
	"github.com/muasal/muasal/server/internal/httpapi"
	"github.com/muasal/muasal/server/internal/migrate"
)

const usage = `usage:
  app serve                                  run the API (migrates first when MIGRATE_DATABASE_URL is set)
  app migrate up                             apply migrations and prepare the app database role
  app admin create-admin --email E --name N  create an admin and print a one-time setup link
  app healthcheck                            exit 0 when the API on LISTEN_ADDR is ready`

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(context.Background(), os.Args[1:], log); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, log *slog.Logger) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	switch {
	case args[0] == "serve":
		return serve(ctx, cfg, log)
	case args[0] == "healthcheck":
		return healthcheck(cfg)
	case len(args) >= 2 && args[0] == "migrate" && args[1] == "up":
		if cfg.MigrateDatabaseURL == "" {
			return errors.New("MIGRATE_DATABASE_URL is required for migrate")
		}
		return migrate.Up(ctx, cfg.MigrateDatabaseURL, cfg.DatabaseURL)
	case len(args) >= 2 && args[0] == "admin" && args[1] == "create-admin":
		return createAdmin(ctx, cfg, log, args[2:])
	}
	return errors.New(usage)
}

func serve(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.MigrateDatabaseURL != "" {
		if err := migrate.Up(ctx, cfg.MigrateDatabaseURL, cfg.DatabaseURL); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: httpapi.New(cfg, pool, log).Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "addr", cfg.ListenAddr)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

func createAdmin(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	email := fs.String("email", "", "admin email address (required)")
	name := fs.String("name", "", "admin display name (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	link, err := httpapi.New(cfg, pool, log).CreateAdmin(ctx, *email, *name)
	if err != nil {
		return err
	}
	fmt.Printf("Admin %s created. Open this link within 72 hours to set the password:\n%s\n", *email, link)
	return nil
}

// healthcheck lets the distroless image report readiness without curl.
func healthcheck(cfg config.Config) error {
	addr := cfg.ListenAddr
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	res, err := http.Get("http://" + addr + "/readyz")
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("readyz answered %s", res.Status)
	}
	return nil
}
```

`server/Dockerfile`:

```dockerfile
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/app

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app"]
CMD ["serve"]
```

- [ ] **Step 4: Run the tests and build the image**

```bash
cd server && go test ./cmd/app/ && go vet ./...
docker build -t muasal-app:dev .
docker run --rm muasal-app:dev; echo "exit=$?"
```

Expected: `ok` for the tests, a clean `go vet`, a built image, and the container printing `error: DATABASE_URL is required` plus the PUBLIC_URL message, then `exit=1`.

- [ ] **Step 5: Commit**

```bash
git add server
git commit -m "feat(server): app binary with serve, migrate, create-admin, healthcheck; image"
```

---

### Task 8: Web app scaffold

**Files:**
- Create: `web/package.json`, `web/tsconfig.json`, `web/next.config.ts`, `web/postcss.config.mjs`, `web/proxy.ts`
- Create: `web/i18n/request.ts`, `web/messages/en.json`, `web/messages/id.json`
- Generate: `web/lib/api-types.ts`
- Create: `web/lib/api.ts`, `web/lib/server-api.ts`, `web/lib/problem.ts`
- Create: `web/app/globals.css`, `web/app/layout.tsx`, `web/app/page.tsx`, `web/app/SignOutButton.tsx`

**Interfaces:**
- Consumes: `api/openapi.yaml` (Task 2).
- Produces: `api` (browser client for `/api/v1`) from `@/lib/api`; `serverApi()` and `getMe()` from `@/lib/server-api`; types `User` and `Problem` and function `problemKey(p?: Problem): string` from `@/lib/problem`; message namespaces `login`, `setup`, `home`, `profile`, `users`, `errors`; a `locale` cookie (`id` or `en`) that picks the UI language.

- [ ] **Step 1: Create the package manifest and install**

`web/package.json`:

```json
{
  "name": "muasal-web",
  "private": true,
  "scripts": {
    "dev": "next dev",
    "build": "next build",
    "gen:api": "openapi-typescript ../api/openapi.yaml -o lib/api-types.ts",
    "e2e": "playwright test"
  },
  "dependencies": {
    "next": "16.3.6",
    "next-intl": "4.14.6",
    "openapi-fetch": "0.17.0",
    "react": "19.3.0",
    "react-dom": "19.3.0"
  },
  "devDependencies": {
    "@playwright/test": "1.63.0",
    "@tailwindcss/postcss": "4.3.3",
    "@types/node": "24.13.6",
    "@types/react": "19.3.0",
    "@types/react-dom": "19.3.0",
    "openapi-typescript": "7.13.0",
    "tailwindcss": "4.3.3",
    "typescript": "5.9.3"
  }
}
```

```bash
cd web && npm install && npm run gen:api && grep -c '"/admin/users/{id}/setup-link"' lib/api-types.ts
```

Expected: `npm install` writes `package-lock.json`, and the grep prints `1`.

- [ ] **Step 2: Write the build configuration**

`web/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["dom", "dom.iterable", "esnext"],
    "skipLibCheck": true,
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "react-jsx",
    "incremental": true,
    "plugins": [{ "name": "next" }],
    "paths": { "@/*": ["./*"] }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"],
  "exclude": ["node_modules"]
}
```

`web/next.config.ts`:

```ts
import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";

const withNextIntl = createNextIntlPlugin();

const nextConfig: NextConfig = {
  output: "standalone",
  // Pin the root to web/, or a lockfile higher up (e.g. in the home folder) moves server.js inside .next/standalone.
  outputFileTracingRoot: __dirname,
  poweredByHeader: false,
  async rewrites() {
    // Only for `next dev`: send /api to a Go server on :8080. In Docker, Caddy does this.
    return process.env.NODE_ENV === "development"
      ? [{ source: "/api/:path*", destination: "http://localhost:8080/api/:path*" }]
      : [];
  },
};

export default withNextIntl(nextConfig);
```

`web/postcss.config.mjs`:

```js
export default { plugins: { "@tailwindcss/postcss": {} } };
```

`web/app/globals.css`:

```css
@import "tailwindcss";
```

- [ ] **Step 3: Add translations and the request config**

`web/i18n/request.ts`:

```ts
import { getRequestConfig } from "next-intl/server";
import { cookies } from "next/headers";

// The UI language comes from the `locale` cookie, which the app sets from the
// user's profile at sign-in (FSD §6.3). Indonesian is the default.
export default getRequestConfig(async () => {
  const store = await cookies();
  const locale = store.get("locale")?.value === "en" ? "en" : "id";
  return { locale, messages: (await import(`../messages/${locale}.json`)).default };
});
```

`web/messages/en.json`:

```json
{
  "login": {
    "title": "Sign in to Muasal",
    "email": "Email",
    "password": "Password",
    "submit": "Sign in",
    "failed": "Email or password is wrong",
    "locked": "Too many attempts, try again in 15 minutes",
    "rateLimited": "Too many sign-in attempts from this network. Wait a minute."
  },
  "setup": {
    "title": "Set your password",
    "password": "New password",
    "confirm": "Repeat the password",
    "hint": "At least 12 characters",
    "mismatch": "The passwords do not match",
    "submit": "Save password",
    "done": "Password saved. You can sign in now.",
    "toLogin": "Go to sign in"
  },
  "home": {
    "signedInAs": "Signed in as {name}",
    "profile": "Profile",
    "users": "Users",
    "signOut": "Sign out"
  },
  "profile": {
    "title": "Profile",
    "name": "Name",
    "language": "Language",
    "timezone": "Timezone",
    "passwordTitle": "Change password",
    "currentPassword": "Current password",
    "newPassword": "New password",
    "save": "Save",
    "saved": "Saved",
    "back": "Back"
  },
  "users": {
    "title": "Users",
    "name": "Name",
    "email": "Email",
    "admin": "Admin",
    "status": "Status",
    "lastLogin": "Last sign-in",
    "never": "Never",
    "active": "Active",
    "disabled": "Disabled",
    "create": "Create user",
    "linkFor": "Setup link for {name} (valid 72 hours):",
    "disable": "Disable",
    "enable": "Enable",
    "resetPassword": "Reset password",
    "adminsOnly": "Only admins can manage users.",
    "back": "Back"
  },
  "errors": {
    "generic": "Something went wrong. Try again.",
    "setup_link_invalid": "This link has expired or was already used. Ask your admin for a new one.",
    "password_too_short": "Use at least 12 characters",
    "password_too_common": "This password is too common; choose another",
    "wrong_password": "The current password is wrong",
    "email_taken": "A user with this email already exists",
    "invalid": "Check this field",
    "required": "This field is required",
    "cannot_change_self": "You cannot disable yourself or remove your own admin role"
  }
}
```

`web/messages/id.json`:

```json
{
  "login": {
    "title": "Masuk ke Muasal",
    "email": "Email",
    "password": "Kata sandi",
    "submit": "Masuk",
    "failed": "Email atau kata sandi salah",
    "locked": "Terlalu banyak percobaan, coba lagi dalam 15 menit",
    "rateLimited": "Terlalu banyak percobaan masuk dari jaringan ini. Tunggu satu menit."
  },
  "setup": {
    "title": "Atur kata sandi",
    "password": "Kata sandi baru",
    "confirm": "Ulangi kata sandi",
    "hint": "Minimal 12 karakter",
    "mismatch": "Kata sandi tidak sama",
    "submit": "Simpan kata sandi",
    "done": "Kata sandi tersimpan. Silakan masuk.",
    "toLogin": "Ke halaman masuk"
  },
  "home": {
    "signedInAs": "Masuk sebagai {name}",
    "profile": "Profil",
    "users": "Pengguna",
    "signOut": "Keluar"
  },
  "profile": {
    "title": "Profil",
    "name": "Nama",
    "language": "Bahasa",
    "timezone": "Zona waktu",
    "passwordTitle": "Ganti kata sandi",
    "currentPassword": "Kata sandi saat ini",
    "newPassword": "Kata sandi baru",
    "save": "Simpan",
    "saved": "Tersimpan",
    "back": "Kembali"
  },
  "users": {
    "title": "Pengguna",
    "name": "Nama",
    "email": "Email",
    "admin": "Admin",
    "status": "Status",
    "lastLogin": "Terakhir masuk",
    "never": "Belum pernah",
    "active": "Aktif",
    "disabled": "Nonaktif",
    "create": "Buat pengguna",
    "linkFor": "Tautan pengaturan untuk {name} (berlaku 72 jam):",
    "disable": "Nonaktifkan",
    "enable": "Aktifkan",
    "resetPassword": "Reset kata sandi",
    "adminsOnly": "Hanya admin yang dapat mengelola pengguna.",
    "back": "Kembali"
  },
  "errors": {
    "generic": "Terjadi kesalahan. Coba lagi.",
    "setup_link_invalid": "Tautan ini sudah kedaluwarsa atau sudah dipakai. Minta tautan baru ke admin.",
    "password_too_short": "Gunakan minimal 12 karakter",
    "password_too_common": "Kata sandi ini terlalu umum; pilih yang lain",
    "wrong_password": "Kata sandi saat ini salah",
    "email_taken": "Pengguna dengan email ini sudah ada",
    "invalid": "Periksa isian ini",
    "required": "Isian ini wajib diisi",
    "cannot_change_self": "Anda tidak dapat menonaktifkan diri sendiri atau mencabut peran admin Anda"
  }
}
```

- [ ] **Step 4: Write the API clients and the auth redirect**

`web/lib/api.ts`:

```ts
import createClient from "openapi-fetch";
import type { paths } from "./api-types";

// Browser client: same origin, so the session cookie and the Origin header go along by themselves.
export const api = createClient<paths>({ baseUrl: "/api/v1" });
```

`web/lib/server-api.ts`:

```ts
import createClient from "openapi-fetch";
import { cookies } from "next/headers";
import type { paths } from "./api-types";

// Server Components read through the Go API on the internal network and
// forward the session cookie. Next.js never writes data (FSD §3.1).
export async function serverApi() {
  const store = await cookies();
  const sid = store.get("sid")?.value;
  return createClient<paths>({
    baseUrl: `${process.env.API_INTERNAL_URL ?? "http://localhost:8080"}/api/v1`,
    headers: sid ? { cookie: `sid=${sid}` } : {},
  });
}

/** The signed-in user, or undefined when the session is missing or has ended. */
export async function getMe() {
  const { data } = await (await serverApi()).GET("/me");
  return data;
}
```

`web/lib/problem.ts`:

```ts
import type { components } from "./api-types";

export type User = components["schemas"]["User"];
export type Problem = components["schemas"]["Problem"];

/** The translation key for an API error: the first field error's code, else the problem code. */
export function problemKey(p?: Problem): string {
  return p?.errors?.[0]?.code ?? p?.code ?? "generic";
}
```

`web/proxy.ts`:

```ts
import { NextResponse, type NextRequest } from "next/server";

// Only checks that a session cookie exists; the Go API decides whether it is valid.
export function proxy(request: NextRequest) {
  if (!request.cookies.has("sid")) {
    return NextResponse.redirect(new URL("/login", request.url));
  }
  return NextResponse.next();
}

export const config = {
  // Everything except sign-in, password setup, the API and Next.js assets.
  matcher: ["/((?!login|setup|api|_next|favicon.ico).*)"],
};
```

- [ ] **Step 5: Write the layout and the home page**

`web/app/layout.tsx`:

```tsx
import type { Metadata } from "next";
import { NextIntlClientProvider } from "next-intl";
import { getLocale } from "next-intl/server";
import "./globals.css";

export const metadata: Metadata = { title: "Muasal" };

export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const locale = await getLocale();
  return (
    <html lang={locale}>
      <body className="min-h-screen bg-neutral-50 text-neutral-900 antialiased">
        <NextIntlClientProvider>{children}</NextIntlClientProvider>
      </body>
    </html>
  );
}
```

`web/app/page.tsx`:

```tsx
import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getMe } from "@/lib/server-api";
import SignOutButton from "./SignOutButton";

export default async function Home() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("home");
  return (
    <main className="mx-auto max-w-2xl p-8">
      <h1 className="text-2xl font-semibold">Muasal</h1>
      <p className="mt-2">{t("signedInAs", { name: me.name })}</p>
      <nav className="mt-6 flex gap-4">
        <Link className="underline" href="/settings/profile">{t("profile")}</Link>
        {me.is_admin && <Link className="underline" href="/admin/users">{t("users")}</Link>}
        <SignOutButton label={t("signOut")} />
      </nav>
    </main>
  );
}
```

`web/app/SignOutButton.tsx`:

```tsx
"use client";

import { useRouter } from "next/navigation";
import { api } from "@/lib/api";

export default function SignOutButton({ label }: { label: string }) {
  const router = useRouter();
  async function signOut() {
    await api.POST("/auth/logout");
    router.push("/login");
    router.refresh();
  }
  return (
    <button type="button" className="underline" onClick={signOut}>
      {label}
    </button>
  );
}
```

- [ ] **Step 6: Build and check the redirect and the default language**

The login page arrives in Task 9, so `/login` answers 404 for now; the redirect and the build are what this step proves.

```bash
cd web && npm run build
PORT=3100 node .next/standalone/server.js & sleep 3
curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" http://localhost:3100/
kill %1
```

Expected: the build succeeds (type check included), and curl prints `307 http://localhost:3100/login`.

- [ ] **Step 7: Commit**

```bash
git add web
git commit -m "feat(web): Next.js scaffold with i18n, typed API clients and auth redirect"
```

---

### Task 9: Sign-in, setup, profile and user-admin screens

**Files:**
- Create: `web/app/login/page.tsx`, `web/app/login/LoginForm.tsx`
- Create: `web/app/setup/[token]/page.tsx`, `web/app/setup/[token]/SetupForm.tsx`
- Create: `web/app/settings/profile/page.tsx`, `web/app/settings/profile/ProfileForm.tsx`
- Create: `web/app/admin/users/page.tsx`, `web/app/admin/users/UsersAdmin.tsx`

**Interfaces:**
- Consumes: `api`, `serverApi`, `getMe`, `problemKey`, `User`, `Problem` and the message namespaces from Task 8.
- Produces: pages `/login`, `/setup/[token]`, `/settings/profile`, `/admin/users`. Accessible names the end-to-end test relies on (Indonesian, the default language): labels "Email", "Kata sandi", "Kata sandi baru", "Ulangi kata sandi", "Nama"; buttons "Masuk", "Simpan kata sandi", "Buat pengguna"; status text "Kata sandi tersimpan. Silakan masuk."; link "Pengguna"; the created setup link inside `data-testid="setup-link"`.

- [ ] **Step 1: Sign-in screen**

`web/app/login/page.tsx`:

```tsx
import { getTranslations } from "next-intl/server";
import LoginForm from "./LoginForm";

export default async function LoginPage() {
  const t = await getTranslations("login");
  return (
    <main className="mx-auto mt-24 max-w-sm rounded-lg border bg-white p-8 shadow-sm">
      <h1 className="mb-6 text-xl font-semibold">{t("title")}</h1>
      <LoginForm />
    </main>
  );
}
```

`web/app/login/LoginForm.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";

const messageFor: Record<string, string> = { account_locked: "locked", rate_limited: "rateLimited" };

export default function LoginForm() {
  const t = useTranslations("login");
  const router = useRouter();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    const { data, error } = await api.POST("/auth/login", {
      body: { email: String(form.get("email")), password: String(form.get("password")) },
    });
    setBusy(false);
    if (error) {
      setError(t(messageFor[error.code] ?? "failed"));
      return;
    }
    document.cookie = `locale=${data.locale}; path=/; max-age=31536000; samesite=lax`;
    router.push("/");
    router.refresh();
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      <label className="flex flex-col gap-1 text-sm">
        {t("email")}
        <input name="email" type="email" required autoComplete="username" className="rounded border px-3 py-2" />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("password")}
        <input name="password" type="password" required autoComplete="current-password" className="rounded border px-3 py-2" />
      </label>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <button disabled={busy} className="rounded bg-neutral-900 px-4 py-2 text-white disabled:opacity-50">
        {t("submit")}
      </button>
    </form>
  );
}
```

- [ ] **Step 2: Password setup screen**

`web/app/setup/[token]/page.tsx`:

```tsx
import { getTranslations } from "next-intl/server";
import SetupForm from "./SetupForm";

export default async function SetupPage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  const t = await getTranslations("setup");
  return (
    <main className="mx-auto mt-24 max-w-sm rounded-lg border bg-white p-8 shadow-sm">
      <h1 className="mb-6 text-xl font-semibold">{t("title")}</h1>
      <SetupForm token={token} />
    </main>
  );
}
```

`web/app/setup/[token]/SetupForm.tsx`:

```tsx
"use client";

import Link from "next/link";
import { useState } from "react";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { problemKey } from "@/lib/problem";

export default function SetupForm({ token }: { token: string }) {
  const t = useTranslations("setup");
  const tErr = useTranslations("errors");
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const password = String(form.get("password"));
    if (password !== String(form.get("confirm"))) {
      setError(t("mismatch"));
      return;
    }
    setBusy(true);
    const { error } = await api.POST("/auth/setup", { body: { token, password } });
    setBusy(false);
    if (error) {
      const key = problemKey(error);
      setError(tErr.has(key) ? tErr(key) : tErr("generic"));
      return;
    }
    setDone(true);
  }

  if (done) {
    return (
      <div className="flex flex-col gap-4">
        <p role="status">{t("done")}</p>
        <Link className="underline" href="/login">{t("toLogin")}</Link>
      </div>
    );
  }
  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      <label className="flex flex-col gap-1 text-sm">
        {t("password")}
        <input name="password" type="password" required minLength={12} autoComplete="new-password" className="rounded border px-3 py-2" />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("confirm")}
        <input name="confirm" type="password" required minLength={12} autoComplete="new-password" className="rounded border px-3 py-2" />
      </label>
      <p className="text-xs text-neutral-500">{t("hint")}</p>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <button disabled={busy} className="rounded bg-neutral-900 px-4 py-2 text-white disabled:opacity-50">
        {t("submit")}
      </button>
    </form>
  );
}
```

- [ ] **Step 3: Profile screen**

`web/app/settings/profile/page.tsx`:

```tsx
import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getMe } from "@/lib/server-api";
import ProfileForm from "./ProfileForm";

export default async function ProfilePage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("profile");
  return (
    <main className="mx-auto max-w-md p-8">
      <Link className="text-sm underline" href="/">{t("back")}</Link>
      <h1 className="mb-6 mt-2 text-2xl font-semibold">{t("title")}</h1>
      <ProfileForm me={me} />
    </main>
  );
}
```

`web/app/settings/profile/ProfileForm.tsx`:

```tsx
"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { problemKey, type User } from "@/lib/problem";

export default function ProfileForm({ me }: { me: User }) {
  const t = useTranslations("profile");
  const tErr = useTranslations("errors");
  const router = useRouter();
  const [status, setStatus] = useState("");
  // The server renders only the current zone; the browser adds the full list, so hydration always matches.
  const [zones, setZones] = useState<string[]>([me.timezone]);
  useEffect(() => setZones(Intl.supportedValuesOf("timeZone")), []);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const newPassword = String(form.get("new_password") ?? "");
    const { data, error } = await api.PATCH("/me", {
      body: {
        name: String(form.get("name")),
        locale: form.get("locale") === "en" ? "en" : "id",
        timezone: String(form.get("timezone")),
        ...(newPassword ? { current_password: String(form.get("current_password")), new_password: newPassword } : {}),
      },
    });
    if (error) {
      const key = problemKey(error);
      setStatus(tErr.has(key) ? tErr(key) : tErr("generic"));
      return;
    }
    document.cookie = `locale=${data.locale}; path=/; max-age=31536000; samesite=lax`;
    setStatus(t("saved"));
    router.refresh();
  }

  const input = "rounded border px-3 py-2";
  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      <label className="flex flex-col gap-1 text-sm">
        {t("name")}
        <input name="name" defaultValue={me.name} required maxLength={200} className={input} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("language")}
        <select name="locale" defaultValue={me.locale} className={input}>
          <option value="id">Bahasa Indonesia</option>
          <option value="en">English</option>
        </select>
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("timezone")}
        <select name="timezone" defaultValue={me.timezone} className={input}>
          {zones.map((z) => (
            <option key={z} value={z}>{z}</option>
          ))}
        </select>
      </label>
      <fieldset className="flex flex-col gap-4 border-t pt-4">
        <legend className="text-sm font-medium">{t("passwordTitle")}</legend>
        <label className="flex flex-col gap-1 text-sm">
          {t("currentPassword")}
          <input name="current_password" type="password" autoComplete="current-password" className={input} />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          {t("newPassword")}
          <input name="new_password" type="password" minLength={12} autoComplete="new-password" className={input} />
        </label>
      </fieldset>
      {status && <p role="status" className="text-sm">{status}</p>}
      <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("save")}</button>
    </form>
  );
}
```

- [ ] **Step 4: User administration screen**

`web/app/admin/users/page.tsx`:

```tsx
import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getMe, serverApi } from "@/lib/server-api";
import UsersAdmin from "./UsersAdmin";

export default async function UsersPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("users");
  if (!me.is_admin) {
    return <main className="p-8">{t("adminsOnly")}</main>;
  }
  const { data } = await (await serverApi()).GET("/admin/users");
  return (
    <main className="mx-auto max-w-5xl p-8">
      <Link className="text-sm underline" href="/">{t("back")}</Link>
      <h1 className="mb-6 mt-2 text-2xl font-semibold">{t("title")}</h1>
      <UsersAdmin users={data?.items ?? []} meId={me.id} />
    </main>
  );
}
```

`web/app/admin/users/UsersAdmin.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { problemKey, type Problem, type User } from "@/lib/problem";

// Deterministic on server and browser, so hydration matches (profile timezones arrive in a later iteration).
function utc(iso: string) {
  return `${iso.slice(0, 16).replace("T", " ")} UTC`;
}

export default function UsersAdmin({ users, meId }: { users: User[]; meId: number }) {
  const t = useTranslations("users");
  const tErr = useTranslations("errors");
  const router = useRouter();
  const [link, setLink] = useState<{ name: string; url: string } | null>(null);
  const [error, setError] = useState("");

  function show(p: Problem) {
    const key = problemKey(p);
    setError(tErr.has(key) ? tErr(key) : tErr("generic"));
  }

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formEl = e.currentTarget;
    const form = new FormData(formEl);
    const { data, error } = await api.POST("/admin/users", {
      body: { name: String(form.get("name")), email: String(form.get("email")), is_admin: form.get("is_admin") === "on" },
    });
    if (error) return show(error);
    setError("");
    setLink({ name: data.user.name, url: data.setup_link.url });
    formEl.reset();
    router.refresh();
  }

  async function setDisabled(u: User, disabled: boolean) {
    const { error } = await api.PATCH("/admin/users/{id}", { params: { path: { id: u.id } }, body: { disabled } });
    if (error) return show(error);
    router.refresh();
  }

  async function resetPassword(u: User) {
    const { data, error } = await api.POST("/admin/users/{id}/setup-link", { params: { path: { id: u.id } } });
    if (error) return show(error);
    setLink({ name: u.name, url: data.url });
    router.refresh();
  }

  const input = "rounded border px-3 py-2";
  return (
    <div className="flex flex-col gap-6">
      <form onSubmit={create} className="flex flex-wrap items-end gap-3 rounded-lg border bg-white p-4">
        <label className="flex flex-col gap-1 text-sm">
          {t("name")}
          <input name="name" required maxLength={200} className={input} />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          {t("email")}
          <input name="email" type="email" required className={input} />
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input name="is_admin" type="checkbox" />
          {t("admin")}
        </label>
        <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("create")}</button>
      </form>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      {link && (
        <p role="status" className="rounded border border-amber-300 bg-amber-50 p-3 text-sm">
          {t("linkFor", { name: link.name })}{" "}
          <code data-testid="setup-link" className="break-all">{link.url}</code>
        </p>
      )}
      <table className="w-full border-collapse bg-white text-left text-sm">
        <thead>
          <tr className="border-b">
            <th className="p-2">{t("name")}</th>
            <th className="p-2">{t("email")}</th>
            <th className="p-2">{t("admin")}</th>
            <th className="p-2">{t("status")}</th>
            <th className="p-2">{t("lastLogin")}</th>
            <th className="p-2" />
          </tr>
        </thead>
        <tbody>
          {users.map((u) => (
            <tr key={u.id} className="border-b">
              <td className="p-2">{u.name}</td>
              <td className="p-2">{u.email}</td>
              <td className="p-2">{u.is_admin ? "✓" : ""}</td>
              <td className="p-2">{u.disabled ? t("disabled") : t("active")}</td>
              <td className="p-2">{u.last_login_at ? utc(u.last_login_at) : t("never")}</td>
              <td className="flex gap-3 p-2">
                {u.id !== meId && (
                  <>
                    <button type="button" className="underline" onClick={() => setDisabled(u, !u.disabled)}>
                      {u.disabled ? t("enable") : t("disable")}
                    </button>
                    <button type="button" className="underline" onClick={() => resetPassword(u)}>
                      {t("resetPassword")}
                    </button>
                  </>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
```

- [ ] **Step 5: Build and check that the sign-in page renders in both languages**

```bash
cd web && npm run build
PORT=3100 node .next/standalone/server.js & sleep 3
curl -s http://localhost:3100/login | grep -o "Masuk ke Muasal"
curl -s -H "Cookie: locale=en" http://localhost:3100/login | grep -o "Sign in to Muasal"
kill %1
```

Expected: the build succeeds and the two greps print `Masuk ke Muasal` and `Sign in to Muasal`.

- [ ] **Step 6: Commit**

```bash
git add web
git commit -m "feat(web): sign-in, password setup, profile and user admin screens"
```

---

### Task 10: Compose stack

**Files:**
- Create: `deploy/compose.yaml`, `deploy/Caddyfile`, `deploy/.env.example`
- Create: `web/Dockerfile`, `web/.dockerignore`

**Interfaces:**
- Consumes: `server/Dockerfile` and `app healthcheck` (Task 7); the web app (Tasks 8–9).
- Produces: `make up` serving Muasal at `http://localhost` (`/api/*` → app, everything else → web); `make admin EMAIL=… NAME=…` printing a setup link; the service names `caddy`, `web`, `app`, `db` that the end-to-end test uses.

- [ ] **Step 1: Write the web image**

`web/Dockerfile`:

```dockerfile
FROM node:24-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

FROM node:24-alpine AS build
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:24-alpine
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
USER node
EXPOSE 3000
CMD ["node", "server.js"]
```

`web/.dockerignore`:

```text
node_modules
.next
test-results
playwright-report
```

- [ ] **Step 2: Write the stack**

`deploy/compose.yaml`:

```yaml
name: muasal
services:
  caddy:
    image: caddy:2
    # Caddy listens on the port in PUBLIC_URL, so the container port matches the host port.
    ports: ["${HTTP_PORT:-80}:${HTTP_PORT:-80}"]
    environment:
      PUBLIC_URL: ${PUBLIC_URL}
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
    depends_on: [web, app]
    networks: [edge, app]
    restart: unless-stopped

  web:
    build: ../web
    image: muasal-web:${VERSION:-dev}
    environment:
      API_INTERNAL_URL: http://app:8080
      NEXT_TELEMETRY_DISABLED: "1"
    networks: [app]
    restart: unless-stopped

  app:
    build: ../server
    image: muasal-app:${VERSION:-dev}
    command: ["serve"]
    environment:
      DATABASE_URL: postgres://app:${DB_APP_PASSWORD}@db:5432/muasal?sslmode=disable
      MIGRATE_DATABASE_URL: postgres://owner:${DB_OWNER_PASSWORD}@db:5432/muasal?sslmode=disable
      PUBLIC_URL: ${PUBLIC_URL}
    healthcheck:
      test: ["CMD", "/app", "healthcheck"]
      interval: 5s
      retries: 30
    depends_on:
      db: { condition: service_healthy }
    networks: [app]
    restart: unless-stopped

  db:
    image: pgvector/pgvector:0.8.6-pg18-trixie
    environment:
      POSTGRES_DB: muasal
      POSTGRES_USER: owner
      POSTGRES_PASSWORD: ${DB_OWNER_PASSWORD}
    volumes: [pgdata:/var/lib/postgresql]
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "owner", "-d", "muasal"]
      interval: 5s
      retries: 20
    networks: [app]
    restart: unless-stopped

networks:
  edge: {}
  app: { internal: true } # no route to the internet

volumes:
  caddy_data: {}
  pgdata: {}
```

`deploy/Caddyfile`:

```text
{$PUBLIC_URL} {
	handle /api/* {
		reverse_proxy app:8080 {
			flush_interval -1
		}
	}
	handle {
		encode zstd gzip
		reverse_proxy web:3000
	}
}
```

`deploy/.env.example`:

```bash
# Copy to deploy/.env and change both passwords.
VERSION=dev
# The origin users open. With HTTP_PORT other than 80, include the port, e.g. http://localhost:8080.
PUBLIC_URL=http://localhost
HTTP_PORT=80
DB_OWNER_PASSWORD=change-me-owner
DB_APP_PASSWORD=change-me-app
```

- [ ] **Step 3: Start the stack and sign in with curl**

Run these one at a time, in order:

```bash
cp deploy/.env.example deploy/.env
make up
curl -s -o /dev/null -w "%{http_code}\n" http://localhost/login
make admin EMAIL=admin@example.com NAME="Admin"
```

Expected: `make up` ends with every container healthy or started, curl prints `200`, and `make admin` prints a line `http://localhost/setup/<token>`. Put the token part of that link into `TOKEN`:

```bash
TOKEN=<token from the printed link>
curl -s -o /dev/null -w "%{http_code}\n" -X POST http://localhost/api/v1/auth/setup \
  -H 'Origin: http://localhost' -H 'Content-Type: application/json' \
  -d "{\"token\":\"$TOKEN\",\"password\":\"a-long-demo-passphrase\"}"
curl -s -c /tmp/muasal-cookies -X POST http://localhost/api/v1/auth/login \
  -H 'Origin: http://localhost' -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"a-long-demo-passphrase"}'
curl -s -b /tmp/muasal-cookies http://localhost/api/v1/me
```

Expected: `204`, then a JSON user with `"email":"admin@example.com"` twice (login and `/me`).

- [ ] **Step 4: Commit**

```bash
git add deploy web/Dockerfile web/.dockerignore
git commit -m "feat(deploy): compose stack with Caddy, web, app and PostgreSQL"
```

---

### Task 11: End-to-end sign-in test

**Files:**
- Create: `web/playwright.config.ts`, `web/e2e/global-setup.ts`
- Test: `web/e2e/signin.spec.ts`

**Interfaces:**
- Consumes: the running stack from Task 10 (`make up`), `app admin create-admin`, the accessible names listed in Task 9.
- Produces: `make e2e`, which proves the Iteration 0 exit check in a real browser.

- [ ] **Step 1: Write the configuration and the global setup**

`web/playwright.config.ts`:

```ts
import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  globalSetup: "./e2e/global-setup.ts",
  use: { baseURL: process.env.E2E_BASE_URL ?? "http://localhost", trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
```

`web/e2e/global-setup.ts`:

```ts
import { execFileSync } from "node:child_process";

const compose = ["compose", "-f", "../deploy/compose.yaml", "--env-file", "../deploy/.env"];

// Waits for the stack, then creates a fresh admin through the CLI, as an installer would.
export default async function globalSetup() {
  const baseURL = process.env.E2E_BASE_URL ?? "http://localhost";
  const deadline = Date.now() + 60_000;
  for (;;) {
    try {
      if ((await fetch(`${baseURL}/api/v1/me`)).status === 401) break; // API and database answer
    } catch {
      // not up yet
    }
    if (Date.now() > deadline) throw new Error(`Muasal is not answering at ${baseURL}; run \`make up\` first`);
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  const email = `admin-${Date.now()}@example.com`;
  const out = execFileSync(
    "docker",
    [...compose, "exec", "-T", "app", "/app", "admin", "create-admin", "--email", email, "--name", "E2E Admin"],
    { encoding: "utf8" },
  );
  const link = out.match(/https?:\/\/\S+\/setup\/\S+/)?.[0];
  if (!link) throw new Error(`no setup link in: ${out}`);
  process.env.E2E_ADMIN_EMAIL = email;
  process.env.E2E_ADMIN_LINK = link;
}
```

- [ ] **Step 2: Write the test**

`web/e2e/signin.spec.ts`:

```ts
import { expect, test, type Page } from "@playwright/test";

// The test runs in the default UI language, Indonesian: after sign-in the UI
// follows the user's profile language, and new users start with `id`.
const adminPassword = "e2e-admin-passphrase-1";
const userPassword = "e2e-budi-passphrase-2";

async function setPassword(page: Page, link: string, password: string) {
  await page.goto(link);
  await page.getByLabel("Kata sandi baru").fill(password);
  await page.getByLabel("Ulangi kata sandi").fill(password);
  await page.getByRole("button", { name: "Simpan kata sandi" }).click();
  await expect(page.getByRole("status")).toHaveText("Kata sandi tersimpan. Silakan masuk.");
}

async function signIn(page: Page, email: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Kata sandi").fill(password);
  await page.getByRole("button", { name: "Masuk" }).click();
  await expect(page).toHaveURL(/\/$/);
}

// FSD §21 Iteration 0 exit check: a user signs in on the compose stack.
test("an admin creates a user who sets a password and signs in", async ({ page, browser }) => {
  await setPassword(page, process.env.E2E_ADMIN_LINK!, adminPassword);
  await signIn(page, process.env.E2E_ADMIN_EMAIL!, adminPassword);
  await expect(page.getByText("Masuk sebagai E2E Admin")).toBeVisible();

  await page.getByRole("link", { name: "Pengguna" }).click();
  const email = `budi-${Date.now()}@example.com`;
  await page.getByLabel("Nama").fill("Budi");
  await page.getByLabel("Email").fill(email);
  await page.getByRole("button", { name: "Buat pengguna" }).click();
  const link = await page.getByTestId("setup-link").textContent();
  expect(link).toContain("/setup/");

  const budi = await (await browser.newContext()).newPage();
  await setPassword(budi, link!, userPassword);
  await signIn(budi, email, userPassword);
  await expect(budi.getByText("Masuk sebagai Budi")).toBeVisible();
});
```

- [ ] **Step 3: Run it against the stack**

```bash
make up
cd web && npx playwright install chromium && npx playwright test
```

Expected: `1 passed`. If it fails, open the trace with `npx playwright show-trace test-results/*/trace.zip`.

- [ ] **Step 4: Commit**

```bash
git add web/playwright.config.ts web/e2e
git commit -m "test(e2e): admin creates a user who signs in on the compose stack"
```

---

### Task 12: Continuous integration

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: `go generate ./...` (Tasks 2–3), `TEST_DATABASE_URL` (Task 3), `npm run gen:api` and `npm run build` (Task 8), the stack (Task 10), the end-to-end test (Task 11).
- Produces: three required checks: `server`, `web`, `e2e`.

- [ ] **Step 1: Write the workflow**

`.github/workflows/ci.yml`:

```yaml
name: ci
on:
  push:
    branches: [main]
  pull_request:

jobs:
  server:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: pgvector/pgvector:0.8.6-pg18-trixie
        env:
          POSTGRES_USER: owner
          POSTGRES_PASSWORD: owner
        ports: ["5432:5432"]
        options: >-
          --health-cmd "pg_isready -U owner"
          --health-interval 5s --health-timeout 5s --health-retries 20
    env:
      TEST_DATABASE_URL: postgres://owner:owner@localhost:5432/postgres?sslmode=disable
    defaults:
      run:
        working-directory: server
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: "1.27.1"
          cache-dependency-path: server/go.sum
      - run: go generate ./...
      - name: Generated Go code is up to date
        run: git diff --exit-code
      - run: go vet ./...
      - run: go test ./...

  web:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-node@v7
        with:
          node-version: 24
          cache: npm
          cache-dependency-path: web/package-lock.json
      - run: npm ci
      - run: npm run gen:api
      - name: Generated API types are up to date
        run: git diff --exit-code
      - run: npm run build

  e2e:
    runs-on: ubuntu-latest
    needs: [server, web]
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-node@v7
        with:
          node-version: 24
          cache: npm
          cache-dependency-path: web/package-lock.json
      - run: cp deploy/.env.example deploy/.env
      - run: docker compose -f deploy/compose.yaml --env-file deploy/.env up -d --build --wait
      - working-directory: web
        run: npm ci && npx playwright install --with-deps chromium && npx playwright test
      - if: failure()
        run: docker compose -f deploy/compose.yaml --env-file deploy/.env logs
```

- [ ] **Step 2: Check the workflow locally, then push**

```bash
cd server && go generate ./... && git diff --exit-code && cd ..
cd web && npm run gen:api && git diff --exit-code && cd ..
```

Expected: both `git diff --exit-code` calls print nothing and exit 0, which is what CI checks. Then create the GitHub repository (the `muasal` organization handle was free on 23 Sep 2026), push, and confirm the `server`, `web` and `e2e` jobs pass.

- [ ] **Step 3: Commit**

```bash
git add .github
git commit -m "ci: Go tests with codegen drift check, web build, end-to-end test"
```

---

## Exit check (FSD §21, Iteration 0)

- `make test` passes (Go unit and integration tests).
- `make up` followed by `make e2e` passes: an admin created by the CLI sets a password, signs in, creates a user, and that user sets a password and signs in.
- CI is green on `server`, `web` and `e2e`.
