# Muasal Pilot P1b — API Tokens, Idempotency, CSV Export, Tree Import and Node Merge

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The second slice of the Pilot phase. It opens the API to scripts (FSD §14.3), makes ticket creation safe to retry (§17.1), exports the ticket list (§8.5), imports a module tree from CSV (§7.5), and folds duplicate nodes together (R-MR-6).

**How this plan was written:** each task was built on branch `feat/iteration-p1b` and committed as named below; the commits hold the code.

**Spec:** Claude Docs "FSD — Muasal" (rev 112): §7.5, §8.5, §14.3, §14.4, §17.1, §17.2, R-MR-6.

**Change to the P1 split:** ticket import from CSV and Jira (§14.2) is a slice of its own, P1b-2, next. It is as large as the rest of P1b together:
- import runs;
- column mapping with the Jira preset;
- a resumable background job;
- idempotent comments;
- a Needs-linking queue.

## Deliberate Deviations from the FSD

- **Token management:** a token cannot create or revoke tokens; that needs a browser session (403 `session_required`), so a leaked token cannot entrench itself.
- **Read-only tokens:** they may `POST /ask`, which reads and changes nothing but the Ask log.
- **Rate limit:** "60 a minute, bursting to 120" is one fixed two-minute window of 120 requests per token.
- **CSV export:** it is `GET /projects/{key}/tickets?format=csv` on the list's own filters, not a separate path. It returns at most 10,000 rows. Cells that start like a formula get a leading apostrophe.
- **Tree import:** it runs synchronously in one transaction rather than as a background job: 5,000 rows apply in well under a second. Any row error stops the whole import (AC-MR-7). A dry run returns the same plan.
- **Merging nodes:** it also moves the duplicate's live sub-nodes under the target and adds its name and aliases to the target's aliases, so questions that name the old node still find the new one.

## Tasks

### Task 1: Personal API tokens

**Commit:** `feat: personal API tokens (FSD §14.3): …`
- **Schema:** migration `00009_tokens.sql` adds `api_tokens` and `idempotency_keys`.
- **API:**
  - `GET, POST /me/tokens` and `DELETE /me/tokens/{id}`;
  - the secret is `msl_` plus 52 base32 characters, stored as a SHA-256 hash and shown once.
- **Middleware:**
  - `Authorization: Bearer msl_…` authenticates as the owner (R-AC-9).
  - A revoked or expired token, or one whose owner is disabled, gets 401 `invalid_token`.
  - A read-only token gets 403 `token_read_only` on writes (AC-IN-4).
  - 429 `rate_limited` comes past the limit.
  - Token requests skip the Origin check, and the audit log records them as `via = api`.
- **Tests:**
  - `TestTokensActAsTheirOwner`;
  - `TestReadOnlyTokensOnlyRead`;
  - `TestExpiredTokensAreRefused`;
  - `TestTokensAreRateLimited`.

### Task 2: Idempotent ticket creation

**Commit:** `feat: Idempotency-Key on ticket creation (FSD §17.1): …`
- **Header:** `Idempotency-Key` on `POST /projects/{key}/tickets`.
- **Replay:** an advisory lock on the user and key serialises retries. A key seen within 24 hours returns the first ticket (200, `Idempotent-Replayed: true`).
- **Cleanup:** the daily purge job drops old keys.
- **Test:** `TestIdempotentTicketCreation`.

### Task 3: CSV export

**Commit:** `feat: CSV export of the ticket list's filter (FSD §8.5), …`
- **Test:** `TestTicketListExportsCSV` checks the scope, the header and a formula-looking title.

### Task 4: Node merge

**Commit:** `feat: merge a duplicate node into another (R-MR-6): …`
- **API:** `POST /nodes/{id}/merge` with `{into_id}`, for project admins.
- **Refused targets:** the node itself, anything below it, archived nodes and other projects.
- **Side effects:** the tickets and notes that moved are re-indexed, and both nodes are audited.
- **Test:** `TestMergeFoldsADuplicateNode`.

### Task 5: Module-tree import

**Commit:** `feat: module-tree CSV import (FSD §7.5): …`
- **`internal/treeimport`:** `Parse` reads the path shape and the adjacency shape. `Diff` matches rows by code, else by path, ignoring case. It also finds row errors and lists the live nodes missing from the file. **Row errors:**
  - duplicate sibling, duplicate code;
  - missing parent, cycle;
  - unknown client, bad type or scope, empty or long name.
- **API:** `POST /projects/{key}/nodes/import` (multipart `file`, `dry_run`) → `NodeImportResult`.
- **Tests:**
  - `TestParsePathShape`, `TestParseAdjacencyShape`;
  - `TestDiffFlagsDuplicateSiblings` (AC-MR-7);
  - `TestDiffMatchesByCodeThenPath`;
  - `TestTreeImportPlansThenApplies`, `TestTreeImportStopsOnRowErrors`.

### Task 6: Web

**Commit:** `feat(web): API tokens page, CSV export, module-tree import with preview, node merge; e2e for all four`
- `/settings/tokens`, linked from the user menu.
- An "Export CSV" button on the ticket list.
- `/p/[key]/modules/import`, with preview, row errors, missing nodes and apply.
- A merge form on a node's Details tab.
- **E2e:** `web/e2e/tools.spec.ts`.

### Task 7: Checks

- [x] Go tests, web type check, i18n check, unit tests, licence scan and build.
- [x] Playwright: 11 of 11 pass on a fresh stack.
- [ ] CI green; merge.
