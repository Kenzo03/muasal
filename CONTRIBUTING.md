# Contributing to Muasal

Thank you for helping. Muasal is a self-hosted ticketing tool that records why every screen works the way it does. The behaviour it must have is specified in the FSD, and code comments cite its sections, such as "FSD §9.3" or "AC-DC-6".

## Report first

- **Bugs and ideas:** open an issue with what you did, what you expected and what happened. Screenshots help; remove client names.
- **Security problems:** never in an issue. Follow [SECURITY.md](SECURITY.md).
- **Larger changes:** open an issue before writing code, so we can agree on the shape.

## Set up

You need Go 1.27, Node.js 24, Docker with Compose v2, and `make`.

```sh
cp deploy/.env.example deploy/.env    # then change both passwords
make up                                # builds and starts the stack at http://localhost
make admin EMAIL=you@example.com NAME="Your Name"
```

For development:

```sh
make testdb      # throwaway PostgreSQL on localhost:55432 for the Go tests
make test        # Go unit and integration tests
make generate    # regenerate Go stubs, sqlc code and TypeScript API types
make e2e         # Playwright against a running `make up` stack
```

## How the code is laid out

- `api/openapi.yaml`: the one API contract. Change it first; `make generate` then updates the Go server interface (oapi-codegen) and the TypeScript types.
- `server/`: one Go binary.
  - `internal/httpapi` holds the handlers.
  - `internal/db/queries/*.sql` holds the SQL, which sqlc turns into Go.
  - `migrations/` holds forward-only goose migrations, each with a Down.
  - Background work runs as River jobs.
- `web/`: Next.js, which only renders; every request goes through the Go API.
  - Strings live in `messages/id.json` (the default) and `messages/en.json`.
- `deploy/`: Compose files, the installer, backups and the load test.
- `docs/`: the documentation site.

## Rules we keep

- **Visibility is enforced in SQL.** Every list, search, export, summary and Ask query filters by the user's projects and clients before anything else. A new endpoint gets a case in `internal/httpapi/permission_test.go`.
- **Tests come with the change.** Go handlers are tested through the HTTP API against a real PostgreSQL (see the existing `*_test.go`). User-visible flows get a Playwright spec in `web/e2e/`.
- **Both languages.** Every new string goes in both message files; `npm run check:i18n` fails otherwise.
- **The model is never trusted.** Model output goes through a JSON schema and server-side validation, and citations must name evidence the asker may open.
- **No outbound calls** in local or off AI mode: no telemetry, CDNs or remote fonts.
- **Licences:** dependencies must be compatible with Apache-2.0. CI runs the licence scans (`go run ./cmd/licenses` in `server`, `npm run check:licenses` in `web`) and vulnerability scans.
- **Style:**
  - Run `gofmt`; `go vet` must be clean.
  - Match the comment style of the surrounding code: short, and citing the FSD where it applies.
  - Prefer the standard library, and hold back from new dependencies.

## Pull requests

1. Branch from `main`; keep a pull request to one change.
2. Run `make generate`, `make test`, and in `web/`: `npm run check:i18n`, `npm test`, `npx tsc --noEmit`.
3. Describe what changed and why, and name the FSD sections it implements or deviates from.
4. CI must be green: server, web, e2e and the licence and vulnerability checks.

Commit messages use a short imperative subject with a type, such as `feat:`, `fix:`, `docs:` or `test:`, and a body that says why.

## Licence

Muasal is licensed under Apache-2.0. By contributing, you agree that your contribution is licensed under the same terms (inbound = outbound), as section 5 of the licence says.

## Releases

Maintainers tag `vX.Y.Z` on `main`. The release workflow then publishes signed images for amd64 and arm64 to GHCR, the offline bundle and the deploy files with signed checksums (see [docs/operations.md](docs/operations.md#get-a-release)). The documentation site deploys from `main`.
