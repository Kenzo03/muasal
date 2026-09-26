# Muasal Iteration 5a — Ask UI, Ask Log and Audit Log Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** people ask questions where they work, and system admins can see every question and every change (FSD §21 Iteration 5, first half):
- the Ask UI (§10): the `/ask` page with threads, the Ask panel from the top bar, the node page's Ask tab, "Ask about this ticket" and the Home Ask box;
- the Ask log (§10.8, §15.4) with quick filters, entry pages, and a retention job;
- the audit log screen with filters and CSV export (§15.4);
- i18n completion: CI fails on a missing translation key (§18).

Iteration 5 is split in two, like Iteration 4. This plan, 5a, is the product surface. Plan 5b holds the operations work: security hardening, system status, backups, the offline installer, and the exit check (100,000-ticket load test, offline install).

**How this plan was written:** each task was built test-first on branch `feat/iteration-5a`, in the order below, and its commit is named in the task. Unlike the 4a and 4b plans, it does not repeat the code: the commits hold it. The tests, interfaces and deliberate deviations are listed here.

**Architecture:** Same stack and patterns as Iterations 0–4.
- **Stream:** the browser posts to `/api/v1/ask` with `Accept: text/event-stream` and reads the body with a small SSE parser (`web/lib/sse.ts`). Each `scope`, `evidence`, `claim` and `result` event updates one turn. The parser is unit-tested with Node's built-in test runner (`npm test`), which runs TypeScript directly.
- **Chips:** the page presets explicit chips (project, node, client) with their names. Detected chips come named from the server: detection now returns `labels`, which it already has, so no ID-to-name lookup can reveal hidden names. Removing a detected chip re-runs the question with `ignore`, and detection leaves that chip out.
- **Threads:** `GET /ask/threads/{id}` now returns each answer's evidence, filtered to the tickets the asker may still open (`VisibleTicketIDs`), so saved answers keep their citation cards.
- **Logs:** `GET /admin/ask-log`, `GET /admin/ask-log/{id}`, `GET /admin/audit` and `GET /admin/audit/export` (CSV) are for system admins (403 otherwise), paged newest first with `before`.
- **Retention:** a daily River job, `purge_ask_log`, deletes questions older than `ASK_LOG_RETENTION_DAYS` (365 by default; 0 keeps them), then the threads left empty.

**Spec:** Claude Docs "FSD — Muasal" (rev 109): §10, §15.4, §18 (Language), §21.

## Global Constraints

- **Carried over:** everything in the Iteration 0–4 Global Constraints still holds.
- **No hints of hidden tickets:** a saved thread shows only the evidence the asker may open now, and chip labels come from the asker's own catalog.
- **Fixed wording (AC-AK-5):** "Not enough information in the tickets you can access to answer this." is a UI string in both languages; the status decides it, not the server's English message.
- **Admin only:** both logs answer 403 to anyone but a system admin; the Ask log is the only place question text is shown to anyone but its asker (§18.2).

## Deliberate Deviations from the FSD

- Iteration 5 is split into 5a (this plan) and 5b (operations), each with its own PR.
- The user adds only a date range as a chip in the Ask box; project, node and client chips come from the page (§10.1 presets) and from detection. A chip picker for clients and people can come with the pilot's feedback.
- Starting a thread keeps the live answer on screen instead of navigating to `?thread=`, so its scope chips and keyword results stay; the new thread appears in the side list.
- The Home Ask box opens the question on `/ask` rather than answering inline, so Home stays a server-rendered page.
- Feedback (thumbs), follow-ups and "Open as change summary" remain P1 (§10.7, §11.9, §12).

## Tasks

### Task 1: Detected chips carry labels

**Commit:** `feat(server): Ask log and audit log APIs, chip labels and thread evidence` (with Tasks 2–4)
- `ask.Detected.Labels []Label{Kind, ID, Label}`, built by `Detect` from the catalog; a node's label is its path ("HR › Attendance › Overtime Approval").
- API: `AskDetected.labels` (`AskLabel`).
- Test: `TestDetectLabelsItsChips` (`internal/ask`).

### Task 2: A thread shows its evidence

