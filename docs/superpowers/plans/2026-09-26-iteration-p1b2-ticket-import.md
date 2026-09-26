# Muasal Pilot P1b-2 — Ticket Import from CSV and Jira

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Teams bring their old tickets in once, from a CSV file or Jira's "Export Excel CSV (all fields)", so Ask has history from day one (FSD §14.2, IN-2). Imports are idempotent (R-IN-1, AC-IN-3).

**How this plan was written:** each task was built on branch `feat/iteration-p1b2` and committed as named below; the commits hold the code.

**Spec:** Claude Docs "FSD — Muasal" (rev 113): §14.2, §14.4, §16, §17.2.

## Architecture

**Migration `00010_imports.sql`:**
- `tickets.source`, `external_ref` (unique per project) and `external_meta`;
- `comments.author_id` becomes optional, with `author_label` and `external_hash`, unique per ticket;
- `import_runs`.

**`internal/ticketimport`:**
- **`Read`** streams records under a `Mapping`. It reads repeated headers (Comment, Labels, Attachment), Jira's `date;author;body` comments and Jira's date formats, comma or semicolon separated.
- **`JiraPreset`** maps Jira's columns, types (Story → Feature, Improvement → Change request, …) and priorities.
- **`Project.Resolve`** turns values into a type, priority, status (by the mapping, else by name, else Done or the default), client, and menus. Components and labels become menus by the mapping, else by name, alias or code.
- **`Plan`** is the dry run: rows, create, update, reject, the first 20 errors and module coverage.
- **`Writer.Write`** creates or updates one ticket, adds its menus, and inserts its comments on their hash (R-IN-1):
  - An unknown reporter becomes an internal contact (R-IN-4).
  - Attachment links go in a comment (R-IN-5).
- **`Run`** writes batches of 500, each in a transaction with its history and index jobs. It saves progress after every batch and resumes after the last line written.
- **`Worker`** runs `ImportTickets` in River; `indexer.Options.Register` lets `app serve` add it.

**Old keys (R-IN-2):**
- search matches `external_ref`;
- the ticket chunk says "Imported, old key PAY-332";
- Ask's evidence block shows "(old key PAY-332)".

## Deliberate Deviations from the FSD

- **The dry run** happens on upload (`POST /imports`). `POST /imports/{id}/plan` (not in §17.2) changes the mapping and dry-runs again; `POST /imports/{id}/run` then queues the job once.
- **Evidence wording:** it says "(old key PAY-332)" rather than "(Jira PAY-332)", because CSV imports come from other tools too.
- **Decision records:** imports create none. A closed imported ticket keeps its old status and close date, and skips the close dialog (R-IN-3). Its description and comments are evidence, but Muasal does not invent a confirmed decision for it.
- **The "Needs linking" queue** is the ticket list's existing Missing filter (reason or menus). The finished import links to it.
- **Rejected rows** (no key, no title, an unknown type or client, a repeated key) are skipped by the run. The dry run lists them, and a value map fixes most of them.
- **People:** reporters and comment authors match accounts by name or email. An assignee without an account stays in `external_meta`.

## Tasks

### Task 1: Schema and import engine

**Commit:** `feat: ticket import from CSV and Jira (FSD §14.2): …`
- **API:**
  - `GET, POST /imports`, `GET /imports/{id}`, `POST /imports/{id}/plan` and `POST /imports/{id}/run`, for system admins;
  - files up to 200 MB, stored under the attachments volume in `imports/`.
- **Tests:**
  - `TestJiraImportIsIdempotent` covers the dry-run counts, the rejected row, reporter mapping and the contact for an unknown author. It also checks Component/s → menu, and that a second run changes no counts (AC-IN-3).
  - `TestJiraImportThroughTheAPI` covers members being refused, a value map fixing a row, one run only, progress, and the old key in search.

### Task 2: Web

**Commit:** `feat(web): Admin → Imports: …`
- **Pages:** `/admin/imports` (upload with project and format) and `/admin/imports/[id]`:
  - dry-run figures and errors;
  - a column mapping from the file's headers, and value maps as editable lines;
  - dry-run again, then run, with a progress bar that polls every two seconds.
- **E2e:** `web/e2e/imports.spec.ts` imports a Jira export twice through the real worker.

### Task 3: Checks

- [x] Go tests, web type check, i18n check, unit tests, licence scan and build.
- [x] Playwright: 12 of 12 pass on a fresh stack.
- [ ] CI green; merge.