- `ListThreadQueries` returns `evidence`; `Engine.ItemsFor(ctx, asker, ids)` reads tickets as items, keeping only those the asker may open (nil asker: all, for admins).
- API: `AskThreadQuery.evidence`.
- Test: `TestThreadShowsTheEvidence`.

### Task 3: Ask log API

- Queries `ListAskLog` (status, slow > 30 s, user, before, limit) and `GetAskLogEntry`, with evidence and citation counts computed in SQL.
- Endpoints `GET /admin/ask-log` → `AskLogPage`, `GET /admin/ask-log/{id}` → `AskLogDetail` (scope and dropped as logged, evidence with fused scores, claims, latencies, `llm_called`).
- Test: `TestAskLogIsForSystemAdmins`: newest first, three quick filters, the entry's evidence, score and model, AC-AK-5's `llm_called = false`, 403 for members and 404 for a missing entry.

### Task 4: Audit log API

- Query `ListAudit` (actor, entity, action, since, until, before, limit), with actor name and project key; the admin's timezone turns whole days into bounds.
- Endpoints `GET /admin/audit` → `AuditPage` and `GET /admin/audit/export` → `text/csv` (at most 100,000 rows; header `id,occurred_at,actor,via,entity,entity_id,project,action,changes`).
- Test: `TestAuditLogFiltersAndExports`.

### Task 5: Ask log retention

**Commit:** `feat(server): Ask log retention, 365 days by default`
- `config.AskLogRetentionDays` from `ASK_LOG_RETENTION_DAYS` (default 365, 0 keeps; negative or non-numeric fails at start); compose passes it through, `.env.example` documents it.
- `indexer.PurgeAskLog(ctx, pool, days, now)` in one transaction; `PurgeAsk` periodic job every 24 hours on the index queue.
- Tests: `TestAskLogRetention` (config), `TestPurgeAskLog` (old questions go, a thread with a newer question stays, an emptied thread goes, 0 days keeps all).

### Task 6: Removing a detected chip

**Commit:** `feat(server): a removed detected chip stays off when the question re-runs`
- `AskRequest.ignore: AskIgnore[]` (`kind` client, node, user, contact or date; `id`); `Detected.Without(ignore)` drops them and their labels before the scope is merged and logged.
- Test: `TestRemovedDetectedChipsStayOff`.

### Task 7: The Ask UI, the log screens and the translation check

**Commit:** `feat(web): the Ask UI, the Ask log and audit screens, and a translation check in CI`
- `web/lib/sse.ts` + `sse.test.ts` (chunks split mid-event, keep-alive comments, CRLF, multi-line data); `web/lib/ask.ts` (`askStream`, `answerMarkdown`).
- `components/ask/Answer.tsx`: status line, claims with citation chips (hover card: title, client, requester, date; opens in a new tab), Sources, Also retrieved, Scope used with detected chips (dotted, "detected", removable), model badge, Copy as markdown, the waiting line, not enough information with closest tickets and suggestions, AI off with keyword results, and error messages per code.
- `components/ask/AskView.tsx`: preset chips, a date-range chip, the answer-language toggle, Enter to send, one thread per view.
- Entry points: `/ask` (threads, new, hide, presets from `?project=&node=&client=`, `?q=` from Home), `AskPanel` in the top bar, the node page's Ask tab, "Ask about this ticket", the Home Ask box.
- Admin: `/admin/ask-log` (quick filters, status, paging), `/admin/ask-log/[id]`, `/admin/audit` (filters, paging, Export CSV); top-bar tabs.
- `scripts/check-i18n.mjs`: every key in both languages, with the same ICU arguments (branch text is not an argument); CI runs it and `npm test` in the web job.
- End-to-end: `web/e2e/ask.spec.ts` with its own admin in `global-setup.ts`.

### Task 8: Final checks

- [ ] `cd server && gofmt -l . && go vet ./... && go test ./...`: no output, `ok` everywhere.
- [ ] `cd web && npm run gen:api && git diff --exit-code lib/api-types.ts && npm run check:i18n && npm test && npm run build && npx tsc --noEmit`.
- [ ] `make up && cd web && E2E_BASE_URL=http://localhost:8080 npx playwright test`: 7 passed (admin-ai, ask, decisions, registry, signin, tickets, web).
- [ ] FSD §21 Progress records 5a.
