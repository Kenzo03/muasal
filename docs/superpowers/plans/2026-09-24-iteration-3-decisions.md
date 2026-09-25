# Muasal Iteration 3 — Decisions, Node Pages, Search and Home Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A developer opens a menu's page and sees its history newest first, each ticket with what changed and why. This is the exit check for FSD §21 Iteration 3 (story 1). To get there:
- closing a ticket asks for the decision record in a close dialog, from the ticket page and from the board;
- every menu gets a page with a timeline and its current behaviors per client;
- search in the top bar finds tickets and menus, and a key jumps to its ticket;
- Home shows my open tickets, what changed in my projects and my projects (FSD §6.4).

**Architecture:** Same stack and patterns as Iterations 0–2, on the Terakota UI foundation (PR #4).
- **Closing:** a close is a transition. The status, the reason, the menus and a confirmed decision record commit in one transaction with their audit events, or nothing changes (R-DC-1). A reopen turns the record back into a draft (R-DC-6).
- **Decision records:** one row per ticket in `decision_records`. Their history is audit events on the ticket, so the Activity view shows every version.
- **Node pages:** they read the visible tickets on a node and, by default, on its sub-nodes. Sub-nodes are computed from the same visible tree as the ticket list's node filter.
- **Search and Home:** one SQL query per list, across every project the user belongs to. The membership check is inside the query, so a hidden row never leaves the database. Ticket text search uses a GIN expression index and titles a trigram index.
- **Recent changes:** comments, files and decision records touch their ticket's `updated_at` through triggers, so "Recently updated" is one ordered read plus the latest event or comment of ten tickets.
- **Web:** the close dialog is a native `<dialog>`, opened from the board (drop or status menu) and from the ticket page's status menu. Node pages, search and Home are server components built from the shared pieces in `lib/ui.ts`, `components/Chips.tsx`, `Icon`, `Menu` and `PageBar`, and follow the "Muasal UI design" canvas: B3Close, B3Ticket (decision card), B3Menu (timeline), B3Search and Beranda A.

**Tech Stack:** Unchanged; no new dependencies. `pg_trgm` is a contrib extension that ships with PostgreSQL. The weak-reason rule is a few lines in `web/lib/weak.ts`.

**Spec:** Claude Docs "FSD — Muasal" (rev 107):
- §9.1–9.2 and §9.5: closing and decision records.
- §7.3–7.4 and §7.8: module tree link and node page.
- §6.1–6.2 and §6.4: top bar, search, routes and Home.
- §8.4: the board's closed columns and drops.
- §8.6: ticket page.
- §5: visibility.
- §16: data model.
- §17: API, including `GET /me/tickets` and `GET /me/updates`.
- §21: delivery plan.

**Design:** the "Muasal UI design" canvas, https://claude.ai/artifact/9kgEheSwXZKimGpy9HK6TZ. Colors are the tokens in `web/app/globals.css`; class lists come from `web/lib/ui.ts`.

## Global Constraints

- **Carried over:** everything in the Iteration 0–2 Global Constraints still holds, notably:
  - "hidden and missing look the same" (404);
  - "one transaction per mutation with its audit row";
  - optimistic locking with `If-Match` and 412 `stale`.
- **Close fields (FSD §9.1):**
  - reason: 10–2,000 characters, asked for only when the ticket has none;
  - at least one menu, asked for only when the ticket has none;
  - what changed (Done) or what was decided (Cancelled): 10–1,000 characters;
  - why: 10–2,000 characters;
  - alternatives rejected: up to 2,000 characters.
  - A close through the API without them answers 422 `close_validation_failed` with field errors (R-DC-7).
- **Close prefills:**
  - What changed: the ticket title for Done, "Not implemented" for Cancelled.
  - Why: the ticket's reason.
  - A ticket that already has a record, such as a reopened one's draft, prefills all three from it instead.
- **Close dialog (AC-DC-2):** "Close ticket" stays disabled while a required field is empty, and that field says "Required". Cancel, Escape and × change nothing.
- **Outcome (R-DC-2):** Done records `implemented`, Cancelled records `rejected`. The closing user is the confirmer, with the time (R-DC-4).
- **Reopen (R-DC-6):** leaving Done or Cancelled clears `closed_at` and turns the record into a draft without a confirmer. The next close confirms it again. Earlier versions stay in the ticket's history. The ticket page shows a draft as "Draft, confirm on close" (AC-DC-3).
- **Editing a confirmed record (R-DC-5):** project admins and the confirmer only. Every edit is audited.
- **Weak text (R-DC-8):** a reason or why is weak when, after trimming, it has fewer than 20 characters, or fewer than 20 once the stock phrases are removed. The phrases are "per request", "as requested", "client request", "sesuai permintaan", "permintaan klien", "permintaan client", "request user", "ok" and "done", matched as whole words in any case. Weak text shows "Say why the client needs this, e.g. their policy or the problem it solves" and never blocks. Empty text is missing, not weak.
- **Board (FSD §8.4):** Done and Cancelled columns show tickets closed in the last 14 days, with "Show all". Dropping a card there, or choosing a closed status in its menu, opens the close dialog; cancelling it leaves the card where it was (AC-TK-2).
- **Node page timeline (FSD §7.4):**
  - Open tickets are pinned under "In progress". Closed ones follow, newest first by close date.
  - Filters: client, type, date range, and sub-nodes (included by default).
  - The first 50 entries show; "Load more" shows 50 more.
  - The empty timeline says "No tickets linked yet. Link this menu from a ticket form." (FSD §6.3).
- **Behaviors by client:** confirmed and implemented decisions, "All clients" first, then each client in the user's scope.
- **Search (FSD §6.1–6.2):**
  - The top bar's search box opens `/search?q=`.
  - Tickets match by key, by words anywhere in the ticket, or by part of the title.
  - Nodes match by part of their name, an alias or their code.
  - Results hold only what the user may open (R-AC-7), and a query that is a visible ticket's key jumps straight to the ticket.
- **Home (FSD §6.4):**
  - My tickets: open tickets (To do and In progress categories) assigned to me in every project, after the visibility predicate. Order: due date, earliest first, then priority and key; undated last.
  - Tabs All, Overdue, Next 7 days (due today through 7 days ahead) and Missing details (no reason or no menu), each with a count over all my open tickets. The tab is in the URL (`/?mine=overdue`); 50 rows a page.
  - Recently updated: the 10 visible tickets in my projects that changed last, each with its latest change as the activity sentence and a relative time. My own changes read "You".
  - My projects: key, name and my open-ticket count; each opens the board.
  - No charts on Home.
- **Top bar (FSD §6.1):** off a project page, New ticket asks which project; with one project it opens that project's form.
- **UI strings:** every string ships in Indonesian (default) and English.

## Deliberate Deviations from the FSD

**Data**
- `decision_records` leaves out `ai_drafted`, `needs_review` and `superseded_by`. They belong to AI drafts, imports and ticket links (all P1) and arrive with them.
- Ticket search uses a GIN index on the expression `to_tsvector('simple', key || ' ' || title || ' ' || reason || ' ' || description)` instead of a stored `search_tsv` column. That way ticket reads (`RETURNING *`, `sqlc.embed`) do not carry a tsvector, and the search behaves the same.
- External refs are not searchable yet, because there is no `external_ref` column until imports.
- Comments, files and decision records touch their ticket's `updated_at` (triggers in `00006_recent_changes.sql`). So the ticket list's "Updated" column and sort now move on comments too.

**Behavior**
- The weak-reason phrase list is fixed in `web/lib/weak.ts`. The admin-edited list and the weak counts in the metrics report come with the settings page and metrics (P1).
- Decision edits take no `If-Match`: the last write wins, and the history keeps every version.
- R-DC-1's index jobs and R-DC-5's re-index arrive with the indexing pipeline (Iteration 4).
- A status that tickets use keeps its kind: while tickets use it, its category can change only between To do and In progress. Removing a closed status moves its tickets to one of the same category, so decisions keep their outcome. This makes R-TK-3 hold for status edits.

**Node page, search, Home and dialog**
- The timeline's client filter is a single choice (a client, or core), not a multi-select.
- The timeline's date range filters on the entry date: the close date, or the creation date while open.
- "Load more" asks for a longer first page (50 more each time), so the URL stays shareable.
- Link badges and "Superseded by" come with ticket links (P1). "Ask about this menu", the Ask tab and Home's Ask box come with Ask (Iterations 4–5).
- The Details tab edits the node for project admins. The node's change history comes with the audit log work, together with project audit search.
- The module tree's right pane gets an "Open page" link. Its last five timeline entries and per-node ticket counts come later.
- Search returns at most 50 tickets and 20 nodes, without text snippets. Decision notes join search when they exist (P1).
- Home's menu column shows the first menu's parent and name ("Payroll › Payslip"), not the full path.
- Bug forms do not link to the Behaviors tab yet; the link comes with the create modal (Iteration 4).
- The close dialog, the node page, search and Home are covered by the end-to-end test instead of component tests, so no test framework is added.

- `main` is at d91f676 (Iteration 2 and the UI foundation merged); work on branch `feat/iteration-3`.
- `make testdb` for the Go tests; `make up` (rebuilt) for the end-to-end tests.

## File Structure

```text
server/
├── migrations/00005_decisions.sql              closed_at, decision_records, ticket search indexes (pg_trgm)
├── migrations/00006_recent_changes.sql         comments, files and decisions touch updated_at; Home indexes
├── internal/
│   ├── db/queries/{decisions,timeline,search,home}.sql (+ changes to tickets.sql, statuses.sql)
│   ├── db/decisions_test.go                    close columns and decision record rules
│   └── httpapi/
│       ├── decisions.go (+ decisions_test.go)  decision checks, history and edits
│       ├── tickets.go (+ close_test.go)        transition: close and reopen; closed tickets keep reason and menus
│       ├── statuses.go                         statuses in use keep their kind
│       ├── node_page.go (+ node_page_test.go)  node read, timeline, behaviors
│       ├── search.go (+ search_test.go)        search across projects
│       ├── home.go (+ home_test.go)            my tickets and recent changes
│       ├── ticket_list.go, nodes.go            shared paging and node helpers; closed_days
│       └── permission_test.go, seed_test.go    the suite covers decisions, node pages, search and Home
web/
├── lib/weak.ts, lib/nodes.ts, lib/activity.ts  weak text (R-DC-8), node paths, activity sentences
├── components/NodePicker.tsx                   the menu picker of the ticket form and the close dialog
├── components/CloseDialog.tsx                  the close dialog
├── app/t/[ticketKey]/DecisionCard.tsx          the decision record on the ticket page
├── app/p/[key]/board/                          closing from the board; recent closes
├── app/p/[key]/modules/[nodeId]/               the node page: Timeline, Behaviors, Details
├── app/search/page.tsx, app/TopBar.tsx         search results, the key jump and the top bar's box
├── app/page.tsx                                Home
└── e2e/decisions.spec.ts                       the Iteration 3 exit check
```

---

### Task 1: Data layer for closing, decisions and search

**Files:**
- Create: `server/migrations/00005_decisions.sql`
- Create: `server/internal/db/queries/decisions.sql`, `timeline.sql`, `search.sql`
- Modify: `server/internal/db/queries/tickets.sql` (`SetTicketStatus`, `SetTicketReason`, `ListTickets`), `statuses.sql` (`ListStatusIDsInUse`)
- Test: `server/internal/db/decisions_test.go`

**Interfaces:**
- Consumes: the Iteration 2 schema and queries.
- Produces (package `db`):
  - Models: `DecisionRecord{TicketID int64; WhatChanged, Why, Alternatives, Outcome, State string; ConfirmedBy *int64; ConfirmedAt *time.Time}`; `Ticket` gains `ClosedAt *time.Time`.
  - `SetTicketStatus(ctx, SetTicketStatusParams{StatusID int64; Closed bool; ID int64; Version *int32}) (Ticket, error)`: a close stamps `closed_at` and any other move clears it. With a `Version`, a stale one returns `pgx.ErrNoRows`.
  - `SetTicketReason(ctx, SetTicketReasonParams{ID int64; Reason string}) error`
  - `ListTicketsParams` gains `ClosedDays *int32`.
  - `ListStatusIDsInUse(ctx, projectID int64) ([]int64, error)`
  - `GetDecision(ctx, ticketID int64) (GetDecisionRow{DecisionRecord DecisionRecord; ConfirmerName *string}, error)`
  - `ConfirmDecision(ctx, ConfirmDecisionParams{TicketID int64; WhatChanged, Why, Alternatives, Outcome string; ConfirmedBy int64}) (DecisionRecord, error)`: inserts the record, or confirms an existing one again.
  - `DraftDecision(ctx, ticketID int64) error`
  - `UpdateDecision(ctx, UpdateDecisionParams{WhatChanged, Why, Alternatives string; TicketID int64}) error`
  - `ListNodeTimeline(ctx, ListNodeTimelineParams{ProjectID int64; NodeIds []int64; AllClients bool; ClientIds []int64; ClientID *int64; CoreOnly bool; Type *string; FromDate, ToDate *time.Time; Lim, Off int32}) ([]ListNodeTimelineRow, error)`. Each row carries:
    - ticket fields: `ID int64; Key, Title, Type string; ClientID *int64; ClientName *string; CreatedAt time.Time; ClosedAt *time.Time; Status Status`;
    - the requester: `RequesterContactID *int64; RequesterContactName, RequesterContactTitle *string; RequesterUserID *int64; RequesterUserName *string`;
    - the decision: `WhatChanged, Why, Alternatives, Outcome, State *string; ConfirmedBy *int64; ConfirmerName *string; ConfirmedAt *time.Time`.
  - `ListNodeBehaviors(ctx, ListNodeBehaviorsParams{ProjectID int64; NodeIds []int64; AllClients bool; ClientIds []int64}) ([]ListNodeBehaviorsRow{Key, Title string; ClientID *int64; ClientName *string; ClosedAt *time.Time; WhatChanged, Why, Alternatives string}, error)`
  - `SearchTickets(ctx, SearchTicketsParams{IsAdmin bool; UserID int64; Q string}) ([]SearchTicketsRow{Key, Title, ProjectKey string; ClientID *int64; ClientName *string; Status Status}, error)`
  - `SearchNodes(ctx, SearchNodesParams{IsAdmin bool; UserID int64; Q string}) ([]SearchNodesRow{ID int64; Name, Type string; Code *string; Aliases []string; ProjectKey string; Path []string}, error)`

- [ ] **Step 1: Write the failing test**

`server/internal/db/decisions_test.go`:

```go
package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

// R-DC-2, R-DC-4 and R-DC-6 at the database: a close stamps closed_at, a
// stale version moves nothing, a record is confirmed by a user, a reopen turns
// it back into a draft, and the constraints the handlers rely on hold.
func TestDecisionRecordsFollowTheTicket(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	u := must(q.CreateUser(ctx, db.CreateUserParams{Email: "u@example.com", Name: "U", Locale: "id", Timezone: "Asia/Jakarta"}))
	p := must(q.CreateProject(ctx, db.CreateProjectParams{Key: "HRIS", Name: "HRIS"}))
	statuses := must(q.ListStatuses(ctx, p.ID))
	todo, done := statuses[0], statuses[3]
	tk := must(q.CreateTicket(ctx, db.CreateTicketParams{
		ProjectID: p.ID, Number: must(q.NextTicketNumber(ctx, p.ID)), Key: "HRIS-1", Type: "bug", Title: "A ticket",
		StatusID: todo.ID, RequesterUserID: &u.ID, ReporterID: u.ID, Priority: "medium",
	}))

	closed := must(q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: tk.ID, StatusID: done.ID, Closed: true, Version: &tk.Version}))
	if closed.ClosedAt == nil || closed.Version != 2 {
		t.Fatalf("close: %+v", closed)
	}
	if _, err := q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: tk.ID, StatusID: todo.ID, Version: &tk.Version}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a stale version moved the ticket: %v", err)
	}
	if inUse := must(q.ListStatusIDsInUse(ctx, p.ID)); len(inUse) != 1 || inUse[0] != done.ID {
		t.Fatalf("statuses in use: %v", inUse)
	}

	rec := must(q.ConfirmDecision(ctx, db.ConfirmDecisionParams{
		TicketID: tk.ID, WhatChanged: "Skips the supervisor", Why: "Supervisors are on leave", Outcome: "implemented", ConfirmedBy: u.ID,
	}))
	if rec.State != "confirmed" || rec.ConfirmedBy == nil || *rec.ConfirmedBy != u.ID || rec.ConfirmedAt == nil {
		t.Fatalf("confirmed: %+v", rec)
	}
	check(q.DraftDecision(ctx, tk.ID))
	draft := must(q.GetDecision(ctx, tk.ID))
	if draft.DecisionRecord.State != "draft" || draft.DecisionRecord.ConfirmedBy != nil || draft.DecisionRecord.WhatChanged != "Skips the supervisor" {
		t.Fatalf("draft: %+v", draft)
	}
	again := must(q.ConfirmDecision(ctx, db.ConfirmDecisionParams{
		TicketID: tk.ID, WhatChanged: "Skips the supervisor for Client A", Why: "Supervisors are on leave", Outcome: "rejected", ConfirmedBy: u.ID,
	}))
	if again.Outcome != "rejected" || again.State != "confirmed" || again.WhatChanged != "Skips the supervisor for Client A" {
		t.Fatalf("confirmed again: %+v", again)
	}
	if row := must(q.GetDecision(ctx, tk.ID)); row.ConfirmerName == nil || *row.ConfirmerName != "U" {
		t.Fatalf("confirmer name: %+v", row)
	}

	for _, c := range []struct{ constraint, sql string }{
		{"decision_records_outcome_check", "UPDATE decision_records SET outcome = 'maybe'"},
		{"decision_records_state_check", "UPDATE decision_records SET state = 'final'"},
		{"decision_records_confirmed", "UPDATE decision_records SET confirmed_by = NULL"},
	} {
		_, err := d.Pool.Exec(ctx, c.sql)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != c.constraint {
			t.Errorf("%s: %v", c.constraint, err)
		}
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd server && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./internal/db/ -run TestDecisionRecordsFollowTheTicket`
Expected: a compile error: `db.SetTicketStatusParams` has no field `Closed`, and `ConfirmDecision` is undefined.

- [ ] **Step 3: Write the migration**

`server/migrations/00005_decisions.sql`:

```sql
-- +goose Up
-- Closing, decision records and ticket search (FSD §9, §16).
ALTER TABLE tickets ADD COLUMN closed_at timestamptz; -- set on close, cleared on reopen (FSD §8.1)
CREATE INDEX tickets_closed_idx ON tickets (project_id, closed_at DESC);

-- One record per ticket, written on close (DC-1). Client and menus come from
-- the ticket (R-DC-3). A draft has no confirmer; a confirmed record does (R-DC-4).
CREATE TABLE decision_records (
  ticket_id    bigint PRIMARY KEY REFERENCES tickets (id),
  what_changed text NOT NULL,
  why          text NOT NULL,
  alternatives text NOT NULL DEFAULT '',
  outcome      text NOT NULL CHECK (outcome IN ('implemented', 'rejected')),
  state        text NOT NULL CHECK (state IN ('draft', 'confirmed')),
  confirmed_by bigint REFERENCES users (id),
  confirmed_at timestamptz,
  CONSTRAINT decision_records_confirmed CHECK (state = 'draft' OR (confirmed_by IS NOT NULL AND confirmed_at IS NOT NULL))
);

-- Search (FSD §6.1): words anywhere in a ticket, and parts of titles.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX tickets_search_idx ON tickets USING gin (to_tsvector('simple', key || ' ' || title || ' ' || reason || ' ' || description));
CREATE INDEX tickets_title_trgm ON tickets USING gin (title gin_trgm_ops);

-- +goose Down
DROP INDEX tickets_title_trgm;
DROP INDEX tickets_search_idx;
DROP EXTENSION IF EXISTS pg_trgm;
DROP TABLE decision_records;
DROP INDEX tickets_closed_idx;
ALTER TABLE tickets DROP COLUMN closed_at;
```

- [ ] **Step 4: Write the queries**

In `server/internal/db/queries/tickets.sql`, replace `SetTicketStatus` with:

```sql
-- name: SetTicketStatus :one
-- A close stamps closed_at and any other move clears it (FSD §8.1). With a
-- version, a stale one matches no row (If-Match).
UPDATE tickets SET status_id = sqlc.arg('status_id'),
  closed_at  = CASE WHEN sqlc.arg('closed')::boolean THEN now() END,
  version    = version + 1,
  updated_at = now()
WHERE id = sqlc.arg('id') AND (sqlc.narg('version')::int IS NULL OR version = sqlc.narg('version')::int)
RETURNING *;

-- name: SetTicketReason :exec
UPDATE tickets SET reason = $2 WHERE id = $1;
```

In the same file, in `ListTickets`, add this line after the `open_only` line:

```sql
  AND (sqlc.narg('closed_days')::int IS NULL OR t.closed_at IS NULL OR t.closed_at >= now() - make_interval(days => sqlc.narg('closed_days')::int))
```

Append to `server/internal/db/queries/statuses.sql`:

```sql

-- name: ListStatusIDsInUse :many
SELECT DISTINCT status_id FROM tickets WHERE project_id = $1;
```

`server/internal/db/queries/decisions.sql`:

```sql
-- name: GetDecision :one
SELECT sqlc.embed(d), u.name AS confirmer_name
FROM decision_records d
LEFT JOIN users u ON u.id = d.confirmed_by
WHERE d.ticket_id = $1;

-- name: ConfirmDecision :one
-- A close writes the record, or confirms a reopened ticket's draft again, with
-- the closing user as confirmer (R-DC-4, R-DC-6).
INSERT INTO decision_records (ticket_id, what_changed, why, alternatives, outcome, state, confirmed_by, confirmed_at)
VALUES (sqlc.arg('ticket_id')::bigint, sqlc.arg('what_changed')::text, sqlc.arg('why')::text, sqlc.arg('alternatives')::text,
        sqlc.arg('outcome')::text, 'confirmed', sqlc.arg('confirmed_by')::bigint, now())
ON CONFLICT (ticket_id) DO UPDATE SET
  what_changed = excluded.what_changed, why = excluded.why, alternatives = excluded.alternatives,
  outcome = excluded.outcome, state = 'confirmed', confirmed_by = excluded.confirmed_by, confirmed_at = excluded.confirmed_at
RETURNING *;

-- name: DraftDecision :exec
-- A reopen turns the record back into a draft; the history keeps who confirmed it (R-DC-6).
UPDATE decision_records SET state = 'draft', confirmed_by = NULL, confirmed_at = NULL WHERE ticket_id = $1;

-- name: UpdateDecision :exec
UPDATE decision_records SET what_changed = sqlc.arg('what_changed'), why = sqlc.arg('why'), alternatives = sqlc.arg('alternatives')
WHERE ticket_id = sqlc.arg('ticket_id');
```

`server/internal/db/queries/timeline.sql`:

```sql
-- name: ListNodeTimeline :many
-- The visible tickets on these nodes (R-AC-3, R-AC-7) for a node page (FSD
-- §7.4): open ones first, newest first, then closed ones by close date, newest
-- first, each with its decision record. An entry's date is its close date, or
-- its creation date while open; from_date and to_date filter on it.
SELECT t.id, t.key, t.title, t.type, t.client_id, c.name AS client_name,
       t.requester_contact_id, rc.name AS requester_contact_name, rc.title AS requester_contact_title,
       t.requester_user_id, ru.name AS requester_user_name, t.created_at, t.closed_at, sqlc.embed(s),
       d.what_changed, d.why, d.alternatives, d.outcome, d.state, d.confirmed_by, cu.name AS confirmer_name, d.confirmed_at
FROM tickets t
JOIN statuses s ON s.id = t.status_id
LEFT JOIN clients c ON c.id = t.client_id
LEFT JOIN contacts rc ON rc.id = t.requester_contact_id
LEFT JOIN users ru ON ru.id = t.requester_user_id
LEFT JOIN decision_records d ON d.ticket_id = t.id
LEFT JOIN users cu ON cu.id = d.confirmed_by
WHERE t.project_id = sqlc.arg('project_id')
  AND EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id AND tn.node_id = ANY (sqlc.arg('node_ids')::bigint[]))
  AND (sqlc.arg('all_clients')::boolean OR t.client_id IS NULL OR t.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
  AND (sqlc.narg('client_id')::bigint IS NULL OR t.client_id = sqlc.narg('client_id')::bigint)
  AND (NOT sqlc.arg('core_only')::boolean OR t.client_id IS NULL)
  AND (sqlc.narg('type')::text IS NULL OR t.type = sqlc.narg('type')::text)
  AND (sqlc.narg('from_date')::date IS NULL OR coalesce(t.closed_at, t.created_at) >= sqlc.narg('from_date')::date)
  AND (sqlc.narg('to_date')::date IS NULL OR coalesce(t.closed_at, t.created_at) < sqlc.narg('to_date')::date + 1)
ORDER BY t.closed_at IS NOT NULL, coalesce(t.closed_at, t.created_at) DESC, t.id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: ListNodeBehaviors :many
-- The decisions in force on these nodes: confirmed and implemented, on tickets
-- the scope may see; core work first, then by client, newest first (FSD §7.4).
-- ponytail: at most 500; superseded decisions leave this list once ticket links (P1) exist.
SELECT t.key, t.title, t.client_id, c.name AS client_name, t.closed_at, d.what_changed, d.why, d.alternatives
FROM tickets t
JOIN decision_records d ON d.ticket_id = t.id
LEFT JOIN clients c ON c.id = t.client_id
WHERE t.project_id = sqlc.arg('project_id')
  AND d.state = 'confirmed' AND d.outcome = 'implemented'
  AND EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id AND tn.node_id = ANY (sqlc.arg('node_ids')::bigint[]))
  AND (sqlc.arg('all_clients')::boolean OR t.client_id IS NULL OR t.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
ORDER BY t.client_id IS NOT NULL, lower(c.name), t.client_id, t.closed_at DESC, t.id DESC
LIMIT 500;
```

`server/internal/db/queries/search.sql`:

```sql
-- name: SearchTickets :many
-- Tickets the user may see in any project (R-AC-2, R-AC-3): a key, words
-- anywhere in the ticket, or part of the title (FSD §6.1). An exact key comes
-- first, then title matches, then the latest updates.
-- ponytail: at most 50 results; pages come when people ask for them.
SELECT t.key, t.title, p.key AS project_key, t.client_id, c.name AS client_name, sqlc.embed(s)
FROM tickets t
JOIN projects p ON p.id = t.project_id
JOIN statuses s ON s.id = t.status_id
LEFT JOIN clients c ON c.id = t.client_id
WHERE (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))))
  AND (t.key = upper(sqlc.arg('q')::text)
       OR t.title ILIKE '%' || sqlc.arg('q')::text || '%'
       OR to_tsvector('simple', t.key || ' ' || t.title || ' ' || t.reason || ' ' || t.description)
          @@ websearch_to_tsquery('simple', sqlc.arg('q')::text))
ORDER BY t.key = upper(sqlc.arg('q')::text) DESC, t.title ILIKE '%' || sqlc.arg('q')::text || '%' DESC, t.updated_at DESC
LIMIT 50;

-- name: SearchNodes :many
-- Live nodes the user may see (R-AC-5) whose name, alias or code holds q, with
-- the names on their path from the top of the tree. A client-specific menu,
-- and everything under it, needs one of its clients in the user's scope.
-- UNION, not UNION ALL, so a cycle could never loop forever.
WITH RECURSIVE visible AS (
  SELECT n.id, ARRAY[n.name]::text[] AS path
  FROM nodes n
  WHERE n.parent_id IS NULL AND n.archived_at IS NULL
    AND (sqlc.arg('is_admin')::boolean OR EXISTS (
          SELECT 1 FROM memberships m
          WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = n.project_id
            AND (NOT n.client_specific OR m.all_clients OR EXISTS (
                  SELECT 1 FROM node_clients nc
                  JOIN membership_clients mc ON mc.client_id = nc.client_id AND mc.project_id = m.project_id AND mc.user_id = m.user_id
                  WHERE nc.node_id = n.id))))
  UNION
  SELECT n.id, v.path || n.name
  FROM nodes n
  JOIN visible v ON n.parent_id = v.id
  WHERE n.archived_at IS NULL
    AND (sqlc.arg('is_admin')::boolean OR EXISTS (
          SELECT 1 FROM memberships m
          WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = n.project_id
            AND (NOT n.client_specific OR m.all_clients OR EXISTS (
                  SELECT 1 FROM node_clients nc
                  JOIN membership_clients mc ON mc.client_id = nc.client_id AND mc.project_id = m.project_id AND mc.user_id = m.user_id
                  WHERE nc.node_id = n.id))))
)
SELECT n.id, n.name, n.type, n.code, n.aliases, p.key AS project_key, v.path::text[] AS path
FROM visible v
JOIN nodes n ON n.id = v.id
JOIN projects p ON p.id = n.project_id
WHERE n.name ILIKE '%' || sqlc.arg('q')::text || '%'
   OR n.code ILIKE '%' || sqlc.arg('q')::text || '%'
   OR array_to_string(n.aliases, ' ') ILIKE '%' || sqlc.arg('q')::text || '%'
ORDER BY lower(n.name) = lower(sqlc.arg('q')::text) DESC, p.key, v.path
LIMIT 20;
```

- [ ] **Step 5: Generate and check the names**

```bash
cd server && go generate ./... && go build ./... && git diff --stat internal/db
```

Expected:
- The build succeeds. Handlers that call `SetTicketStatus` with `{ID, StatusID}` still compile, and moves between open statuses keep `closed_at` NULL.
- `internal/db/models.go` has `DecisionRecord` and `Ticket.ClosedAt`.
- If sqlc names a field differently from the Interfaces block, use the generated name in every later task.

- [ ] **Step 6: Run the test**

Run: `cd server && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...`
Expected: `ok` for every package, `TestDecisionRecordsFollowTheTicket` included.

- [ ] **Step 7: Commit**

```bash
git add server/migrations/00005_decisions.sql server/internal/db
git commit -m "feat(server): decision records, closed_at and search indexes"
```

### Task 2: Statuses in use keep their kind

**Files:**
- Modify: `server/internal/httpapi/statuses.go` (`SetStatuses`, `validateStatuses`)
- Test: `server/internal/httpapi/statuses_test.go`

**Interfaces:**
- Consumes: `ListStatusIDsInUse` and `SetTicketStatus{…, Closed}` (Task 1).
- Produces: `PUT /projects/{key}/statuses` refuses a category change of a status in use that opens or closes its tickets, or flips Done and Cancelled. That answers 422 with field code `status_category_in_use` on `statuses[i].category`. `move_to` from a closed status must target the same category.

- [ ] **Step 1: Write the failing test**

Append to `server/internal/httpapi/statuses_test.go`:

```go
// R-TK-3 for status edits: a status that tickets use cannot open or close them
// by changing its category, and a removed Done status moves its tickets to
// another Done status, never to Cancelled (R-DC-2).
func TestStatusesInUseKeepTheirKind(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	proj := e.seedProject("HRIS")
	admin, au := e.signedIn("admin@example.com", true)
	var list httpapi.StatusList
	e.call(admin, http.MethodGet, "/projects/HRIS/statuses", nil, &list)
	review, done := list.Items[2], list.Items[3]
	waiting := e.seedTicket(proj, au, "Waiting for review", nil)
	shipped := e.seedTicket(proj, au, "Shipped last week", nil)
	if _, err := e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: waiting.ID, StatusID: review.Id}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: shipped.ID, StatusID: done.Id, Closed: true}); err != nil {
		t.Fatal(err)
	}
	rows := func() []map[string]any {
		out := make([]map[string]any, len(list.Items))
		for i, s := range list.Items {
			out[i] = statusInput(s)
		}
		return out
	}
	put := func(body map[string]any) (int, httpapi.Problem) {
		var prob httpapi.Problem
		code := e.call(admin, http.MethodPut, "/projects/HRIS/statuses", body, &prob)
		if code == http.StatusOK {
			e.call(admin, http.MethodGet, "/projects/HRIS/statuses", nil, &list)
		}
		return code, prob
	}

	reviewDone, doneCancelled, reviewTodo := rows(), rows(), rows()
	reviewDone[2]["category"] = "done"
	doneCancelled[3]["category"] = "cancelled"
	reviewTodo[2]["category"] = "todo"
	for _, c := range []struct {
		name  string
		body  map[string]any
		code  int
		field string
	}{
		{"In review becomes Done", map[string]any{"statuses": reviewDone}, 422, "statuses[2].category"}, // closes without the close checks
		{"Done becomes Cancelled", map[string]any{"statuses": doneCancelled}, 422, "statuses[3].category"}, // flips a decision's outcome
		{"In review becomes To do", map[string]any{"statuses": reviewTodo}, 200, ""},                  // open stays open
	} {
		code, prob := put(c.body)
		if f := firstError(prob); code != c.code || f.Field != c.field || (c.field != "" && f.Code != "status_category_in_use") {
			t.Errorf("%s: %d %+v", c.name, code, prob)
		}
	}

	// Removing Done: its tickets may go to another Done status, not to Cancelled.
	withDeployed := append(rows(), map[string]any{"name": "Deployed", "category": "done", "color": "#15803D"})
	if code, prob := put(map[string]any{"statuses": withDeployed}); code != http.StatusOK {
		t.Fatalf("add Deployed: %d %+v", code, prob)
	}
	cancelled, deployed := list.Items[4], list.Items[5]
	withoutDone := append(rows()[:3], rows()[4:]...)
	toCancelled := map[string]any{"statuses": withoutDone, "move_to": []map[string]any{{"from": done.Id, "to": cancelled.Id}}}
	if code, prob := put(toCancelled); code != http.StatusUnprocessableEntity || firstError(prob).Field != "move_to[0]" {
		t.Fatalf("Done to Cancelled: %d %+v", code, prob)
	}
	toDeployed := map[string]any{"statuses": withoutDone, "move_to": []map[string]any{{"from": done.Id, "to": deployed.Id}}}
	if code, prob := put(toDeployed); code != http.StatusOK {
		t.Fatalf("Done to Deployed: %d %+v", code, prob)
	}
	moved, err := e.q.GetTicketByKey(ctx, shipped.Key)
	if err != nil || moved.Ticket.StatusID != deployed.Id || moved.Ticket.ClosedAt == nil {
		t.Fatalf("the shipped ticket after the move: %+v %v", moved.Ticket, err)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd server && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./internal/httpapi/ -run TestStatusesInUseKeepTheirKind`
Expected: FAIL. "In review becomes Done" and "Done becomes Cancelled" answer 200, and "Done to Cancelled" answers 200.

- [ ] **Step 3: Implement**

In `server/internal/httpapi/statuses.go`:

1. Add `"slices"` to the imports.
2. In `SetStatuses`, replace

```go
	if fields := validateStatuses(in, before); len(fields) > 0 {
```

with

```go
	inUse, err := s.q.ListStatusIDsInUse(ctx, pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if fields := validateStatuses(in, before, inUse); len(fields) > 0 {
```

3. Replace the comment and signature of `validateStatuses`, and its `if st.Id != nil { … }` block, with:

```go
// validateStatuses checks R-TK-2 and the moves of R-TK-4 against the current
// list. A status that tickets use keeps its kind, so no ticket opens or closes
// without its close checks (R-TK-3): only To do and In progress swap.
func validateStatuses(in StatusesUpdate, before []db.Status, inUse []int64) []FieldError {
```

```go
		if st.Id != nil {
			o, ok := old[*st.Id]
			switch {
			case !ok:
				f = append(f, FieldError{Field: at + "id", Code: "invalid", Message: "Unknown status"})
			case o.Category != string(st.Category) && slices.Contains(inUse, *st.Id) &&
				(closedCategory(o.Category) || closedCategory(string(st.Category))):
				f = append(f, FieldError{Field: at + "category", Code: "status_category_in_use", Message: "Tickets use this status; move them before it opens or closes"})
			}
			kept[*st.Id] = st.Category
		}
```

4. Replace the `move_to` loop at the end of `validateStatuses` with:

```go
	for i, m := range deref(in.MoveTo) {
		from, existed := old[m.From]
		_, stays := kept[m.From]
		to, isKept := kept[m.To]
		// Open tickets land in any open status; closed ones only in the same
		// category, so their decisions keep their outcome (R-DC-2).
		sameKind := from.Category == string(to) || (!closedCategory(from.Category) && !closedCategory(string(to)))
		if !existed || stays || !isKept || !sameKind {
			f = append(f, FieldError{Field: fmt.Sprintf("move_to[%d]", i), Code: "invalid", Message: "Move tickets from a removed status to a kept one of the same kind"})
		}
	}
```

- [ ] **Step 4: Run the tests**

Run: `cd server && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./internal/httpapi/`
Expected: `ok`. `TestStatusRules` still passes; its In review → Done move is refused as before.

- [ ] **Step 5: Commit**

```bash
git add server/internal/httpapi/statuses.go server/internal/httpapi/statuses_test.go
git commit -m "feat(server): statuses in use keep their open or closed kind"
```

### Task 3: Close and reopen

**Files:**
- Modify: `api/openapi.yaml` (the transition, the decision schemas, `Ticket.closed_at` and `Ticket.decision`)
- Create: `server/internal/httpapi/decisions.go`
- Modify: `server/internal/httpapi/tickets.go` (`TransitionTicket`, `UpdateTicket`, `checkTicket`, `ticketFromRow`)
- Regenerate: `server/internal/httpapi/api.gen.go`, `web/lib/api-types.ts`
- Test: `server/internal/httpapi/close_test.go`; Modify: `server/internal/httpapi/tickets_test.go`

**Interfaces:**
- Consumes: `SetTicketStatus`, `SetTicketReason`, `ConfirmDecision`, `DraftDecision`, `GetDecision` (Task 1).
- Produces:
  - API types:
    - `DecisionInput{WhatChanged, Why string; Alternatives *string}`;
    - `DecisionOutcome` (`DecisionOutcomeImplemented`, `DecisionOutcomeRejected`) and `DecisionState` (`DecisionStateDraft`, `DecisionStateConfirmed`);
    - `DecisionRecord{WhatChanged, Why, Alternatives string; Outcome DecisionOutcome; State DecisionState; ConfirmedBy *Ref; ConfirmedAt *time.Time}`;
    - `TransitionRequest{StatusId int64; Reason *string; NodeIds *[]int64; Decision *DecisionInput}`;
    - `TransitionTicketParams{IfMatch *string}`;
    - `Ticket` gains `ClosedAt *time.Time` and `Decision *DecisionRecord`.
  - `POST /tickets/{key}/transition`:
    - A close without its fields answers 422 `close_validation_failed`, field codes as named:
      - `reason`: `required` or `invalid`;
      - `node_ids`: `min_items` or `invalid`;
      - `decision.what_changed`, `decision.why`: `required` or `invalid`;
      - `decision.alternatives`: `invalid`.
    - A stale `If-Match` answers 412 `stale`.
    - History actions: `transition` (status, plus reason and menus when a close set them), `decision_confirm` and `decision_draft`.
  - Helpers in `decisions.go`:
    - `checkDecision(prefix string, in *DecisionInput) (decisionText, []FieldError)` and `closeFieldErrors(reason string, menus int) []FieldError`;
    - `decisionOf(ctx, q, ticketID) (*DecisionRecord, error)`, `toAPIDecision(db.GetDecisionRow) DecisionRecord` and `decisionAudit(*DecisionRecord) map[string]any`.
  - Helpers in `tickets.go`: `ifMatch(w, header string) (int32, bool)` and `(s *Server) liveNodeIDs(ctx, pc, ids []int64) ([]int64, bool, error)`.

- [ ] **Step 1: Write the failing tests**

`server/internal/httpapi/close_test.go`:

```go
package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// statusID finds one of HRIS's statuses by name.
func statusID(e *env, c *http.Client, name string) int64 {
	e.t.Helper()
	var st httpapi.StatusList
	e.call(c, http.MethodGet, "/projects/HRIS/statuses", nil, &st)
	for _, s := range st.Items {
		if s.Name == name {
			return s.Id
		}
	}
	e.t.Fatalf("no status %q", name)
	return 0
}

func decisionBody(whatChanged, why string) map[string]any {
	return map[string]any{"what_changed": whatChanged, "why": why, "alternatives": "Backup supervisor, rejected: Client A has no such role."}
}

// ticketActions lists a ticket's history actions, oldest first.
func ticketActions(e *env, ticketID int64) []string {
	events, err := e.q.ListTicketEvents(context.Background(), ticketID)
	if err != nil {
		e.t.Fatal(err)
	}
	out := []string{}
	for _, ev := range events {
		out = append(out, ev.Action)
	}
	return out
}

// AC-DC-1 and R-DC-1 at API level: a close commits the status, the reason, the
// menus and a confirmed decision record together; R-DC-7: without them it is refused.
func TestClosingRecordsTheDecision(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval", &w.a) // no reason, no menus yet
	path := "/tickets/" + tk.Key + "/transition"
	done := statusID(e, w.pm, "Done")

	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"status_id": done}, &p); code != http.StatusUnprocessableEntity || p.Code != "close_validation_failed" {
		t.Fatalf("close without its fields: %d %+v", code, p)
	}
	var fields []string
	for _, f := range *p.Errors {
		fields = append(fields, f.Field+":"+f.Code)
	}
	if want := []string{"reason:required", "node_ids:min_items", "decision.what_changed:required", "decision.why:required"}; !slices.Equal(fields, want) {
		t.Fatalf("field errors: %v, want %v", fields, want)
	}

	var out httpapi.Ticket
	body := map[string]any{
		"status_id": done, "reason": "Client A supervisors are often on leave; HR approves overtime directly.",
		"node_ids": []int64{w.ot.ID},
		"decision": decisionBody("Overtime approval skips the supervisor step for Client A.", "Approvals stalled for days during leave periods."),
	}
	code, h := e.callWith(w.pm, http.MethodPost, path, map[string]string{"If-Match": `"1"`}, body, &out)
	if code != http.StatusOK || out.Status.Name != "Done" || out.ClosedAt == nil || len(out.Nodes) != 1 || h.Get("ETag") != `"2"` ||
		out.Reason != "Client A supervisors are often on leave; HR approves overtime directly." {
		t.Fatalf("close: %d %+v", code, out)
	}
	d := out.Decision
	if d == nil || d.State != httpapi.DecisionStateConfirmed || d.Outcome != httpapi.DecisionOutcomeImplemented ||
		d.ConfirmedBy == nil || d.ConfirmedBy.Id != w.pmUser.ID || d.ConfirmedAt == nil ||
		d.WhatChanged != "Overtime approval skips the supervisor step for Client A." {
		t.Fatalf("decision: %+v", d)
	}
	if got := ticketActions(e, tk.ID); !slices.Equal(got, []string{"transition", "decision_confirm"}) {
		t.Fatalf("history: %v", got)
	}
	events, _ := e.q.ListTicketEvents(context.Background(), tk.ID)
	var changes map[string]map[string]any
	if json.Unmarshal(events[0].Changes, &changes) != nil || changes["status"]["new"] != "Done" ||
		changes["reason"]["old"] != "" || changes["menus"] == nil {
		t.Fatalf("the close's changes: %s", events[0].Changes)
	}
}

// R-DC-2 and R-DC-6: Cancelled records a rejection, a reopen turns the record
// back into a draft (AC-DC-3), and the next close confirms it again.
func TestReopeningDraftsTheDecisionAndTheNextCloseConfirmsIt(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Weekend overtime at double rate", &w.a, w.ot)
	path := "/tickets/" + tk.Key + "/transition"
	closeAs := func(status, whatChanged string) httpapi.Ticket {
		t.Helper()
		var out httpapi.Ticket
		body := map[string]any{"status_id": statusID(e, w.pm, status), "reason": "Client A asked for weekend double pay.",
			"decision": decisionBody(whatChanged, "Their budget has no room for it this year.")}
		if code := e.call(w.pm, http.MethodPost, path, body, &out); code != http.StatusOK {
			t.Fatalf("close as %s: %d", status, code)
		}
		return out
	}
	if out := closeAs("Cancelled", "Weekend overtime stays at the normal rate."); out.Decision.Outcome != httpapi.DecisionOutcomeRejected {
		t.Fatalf("cancelled: %+v", out.Decision)
	}
	var out httpapi.Ticket
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"status_id": statusID(e, w.pm, "In progress")}, &out); code != http.StatusOK ||
		out.ClosedAt != nil || out.Decision == nil || out.Decision.State != httpapi.DecisionStateDraft || out.Decision.ConfirmedBy != nil ||
		out.Decision.WhatChanged != "Weekend overtime stays at the normal rate." {
		t.Fatalf("reopen: %d %+v", code, out)
	}
	if out := closeAs("Done", "Weekend overtime pays double from March."); out.Decision.State != httpapi.DecisionStateConfirmed ||
		out.Decision.Outcome != httpapi.DecisionOutcomeImplemented || out.Decision.WhatChanged != "Weekend overtime pays double from March." {
		t.Fatalf("closed again: %+v", out.Decision)
	}
	want := []string{"transition", "decision_confirm", "transition", "decision_draft", "transition", "decision_confirm"}
	if got := ticketActions(e, tk.ID); !slices.Equal(got, want) {
		t.Fatalf("history: %v, want %v", got, want)
	}
}

// AC-TK-5 for closes: a stale If-Match changes nothing.
func TestStaleClosesAreRefused(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime cap of 40 hours", &w.a, w.ot)
	body := map[string]any{"status_id": statusID(e, w.pm, "Done"), "reason": "The labor agreement caps overtime.",
		"decision": decisionBody("Overtime is capped at 40 hours a month.", "The labor agreement sets the cap.")}
	var p httpapi.Problem
	if code, _ := e.callWith(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/transition", map[string]string{"If-Match": `"7"`}, body, &p); code != http.StatusPreconditionFailed || p.Code != "stale" {
		t.Fatalf("stale close: %d %+v", code, p)
	}
	var got httpapi.Ticket
	e.call(w.pm, http.MethodGet, "/tickets/"+tk.Key, nil, &got)
	if got.Status.Name != "To do" || got.Decision != nil || got.Reason != "" || len(ticketActions(e, tk.ID)) != 0 {
		t.Fatalf("after a stale close: %+v", got)
	}
}

// AC-DC-5: when the database refuses part of a close, nothing changes: the
// status, the reason, the decision record and the history stay as they were.
func TestAFailedCloseChangesNothing(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, e.d.OwnerURL)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	for _, sql := range []string{
		`CREATE FUNCTION refuse_decision() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'disk full'; END $$`,
		`CREATE TRIGGER refuse_decision BEFORE INSERT ON decision_records FOR EACH ROW EXECUTE FUNCTION refuse_decision()`,
	} {
		if _, err := owner.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	body := map[string]any{"status_id": statusID(e, w.pm, "Done"), "reason": "The payroll team imports overtime monthly.",
		"decision": decisionBody("Overtime exports as CSV for payroll.", "Payroll imports it every month.")}
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/transition", body, nil); code != http.StatusInternalServerError {
		t.Fatalf("close: %d", code)
	}
	var got httpapi.Ticket
	e.call(w.pm, http.MethodGet, "/tickets/"+tk.Key, nil, &got)
	if got.Status.Name != "To do" || got.ClosedAt != nil || got.Decision != nil || got.Reason != "" || got.Version != 1 ||
		len(ticketActions(e, tk.ID)) != 0 {
		t.Fatalf("after a failed close: %+v", got)
	}
}
```

In `server/internal/httpapi/tickets_test.go`, `TestTransitionsMoveBetweenOpenStatuses` still expects `close_unavailable`. Replace

```go
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"status_id": st.Items[3].Id}, &p); code != http.StatusUnprocessableEntity || p.Code != "close_unavailable" {
		t.Fatalf("to Done: %d %+v", code, p)
	}
```

with

```go
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"status_id": st.Items[3].Id}, &p); code != http.StatusUnprocessableEntity || p.Code != "close_validation_failed" {
		t.Fatalf("to Done without the close fields: %d %+v", code, p)
	}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd server && go vet ./internal/httpapi/`
Expected: compile errors: `httpapi.DecisionStateConfirmed`, `httpapi.DecisionOutcomeImplemented` and the fields `Ticket.ClosedAt` and `Ticket.Decision` are undefined.

- [ ] **Step 3: Extend the contract**

In `api/openapi.yaml`, replace the whole `/tickets/{key}/transition:` path with:

```yaml
  /tickets/{key}/transition:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    post:
      operationId: transitionTicket
      tags: [tickets]
      description: >-
        Members and project admins. Entering a Done or Cancelled status is a close: it needs a reason, at least one
        menu and the decision record, else 422 close_validation_failed (FSD §9.1). Leaving one reopens the ticket and
        turns its decision record back into a draft. A stale If-Match answers 412.
      parameters:
        - { name: If-Match, in: header, schema: { type: string, example: '"7"' } }
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/TransitionRequest" }
      responses:
        "200":
          description: The ticket in its new status; ETag carries its version.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Ticket" }
        default: { $ref: "#/components/responses/Problem" }
```

Replace the `TransitionRequest` schema with:

```yaml
    TransitionRequest:
      type: object
      required: [status_id]
      properties:
        status_id: { type: integer, format: int64 }
        reason: { type: string, maxLength: 2000, description: A close only; replaces the ticket's reason. }
        node_ids:
          type: array
          description: A close only; replaces the ticket's menus.
          items: { type: integer, format: int64 }
        decision: { $ref: "#/components/schemas/DecisionInput" }
    DecisionInput:
      type: object
      required: [what_changed, why]
      properties:
        what_changed: { type: string, maxLength: 1000 }
        why: { type: string, maxLength: 2000 }
        alternatives: { type: string, maxLength: 2000 }
    DecisionOutcome:
      type: string
      enum: [implemented, rejected]
    DecisionState:
      type: string
      enum: [draft, confirmed]
    DecisionRecord:
      type: object
      required: [what_changed, why, alternatives, outcome, state]
      properties:
        what_changed: { type: string }
        why: { type: string }
        alternatives: { type: string }
        outcome: { $ref: "#/components/schemas/DecisionOutcome" }
        state: { $ref: "#/components/schemas/DecisionState" }
        confirmed_by: { $ref: "#/components/schemas/Ref" }
        confirmed_at: { type: string, format: date-time }
```

In the `Ticket` schema, add these two properties after its `updated_at`:

```yaml
        closed_at: { type: string, format: date-time, nullable: true, description: Set on close, cleared on reopen. }
        decision: { $ref: "#/components/schemas/DecisionRecord" }
```

Then regenerate both sides:

```bash
make generate
```

Expected: `api.gen.go` and `web/lib/api-types.ts` change. The Go build now fails, because `TransitionTicket` lacks its `params` argument.

- [ ] **Step 4: Implement**

`server/internal/httpapi/decisions.go`:

```go
package httpapi

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/db"
)

// decisionText is a decision record's words, trimmed.
type decisionText struct{ WhatChanged, Why, Alternatives string }

// checkDecision validates a decision record's words (FSD §9.1): what changed
// 10–1,000 characters, why 10–2,000, alternatives up to 2,000. prefix names the
// fields the way the request nests them.
func checkDecision(prefix string, in *DecisionInput) (decisionText, []FieldError) {
	var d decisionText
	if in != nil {
		d = decisionText{WhatChanged: strings.TrimSpace(in.WhatChanged), Why: strings.TrimSpace(in.Why), Alternatives: strings.TrimSpace(deref(in.Alternatives))}
	}
	f := textRange(prefix+"what_changed", d.WhatChanged, 10, 1000, "Say what changed in 10 to 1,000 characters")
	f = append(f, textRange(prefix+"why", d.Why, 10, 2000, "Say why in 10 to 2,000 characters")...)
	if utf8.RuneCountInString(d.Alternatives) > 2000 {
		f = append(f, FieldError{Field: prefix + "alternatives", Code: "invalid", Message: "Use at most 2,000 characters"})
	}
	return d, f
}

// closeFieldErrors checks what a closed ticket needs of its own fields (TK-3):
// a reason of 10–2,000 characters and at least one menu (FSD §9.1).
func closeFieldErrors(reason string, menus int) []FieldError {
	f := textRange("reason", reason, 10, 2000, "Reason is required to close: 10 to 2,000 characters")
	if menus == 0 {
		f = append(f, FieldError{Field: "node_ids", Code: "min_items", Message: "Link at least one menu or module"})
	}
	return f
}

// textRange checks a required text: empty is "required", and a length outside
// lo–hi characters is "invalid".
func textRange(field, s string, lo, hi int, msg string) []FieldError {
	switch n := utf8.RuneCountInString(s); {
	case n == 0:
		return []FieldError{{Field: field, Code: "required", Message: msg}}
	case n < lo || n > hi:
		return []FieldError{{Field: field, Code: "invalid", Message: msg}}
	}
	return nil
}

// decisionOf reads a ticket's decision record; nil when it has none.
func decisionOf(ctx context.Context, q *db.Queries, ticketID int64) (*DecisionRecord, error) {
	row, err := q.GetDecision(ctx, ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ptr(toAPIDecision(row)), nil
}

func toAPIDecision(row db.GetDecisionRow) DecisionRecord {
	d := row.DecisionRecord
	out := DecisionRecord{
		WhatChanged: d.WhatChanged, Why: d.Why, Alternatives: d.Alternatives,
		Outcome: DecisionOutcome(d.Outcome), State: DecisionState(d.State), ConfirmedAt: d.ConfirmedAt,
	}
	if d.ConfirmedBy != nil {
		out.ConfirmedBy = &Ref{Id: *d.ConfirmedBy, Name: deref(row.ConfirmerName)}
	}
	return out
}

// decisionAudit is what the ticket's history keeps of its decision record, so
// every earlier version stays readable (R-DC-6).
func decisionAudit(d *DecisionRecord) map[string]any {
	if d == nil {
		return map[string]any{}
	}
	m := map[string]any{
		"what_changed": d.WhatChanged, "why": d.Why, "alternatives": d.Alternatives,
		"outcome": string(d.Outcome), "state": string(d.State), "confirmed_by": nil,
	}
	if d.ConfirmedBy != nil {
		m["confirmed_by"] = d.ConfirmedBy.Name
	}
	return m
}
```

In `server/internal/httpapi/tickets.go`:

1. Below `clientIDField`, add:

```go
var nodeIDsField = FieldError{Field: "node_ids", Code: "invalid", Message: "Choose menus and modules of this project"}
```

2. In `UpdateTicket`, replace

```go
	version, err := strconv.ParseInt(strings.Trim(params.IfMatch, `"`), 10, 32)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_parameter", "If-Match must carry the ticket's version, as its ETag gave it")
		return
	}
```

with

```go
	version, ok := ifMatch(w, params.IfMatch)
	if !ok {
		return
	}
```

and `Version: int32(version),` with `Version: version,`.

3. Replace the whole `TransitionTicket` function, comment included, with:

```go
// TransitionTicket moves a ticket to another status (R-TK-3). Entering Done or
// Cancelled is a close: the reason and menus it names and a confirmed decision
// record commit with the status in one transaction, or nothing changes (R-DC-1,
// R-DC-7). Leaving them reopens the ticket and turns its record back into a
// draft (R-DC-6). If-Match is optional; a stale version answers 412.
func (s *Server) TransitionTicket(w http.ResponseWriter, r *http.Request, key string, params TransitionTicketParams) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	var version *int32
	if params.IfMatch != nil {
		v, ok := ifMatch(w, *params.IfMatch)
		if !ok {
			return
		}
		version = &v
	}
	var in TransitionRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	st, err := s.q.GetStatus(ctx, in.StatusId)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && st.ProjectID != row.Ticket.ProjectID) {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "status_id", Code: "invalid", Message: "Choose a status of this project"})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	moving, closing := st.ID != row.Ticket.StatusID, closedCategory(st.Category)
	reason := row.Ticket.Reason
	if in.Reason != nil {
		reason = strings.TrimSpace(*in.Reason)
	}
	var nodeIDs []int64 // replaces the ticket's menus when not nil
	var text decisionText
	if moving && closing {
		menus, err := s.q.ListTicketNodes(ctx, row.Ticket.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var fields []FieldError
		count := len(menus)
		if in.NodeIds != nil {
			ids, live, err := s.liveNodeIDs(ctx, pc, *in.NodeIds)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			if !live {
				fields = append(fields, nodeIDsField)
			}
			nodeIDs, count = ids, len(ids)
		}
		fields = append(fields, closeFieldErrors(reason, count)...)
		var decisionFields []FieldError
		text, decisionFields = checkDecision("decision.", in.Decision)
		if fields = append(fields, decisionFields...); len(fields) > 0 {
			writeProblem(w, http.StatusUnprocessableEntity, "close_validation_failed", "Ticket can't be closed yet", fields...)
			return
		}
	}
	var out Ticket
	err = s.inTx(ctx, func(q *db.Queries) error {
		before, err := ticketFromRow(ctx, q, row)
		if err != nil || !moving {
			out = before
			return err
		}
		_, err = q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: row.Ticket.ID, StatusID: st.ID, Closed: closing, Version: version})
		if errors.Is(err, pgx.ErrNoRows) {
			return errStale
		}
		if err != nil {
			return err
		}
		action := ""
		switch {
		case closing:
			if in.Reason != nil {
				if err := q.SetTicketReason(ctx, db.SetTicketReasonParams{ID: row.Ticket.ID, Reason: reason}); err != nil {
					return err
				}
			}
			if nodeIDs != nil {
				if err := q.ClearTicketNodes(ctx, row.Ticket.ID); err != nil {
					return err
				}
				if err := q.AddTicketNodes(ctx, db.AddTicketNodesParams{TicketID: row.Ticket.ID, NodeIds: nodeIDs}); err != nil {
					return err
				}
			}
			outcome := DecisionOutcomeImplemented // R-DC-2: Done implements, Cancelled rejects
			if st.Category == string(StatusCategoryCancelled) {
				outcome = DecisionOutcomeRejected
			}
			if _, err := q.ConfirmDecision(ctx, db.ConfirmDecisionParams{
				TicketID: row.Ticket.ID, WhatChanged: text.WhatChanged, Why: text.Why, Alternatives: text.Alternatives,
				Outcome: string(outcome), ConfirmedBy: pc.user.ID,
			}); err != nil {
				return err
			}
			action = "decision_confirm"
		case closedCategory(row.Status.Category):
			if err := q.DraftDecision(ctx, row.Ticket.ID); err != nil {
				return err
			}
			action = "decision_draft"
		}
		if out, err = readTicket(ctx, q, row.Ticket.Key); err != nil {
			return err
		}
		m := webMeta(r).inProject(pc.project.ID)
		if err := audit(ctx, q, m, &pc.user.ID, "ticket", row.Ticket.ID, "transition", changed(ticketAudit(before), ticketAudit(out))); err != nil {
			return err
		}
		if d := changed(decisionAudit(before.Decision), decisionAudit(out.Decision)); action != "" && len(d) > 0 {
			return audit(ctx, q, m, &pc.user.ID, "ticket", row.Ticket.ID, action, d)
		}
		return nil
	})
	switch {
	case errors.Is(err, errStale):
		writeProblem(w, http.StatusPreconditionFailed, "stale", "Someone updated this ticket a moment ago")
	case err != nil:
		s.fail(w, r, err)
	default:
		w.Header().Set("ETag", etag(out.Version))
		writeJSON(w, http.StatusOK, out)
	}
}
```

4. In `checkTicket`, replace everything from `nodeIDs := slices.Compact(…)` to the end of the function with:

```go
	nodeIDs, live, err := s.liveNodeIDs(ctx, pc, in.NodeIDs)
	if err != nil {
		return t, nil, nil, err
	}
	if !live {
		f = append(f, nodeIDsField)
	}
	return t, nodeIDs, f, nil
}

// liveNodeIDs sorts and dedupes ids, and reports whether each is a live node of
// the project that the caller sees.
func (s *Server) liveNodeIDs(ctx context.Context, pc projectCtx, ids []int64) ([]int64, bool, error) {
	out := slices.Compact(slices.Sorted(slices.Values(ids)))
	if len(out) == 0 {
		return []int64{}, true, nil
	}
	visible, err := s.q.ListNodes(ctx, db.ListNodesParams{ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs)})
	if err != nil {
		return nil, false, err
	}
	live := map[int64]bool{}
	for _, n := range visible {
		live[n.ID] = true
	}
	for _, id := range out {
		if !live[id] {
			return out, false, nil
		}
	}
	return out, true, nil
}
```

5. In `ticketFromRow`, after the `files` lookup, add:

```go
	decision, err := decisionOf(ctx, q, t.ID)
	if err != nil {
		return Ticket{}, err
	}
```

and in the `out := Ticket{…}` literal replace `CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,` with `CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, ClosedAt: t.ClosedAt, Decision: decision,`.

6. After `etag`, add:

```go
// ifMatch reads the version an If-Match header carries; a malformed one answers 400.
func ifMatch(w http.ResponseWriter, header string) (int32, bool) {
	v, err := strconv.ParseInt(strings.Trim(header, `"`), 10, 32)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_parameter", "If-Match must carry the ticket's version, as its ETag gave it")
		return 0, false
	}
	return int32(v), true
}
```

- [ ] **Step 5: Run the tests**

Run: `cd server && gofmt -l . && go vet ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...`
Expected: no gofmt output, no vet findings, `ok` for every package.

- [ ] **Step 6: Commit**

```bash
git add api/openapi.yaml server/internal/httpapi web/lib/api-types.ts
git commit -m "feat(server): closing records the decision; reopening drafts it"
```

### Task 4: Decision edits, closed tickets and the board's recent closes

**Files:**
- Modify: `api/openapi.yaml` (`PUT /tickets/{key}/decision`, `closed_days` on the ticket list)
- Modify: `server/internal/httpapi/decisions.go` (`UpdateDecision`), `tickets.go` (`UpdateTicket`), `ticket_list.go` (`ClosedDays`)
- Regenerate: `server/internal/httpapi/api.gen.go`, `web/lib/api-types.ts`
- Test: `server/internal/httpapi/decisions_test.go`, `server/internal/httpapi/ticket_list_test.go`

**Interfaces:**
- Consumes: `checkDecision`, `closeFieldErrors`, `decisionOf`, `decisionAudit` (Task 3); `UpdateDecision` and `ListTicketsParams.ClosedDays` (Task 1).
- Produces:
  - `PUT /tickets/{key}/decision` takes `DecisionInput` and returns `DecisionRecord`:
    - 409 `decision_not_confirmed` without a confirmed record;
    - 403 for members who are neither project admin nor confirmer;
    - 422 `validation_failed` with fields `what_changed`, `why`, `alternatives`;
    - history action `decision_edit`.
  - `PUT /tickets/{key}` on a closed ticket answers 422 with `reason` or `node_ids` field errors when an edit would drop them.
  - `GET /projects/{key}/tickets?closed_days=14` leaves out tickets closed earlier (`ListTicketsParams.ClosedDays *int32`).

- [ ] **Step 1: Write the failing tests**

`server/internal/httpapi/decisions_test.go`:

```go
package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// R-DC-5: project admins and the confirmer reword a confirmed record, and every
// edit keeps the earlier words in the ticket's history.
func TestOnlyAdminsAndTheConfirmerEditADecision(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	other, ou := e.signedIn("ani@example.com", false)
	e.seedMember(ou, w.p, "member")
	admin, au := e.signedIn("hana@example.com", false)
	e.seedMember(au, w.p, "admin")
	tk := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval", &w.a, w.ot)
	path := "/tickets/" + tk.Key + "/decision"
	edit := map[string]any{"what_changed": "Overtime approval skips the supervisor for Client A.", "why": "Supervisors are often on leave."}

	var early httpapi.Problem
	if code := e.call(w.pm, http.MethodPut, path, edit, &early); code != http.StatusConflict || early.Code != "decision_not_confirmed" {
		t.Fatalf("an edit before the close: %d %+v", code, early)
	}
	closeBody := map[string]any{"status_id": statusID(e, w.pm, "Done"), "reason": "Client A supervisors are often on leave.",
		"decision": decisionBody("Overtime approval skips the supervisor.", "Supervisors are often on leave.")}
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/transition", closeBody, nil); code != http.StatusOK {
		t.Fatalf("close: %d", code)
	}
	if code := e.call(other, http.MethodPut, path, edit, nil); code != http.StatusForbidden {
		t.Fatalf("another member edits: %d", code)
	}
	for _, c := range []*http.Client{w.pm, admin} { // the confirmer, then a project admin
		var out httpapi.DecisionRecord
		if code := e.call(c, http.MethodPut, path, edit, &out); code != http.StatusOK || out.WhatChanged != edit["what_changed"] ||
			out.Alternatives != "" || out.ConfirmedBy == nil || out.ConfirmedBy.Id != w.pmUser.ID {
			t.Fatalf("edit: %d %+v", code, out)
		}
	}
	var short httpapi.Problem
	if code := e.call(admin, http.MethodPut, path, map[string]any{"what_changed": "Short", "why": "Supervisors are often on leave."}, &short); code != http.StatusUnprocessableEntity || firstError(short).Field != "what_changed" {
		t.Fatalf("a short edit: %d %+v", code, short)
	}
	// The admin's edit repeated the pm's words, so the history holds one edit.
	if got := ticketActions(e, tk.ID); !slices.Equal(got, []string{"transition", "decision_confirm", "decision_edit"}) {
		t.Fatalf("history: %v", got)
	}
	events, _ := e.q.ListTicketEvents(context.Background(), tk.ID)
	var changes map[string]map[string]any
	if json.Unmarshal(events[2].Changes, &changes) != nil || changes["what_changed"]["old"] != "Overtime approval skips the supervisor." ||
		changes["alternatives"]["old"] != "Backup supervisor, rejected: Client A has no such role." {
		t.Fatalf("the edit's changes: %s", events[2].Changes)
	}
}

// TK-3 holds after the close: a closed ticket keeps a reason and a menu.
func TestClosedTicketsKeepTheirReasonAndMenus(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime cap of 40 hours", &w.a, w.ot)
	closeBody := map[string]any{"status_id": statusID(e, w.pm, "Done"), "reason": "The labor agreement caps overtime.",
		"decision": decisionBody("Overtime is capped at 40 hours a month.", "The labor agreement sets the cap.")}
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/transition", closeBody, nil); code != http.StatusOK {
		t.Fatalf("close: %d", code)
	}
	edit := map[string]any{"type": "change_request", "title": "Overtime cap of 40 hours", "client_id": w.a.ID,
		"requester_user_id": w.pmUser.ID, "node_ids": []int64{}, "reason": ""}
	var p httpapi.Problem
	code, _ := e.callWith(w.pm, http.MethodPut, "/tickets/"+tk.Key, map[string]string{"If-Match": `"2"`}, edit, &p)
	var fields []string
	if p.Errors != nil {
		for _, f := range *p.Errors {
			fields = append(fields, f.Field+":"+f.Code)
		}
	}
	if code != http.StatusUnprocessableEntity || !slices.Equal(fields, []string{"reason:required", "node_ids:min_items"}) {
		t.Fatalf("emptying a closed ticket: %d %v", code, fields)
	}
}
```

Append to `server/internal/httpapi/ticket_list_test.go`:

```go
// FSD §8.4: the board asks only for tickets closed in the last 14 days.
func TestTicketListCanLeaveOutOldCloses(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	done := statusID(e, w.pm, "Done")
	old := e.seedTicket(w.p, w.pmUser, "Closed long ago", &w.a, w.ot)
	recent := e.seedTicket(w.p, w.pmUser, "Closed yesterday", &w.a, w.ot)
	open := e.seedTicket(w.p, w.pmUser, "Still open", &w.a, w.ot)
	for _, tk := range []db.Ticket{old, recent} {
		if _, err := e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: tk.ID, StatusID: done, Closed: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.d.Pool.Exec(ctx, "UPDATE tickets SET closed_at = now() - interval '20 days' WHERE id = $1", old.ID); err != nil {
		t.Fatal(err)
	}
	for query, want := range map[string][]string{
		"?sort=key":                {old.Key, recent.Key, open.Key},
		"?sort=key&closed_days=14": {recent.Key, open.Key},
	} {
		var page httpapi.TicketPage
		e.call(w.pm, http.MethodGet, "/projects/HRIS/tickets"+query, nil, &page)
		if got := ticketKeys(page); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", query, got, want)
		}
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd server && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./internal/httpapi/ -run 'Decision|ClosedTickets|OldCloses'`
Expected: FAIL.
- `PUT /tickets/{key}/decision` answers 404 or 405, because it has no route yet.
- Emptying a closed ticket answers 200.
- `closed_days` answers 400, because it is an unknown parameter; or it is ignored.

- [ ] **Step 3: Extend the contract**

In `api/openapi.yaml`, after the `/tickets/{key}/transition` path, add:

```yaml
  /tickets/{key}/decision:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    put:
      operationId: updateDecision
      tags: [tickets]
      description: >-
        Project admins and the confirmer reword a confirmed decision record (R-DC-5); every edit is audited.
        A ticket without a confirmed record answers 409 decision_not_confirmed.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/DecisionInput" }
      responses:
        "200":
          description: The record as saved.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/DecisionRecord" }
        default: { $ref: "#/components/responses/Problem" }
```

In the `listTickets` parameters, after the `open` line, add:

```yaml
        - { name: closed_days, in: query, schema: { type: integer, format: int32, minimum: 1 }, description: Closed tickets only when closed within this many days; the board asks for 14. }
```

Then run `make generate`.

- [ ] **Step 4: Implement**

In `server/internal/httpapi/decisions.go`, add `"net/http"` and `"github.com/kenzo03/muasal/server/internal/access"` to the imports, then append:

```go
// UpdateDecision rewords a confirmed decision record. Project admins and the
// confirmer may; every edit keeps the old words in the ticket's history (R-DC-5).
func (s *Server) UpdateDecision(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	var in DecisionInput
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	before, err := decisionOf(ctx, s.q, row.Ticket.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if before == nil || before.State != DecisionStateConfirmed {
		writeProblem(w, http.StatusConflict, "decision_not_confirmed", "Close the ticket to confirm its decision record first")
		return
	}
	if !pc.scope.Allows(access.Admin) && (before.ConfirmedBy == nil || before.ConfirmedBy.Id != pc.user.ID) {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only project admins and the confirmer edit a decision record")
		return
	}
	text, fields := checkDecision("", &in)
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var out *DecisionRecord
	err = s.inTx(ctx, func(q *db.Queries) error {
		if err := q.UpdateDecision(ctx, db.UpdateDecisionParams{
			TicketID: row.Ticket.ID, WhatChanged: text.WhatChanged, Why: text.Why, Alternatives: text.Alternatives,
		}); err != nil {
			return err
		}
		var err error
		if out, err = decisionOf(ctx, q, row.Ticket.ID); err != nil {
			return err
		}
		if d := changed(decisionAudit(before), decisionAudit(out)); len(d) > 0 {
			return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", row.Ticket.ID, "decision_edit", d)
		}
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
```

In `server/internal/httpapi/tickets.go`, `UpdateTicket`, after the `requester_contact_id` required check, add:

```go
	// TK-3 holds after the close: a closed ticket keeps a reason and a menu (FSD §9.1).
	if closedCategory(row.Status.Category) {
		fields = append(fields, closeFieldErrors(draft.Reason, len(nodeIDs))...)
	}
```

In `server/internal/httpapi/ticket_list.go`, add `ClosedDays: params.ClosedDays,` to the `db.ListTicketsParams` literal.

- [ ] **Step 5: Run the tests**

Run: `cd server && gofmt -l . && go vet ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...`
Expected: no output from gofmt and vet, and `ok` for every package.

- [ ] **Step 6: Commit**

```bash
git add api/openapi.yaml server/internal/httpapi web/lib/api-types.ts
git commit -m "feat(server): decision edits, closed tickets keep reason and menus, recent closes"
```

### Task 5: The node page API

**Files:**
- Modify: `api/openapi.yaml` (`GET /nodes/{id}`, `/nodes/{id}/timeline`, `/nodes/{id}/behaviors`, their schemas)
- Create: `server/internal/httpapi/node_page.go`
- Modify: `server/internal/httpapi/nodes.go` (`toAPINodeRow`), `ticket_list.go` (`paging`, `visibleNodes`)
- Regenerate: `server/internal/httpapi/api.gen.go`, `web/lib/api-types.ts`
- Test: `server/internal/httpapi/node_page_test.go`; Modify: `server/internal/httpapi/seed_test.go` (`seedClose`)

**Interfaces:**
- Consumes: `ListNodeTimeline`, `ListNodeBehaviors` (Task 1); `nodeFor`, `subtree`, `toAPIStatus` (Iterations 1–2).
- Produces:
  - API types:
    - `NodeDetail{Node Node; Path []Ref; ProjectKey string}`;
    - `TimelineEntry{Key, Title string; Type TicketType; Status Status; Client *Ref; Requester TicketRequester; CreatedAt time.Time; ClosedAt *time.Time; Decision *DecisionRecord}` and `TimelinePage{Items []TimelineEntry; NextCursor *string}`;
    - `Behavior{Key, Title string; Client *Ref; ClosedAt *time.Time; WhatChanged, Why, Alternatives string}` and `BehaviorList{Items []Behavior}`.
  - Endpoints:
    - `GET /nodes/{id}`: archived nodes too; hidden ones answer 404.
    - `GET /nodes/{id}/timeline?sub_nodes=&client_id=&core=&type=&from=&to=&limit=&cursor=`
    - `GET /nodes/{id}/behaviors?sub_nodes=`
  - Helpers:
    - `paging(w, limit *int32, cursor *string) (int, int, bool)`;
    - `(s *Server) visibleNodes(ctx, pc) ([]db.ListNodesRow, error)`;
    - `toAPINodeRow(db.ListNodesRow) Node`.
  - Test seeder `(e *env) seedClose(tk db.Ticket, status int64, by db.User, whatChanged string)`.

- [ ] **Step 1: Write the failing tests**

Append to `server/internal/httpapi/seed_test.go`:

```go
// seedClose closes a ticket in status with a decision record confirmed by by;
// a Cancelled status records a rejection (R-DC-2).
func (e *env) seedClose(tk db.Ticket, status int64, by db.User, whatChanged string) {
	e.t.Helper()
	ctx := context.Background()
	st, err := e.q.GetStatus(ctx, status)
	if err == nil {
		_, err = e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: tk.ID, StatusID: status, Closed: true})
	}
	outcome := "implemented"
	if st.Category == "cancelled" {
		outcome = "rejected"
	}
	if err == nil {
		_, err = e.q.ConfirmDecision(ctx, db.ConfirmDecisionParams{
			TicketID: tk.ID, WhatChanged: whatChanged, Why: "Because the client asked for it this way.", Outcome: outcome, ConfirmedBy: by.ID,
		})
	}
	if err != nil {
		e.t.Fatal(err)
	}
}
```

`server/internal/httpapi/node_page_test.go`:

```go
package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// timelineKeys lists a timeline's ticket keys in the order the API gave them.
func timelineKeys(e *env, c *http.Client, path string) []string {
	e.t.Helper()
	var page httpapi.TimelinePage
	if code := e.call(c, http.MethodGet, path, nil, &page); code != http.StatusOK {
		e.t.Fatalf("GET %s: %d", path, code)
	}
	keys := []string{}
	for _, it := range page.Items {
		keys = append(keys, it.Key)
	}
	return keys
}

// AC-MR-5: an archived node's page still opens, with its path; a node hidden
// by the client scope looks missing (R-AC-5).
func TestNodePageReadsArchivedNodesWithTheirPath(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, au := e.signedIn("hana@example.com", false)
	e.seedMember(au, w.p, "admin")
	e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	if code := e.call(admin, http.MethodPatch, fmt.Sprintf("/nodes/%d", w.ot.ID), map[string]any{"archived": true}, nil); code != http.StatusOK {
		t.Fatalf("archive: %d", code)
	}
	var got httpapi.NodeDetail
	if code := e.call(w.pm, http.MethodGet, fmt.Sprintf("/nodes/%d", w.ot.ID), nil, &got); code != http.StatusOK || !got.Node.Archived ||
		got.Node.Name != "Overtime Approval" || got.ProjectKey != "HRIS" || !slices.Equal(got.Path, []httpapi.Ref{{Id: w.hr.ID, Name: "HR"}}) {
		t.Fatalf("node page: %d %+v", code, got)
	}
	if code := e.call(w.pm, http.MethodGet, fmt.Sprintf("/nodes/%d", w.secret.ID), nil, nil); code != http.StatusNotFound {
		t.Fatalf("a hidden node: %d", code)
	}
}

// AC-MR-3: a menu's timeline lists its closed tickets newest first by close
// date, each with what changed and why; open tickets come first (FSD §7.4).
func TestNodeTimelineListsHistoryNewestFirst(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	done := statusID(e, w.pm, "Done")
	for i, closedOn := range []string{"2024-03-01", "2026-02-02", "2025-06-10"} {
		tk := e.seedTicket(w.p, w.pmUser, fmt.Sprintf("Overtime change %d", i+1), &w.a, w.ot)
		e.seedClose(tk, done, w.pmUser, fmt.Sprintf("Change %d", i+1))
		at, err := time.Parse(time.DateOnly, closedOn)
		if err == nil {
			_, err = e.d.Pool.Exec(ctx, "UPDATE tickets SET closed_at = $2 WHERE id = $1", tk.ID, at)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	var page httpapi.TimelinePage
	if code := e.call(w.pm, http.MethodGet, fmt.Sprintf("/nodes/%d/timeline", w.ot.ID), nil, &page); code != http.StatusOK || len(page.Items) != 4 {
		t.Fatalf("timeline: %d %+v", code, page)
	}
	var keys []string
	for _, it := range page.Items {
		keys = append(keys, it.Key)
	}
	if want := []string{open.Key, "HRIS-2", "HRIS-3", "HRIS-1"}; !slices.Equal(keys, want) {
		t.Fatalf("order: %v, want %v", keys, want)
	}
	first, latest := page.Items[0], page.Items[1]
	if first.ClosedAt != nil || first.Decision != nil || first.Client == nil || first.Client.Name != "Client A" || first.Requester.Name != "pm@example.com" {
		t.Fatalf("the open entry: %+v", first)
	}
	if latest.ClosedAt == nil || latest.Decision == nil || latest.Decision.WhatChanged != "Change 2" || latest.Decision.Why == "" || latest.Status.Name != "Done" {
		t.Fatalf("the latest close: %+v", latest)
	}
}

// AC-MR-4: a module's timeline holds its menus' tickets unless sub-nodes are
// left out; the filters narrow it, and hidden tickets never show (R-AC-7).
func TestNodeTimelineIncludesSubNodesByDefault(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	leave := e.seedNode(w.p, &w.hr, "menu", "Leave Request")
	onModule := e.seedTicket(w.p, w.pmUser, "HR module rename", nil, w.hr)
	onOT := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	onLeave := e.seedTicket(w.p, w.pmUser, "Leave carry-over", nil, leave)
	e.seedTicket(w.p, w.pmUser, "Client B report", &w.b, w.secret) // hidden from the PM, who sees Client A
	hr := fmt.Sprintf("/nodes/%d/timeline", w.hr.ID)
	sorted := func(keys []string) []string {
		slices.Sort(keys)
		return keys
	}
	for _, c := range []struct {
		query string
		want  []string
	}{
		{"", []string{onModule.Key, onOT.Key, onLeave.Key}},
		{"?sub_nodes=false", []string{onModule.Key}},
		{"?core=true", []string{onModule.Key, onLeave.Key}},
		{fmt.Sprintf("?client_id=%d", w.a.ID), []string{onOT.Key}},
		{"?type=bug", []string{}},
		{"?from=2999-01-01", []string{}},
	} {
		if got := sorted(timelineKeys(e, w.pm, hr+c.query)); !slices.Equal(got, sorted(c.want)) {
			t.Errorf("timeline%s: %v, want %v", c.query, got, c.want)
		}
	}
}

// Story 5: the Behaviors tab lists the decisions in force, core work first,
// then by client; a Cancelled decision and an open ticket are no behavior.
func TestNodeBehaviorsListDecisionsInForceByClient(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, au := e.signedIn("hana@example.com", false)
	e.seedMember(au, w.p, "admin")
	done, cancelled := statusID(e, admin, "Done"), statusID(e, admin, "Cancelled")
	core := e.seedTicket(w.p, w.pmUser, "Core rule", nil, w.hr)
	forA := e.seedTicket(w.p, w.pmUser, "Client A rule", &w.a, w.ot)
	forB := e.seedTicket(w.p, w.pmUser, "Client B rule", &w.b, w.secret)
	declined := e.seedTicket(w.p, w.pmUser, "Declined for Client A", &w.a, w.ot)
	e.seedTicket(w.p, w.pmUser, "Still open", &w.a, w.ot)
	for _, tk := range []db.Ticket{core, forA, forB} {
		e.seedClose(tk, done, au, "Decided: "+tk.Title)
	}
	e.seedClose(declined, cancelled, au, "Declined: multi-level approval")
	path := fmt.Sprintf("/nodes/%d/behaviors", w.hr.ID)
	for c, want := range map[*http.Client][]string{admin: {core.Key, forA.Key, forB.Key}, w.pm: {core.Key, forA.Key}} {
		var list httpapi.BehaviorList
		if code := e.call(c, http.MethodGet, path, nil, &list); code != http.StatusOK {
			t.Fatalf("behaviors: %d", code)
		}
		var keys []string
		for _, b := range list.Items {
			keys = append(keys, b.Key)
		}
		if !slices.Equal(keys, want) {
			t.Errorf("behaviors: %v, want %v", keys, want)
		}
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd server && go vet ./internal/httpapi/`
Expected: compile errors: `httpapi.NodeDetail`, `httpapi.TimelinePage` and `httpapi.BehaviorList` are undefined.

- [ ] **Step 3: Extend the contract**

In `api/openapi.yaml`, under the existing `/nodes/{id}:` path, add before `patch:`:

```yaml
    get:
      operationId: getNode
      tags: [nodes]
      description: Every member who sees the node, archived or not (R-MR-4), with the path of its parents.
      responses:
        "200":
          description: The node and its path.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/NodeDetail" }
        default: { $ref: "#/components/responses/Problem" }
```

After the `/nodes/{id}:` path, add:

```yaml
  /nodes/{id}/timeline:
    parameters:
      - { name: id, in: path, required: true, schema: { type: integer, format: int64 } }
    get:
      operationId: getNodeTimeline
      tags: [nodes]
      description: >-
        The visible tickets on the node, and by default on its sub-nodes: open ones first, newest first, then closed
        ones by close date, newest first, with their decision records (FSD §7.4). Pages follow next_cursor.
      parameters:
        - { name: sub_nodes, in: query, schema: { type: boolean, default: true } }
        - { name: client_id, in: query, schema: { type: integer, format: int64 } }
        - { name: core, in: query, schema: { type: boolean }, description: Only core work (no client). }
        - { name: type, in: query, schema: { $ref: "#/components/schemas/TicketType" } }
        - { name: from, in: query, schema: { type: string, format: date }, description: Entries on or after this day; an entry's day is its close day, or its creation day while open. }
        - { name: to, in: query, schema: { type: string, format: date }, description: Entries on or before this day. }
        - { name: limit, in: query, schema: { type: integer, format: int32, minimum: 1, maximum: 1000 } }
        - { name: cursor, in: query, schema: { type: string } }
      responses:
        "200":
          description: One page of the timeline.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/TimelinePage" }
        default: { $ref: "#/components/responses/Problem" }
  /nodes/{id}/behaviors:
    parameters:
      - { name: id, in: path, required: true, schema: { type: integer, format: int64 } }
    get:
      operationId: getNodeBehaviors
      tags: [nodes]
      description: The decisions in force on the node, and by default on its sub-nodes; core work first, then by client (FSD §7.4).
      parameters:
        - { name: sub_nodes, in: query, schema: { type: boolean, default: true } }
      responses:
        "200":
          description: Confirmed, implemented decisions.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/BehaviorList" }
        default: { $ref: "#/components/responses/Problem" }
```

After the `NodeMove` schema, add:

```yaml
    NodeDetail:
      type: object
      required: [node, path, project_key]
      properties:
        node: { $ref: "#/components/schemas/Node" }
        path:
          type: array
          description: The node's parents, top first.
          items: { $ref: "#/components/schemas/Ref" }
        project_key: { type: string }
    TimelineEntry:
      type: object
      required: [key, title, type, status, requester, created_at]
      properties:
        key: { type: string }
        title: { type: string }
        type: { $ref: "#/components/schemas/TicketType" }
        status: { $ref: "#/components/schemas/Status" }
        client: { $ref: "#/components/schemas/Ref" }
        requester: { $ref: "#/components/schemas/TicketRequester" }
        created_at: { type: string, format: date-time }
        closed_at: { type: string, format: date-time }
        decision: { $ref: "#/components/schemas/DecisionRecord" }
    TimelinePage:
      type: object
      required: [items, next_cursor]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/TimelineEntry" }
        next_cursor: { type: string, nullable: true }
    Behavior:
      type: object
      required: [key, title, what_changed, why, alternatives]
      properties:
        key: { type: string }
        title: { type: string }
        client: { $ref: "#/components/schemas/Ref" }
        closed_at: { type: string, format: date-time }
        what_changed: { type: string }
        why: { type: string }
        alternatives: { type: string }
    BehaviorList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/Behavior" }
```

Then run `make generate`.

- [ ] **Step 4: Implement**

`server/internal/httpapi/node_page.go`:

```go
package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

// GetNode reads one node for its page (FSD §7.4), archived or not, with its
// parents from the top of the tree. A node the user may not see answers 404
// (R-AC-5), and it names only the clients in the user's scope (R-MR-8).
func (s *Server) GetNode(w http.ResponseWriter, r *http.Request, id int64) {
	pc, _, ok := s.nodeFor(w, r, id, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.visibleNodes(r.Context(), pc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byID := map[int64]db.ListNodesRow{}
	for _, n := range rows {
		byID[n.ID] = n
	}
	out := NodeDetail{Node: toAPINodeRow(byID[id]), Path: []Ref{}, ProjectKey: pc.project.Key}
	for p := byID[id].ParentID; p != nil; p = byID[*p].ParentID {
		out.Path = append([]Ref{{Id: *p, Name: byID[*p].Name}}, out.Path...)
	}
	writeJSON(w, http.StatusOK, out)
}

// GetNodeTimeline lists the visible tickets on a node, by default with its
// sub-nodes (AC-MR-4): open ones first, then closed ones by close date, newest
// first, each with its decision record (FSD §7.4, AC-MR-3).
// ponytail: an offset cursor, like the ticket list.
func (s *Server) GetNodeTimeline(w http.ResponseWriter, r *http.Request, id int64, params GetNodeTimelineParams) {
	pc, _, ok := s.nodeFor(w, r, id, access.Viewer)
	if !ok {
		return
	}
	limit, offset, ok := paging(w, params.Limit, params.Cursor)
	if !ok {
		return
	}
	ctx := r.Context()
	ids, err := s.nodeScope(ctx, pc, id, params.SubNodes)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	filter := db.ListNodeTimelineParams{
		ProjectID: pc.project.ID, NodeIds: ids, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
		ClientID: params.ClientId, CoreOnly: deref(params.Core), Lim: int32(limit + 1), Off: int32(offset),
	}
	if params.Type != nil {
		filter.Type = ptr(string(*params.Type))
	}
	if params.From != nil {
		filter.FromDate = &params.From.Time
	}
	if params.To != nil {
		filter.ToDate = &params.To.Time
	}
	rows, err := s.q.ListNodeTimeline(ctx, filter)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := TimelinePage{Items: make([]TimelineEntry, 0, len(rows))}
	if len(rows) > limit {
		rows = rows[:limit]
		out.NextCursor = ptr(strconv.Itoa(offset + limit))
	}
	for _, t := range rows {
		out.Items = append(out.Items, toTimelineEntry(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetNodeBehaviors lists the decisions in force on a node, by default with its
// sub-nodes: confirmed and implemented, core work first, then by client (FSD
// §7.4, story 5).
func (s *Server) GetNodeBehaviors(w http.ResponseWriter, r *http.Request, id int64, params GetNodeBehaviorsParams) {
	pc, _, ok := s.nodeFor(w, r, id, access.Viewer)
	if !ok {
		return
	}
	ctx := r.Context()
	ids, err := s.nodeScope(ctx, pc, id, params.SubNodes)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rows, err := s.q.ListNodeBehaviors(ctx, db.ListNodeBehaviorsParams{
		ProjectID: pc.project.ID, NodeIds: ids, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := BehaviorList{Items: make([]Behavior, len(rows))}
	for i, b := range rows {
		out.Items[i] = Behavior{Key: b.Key, Title: b.Title, ClosedAt: b.ClosedAt, WhatChanged: b.WhatChanged, Why: b.Why, Alternatives: b.Alternatives}
		if b.ClientID != nil {
			out.Items[i].Client = &Ref{Id: *b.ClientID, Name: deref(b.ClientName)}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// visibleNodes lists the project's nodes the caller sees, archived ones too:
// node pages and sub-node filters keep their history (R-MR-4).
func (s *Server) visibleNodes(ctx context.Context, pc projectCtx) ([]db.ListNodesRow, error) {
	return s.q.ListNodes(ctx, db.ListNodesParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs), IncludeArchived: true,
	})
}

// nodeScope is the node, plus its visible sub-nodes unless subNodes is false.
func (s *Server) nodeScope(ctx context.Context, pc projectCtx, id int64, subNodes *bool) ([]int64, error) {
	if subNodes != nil && !*subNodes {
		return []int64{id}, nil
	}
	rows, err := s.visibleNodes(ctx, pc)
	if err != nil {
		return nil, err
	}
	return subtree(rows, id), nil
}

func toTimelineEntry(t db.ListNodeTimelineRow) TimelineEntry {
	out := TimelineEntry{
		Key: t.Key, Title: t.Title, Type: TicketType(t.Type), Status: toAPIStatus(t.Status),
		CreatedAt: t.CreatedAt, ClosedAt: t.ClosedAt,
	}
	if t.ClientID != nil {
		out.Client = &Ref{Id: *t.ClientID, Name: deref(t.ClientName)}
	}
	if t.RequesterContactID != nil {
		out.Requester = TicketRequester{Kind: TicketRequesterKindContact, Id: *t.RequesterContactID,
			Name: deref(t.RequesterContactName), Title: t.RequesterContactTitle}
	} else {
		out.Requester = TicketRequester{Kind: TicketRequesterKindUser, Id: deref(t.RequesterUserID), Name: deref(t.RequesterUserName)}
	}
	if t.State != nil {
		out.Decision = &DecisionRecord{
			WhatChanged: deref(t.WhatChanged), Why: deref(t.Why), Alternatives: deref(t.Alternatives),
			Outcome: DecisionOutcome(deref(t.Outcome)), State: DecisionState(*t.State), ConfirmedAt: t.ConfirmedAt,
		}
		if t.ConfirmedBy != nil {
			out.Decision.ConfirmedBy = &Ref{Id: *t.ConfirmedBy, Name: deref(t.ConfirmerName)}
		}
	}
	return out
}
```

In `server/internal/httpapi/nodes.go`, replace the body of the loop in `ListNodes`, and the `clients` slice it builds, with `items[i] = toAPINodeRow(n)`. Then add below `toAPINode`:

```go
func toAPINodeRow(n db.ListNodesRow) Node {
	clients := make([]NodeClient, len(n.ClientIds))
	for j, id := range n.ClientIds {
		clients[j] = NodeClient{Id: id, Name: n.ClientNames[j]}
	}
	return Node{
		Id: n.ID, ParentId: n.ParentID, Type: NodeType(n.Type), Name: n.Name, Code: n.Code,
		Aliases: orEmpty(n.Aliases), Description: n.Description, ClientSpecific: n.ClientSpecific,
		Clients: clients, Position: n.Position, Archived: n.Archived,
	}
}
```

In `server/internal/httpapi/ticket_list.go`:
1. Replace the `limit, offset := 50, 0` block, through the cursor check, with:

```go
	limit, offset, ok := paging(w, params.Limit, params.Cursor)
	if !ok {
		return
	}
```

2. In the `params.NodeId` branch, replace the `s.q.ListNodes(ctx, db.ListNodesParams{…})` call with `s.visibleNodes(ctx, pc)`.
3. Add below `ListTickets`:

```go
// paging reads a page size (50 by default, at most 1,000) and an offset cursor
// that an earlier page gave; a cursor it did not give answers 400.
func paging(w http.ResponseWriter, lim *int32, cursor *string) (int, int, bool) {
	limit, offset := 50, 0
	if lim != nil {
		limit = min(max(int(*lim), 1), 1000)
	}
	if cursor != nil {
		n, err := strconv.Atoi(*cursor)
		if err != nil || n < 0 {
			writeProblem(w, http.StatusBadRequest, "invalid_parameter", "The cursor is not one this API gave")
			return 0, 0, false
		}
		offset = n
	}
	return limit, offset, true
}
```

- [ ] **Step 5: Run the tests**

Run: `cd server && gofmt -l . && go vet ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...`
Expected: no output from gofmt and vet, and `ok` for every package.

- [ ] **Step 6: Commit**

```bash
git add api/openapi.yaml server/internal/httpapi web/lib/api-types.ts
git commit -m "feat(server): node page API with timeline and behaviors by client"
```

### Task 6: The search API

**Files:**
- Modify: `api/openapi.yaml` (`GET /search`, `SearchTicket`, `SearchNode`, `SearchResults`)
- Create: `server/internal/httpapi/search.go`
- Regenerate: `server/internal/httpapi/api.gen.go`, `web/lib/api-types.ts`
- Test: `server/internal/httpapi/search_test.go`

**Interfaces:**
- Consumes: `SearchTickets`, `SearchNodes` (Task 1).
- Produces:
  - `GET /search?q=` returns `SearchResults{Tickets []SearchTicket; Nodes []SearchNode}`:
    - `SearchTicket{Key, Title, ProjectKey string; Status Status; Client *Ref}`;
    - `SearchNode{Id int64; Name string; Type NodeType; Code *string; Aliases []string; ProjectKey string; Path []string}`.
  - A query under 2 characters finds nothing; one over 200 answers 400.

- [ ] **Step 1: Write the failing test**

`server/internal/httpapi/search_test.go`:

```go
package httpapi_test

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// FSD §6.1–6.2: search finds tickets by key, by words or by part of a title,
// and nodes by name, alias or code, only where the user may look (R-AC-7).
func TestSearchFindsTicketsAndNodesInScope(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	code := "HR.ATT.OT"
	if _, err := e.q.UpdateNode(ctx, db.UpdateNodeParams{ID: w.ot.ID, Code: &code, Aliases: []string{"Persetujuan Lembur"}}); err != nil {
		t.Fatal(err)
	}
	approval := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval for overtime", &w.a, w.ot)
	if _, err := e.d.Pool.Exec(ctx, "UPDATE tickets SET reason = 'HR approves lembur directly' WHERE id = $1", approval.ID); err != nil {
		t.Fatal(err)
	}
	e.seedTicket(w.p, w.pmUser, "Overtime report for Client B", &w.b, w.secret) // hidden from the PM, who sees Client A
	search := func(q string) (tickets, nodes []string) {
		t.Helper()
		var res httpapi.SearchResults
		if code := e.call(w.pm, http.MethodGet, "/search?q="+url.QueryEscape(q), nil, &res); code != http.StatusOK {
			t.Fatalf("search %q: %d", q, code)
		}
		for _, tk := range res.Tickets {
			tickets = append(tickets, tk.Key)
		}
		for _, n := range res.Nodes {
			nodes = append(nodes, n.Name)
		}
		return tickets, nodes
	}
	for _, c := range []struct {
		q              string
		tickets, nodes []string
	}{
		{"hris-1", []string{"HRIS-1"}, nil},                           // a key, in any case
		{"overt", []string{"HRIS-1"}, []string{"Overtime Approval"}},  // part of a title and of a name
		{"lembur", []string{"HRIS-1"}, []string{"Overtime Approval"}}, // a word in the reason, and an alias
		{"hr.att", nil, []string{"Overtime Approval"}},                // a code
		{"report", nil, nil},                                          // Client B's ticket and menu stay hidden
		{"o", nil, nil},                                               // too short to search
	} {
		tickets, nodes := search(c.q)
		if !slices.Equal(tickets, c.tickets) || !slices.Equal(nodes, c.nodes) {
			t.Errorf("search %q: %v %v, want %v %v", c.q, tickets, nodes, c.tickets, c.nodes)
		}
	}
	var res httpapi.SearchResults
	e.call(w.pm, http.MethodGet, "/search?q=overtime", nil, &res)
	if len(res.Nodes) != 1 || !slices.Equal(res.Nodes[0].Path, []string{"HR", "Overtime Approval"}) || res.Nodes[0].ProjectKey != "HRIS" ||
		res.Nodes[0].Code == nil || *res.Nodes[0].Code != code || !slices.Equal(res.Nodes[0].Aliases, []string{"Persetujuan Lembur"}) {
		t.Fatalf("the node result: %+v", res.Nodes)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd server && go vet ./internal/httpapi/`
Expected: a compile error: `httpapi.SearchResults` is undefined.

- [ ] **Step 3: Extend the contract**

In `api/openapi.yaml`, at the end of `paths:` (after `/attachments/{id}`), add:

```yaml
  /search:
    get:
      operationId: search
      tags: [search]
      description: >-
        Tickets and nodes the caller may open, across their projects (FSD §6.1): a ticket key, words in a ticket or
        part of its title; part of a node's name, an alias or its code. Fewer than 2 characters find nothing.
      parameters:
        - { name: q, in: query, required: true, schema: { type: string, maxLength: 200 } }
      responses:
        "200":
          description: At most 50 tickets and 20 nodes.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/SearchResults" }
        default: { $ref: "#/components/responses/Problem" }
```

At the end of the file, after the last schema, add:

```yaml
    SearchTicket:
      type: object
      required: [key, title, project_key, status]
      properties:
        key: { type: string }
        title: { type: string }
        project_key: { type: string }
        status: { $ref: "#/components/schemas/Status" }
        client: { $ref: "#/components/schemas/Ref" }
    SearchNode:
      type: object
      required: [id, name, type, code, aliases, project_key, path]
      properties:
        id: { type: integer, format: int64 }
        name: { type: string }
        type: { $ref: "#/components/schemas/NodeType" }
        code: { type: string, nullable: true }
        aliases:
          type: array
          items: { type: string }
        project_key: { type: string }
        path:
          type: array
          description: The names from the top of the tree down to this node.
          items: { type: string }
    SearchResults:
      type: object
      required: [tickets, nodes]
      properties:
        tickets:
          type: array
          items: { $ref: "#/components/schemas/SearchTicket" }
        nodes:
          type: array
          items: { $ref: "#/components/schemas/SearchNode" }
```

Then run `make generate`.

- [ ] **Step 4: Implement**

`server/internal/httpapi/search.go`:

```go
package httpapi

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/kenzo03/muasal/server/internal/db"
)

// Search finds the tickets and nodes the user may open, across their projects
// (FSD §6.1, §6.2). The membership check runs in SQL, so a hidden row never
// leaves the database (R-AC-7); the web jumps straight to a ticket whose key
// matches.
func (s *Server) Search(w http.ResponseWriter, r *http.Request, params SearchParams) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	q := strings.TrimSpace(params.Q)
	if utf8.RuneCountInString(q) > 200 {
		writeProblem(w, http.StatusBadRequest, "invalid_parameter", "Search for at most 200 characters")
		return
	}
	out := SearchResults{Tickets: []SearchTicket{}, Nodes: []SearchNode{}}
	if utf8.RuneCountInString(q) < 2 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ctx := r.Context()
	tickets, err := s.q.SearchTickets(ctx, db.SearchTicketsParams{IsAdmin: u.IsAdmin, UserID: u.ID, Q: q})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	nodes, err := s.q.SearchNodes(ctx, db.SearchNodesParams{IsAdmin: u.IsAdmin, UserID: u.ID, Q: q})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, t := range tickets {
		it := SearchTicket{Key: t.Key, Title: t.Title, ProjectKey: t.ProjectKey, Status: toAPIStatus(t.Status)}
		if t.ClientID != nil {
			it.Client = &Ref{Id: *t.ClientID, Name: deref(t.ClientName)}
		}
		out.Tickets = append(out.Tickets, it)
	}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, SearchNode{
			Id: n.ID, Name: n.Name, Type: NodeType(n.Type), Code: n.Code, Aliases: orEmpty(n.Aliases),
			ProjectKey: n.ProjectKey, Path: orEmpty(n.Path),
		})
	}
	writeJSON(w, http.StatusOK, out)
}
```

- [ ] **Step 5: Run the tests**

Run: `cd server && gofmt -l . && go vet ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...`
Expected: no output from gofmt and vet, and `ok` for every package.

- [ ] **Step 6: Commit**

```bash
git add api/openapi.yaml server/internal/httpapi web/lib/api-types.ts
git commit -m "feat(server): search tickets and nodes across projects"
```

### Task 7: The Home API

**Files:**
- Create: `server/migrations/00006_recent_changes.sql`
- Create: `server/internal/db/queries/home.sql`
- Modify: `api/openapi.yaml` (`GET /me/tickets`, `GET /me/updates`, their schemas)
- Create: `server/internal/httpapi/home.go`
- Regenerate: `server/internal/httpapi/api.gen.go`, `web/lib/api-types.ts`
- Test: `server/internal/httpapi/home_test.go`

**Interfaces:**
- Consumes: `paging` (Task 5); `toAPIStatus`, `requireUser`, `deref`, `ptr` (Iterations 0–2); `SetTicketStatus{…, Closed}` (Task 1).
- Produces:
  - Queries (package `db`), all taking `IsAdmin bool; UserID int64`:
    - `ListMyTickets(ctx, ListMyTicketsParams{IsAdmin bool; UserID int64; View string; Lim, Off int32}) ([]ListMyTicketsRow, error)`. A row has `ID int64; Key, Title, Type, Priority string; DueDate *time.Time; ClientID *int64; ClientName *string; Status Status; MissingReason, MissingMenus bool; Menu string` ("" without a menu).
    - `CountMyTickets(ctx, CountMyTicketsParams) (CountMyTicketsRow{AllOpen, Overdue, Week, Incomplete int64}, error)`
    - `CountMyTicketsByProject(ctx, CountMyTicketsByProjectParams) ([]CountMyTicketsByProjectRow{Key string; Open int64}, error)`
    - `ListRecentTickets(ctx, ListRecentTicketsParams) ([]ListRecentTicketsRow{ID int64; Key, Title, Type string; UpdatedAt time.Time; Status Status}, error)`
    - `LatestTicketChanges(ctx, ticketIDs []int64) ([]LatestTicketChangesRow{TicketID int64; At time.Time; ID int64; ActorID int64; ActorName *string; Action string; Changes []byte}, error)`, with `ActorID` 0 for a change without an actor.
  - API types:
    - `MyTicketsView` (`MyTicketsViewAll`, `MyTicketsViewOverdue`, `MyTicketsViewWeek`, `MyTicketsViewIncomplete`);
    - `MyTicket{Key, Title string; Type TicketType; Priority Priority; DueDate *openapi_types.Date; Status Status; Client *Ref; Menu *string; MissingReason, MissingMenus bool}`;
    - `MyTicketsCounts{All, Overdue, Week, Incomplete int}`, `ProjectCount{Key string; Open int}`;
    - `MyTicketsPage{Items []MyTicket; NextCursor *string; Counts MyTicketsCounts; Projects []ProjectCount}`;
    - `RecentTicket{Key, Title string; Type TicketType; Status Status; UpdatedAt time.Time; Change *ActivityItem}` and `RecentTicketList{Items []RecentTicket}`.
  - Endpoints:
    - `GET /me/tickets?view=&limit=&cursor=`: an unknown view answers 400.
    - `GET /me/updates`: at most 10 tickets. A change is an `ActivityItem`: kind `event` with its action and changes, or kind `comment` without its body.

- [ ] **Step 1: Write the failing tests**

`server/internal/httpapi/home_test.go`:

```go
package httpapi_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// assign gives a ticket to a user, due in dueIn days from today (nil: no due
// date), with this reason.
func (e *env) assign(tk db.Ticket, to db.User, dueIn *int, reason string) {
	e.t.Helper()
	_, err := e.d.Pool.Exec(context.Background(),
		"UPDATE tickets SET assignee_id = $2, due_date = current_date + $3::int, reason = $4 WHERE id = $1", tk.ID, to.ID, dueIn, reason)
	if err != nil {
		e.t.Fatal(err)
	}
}

func days(n int) *int { return &n }

func myKeys(p httpapi.MyTicketsPage) []string {
	out := []string{}
	for _, it := range p.Items {
		out = append(out, it.Key)
	}
	return out
}

// FSD §6.4: My tickets are the caller's open tickets that they may see,
// earliest due date first and undated last; the tabs narrow them, and the
// counts cover all of them whatever tab is shown.
func TestHomeListsMyOpenTicketsByDueDate(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	reason := "Client A supervisors are often on leave."
	later := e.seedTicket(w.p, w.pmUser, "Due in five days", &w.a, w.ot)
	e.assign(later, w.pmUser, days(5), reason)
	overdue := e.seedTicket(w.p, w.pmUser, "Due yesterday", &w.a, w.ot)
	e.assign(overdue, w.pmUser, days(-1), reason)
	undated := e.seedTicket(w.p, w.pmUser, "No due date and no menu", &w.a)
	e.assign(undated, w.pmUser, nil, reason)
	noReason := e.seedTicket(w.p, w.pmUser, "Due in ten days, no reason", nil, w.ot)
	e.assign(noReason, w.pmUser, days(10), "")
	closed := e.seedTicket(w.p, w.pmUser, "Already done", &w.a, w.ot)
	e.assign(closed, w.pmUser, days(1), reason)
	if _, err := e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: closed.ID, StatusID: statusID(e, w.pm, "Done"), Closed: true}); err != nil {
		t.Fatal(err)
	}
	hidden := e.seedTicket(w.p, w.pmUser, "Client B work", &w.b, w.secret) // the PM sees Client A only
	e.assign(hidden, w.pmUser, days(2), reason)
	_, ani := e.signedIn("ani@example.com", false)
	e.seedMember(ani, w.p, "member")
	e.assign(e.seedTicket(w.p, w.pmUser, "Ani's work", &w.a, w.ot), ani, days(3), reason)

	var page httpapi.MyTicketsPage
	if code := e.call(w.pm, http.MethodGet, "/me/tickets", nil, &page); code != http.StatusOK {
		t.Fatalf("my tickets: %d", code)
	}
	if got, want := myKeys(page), []string{overdue.Key, later.Key, noReason.Key, undated.Key}; !slices.Equal(got, want) {
		t.Fatalf("order: %v, want %v", got, want)
	}
	if c := page.Counts; c.All != 4 || c.Overdue != 1 || c.Week != 1 || c.Incomplete != 2 {
		t.Fatalf("counts: %+v", c)
	}
	if len(page.Projects) != 1 || page.Projects[0].Key != "HRIS" || page.Projects[0].Open != 4 {
		t.Fatalf("projects: %+v", page.Projects)
	}
	first, last := page.Items[0], page.Items[3]
	if first.Menu == nil || *first.Menu != "HR › Overtime Approval" || first.Status.Name != "To do" || first.MissingReason || first.MissingMenus ||
		first.Client == nil || first.Client.Name != "Client A" || first.DueDate == nil {
		t.Fatalf("the first row: %+v", first)
	}
	if last.Menu != nil || !last.MissingMenus || last.DueDate != nil {
		t.Fatalf("the undated row: %+v", last)
	}
	for view, want := range map[string][]string{
		"overdue":    {overdue.Key},
		"week":       {later.Key},
		"incomplete": {noReason.Key, undated.Key},
	} {
		var p httpapi.MyTicketsPage
		e.call(w.pm, http.MethodGet, "/me/tickets?view="+view, nil, &p)
		if got := myKeys(p); !slices.Equal(got, want) || p.Counts.All != 4 {
			t.Errorf("view %s: %v %+v, want %v", view, got, p.Counts, want)
		}
	}
	if code := e.call(w.pm, http.MethodGet, "/me/tickets?view=someday", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("an unknown view: %d", code)
	}
}

// FSD §6.4: Recently updated lists the tickets the caller may see that changed
// last, newest first, each with its latest change: an event or a comment.
func TestHomeListsRecentChangesInScope(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	create := func(title string) httpapi.Ticket {
		t.Helper()
		var tk httpapi.Ticket
		body := map[string]any{"title": title, "type": "bug", "client_id": w.a.ID, "requester_contact_id": w.budi, "node_ids": []int64{w.ot.ID}}
		if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", body, &tk); code != http.StatusCreated {
			t.Fatalf("create %q: %d", title, code)
		}
		return tk
	}
	untouched, moved, commented := create("Created only"), create("Moved to In progress"), create("Commented on")
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+moved.Key+"/transition", map[string]any{"status_id": statusID(e, w.pm, "In progress")}, nil); code != http.StatusOK {
		t.Fatalf("move: %d", code)
	}
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+commented.Key+"/comments", map[string]any{"body": "Budi confirmed by phone."}, nil); code != http.StatusCreated {
		t.Fatalf("comment: %d", code)
	}
	hidden := e.seedTicket(w.p, w.pmUser, "Client B work", &w.b, w.secret) // changed last, but the PM sees Client A only
	if _, err := e.d.Pool.Exec(ctx, "UPDATE tickets SET updated_at = now() + interval '1 hour' WHERE id = $1", hidden.ID); err != nil {
		t.Fatal(err)
	}

	var list httpapi.RecentTicketList
	if code := e.call(w.pm, http.MethodGet, "/me/updates", nil, &list); code != http.StatusOK {
		t.Fatalf("updates: %d", code)
	}
	var got []string
	for _, it := range list.Items {
		if it.Change == nil {
			t.Fatalf("%s has no change", it.Key)
		}
		got = append(got, it.Key+" "+string(it.Change.Kind)+" "+orBlank(it.Change.Action))
	}
	want := []string{commented.Key + " comment ", moved.Key + " event transition", untouched.Key + " event create"}
	if !slices.Equal(got, want) {
		t.Fatalf("updates: %q, want %q", got, want)
	}
	if c := list.Items[0].Change; c.Actor == nil || c.Actor.Id != w.pmUser.ID || c.Body != nil || list.Items[1].Status.Name != "In progress" {
		t.Fatalf("the comment change: %+v", c)
	}
}

// orBlank is a pointer's text, or "" for nil.
func orBlank(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd server && go vet ./internal/httpapi/`
Expected: compile errors: `httpapi.MyTicketsPage` and `httpapi.RecentTicketList` are undefined.

- [ ] **Step 3: Write the migration and the queries**

`server/migrations/00006_recent_changes.sql`:

```sql
-- +goose Up
-- Home (FSD §6.4). Comments, files and decision records are changes to their
-- ticket, so they touch its updated_at; "Recently updated" then reads tickets
-- by updated_at alone. My tickets read by assignee.
-- +goose StatementBegin
CREATE FUNCTION touch_ticket() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  UPDATE tickets SET updated_at = now() WHERE id = NEW.ticket_id;
  RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER comments_touch_ticket AFTER INSERT OR UPDATE ON comments FOR EACH ROW EXECUTE FUNCTION touch_ticket();
CREATE TRIGGER attachments_touch_ticket AFTER INSERT OR UPDATE ON attachments FOR EACH ROW EXECUTE FUNCTION touch_ticket();
CREATE TRIGGER decision_records_touch_ticket AFTER INSERT OR UPDATE ON decision_records FOR EACH ROW EXECUTE FUNCTION touch_ticket();
CREATE INDEX tickets_updated_idx ON tickets (updated_at DESC);
CREATE INDEX tickets_assignee_idx ON tickets (assignee_id) WHERE assignee_id IS NOT NULL;

-- +goose Down
DROP INDEX tickets_assignee_idx;
DROP INDEX tickets_updated_idx;
DROP TRIGGER decision_records_touch_ticket ON decision_records;
DROP TRIGGER attachments_touch_ticket ON attachments;
DROP TRIGGER comments_touch_ticket ON comments;
DROP FUNCTION touch_ticket();
```

`server/internal/db/queries/home.sql`:

```sql
-- name: ListMyTickets :many
-- Home's My tickets (FSD §6.4): the open tickets assigned to the user that
-- they may see in any project (R-AC-2, R-AC-3), by due date (none last), then
-- priority and key. view narrows them: overdue, week (due today through 7 days
-- ahead) or incomplete (no reason or no menu). menu is the first menu's parent
-- and name, or '' without one.
SELECT t.id, t.key, t.title, t.type, t.priority, t.due_date, t.client_id, c.name AS client_name, sqlc.embed(s),
       t.reason = '' AS missing_reason,
       NOT EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id) AS missing_menus,
       coalesce((SELECT coalesce(pn.name || ' › ', '') || n.name
                 FROM ticket_nodes tn JOIN nodes n ON n.id = tn.node_id LEFT JOIN nodes pn ON pn.id = n.parent_id
                 WHERE tn.ticket_id = t.id ORDER BY lower(n.name), n.id LIMIT 1), '')::text AS menu
FROM tickets t
JOIN statuses s ON s.id = t.status_id
LEFT JOIN clients c ON c.id = t.client_id
WHERE t.assignee_id = sqlc.arg('user_id')::bigint
  AND s.category IN ('todo', 'in_progress')
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))))
  AND (sqlc.arg('view')::text = 'all'
       OR (sqlc.arg('view')::text = 'overdue' AND t.due_date < current_date)
       OR (sqlc.arg('view')::text = 'week' AND t.due_date BETWEEN current_date AND current_date + 7)
       OR (sqlc.arg('view')::text = 'incomplete' AND (t.reason = '' OR NOT EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id))))
ORDER BY t.due_date NULLS LAST, array_position(ARRAY['urgent', 'high', 'medium', 'low'], t.priority), t.project_id, t.number
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountMyTickets :one
-- The counts of Home's tabs, over all of the user's open tickets.
SELECT count(*) AS all_open,
       count(*) FILTER (WHERE t.due_date < current_date) AS overdue,
       count(*) FILTER (WHERE t.due_date BETWEEN current_date AND current_date + 7) AS week,
       count(*) FILTER (WHERE t.reason = '' OR NOT EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id)) AS incomplete
FROM tickets t
JOIN statuses s ON s.id = t.status_id
WHERE t.assignee_id = sqlc.arg('user_id')::bigint
  AND s.category IN ('todo', 'in_progress')
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))));

-- name: CountMyTicketsByProject :many
-- The user's open tickets per project, for Home's My projects.
SELECT p.key, count(*) AS open
FROM tickets t
JOIN statuses s ON s.id = t.status_id
JOIN projects p ON p.id = t.project_id
WHERE t.assignee_id = sqlc.arg('user_id')::bigint
  AND s.category IN ('todo', 'in_progress')
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))))
GROUP BY p.key
ORDER BY p.key;

-- name: ListRecentTickets :many
-- Home's Recently updated (FSD §6.4): the tickets the user may see that changed
-- last. Comments, files and decision records touch updated_at (00006).
SELECT t.id, t.key, t.title, t.type, t.updated_at, sqlc.embed(s)
FROM tickets t
JOIN statuses s ON s.id = t.status_id
WHERE (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))))
ORDER BY t.updated_at DESC, t.id DESC
LIMIT 10;

-- name: LatestTicketChanges :many
-- The latest change on each of these tickets: a history event or a comment.
-- Events of one transaction share a time, so the later row wins. actor_id is 0
-- for a change without an actor.
SELECT DISTINCT ON (x.ticket_id) x.ticket_id::bigint AS ticket_id, x.at::timestamptz AS at, x.id::bigint AS id,
       coalesce(x.actor_id, 0)::bigint AS actor_id, u.name AS actor_name, x.action::text AS action, x.changes::jsonb AS changes
FROM (
  SELECT e.entity_id AS ticket_id, e.occurred_at AS at, e.id, e.actor_id, e.action, e.changes
  FROM audit_events e
  WHERE e.entity = 'ticket' AND e.entity_id = ANY (sqlc.arg('ticket_ids')::bigint[])
  UNION ALL
  SELECT c.ticket_id, c.created_at, c.id, c.author_id, 'comment', '{}'::jsonb
  FROM comments c
  WHERE c.ticket_id = ANY (sqlc.arg('ticket_ids')::bigint[]) AND c.deleted_at IS NULL
) x
LEFT JOIN users u ON u.id = x.actor_id
ORDER BY x.ticket_id, x.at DESC, x.id DESC;
```

Run: `cd server && go generate ./... && go build ./...`
Expected: the build succeeds. If sqlc names a field differently from the Interfaces block (for example `Menu` or `Open`), use the generated name in Step 5.

- [ ] **Step 4: Extend the contract**

In `api/openapi.yaml`, after the `/me:` path, add:

```yaml
  /me/tickets:
    get:
      operationId: listMyTickets
      tags: [me]
      description: >-
        Home's My tickets (FSD §6.4): the open tickets assigned to the caller in every project, as far as the caller
        may see them. view narrows them; the counts cover every view and project, whatever the view.
      parameters:
        - { name: view, in: query, schema: { $ref: "#/components/schemas/MyTicketsView" } }
        - { name: limit, in: query, schema: { type: integer, format: int32, minimum: 1, maximum: 1000 } }
        - { name: cursor, in: query, schema: { type: string } }
      responses:
        "200":
          description: One page of the caller's open tickets, with the counts.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/MyTicketsPage" }
        default: { $ref: "#/components/responses/Problem" }
  /me/updates:
    get:
      operationId: listMyUpdates
      tags: [me]
      description: >-
        Home's Recently updated (FSD §6.4): the 10 tickets the caller may see that changed last, newest first, each
        with its latest change. A comment change carries no text.
      responses:
        "200":
          description: The latest changed tickets.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/RecentTicketList" }
        default: { $ref: "#/components/responses/Problem" }
```

At the end of the file, after the last schema, add:

```yaml
    MyTicketsView:
      type: string
      enum: [all, overdue, week, incomplete]
    MyTicket:
      type: object
      required: [key, title, type, priority, status, missing_reason, missing_menus]
      properties:
        key: { type: string }
        title: { type: string }
        type: { $ref: "#/components/schemas/TicketType" }
        priority: { $ref: "#/components/schemas/Priority" }
        due_date: { type: string, format: date }
        status: { $ref: "#/components/schemas/Status" }
        client: { $ref: "#/components/schemas/Ref" }
        menu: { type: string, description: 'The first menu''s parent and name, e.g. "Payroll › Payslip".' }
        missing_reason: { type: boolean }
        missing_menus: { type: boolean }
    MyTicketsCounts:
      type: object
      required: [all, overdue, week, incomplete]
      properties:
        all: { type: integer }
        overdue: { type: integer }
        week: { type: integer }
        incomplete: { type: integer }
    ProjectCount:
      type: object
      required: [key, open]
      properties:
        key: { type: string }
        open: { type: integer }
    MyTicketsPage:
      type: object
      required: [items, next_cursor, counts, projects]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/MyTicket" }
        next_cursor: { type: string, nullable: true }
        counts: { $ref: "#/components/schemas/MyTicketsCounts" }
        projects:
          type: array
          description: The caller's open tickets per project; projects without any are left out.
          items: { $ref: "#/components/schemas/ProjectCount" }
    RecentTicket:
      type: object
      required: [key, title, type, status, updated_at]
      properties:
        key: { type: string }
        title: { type: string }
        type: { $ref: "#/components/schemas/TicketType" }
        status: { $ref: "#/components/schemas/Status" }
        updated_at: { type: string, format: date-time }
        change: { $ref: "#/components/schemas/ActivityItem" }
    RecentTicketList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/RecentTicket" }
```

Then run `make generate`.

- [ ] **Step 5: Implement**

`server/internal/httpapi/home.go`:

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kenzo03/muasal/server/internal/db"
)

// ListMyTickets serves Home's My tickets (FSD §6.4): the open tickets assigned
// to the caller in any project, as far as the caller may see them (R-AC-7).
// The counts cover every view and project, whatever view the page shows.
func (s *Server) ListMyTickets(w http.ResponseWriter, r *http.Request, params ListMyTicketsParams) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	view := MyTicketsViewAll
	if params.View != nil {
		view = *params.View
	}
	switch view {
	case MyTicketsViewAll, MyTicketsViewOverdue, MyTicketsViewWeek, MyTicketsViewIncomplete:
	default:
		writeProblem(w, http.StatusBadRequest, "invalid_parameter", "view is one of all, overdue, week and incomplete")
		return
	}
	limit, offset, ok := paging(w, params.Limit, params.Cursor)
	if !ok {
		return
	}
	ctx := r.Context()
	rows, err := s.q.ListMyTickets(ctx, db.ListMyTicketsParams{IsAdmin: u.IsAdmin, UserID: u.ID, View: string(view), Lim: int32(limit + 1), Off: int32(offset)})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	counts, err := s.q.CountMyTickets(ctx, db.CountMyTicketsParams{IsAdmin: u.IsAdmin, UserID: u.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	projects, err := s.q.CountMyTicketsByProject(ctx, db.CountMyTicketsByProjectParams{IsAdmin: u.IsAdmin, UserID: u.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := MyTicketsPage{
		Items:    make([]MyTicket, 0, len(rows)),
		Counts:   MyTicketsCounts{All: int(counts.AllOpen), Overdue: int(counts.Overdue), Week: int(counts.Week), Incomplete: int(counts.Incomplete)},
		Projects: make([]ProjectCount, len(projects)),
	}
	if len(rows) > limit {
		rows = rows[:limit]
		out.NextCursor = ptr(strconv.Itoa(offset + limit))
	}
	for _, t := range rows {
		it := MyTicket{
			Key: t.Key, Title: t.Title, Type: TicketType(t.Type), Priority: Priority(t.Priority), Status: toAPIStatus(t.Status),
			MissingReason: t.MissingReason, MissingMenus: t.MissingMenus,
		}
		if t.Menu != "" {
			it.Menu = &t.Menu
		}
		if t.DueDate != nil {
			it.DueDate = &openapi_types.Date{Time: *t.DueDate}
		}
		if t.ClientID != nil {
			it.Client = &Ref{Id: *t.ClientID, Name: deref(t.ClientName)}
		}
		out.Items = append(out.Items, it)
	}
	for i, p := range projects {
		out.Projects[i] = ProjectCount{Key: p.Key, Open: int(p.Open)}
	}
	writeJSON(w, http.StatusOK, out)
}

// ListMyUpdates serves Home's Recently updated (FSD §6.4): the tickets the
// caller may see that changed last, each with its latest change. A comment
// change names its author but carries no text.
func (s *Server) ListMyUpdates(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	tickets, err := s.q.ListRecentTickets(ctx, db.ListRecentTicketsParams{IsAdmin: u.IsAdmin, UserID: u.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ids := make([]int64, len(tickets))
	for i, t := range tickets {
		ids[i] = t.ID
	}
	changes, err := s.q.LatestTicketChanges(ctx, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	latest := map[int64]ActivityItem{}
	for _, c := range changes {
		item := ActivityItem{Kind: ActivityItemKindComment, At: c.At}
		if c.Action != "comment" {
			var fields map[string]any
			if err := json.Unmarshal(c.Changes, &fields); err != nil {
				s.fail(w, r, err)
				return
			}
			item.Kind, item.Action, item.Changes = ActivityItemKindEvent, ptr(c.Action), &fields
		}
		if c.ActorID != 0 {
			item.Actor = &Ref{Id: c.ActorID, Name: deref(c.ActorName)}
		}
		latest[c.TicketID] = item
	}
	out := RecentTicketList{Items: make([]RecentTicket, len(tickets))}
	for i, t := range tickets {
		out.Items[i] = RecentTicket{Key: t.Key, Title: t.Title, Type: TicketType(t.Type), Status: toAPIStatus(t.Status), UpdatedAt: t.UpdatedAt}
		if c, ok := latest[t.ID]; ok {
			out.Items[i].Change = &c
		}
	}
	writeJSON(w, http.StatusOK, out)
}
```

- [ ] **Step 6: Run the tests**

Run: `cd server && gofmt -l . && go vet ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...`
Expected: no output from gofmt and vet, and `ok` for every package. `TestAFailedCloseChangesNothing` still passes: its planted trigger runs before the insert, and the new ones after it.

- [ ] **Step 7: Commit**

```bash
git add api/openapi.yaml server/migrations/00006_recent_changes.sql server/internal web/lib/api-types.ts
git commit -m "feat(server): Home API with my tickets and recent changes"
```

### Task 8: The permission suite covers decisions, node pages, search and Home

**Files:**
- Modify: `server/internal/httpapi/permission_test.go`

**Interfaces:**
- Consumes: every endpoint of Tasks 3–7; `seedClose` (Task 5).
- Produces:
  - `seedWorld` links each ticket to a menu:
    - HRIS-1, HRIS-3 and HRIS-4 to Leave Request;
    - HRIS-2 to Overtime Approval;
    - PAY-1 to Run Payroll.
  - It closes HRIS-2 and HRIS-3 as Done, with decisions hana confirmed.
  - `world.done` holds HRIS's Done status.

- [ ] **Step 1: Seed tickets on menus, and two closes**

In `server/internal/httpapi/permission_test.go`:

1. Update the header comment's ticket line to:

```go
//	Tickets: HRIS-1 core, HRIS-2 Client A (closed), HRIS-3 Client B (closed,
//	with a file), HRIS-4 Client C, PAY-1 Client C. HRIS-2 is on Overtime
//	Approval, PAY-1 on Run Payroll, the others on Leave Request.
```

2. Add `done int64 // HRIS's "Done" status` to `world`, below `inProgress`.
3. Replace the five `e.seedTicket(…)` lines of `seedWorld` with:

```go
	e.seedTicket(hris, u["hana"], "Core fix", nil, w.nodes["Leave Request"])
	closedA := e.seedTicket(hris, u["hana"], "Client A request", &a, w.nodes["Overtime Approval"])
	closedB := e.seedTicket(hris, u["hana"], "Client B request", &b, w.nodes["Leave Request"])
	e.seedTicket(hris, u["hana"], "Client C request", &c, w.nodes["Leave Request"])
	e.seedTicket(pay, u["dodi"], "PAY Client C request", &c, w.nodes["Run Payroll"])
```

4. Replace `w.inProgress = statuses[1].ID` with:

```go
	w.inProgress, w.done = statuses[1].ID, statuses[3].ID
	e.seedClose(closedA, w.done, u["hana"], "Client A approves overtime in HR")
	e.seedClose(closedB, w.done, u["hana"], "Client B exports leave balances")
```

- [ ] **Step 2: Add the reads**

In `TestPermissionSuiteReads`, below the `statuses` variable, add:

```go
	nodePath := func(name, rest string) string { return fmt.Sprintf("/nodes/%d%s", w.nodes[name].ID, rest) }
```

Add these rows to the list table, after the `/projects/HRIS/statuses` row:

```go
		{nodePath("Attendance", "/timeline"), map[string][]string{
			"admin": hrisTickets, "hana": hrisTickets, "ani": hrisTickets,
			"budi": {"HRIS-1", "HRIS-3"}, "citra": {"HRIS-1", "HRIS-2", "HRIS-4"},
		}, map[string]int{"dodi": 404}},
		{nodePath("Overtime Approval", "/timeline"), map[string][]string{
			"admin": {"HRIS-2"}, "hana": {"HRIS-2"}, "ani": {"HRIS-2"}, "citra": {"HRIS-2"},
		}, map[string]int{"budi": 404, "dodi": 404}},
		{nodePath("Run Payroll", "/timeline"), map[string][]string{"admin": {"PAY-1"}, "citra": {"PAY-1"}, "dodi": {"PAY-1"}},
			map[string]int{"hana": 404, "ani": 404, "budi": 404}},
		{nodePath("HR", "/behaviors"), map[string][]string{
			"admin": {"HRIS-2", "HRIS-3"}, "hana": {"HRIS-2", "HRIS-3"}, "ani": {"HRIS-2", "HRIS-3"},
			"budi": {"HRIS-3"}, "citra": {"HRIS-2"},
		}, map[string]int{"dodi": 404}},
		{"/me/updates", map[string][]string{
			"admin": {"HRIS-1", "HRIS-2", "HRIS-3", "HRIS-4", "PAY-1"}, "hana": hrisTickets, "ani": hrisTickets,
			"budi": {"HRIS-1", "HRIS-3"}, "citra": {"HRIS-1", "HRIS-2", "HRIS-4", "PAY-1"}, "dodi": {"PAY-1"},
		}, nil},
```

Add this entry to the single-things map:

```go
		nodePath("Overtime Approval", ""): {"admin", "hana", "ani", "citra"},
```

At the end of the function, add:

```go
	// R-AC-7 in search: results hold only what each user may open, across projects.
	for _, c := range []struct {
		q     string
		nodes bool
		see   map[string][]string
	}{
		{"request", false, map[string][]string{
			"admin": {"HRIS-2", "HRIS-3", "HRIS-4", "PAY-1"}, "hana": {"HRIS-2", "HRIS-3", "HRIS-4"}, "ani": {"HRIS-2", "HRIS-3", "HRIS-4"},
			"budi": {"HRIS-3"}, "citra": {"HRIS-2", "HRIS-4", "PAY-1"}, "dodi": {"PAY-1"},
		}},
		{"overtime", true, map[string][]string{
			"admin": {"Overtime Approval"}, "hana": {"Overtime Approval"}, "ani": {"Overtime Approval"}, "citra": {"Overtime Approval"},
		}},
		{"payroll", true, map[string][]string{
			"admin": {"Payroll", "Run Payroll"}, "citra": {"Payroll", "Run Payroll"}, "dodi": {"Payroll", "Run Payroll"},
		}},
	} {
		for _, user := range suiteUsers {
			var res httpapi.SearchResults
			code := e.call(w.as[user], http.MethodGet, "/search?q="+c.q, nil, &res)
			got := []string{}
			if c.nodes {
				for _, n := range res.Nodes {
					got = append(got, n.Name)
				}
			} else {
				for _, tk := range res.Tickets {
					got = append(got, tk.Key)
				}
			}
			slices.Sort(got)
			if want := c.see[user]; code != http.StatusOK || !slices.Equal(got, want) {
				t.Errorf("search %q as %s: %d %v, want %v", c.q, user, code, got, want)
			}
		}
	}

	// R-AC-7 on Home: My tickets hold only open tickets the assignee may see.
	// HRIS-4 is Client C's, which budi cannot see; HRIS-3 is closed.
	for key, user := range map[string]string{"HRIS-1": "budi", "HRIS-3": "budi", "HRIS-4": "budi", "PAY-1": "citra"} {
		if _, err := e.d.Pool.Exec(context.Background(), "UPDATE tickets SET assignee_id = $1 WHERE key = $2", w.users[user].ID, key); err != nil {
			t.Fatal(err)
		}
	}
	for user, want := range map[string][]string{"budi": {"HRIS-1"}, "citra": {"PAY-1"}, "hana": {}} {
		if got, code := listed(e, w.as[user], "/me/tickets"); code != http.StatusOK || !slices.Equal(got, want) {
			t.Errorf("my tickets as %s: %d %v, want %v", user, code, got, want)
		}
	}
```

- [ ] **Step 3: Add the writes**

In `TestPermissionSuiteWrites`, below `ifMatch`, add:

```go
	decision := map[string]any{"what_changed": "Client A approves overtime in HR.", "why": "Supervisors are often on leave."}
	closeCore := map[string]any{"status_id": w.done, "reason": "Every client asked for the core fix.",
		"decision": map[string]any{"what_changed": "The core fix ships to every client.", "why": "Every client asked for the core fix."}}
```

Add these rows before the `// Allowed, as controls` comment:

```go
		{"budi", http.MethodPut, "/tickets/HRIS-2/decision", nil, decision, 404},
		{"citra", http.MethodPut, "/tickets/HRIS-2/decision", nil, decision, 403},
		{"ani", http.MethodPut, "/tickets/HRIS-2/decision", nil, decision, 403}, // neither project admin nor confirmer
		{"citra", http.MethodPost, "/tickets/HRIS-4/transition", nil, closeCore, 403},
```

and these at the end of the controls:

```go
		{"hana", http.MethodPut, "/tickets/HRIS-2/decision", nil, decision, 200},
		{"budi", http.MethodPost, "/tickets/HRIS-1/transition", nil, closeCore, 200},
```

- [ ] **Step 4: Run the suite, then check that it has teeth**

Run: `cd server && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./internal/httpapi/ -run PermissionSuite -v`
Expected: PASS.

Then plant three leaks, one at a time, and run the suite after each:
- In `timeline.sql`, change the scope line of `ListNodeTimeline` to `AND (true OR …)`.
- In `search.sql`, change `SearchTickets`'s `(sqlc.arg('is_admin')::boolean OR EXISTS (` to `(true OR sqlc.arg('is_admin')::boolean OR EXISTS (`. Keep the argument, or the params struct loses `IsAdmin` and the build fails.
- In `home.sql`, change `ListRecentTickets`'s `(sqlc.arg('is_admin')::boolean OR EXISTS (` the same way.

Run `go generate ./...` after each change. Expected: FAIL, naming budi and citra for the timeline, and every user but admin for search and for `/me/updates`. Revert all three, run `go generate ./...` again, and check that the suite passes and `git diff --stat` shows only `permission_test.go`.

- [ ] **Step 5: Commit**

```bash
git add server/internal/httpapi/permission_test.go
git commit -m "test(server): permission suite covers decisions, node pages, search and Home"
```

### Task 9: Shared web pieces: weak text, node paths and the node picker

**Files:**
- Create: `web/lib/weak.ts`, `web/lib/nodes.ts`, `web/components/NodePicker.tsx`
- Modify: `web/components/TicketForm.tsx` (the picker, the weak-reason hint, a menu to start with)
- Modify: `web/app/p/[key]/tickets/new/page.tsx` (`?node_id=`)
- Modify: `web/components/Icon.tsx` (`check`)
- Modify: `web/lib/problem.ts` (the Iteration 3 API types)
- Modify: `web/messages/en.json`, `web/messages/id.json`

**Interfaces:**
- Consumes: the API types regenerated by Tasks 3–7.
- Produces:
  - `isWeak(text: string): boolean` in `@/lib/weak`.
  - `nodePaths(nodes: Node[]): (id: number) => string` in `@/lib/nodes`: "HR › Attendance › Overtime Approval", or "" for a node not in the list.
  - `<NodePicker nodes selected onToggle legend />` in `@/components/NodePicker`, with `selected: Set<number>` and `onToggle(id: number, on: boolean)`.
  - `<TicketForm nodeId? />`: a new ticket starts with that menu ticked.
  - `Icon` name `check`.
  - Types in `@/lib/problem`: `DecisionRecord`, `TimelineEntry`, `Behavior`, `NodeDetail`, `SearchResults`, `MyTicket`, `RecentTicket`.
  - Message `ticketForm.weakReason`.

The web has no unit test runner, and this task adds none: the end-to-end test of Task 15 checks the weak hint (AC-DC-4) and the picker. This task's check is the type-checking build.

- [ ] **Step 1: Write the weak-text rule**

`web/lib/weak.ts`:

```ts
// R-DC-8: a reason or why is weak when, after trimming, it has fewer than 20
// characters, or fewer than 20 once stock phrases such as "sesuai permintaan"
// are taken out. The hint never blocks a save. Empty text is missing, not weak.
const stock = ["per request", "as requested", "client request", "sesuai permintaan", "permintaan klien", "permintaan client", "request user", "ok", "done"];
const phrase = new RegExp(`(?<![\\p{L}\\p{N}])(?:${stock.join("|")})(?![\\p{L}\\p{N}])`, "giu");

export function isWeak(text: string): boolean {
  const trimmed = text.trim();
  if (trimmed === "") return false;
  return trimmed.replace(phrase, "").replace(/\s+/g, " ").trim().length < 20;
}
```

For example: "per request" and "sesuai permintaan klien Arunika" are weak; "Client A supervisors are often on leave; HR approves overtime directly." is not.

- [ ] **Step 2: Share node paths and the picker**

`web/lib/nodes.ts`:

```ts
import type { Node } from "./problem";

/** Each node's path from the top of the tree, "HR › Attendance › Overtime Approval"; "" for a node not in the list. */
export function nodePaths(nodes: Node[]): (id: number) => string {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  return (id) => {
    const names: string[] = [];
    for (let n = byId.get(id); n; n = n.parent_id === null ? undefined : byId.get(n.parent_id)) names.unshift(n.name);
    return names.join(" › ");
  };
}
```

`web/components/NodePicker.tsx`:

```tsx
"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import Icon from "./Icon";
import { nodePaths } from "@/lib/nodes";
import type { Node } from "@/lib/problem";
import { cx } from "@/lib/ui";

type Props = {
  nodes: Node[];
  selected: Set<number>;
  onToggle: (id: number, on: boolean) => void;
  legend: string; // read by screen readers; the visible label sits beside the picker
};

// The menu picker of the ticket form and the close dialog (FSD §8.3): a filter
// by path, alias or code, then a checkbox per menu or module, named by its path.
export default function NodePicker({ nodes, selected, onToggle, legend }: Props) {
  const t = useTranslations("ticketForm");
  const [filter, setFilter] = useState("");
  const pathOf = useMemo(() => nodePaths(nodes), [nodes]);
  const q = filter.trim().toLowerCase();
  const choices = nodes.filter((n) => !q || [pathOf(n.id), n.code ?? "", ...n.aliases].some((s) => s.toLowerCase().includes(q)));
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="sr-only">{legend}</legend>
      <label className="flex h-[34px] items-center gap-2 rounded border border-field bg-white px-2.5 text-muted focus-within:outline-2 focus-within:outline-accent">
        <Icon name="search" />
        <input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          aria-label={t("menusFilter")}
          placeholder={t("menusFilter")}
          className="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-muted"
        />
      </label>
      <div className="flex max-h-48 flex-col overflow-y-auto rounded border border-line-soft text-[13px]">
        {choices.map((n) => (
          <label key={n.id} className={cx("flex items-center gap-2 border-b border-line-soft px-2.5 py-1.5 last:border-0", selected.has(n.id) && "bg-accent-soft")}>
            <input type="checkbox" checked={selected.has(n.id)} onChange={(e) => onToggle(n.id, e.target.checked)} className="size-4 accent-accent" />
            {pathOf(n.id)}
            {n.code && <span className="ml-auto font-mono text-[11px] text-muted">{n.code}</span>}
          </label>
        ))}
      </div>
    </fieldset>
  );
}
```

- [ ] **Step 3: Use them in the ticket form**

In `web/components/TicketForm.tsx`:

1. Replace `import { useEffect, useMemo, useState } from "react";` with `import { useEffect, useState } from "react";`, and below the `Icon` import add:

```tsx
import NodePicker from "@/components/NodePicker";
```

   Below the `@/lib/ui` import add:

```tsx
import { isWeak } from "@/lib/weak";
```

2. In `Props`, below `statusId?: number; …`, add:

```tsx
  nodeId?: number; // the menu a new ticket starts with (the node page's "New ticket for this menu")
```

   and add `nodeId` to the destructured props of `TicketForm`.
3. Replace

```tsx
  const [nodeIds, setNodeIds] = useState<Set<number>>(() => new Set(ticket?.nodes.map((n) => n.id)));
  const [menuFilter, setMenuFilter] = useState("");
```

   with

```tsx
  const [nodeIds, setNodeIds] = useState<Set<number>>(() => new Set(ticket ? ticket.nodes.map((n) => n.id) : nodeId ? [nodeId] : []));
  const [reason, setReason] = useState(ticket?.reason ?? "");
```

4. Delete the `byId`, `pathOf`, `q` and `menuChoices` lines, from `const byId = useMemo(…)` through `const menuChoices = …;`. Keep the `warnings` line.
5. In `submit`, in the `if (another)` branch, below `setNodeIds(new Set());`, add `setReason("");`.
6. Replace the whole `<fieldset className="flex flex-col gap-2">…</fieldset>` inside the menus `Row` with:

```tsx
          <NodePicker nodes={nodes} selected={nodeIds} onToggle={toggleNode} legend={t("menus")} />
          <p className={field.hint}>{t("menusHint")}</p>
          {warnings.map((n) => (
            <p key={n.id} className="flex items-center gap-1.5 text-xs text-warn">
              <Icon name="warning" className="size-3.5" />
              {t("menuForOtherClients", { menu: n.name, clients: n.clients.map((c) => c.name).join(", ") })}
            </p>
          ))}
```

7. Replace the reason `Row`'s textarea and hint with:

```tsx
          <textarea
            id="tf-reason"
            name="reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            maxLength={2000}
            rows={3}
            aria-describedby="reason-hint"
            className={field.textarea}
          />
          <p id="reason-hint" className={isWeak(reason) ? "text-xs text-warn" : field.hint}>
            {isWeak(reason) ? t("weakReason") : t("reasonHint")}
          </p>
```

In `web/app/p/[key]/tickets/new/page.tsx`, change the `searchParams` type to `Promise<{ status_id?: string; node_id?: string }>`, the destructuring to `const { status_id, node_id } = await searchParams;`, and add this prop to `TicketForm`, below `statusId`:

```tsx
              nodeId={node_id ? Number(node_id) : undefined}
```

- [ ] **Step 4: Add the icon, the types and the strings**

In `web/components/Icon.tsx`, below the `warning` entry, add:

```tsx
  check: (
    <>
      <circle cx="8" cy="8" r="5.5" />
      <path d="m5.5 8 1.8 1.8 3.2-3.6" />
    </>
  ),
```

Append to `web/lib/problem.ts`:

```ts
export type DecisionRecord = components["schemas"]["DecisionRecord"];
export type TimelineEntry = components["schemas"]["TimelineEntry"];
export type Behavior = components["schemas"]["Behavior"];
export type NodeDetail = components["schemas"]["NodeDetail"];
export type SearchResults = components["schemas"]["SearchResults"];
export type MyTicket = components["schemas"]["MyTicket"];
export type RecentTicket = components["schemas"]["RecentTicket"];
```

In `web/messages/en.json`, in `ticketForm`, below `reasonHint`, add:

```json
    "weakReason": "Say why the client needs this, e.g. their policy or the problem it solves",
```

In `web/messages/id.json`, at the same place:

```json
    "weakReason": "Jelaskan mengapa klien membutuhkannya, misalnya kebijakan mereka atau masalah yang diselesaikan",
```

- [ ] **Step 5: Build**

Run: `cd web && npm run build`
Expected: the build succeeds with no type errors. `TicketForm` no longer declares `pathOf`, and nothing else imported it.

- [ ] **Step 6: Commit**

```bash
git add web/lib web/components web/app/p web/messages
git commit -m "feat(web): weak-reason hint, shared node picker and paths"
```

### Task 10: The close dialog and the decision card

**Files:**
- Create: `web/components/CloseDialog.tsx`, `web/app/t/[ticketKey]/DecisionCard.tsx`, `web/lib/activity.ts`
- Modify: `web/app/t/[ticketKey]/TicketView.tsx`, `web/app/t/[ticketKey]/page.tsx`, `web/app/t/[ticketKey]/Activity.tsx`
- Modify: `web/messages/en.json`, `web/messages/id.json`

**Interfaces:**
- Consumes:
  - `POST /tickets/{key}/transition` with `If-Match` and `{status_id, reason?, node_ids?, decision}` (Task 3);
  - `PUT /tickets/{key}/decision` (Task 4);
  - `Ticket.decision`, `Ticket.closed_at` (Task 3);
  - `NodePicker`, `nodePaths`, `isWeak`, `Icon` `check` (Task 9).
- Produces:
  - `<CloseDialog ticket status nodes onDone onCancel />` in `@/components/CloseDialog`: `ticket: Ticket` as read just now, `status: Status` (Done or Cancelled), `nodes: Node[]` (the project's menus), `onDone()` after a close, `onCancel()` when nothing changed.
  - `describeChange(t, item, meId?)`, `shown(value)` and `type Change` in `@/lib/activity`. `t` is a translator of the `activity` messages.
  - Messages: `close.*`, `decision.*`, `activity.commented`, `activity.you`, `activity.decision*`, and error codes `close_validation_failed`, `decision_not_confirmed`, `status_category_in_use` and `min_items`.

- [ ] **Step 1: Write the close dialog**

`web/components/CloseDialog.tsx`:

```tsx
"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { StatusDot } from "./Chips";
import Icon from "./Icon";
import NodePicker from "./NodePicker";
import { api } from "@/lib/api";
import { nodePaths } from "@/lib/nodes";
import { useProblemText, type Node, type Problem, type Status, type Ticket } from "@/lib/problem";
import { button, chip, cx, field } from "@/lib/ui";
import { isWeak } from "@/lib/weak";

type Props = {
  ticket: Ticket; // as read just now: its version guards the close
  status: Status; // the Done or Cancelled status it moves to
  nodes: Node[]; // the project's menus, for a ticket without any
  onDone: () => void; // closed; the caller refreshes
  onCancel: () => void; // nothing changed
};

// The words for a length error, by the field the server names.
const lengths: Record<string, string> = {
  reason: "reasonLength",
  "decision.what_changed": "whatChangedLength",
  "decision.why": "whyLength",
  "decision.alternatives": "alternativesLength",
};

// The close dialog (FSD §9.1): moving a ticket into Done or Cancelled asks what
// changed, why and what was rejected, prefilled so it takes seconds (Goal 2),
// plus a reason and menus when the ticket has none. Nothing changes until
// "Close ticket"; Cancel, Escape and × leave the ticket as it was.
export default function CloseDialog({ ticket, status, nodes, onDone, onCancel }: Props) {
  const t = useTranslations("close");
  const tt = useTranslations("ticket");
  const problemText = useProblemText();
  const ref = useRef<HTMLDialogElement>(null);
  const done = status.category === "done";
  const prior = ticket.decision; // a reopened ticket's draft, or the record of an earlier close (R-DC-6)
  const needsReason = ticket.reason.trim() === "";
  const needsMenus = ticket.nodes.length === 0;
  const pathOf = useMemo(() => nodePaths(nodes), [nodes]);
  const [reason, setReason] = useState("");
  const [menus, setMenus] = useState<Set<number>>(() => new Set());
  const [whatChanged, setWhatChanged] = useState(prior?.what_changed ?? (done ? ticket.title : t("notImplemented")));
  const [why, setWhy] = useState(prior?.why ?? ticket.reason);
  const [alternatives, setAlternatives] = useState(prior?.alternatives ?? "");
  const [problem, setProblem] = useState<Problem>();
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!ref.current?.open) ref.current?.showModal();
  }, []);

  // AC-DC-2: "Close ticket" waits until every required field has text.
  const ready = (!needsReason || reason.trim() !== "") && (!needsMenus || menus.size > 0) && whatChanged.trim() !== "" && why.trim() !== "";
  const serverError = (name: string) => {
    const e = problem?.errors?.find((f) => f.field === name);
    if (!e) return undefined;
    if (name === "node_ids") return t("menusRequired");
    return e.code === "required" ? t("required") : t(lengths[name]);
  };
  const toggleMenu = (id: number, on: boolean) =>
    setMenus((s) => {
      const next = new Set(s);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });

  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    const { error } = await api.POST("/tickets/{key}/transition", {
      params: { path: { key: ticket.key }, header: { "If-Match": `"${ticket.version}"` } },
      body: {
        status_id: status.id,
        reason: needsReason ? reason : undefined,
        node_ids: needsMenus ? [...menus] : undefined,
        decision: { what_changed: whatChanged, why, alternatives },
      },
    });
    setBusy(false);
    if (error) return setProblem(error);
    onDone();
  }

  const context = [ticket.title, ticket.client?.name ?? tt("core"), ...ticket.nodes.map((n) => pathOf(n.id) || n.name)];
  return (
    <dialog
      ref={ref}
      aria-labelledby="close-title"
      onCancel={(e) => {
        e.preventDefault();
        onCancel();
      }}
      className="m-auto w-[min(620px,calc(100vw-2rem))] rounded-md bg-white p-0 text-ink shadow-2xl backdrop:bg-ink/55"
    >
      <form onSubmit={submit} aria-label={t("form")} className="flex max-h-[calc(100vh-4rem)] flex-col">
        <div className="flex flex-col gap-1 border-b border-line px-[22px] pb-3.5 pt-[18px]">
          <div className="flex items-center gap-2.5">
            <StatusDot color={status.color} className="size-2.5" />
            <h2 id="close-title" className="text-lg font-semibold">{t("title", { key: ticket.key, status: status.name })}</h2>
            <button
              type="button"
              onClick={onCancel}
              aria-label={t("dismiss")}
              className="ml-auto inline-flex size-8 cursor-pointer items-center justify-center rounded text-muted hover:bg-paper"
            >
              <Icon name="x" />
            </button>
          </div>
          <p className="text-[13px] text-muted">{context.join(" · ")}</p>
        </div>
        <div className="flex flex-col gap-3.5 overflow-y-auto px-[22px] py-4">
          {needsReason && (
            <Area id="close-reason" label={t("reason")} value={reason} onChange={setReason} max={2000} rows={2} weak error={serverError("reason")} />
          )}
          {needsMenus && (
            <div className="flex flex-col gap-1.5">
              <span className="text-[13px] font-semibold">{t("menus")}</span>
              <NodePicker nodes={nodes} selected={menus} onToggle={toggleMenu} legend={t("menus")} />
              <p className={cx("text-xs", menus.size === 0 || serverError("node_ids") ? "text-danger" : "text-muted")}>
                {serverError("node_ids") ?? (menus.size === 0 ? t("menusRequired") : t("menusChosen", { count: menus.size }))}
              </p>
            </div>
          )}
          <Area
            id="close-what"
            label={done ? t("whatChanged") : t("whatDecided")}
            value={whatChanged}
            onChange={setWhatChanged}
            max={1000}
            rows={2}
            hint={prior ? t("fromRecord") : done ? t("fromTitle") : undefined}
            error={serverError("decision.what_changed")}
          />
          <Area
            id="close-why"
            label={t("why")}
            value={why}
            onChange={setWhy}
            max={2000}
            rows={3}
            weak
            hint={prior ? t("fromRecord") : ticket.reason ? t("fromReason") : undefined}
            error={serverError("decision.why")}
          />
          <Area
            id="close-alternatives"
            label={t("alternatives")}
            value={alternatives}
            onChange={setAlternatives}
            max={2000}
            rows={2}
            optional
            error={serverError("decision.alternatives")}
          />
          {problem && !problem.errors?.length && <p role="alert" className={field.error}>{problemText(problem)}</p>}
        </div>
        <div className="flex items-center gap-2 rounded-b-md border-t border-line bg-paper px-[22px] py-3">
          <span className="text-xs text-muted">{t("outcome")}</span>
          <span className={cx(chip, done ? "bg-ok-soft text-ok" : "bg-well text-[#4A423C]")}>{done ? t("implemented") : t("rejected")}</span>
          <button type="button" onClick={onCancel} className={cx(button.secondary, "ml-auto")}>{t("cancel")}</button>
          <button disabled={!ready || busy} className={button.primary}>{t("submit")}</button>
        </div>
      </form>
    </dialog>
  );
}

// One text field of the dialog: its label and counter, then a line saying what
// is wrong, the weak-reason hint (R-DC-8) or where the prefill came from.
function Area({ id, label, value, onChange, max, rows, hint, weak, optional, error }: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  max: number;
  rows: number;
  hint?: string;
  weak?: boolean;
  optional?: boolean;
  error?: string;
}) {
  const t = useTranslations("close");
  const tf = useTranslations("ticketForm");
  const missing = !optional && value.trim() === "";
  const soft = weak === true && isWeak(value);
  const note = error ?? (missing ? t("required") : soft ? tf("weakReason") : hint);
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline gap-2">
        <label htmlFor={id} className="text-[13px] font-semibold">{label}</label>
        <span className="ml-auto font-mono text-[11px] text-muted">{optional ? t("optional") : t("counter", { count: value.length, max })}</span>
      </div>
      <textarea
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={rows}
        maxLength={max}
        aria-describedby={note ? `${id}-note` : undefined}
        className={field.textarea}
      />
      {note && (
        <p id={`${id}-note`} className={cx("text-xs", error || missing ? "text-danger" : soft ? "text-warn" : "text-muted")}>{note}</p>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Write the decision card**

`web/app/t/[ticketKey]/DecisionCard.tsx`:

```tsx
"use client";

import { Fragment, useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { utc } from "@/lib/format";
import { useProblemText, type DecisionRecord } from "@/lib/problem";
import { button, chip, cx, field, panel } from "@/lib/ui";

// The decision record on the ticket page (FSD §8.6, §9): what changed, why and
// what was rejected. Project admins and the confirmer reword a confirmed record
// (R-DC-5); a reopened ticket's record is a draft until the next close (R-DC-6).
export default function DecisionCard({ ticketKey, decision, canEdit }: { ticketKey: string; decision: DecisionRecord; canEdit: boolean }) {
  const t = useTranslations("decision");
  const locale = useLocale();
  const router = useRouter();
  const problemText = useProblemText();
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState("");
  const confirmed = decision.state === "confirmed";
  const implemented = decision.outcome === "implemented";
  const whatLabel = implemented ? t("whatChanged") : t("whatDecided");

  async function save(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const { error } = await api.PUT("/tickets/{key}/decision", {
      params: { path: { key: ticketKey } },
      body: { what_changed: String(form.get("what_changed")), why: String(form.get("why")), alternatives: String(form.get("alternatives")) },
    });
    if (error) return setError(problemText(error));
    setEditing(false);
    setError("");
    router.refresh();
  }

  const rows: [string, string][] = [
    [whatLabel, decision.what_changed],
    [t("why"), decision.why],
    [t("alternatives"), decision.alternatives],
  ];
  return (
    <section aria-labelledby="decision-title" className={cx(panel, "overflow-hidden")}>
      <div className={cx("flex flex-wrap items-center gap-2.5 border-b px-4 py-2.5", confirmed ? "border-[#D3E4D8] bg-[#EEF5F0]" : "border-line-soft bg-paper")}>
        <Icon name={confirmed ? "check" : "edit"} className={confirmed ? "text-ok" : "text-muted"} />
        <h2 id="decision-title" className="text-sm font-semibold">{t("title")}</h2>
        <span className={cx(chip, implemented ? "bg-ok-soft text-ok" : "bg-well text-[#4A423C]")}>{implemented ? t("implemented") : t("rejected")}</span>
        <span className="text-xs text-muted">
          {confirmed && decision.confirmed_by && decision.confirmed_at
            ? t("confirmedBy", { name: decision.confirmed_by.name, at: utc(decision.confirmed_at, locale) })
            : t("draft")}
        </span>
        {confirmed && canEdit && !editing && (
          <button type="button" onClick={() => setEditing(true)} className={cx(button.secondary, "ml-auto h-7")}>
            {t("edit")}
          </button>
        )}
      </div>
      {editing ? (
        <form onSubmit={save} aria-label={t("editTitle")} className="flex flex-col gap-3 p-4">
          <label className={field.label}>
            {whatLabel}
            <textarea name="what_changed" defaultValue={decision.what_changed} required minLength={10} maxLength={1000} rows={2} className={field.textarea} />
          </label>
          <label className={field.label}>
            {t("why")}
            <textarea name="why" defaultValue={decision.why} required minLength={10} maxLength={2000} rows={3} className={field.textarea} />
          </label>
          <label className={field.label}>
            {t("alternatives")}
            <textarea name="alternatives" defaultValue={decision.alternatives} maxLength={2000} rows={2} className={field.textarea} />
          </label>
          {error && <p role="alert" className={field.error}>{error}</p>}
          <div className="flex gap-2">
            <button className={button.primary}>{t("save")}</button>
            <button type="button" onClick={() => setEditing(false)} className={button.secondary}>{t("cancel")}</button>
          </div>
        </form>
      ) : (
        <dl className="grid gap-x-4 gap-y-3 px-4 py-3.5 text-sm leading-normal md:grid-cols-[190px_minmax(0,1fr)]">
          {rows.map(([label, value]) => (
            <Fragment key={label}>
              <dt className="text-muted">{label}</dt>
              <dd className="whitespace-pre-wrap">{value || "—"}</dd>
            </Fragment>
          ))}
        </dl>
      )}
    </section>
  );
}
```

- [ ] **Step 3: Share the activity sentences**

`web/lib/activity.ts`:

```ts
import type { ActivityItem } from "./problem";

export type Change = { old?: unknown; new?: unknown };

// The two calls describeChange needs from a next-intl translator of the "activity" messages.
type Translate = { (key: string, values?: Record<string, string>): string; has(key: string): boolean };

/** A value as the history shows it: "—" for nothing, lists joined. */
export const shown = (v: unknown) =>
  v === null || v === undefined || v === "" ? "—" : Array.isArray(v) ? v.join(", ") || "—" : String(v);

/** One history event or comment as a sentence, e.g. "Rina changed Status from To do to In progress". With meId, the reader's own changes read "You". */
export function describeChange(t: Translate, it: ActivityItem, meId?: number): string {
  const actor = it.actor ? (it.actor.id === meId ? t("you") : it.actor.name) : t("system");
  if (it.kind === "comment") return t("commented", { actor });
  const c = (it.changes ?? {}) as Record<string, unknown>;
  const field = (k: string) => (t.has(`fields.${k}`) ? t(`fields.${k}`) : k);
  switch (it.action) {
    case "create":
      return t("created", { actor });
    case "transition": {
      const s = (c.status ?? {}) as Change;
      return t("transition", { actor, old: shown(s.old), new: shown(s.new) });
    }
    case "update":
      return t("updated", { actor, fields: Object.keys(c).map(field).join(", ") });
    case "comment_edit":
      return t("commentEdited", { actor });
    case "comment_delete":
      return t("commentDeleted", { actor });
    case "attachment_add":
      return t("attachmentAdded", { actor, file: shown(c.filename) });
    case "attachment_delete":
      return t("attachmentDeleted", { actor, file: shown(c.filename) });
    case "decision_confirm":
      return t("decisionConfirmed", { actor });
    case "decision_draft":
      return t("decisionDrafted", { actor });
    case "decision_edit":
      return t("decisionEdited", { actor });
    default:
      return `${actor}: ${it.action}`;
  }
}
```

In `web/app/t/[ticketKey]/Activity.tsx`:

1. Below the `@/lib/api` import, add `import { describeChange, shown, type Change } from "@/lib/activity";`.
2. Delete `type Change = { old?: unknown; new?: unknown };`, the `shown` constant and the whole `describe` function.
3. In `details`, replace `if (it.action === "update") {` with:

```tsx
    // Updates, closes and decision records list their old and new values.
    if (it.action === "update" || it.action?.startsWith("decision_") || (it.action === "transition" && Object.keys(c).length > 1)) {
```

4. Replace `eventIcon` with:

```tsx
  const eventIcon = (it: ActivityItem) =>
    it.action === "transition" ? "chevronRight"
    : it.action === "create" ? "plus"
    : it.action === "decision_confirm" ? "check"
    : it.action?.startsWith("attachment") ? "file"
    : "edit";
```

5. Replace `{describe(it)}` with `{describeChange(t, it)}`.

- [ ] **Step 4: Put them on the ticket page**

In `web/app/t/[ticketKey]/TicketView.tsx`:

1. Below the `ClientChip` import line, add `import CloseDialog from "@/components/CloseDialog";`; below the `@/lib/format` import, add `import { nodePaths } from "@/lib/nodes";`; below the last import, add `import DecisionCard from "./DecisionCard";`.
2. In `Props`, below `canEdit: boolean;`, add `canEditDecision: boolean; // project admin or the record's confirmer (R-DC-5)`, and add `canEditDecision` to the destructured props.
3. Replace the `byId` and `pathOf` declarations with:

```tsx
  const [closingTo, setClosingTo] = useState<Status | null>(null);
  const pathOf = useMemo(() => nodePaths(nodes), [nodes]);
```

4. Replace the whole `transition` function with:

```tsx
  // Open moves go straight through; Done and Cancelled open the close dialog (FSD §9.1).
  async function transition(statusId: number) {
    const target = statuses.find((s) => s.id === statusId);
    if (!target || target.id === ticket.status.id) return;
    if (closing(target)) return setClosingTo(target);
    const { error } = await api.POST("/tickets/{key}/transition", {
      params: { path: { key: ticket.key } },
      body: { status_id: statusId },
    });
    if (error) return setError(problemText(error));
    setError("");
    router.refresh();
  }
```

5. In the status `<select>`, replace `<option key={s.id} value={s.id} disabled={closing(s)}>{s.name}</option>` with `<option key={s.id} value={s.id}>{s.name}</option>`.
6. Below the closing `</section>` of the details panel and above `{activity}`, add:

```tsx
              {ticket.decision && <DecisionCard ticketKey={ticket.key} decision={ticket.decision} canEdit={canEditDecision} />}
```

7. In the side panel's `<dl>`, below the `updated` pair, add:

```tsx
                  {ticket.closed_at && (
                    <>
                      <dt className="text-muted">{t("closed")}</dt>
                      <dd>{utc(ticket.closed_at, locale)}</dd>
                    </>
                  )}
```

8. Just before `</main>`, add:

```tsx
        {closingTo && (
          <CloseDialog
            ticket={ticket}
            status={closingTo}
            nodes={nodes}
            onDone={() => {
              setClosingTo(null);
              router.refresh();
            }}
            onCancel={() => setClosingTo(null)}
          />
        )}
```

In `web/app/t/[ticketKey]/page.tsx`, add this prop to `TicketView`, below `canEdit={canEdit}`:

```tsx
      canEditDecision={project.role === "admin" || ticket.decision?.confirmed_by?.id === me.id}
```

- [ ] **Step 5: Add the strings**

In `web/messages/en.json`, add these two namespaces after `ticket`:

```json
  "close": {
    "title": "Close {key} as {status}",
    "form": "Decision record",
    "dismiss": "Close dialog",
    "reason": "Reason",
    "menus": "Affected menus",
    "menusRequired": "Choose at least one menu",
    "menusChosen": "{count, plural, one {# menu chosen} other {# menus chosen}}",
    "whatChanged": "What changed",
    "whatDecided": "What was decided",
    "why": "Why",
    "alternatives": "Alternatives rejected",
    "optional": "(optional)",
    "counter": "{count, number} / {max, number}",
    "required": "Required",
    "fromTitle": "Filled from the ticket title; change it if needed.",
    "fromReason": "Filled from the ticket's reason.",
    "fromRecord": "Filled from this ticket's earlier decision record.",
    "notImplemented": "Not implemented",
    "outcome": "Outcome",
    "implemented": "Implemented",
    "rejected": "Rejected",
    "cancel": "Cancel",
    "submit": "Close ticket",
    "reasonLength": "Use 10 to 2,000 characters",
    "whatChangedLength": "Use 10 to 1,000 characters",
    "whyLength": "Use 10 to 2,000 characters",
    "alternativesLength": "Use at most 2,000 characters"
  },
  "decision": {
    "title": "Decision record",
    "implemented": "Implemented",
    "rejected": "Rejected",
    "confirmedBy": "Confirmed by {name} · {at}",
    "draft": "Draft, confirm on close",
    "edit": "Edit",
    "editTitle": "Edit the decision record",
    "whatChanged": "What changed",
    "whatDecided": "What was decided",
    "why": "Why",
    "alternatives": "Alternatives rejected",
    "save": "Save",
    "cancel": "Cancel"
  },
```

In `activity`, below `attachmentDeleted`, add:

```json
    "commented": "{actor} added a comment",
    "decisionConfirmed": "{actor} confirmed the decision record",
    "decisionDrafted": "{actor} turned the decision record back into a draft",
    "decisionEdited": "{actor} edited the decision record",
    "you": "You",
```

and in `activity.fields`, below `status`, add:

```json
    "what_changed": "What changed",
    "why": "Why",
    "alternatives": "Alternatives rejected",
    "outcome": "Outcome",
    "state": "State",
    "confirmed_by": "Confirmed by",
```

In `errors`, replace the `close_unavailable` line with:

```json
    "close_validation_failed": "Fill in what closing this ticket needs",
    "decision_not_confirmed": "Close the ticket to confirm its decision record first",
    "status_category_in_use": "Tickets use this status; move them before it opens or closes",
    "min_items": "Choose at least one",
```

In `web/messages/id.json`, at the same places:

```json
  "close": {
    "title": "Tutup {key} sebagai {status}",
    "form": "Catatan keputusan",
    "dismiss": "Tutup dialog",
    "reason": "Alasan",
    "menus": "Menu terdampak",
    "menusRequired": "Pilih minimal satu menu",
    "menusChosen": "{count} menu dipilih",
    "whatChanged": "Apa yang berubah",
    "whatDecided": "Apa yang diputuskan",
    "why": "Mengapa",
    "alternatives": "Alternatif yang ditolak",
    "optional": "(opsional)",
    "counter": "{count, number} / {max, number}",
    "required": "Wajib diisi",
    "fromTitle": "Terisi dari judul tiket; ubah bila perlu.",
    "fromReason": "Terisi dari alasan tiket.",
    "fromRecord": "Terisi dari catatan keputusan tiket ini sebelumnya.",
    "notImplemented": "Tidak diimplementasikan",
    "outcome": "Hasil",
    "implemented": "Diimplementasikan",
    "rejected": "Ditolak",
    "cancel": "Batal",
    "submit": "Tutup tiket",
    "reasonLength": "Tulis 10 sampai 2.000 karakter",
    "whatChangedLength": "Tulis 10 sampai 1.000 karakter",
    "whyLength": "Tulis 10 sampai 2.000 karakter",
    "alternativesLength": "Tulis paling banyak 2.000 karakter"
  },
  "decision": {
    "title": "Catatan keputusan",
    "implemented": "Diimplementasikan",
    "rejected": "Ditolak",
    "confirmedBy": "Dikonfirmasi oleh {name} · {at}",
    "draft": "Draf, dikonfirmasi saat ditutup",
    "edit": "Ubah",
    "editTitle": "Ubah catatan keputusan",
    "whatChanged": "Apa yang berubah",
    "whatDecided": "Apa yang diputuskan",
    "why": "Mengapa",
    "alternatives": "Alternatif yang ditolak",
    "save": "Simpan",
    "cancel": "Batal"
  },
```

```json
    "commented": "{actor} menambahkan komentar",
    "decisionConfirmed": "{actor} mengonfirmasi catatan keputusan",
    "decisionDrafted": "{actor} mengembalikan catatan keputusan menjadi draf",
    "decisionEdited": "{actor} mengubah catatan keputusan",
    "you": "Anda",
```

```json
    "what_changed": "Apa yang berubah",
    "why": "Mengapa",
    "alternatives": "Alternatif yang ditolak",
    "outcome": "Hasil",
    "state": "Keadaan",
    "confirmed_by": "Dikonfirmasi oleh",
```

```json
    "close_validation_failed": "Lengkapi isian untuk menutup tiket ini",
    "decision_not_confirmed": "Tutup tiket untuk mengonfirmasi catatan keputusannya dulu",
    "status_category_in_use": "Tiket memakai status ini; pindahkan dulu sebelum statusnya dibuka atau ditutup",
    "min_items": "Pilih minimal satu",
```

Keep the JSON valid: every entry you add in the middle of an object ends with a comma, and the last entry of an object has none.

- [ ] **Step 6: Build and look**

Run: `cd web && npm run build`
Expected: the build succeeds.

Then run `make up` and, on a ticket with a reason and a menu, choose Done in the status menu. Expected:
- the dialog opens with What changed holding the title and Why the reason;
- emptying What changed disables "Close ticket" and shows "Wajib diisi";
- Batal closes it and the status stays;
- "Close ticket" closes the ticket, and the page shows the decision card with "Dikonfirmasi oleh …" and the Closed date;
- moving it back to In progress turns the card into "Draf, dikonfirmasi saat ditutup";
- Activity reads "… mengonfirmasi catatan keputusan" and "… mengembalikan catatan keputusan menjadi draf".

- [ ] **Step 7: Commit**

```bash
git add web/components/CloseDialog.tsx web/lib/activity.ts web/app/t web/messages
git commit -m "feat(web): close dialog and decision record card"
```

### Task 11: Closing from the board, and its recent closes

**Files:**
- Modify: `web/app/p/[key]/board/page.tsx` (menus, `closed_days`, the query for links)
- Modify: `web/app/p/[key]/board/Board.tsx` (drops and status choices into Done and Cancelled open the close dialog; "Show all")
- Modify: `web/messages/en.json`, `web/messages/id.json`

**Interfaces:**
- Consumes: `GET /projects/{key}/tickets?closed_days=` (Task 4), `GET /tickets/{key}`, `<CloseDialog />` (Task 10).
- Produces: `Board` props `{ projectKey, statuses, tickets, nodes, canEdit, today, query }`, with `nodes: Node[]` and `query: Record<string, string>` (the page's filters, for the "Show all" link).

- [ ] **Step 1: Load what the board needs**

In `web/app/p/[key]/board/page.tsx`, replace the `Promise.all` block with:

```tsx
  const showAll = values.closed === "all";
  const [clients, statuses, nodes, page] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/statuses", path),
    api.GET("/projects/{key}/nodes", path), // the close dialog's menu picker
    // ponytail: one page of up to 1,000 cards; per-column "Show more" comes with larger boards.
    // Done and Cancelled hold the last 14 days unless "Show all" (FSD §8.4).
    api.GET("/projects/{key}/tickets", {
      params: { path: { key }, query: { ...ticketQuery(values), sort: "priority", limit: 1000, closed_days: showAll ? undefined : 14 } },
    }),
  ]);
```

and replace the `<Board … />` element with:

```tsx
        <Board
          projectKey={key}
          statuses={statuses.data?.items ?? []}
          tickets={page.data?.items ?? []}
          nodes={nodes.data?.items ?? []}
          canEdit={project.role !== "viewer"}
          today={new Date().toISOString().slice(0, 10)}
          query={values}
        />
```

- [ ] **Step 2: Close from the board**

In `web/app/p/[key]/board/Board.tsx`:

1. Add `import CloseDialog from "@/components/CloseDialog";` below the `Chips` import, and replace the `@/lib/problem` import with:

```tsx
import { useProblemText, type Node, type Status, type Ticket, type TicketSummary } from "@/lib/problem";
```

2. Replace the `Props` type, the `closing` helper and the comment above `Board` with:

```tsx
type Props = {
  projectKey: string;
  statuses: Status[];
  tickets: TicketSummary[];
  nodes: Node[];
  canEdit: boolean;
  today: string;
  query: Record<string, string>; // the page's filters, kept by the "Show all" link
};

const closes = (s: Status) => s.category === "done" || s.category === "cancelled";

// Cards move by native drag-and-drop or by their status menu, which keyboards
// reach too. An open move shows at once and rolls back when the API refuses it;
// a move into Done or Cancelled opens the close dialog, and the card stays
// where it was until the close is confirmed (FSD §8.4, AC-TK-2).
```

3. Add `nodes` and `query` to the destructured props, and below `const [error, setError] = useState("");` add:

```tsx
  const [closing, setClosing] = useState<{ ticket: Ticket; status: Status } | null>(null);
  const showAll = query.closed === "all";
  const { closed: _closed, ...recent } = query;
```

4. Replace the first lines of `move`, from `const card = …` through `if (!card || card.status_id === statusId) return;`, with:

```tsx
    const card = items.find((x) => x.id === ticketId);
    const target = statuses.find((s) => s.id === statusId);
    if (!card || !target || card.status_id === statusId) return;
    if (closes(target)) {
      const { data, error } = await api.GET("/tickets/{key}", { params: { path: { key: card.key } } });
      if (error) return setError(problemText(error));
      return setClosing({ ticket: data, status: target });
    }
```

5. Replace `const droppable = canEdit && !closing(s);` with `const droppable = canEdit;`.
6. Replace `{closing(s) && <p className="px-1 text-xs text-muted">{t("closedLater")}</p>}` with:

```tsx
              {closes(s) && (
                <p className="flex items-center gap-2 px-1 text-xs text-muted">
                  {showAll ? t("allClosed") : t("recentClosed")}
                  <Link href={`?${new URLSearchParams(showAll ? recent : { ...query, closed: "all" })}`} className="ml-auto">
                    {showAll ? t("showRecent") : t("showAll")}
                  </Link>
                </p>
              )}
```

7. In the card's status `<select>`, replace `<option key={o.id} value={o.id} disabled={closing(o)}>{o.name}</option>` with `<option key={o.id} value={o.id}>{o.name}</option>`.
8. Just before the last `</div>` of the component, add:

```tsx
      {closing && (
        <CloseDialog
          ticket={closing.ticket}
          status={closing.status}
          nodes={nodes}
          onDone={() => {
            setClosing(null);
            router.refresh();
          }}
          onCancel={() => setClosing(null)}
        />
      )}
```

- [ ] **Step 3: Add the strings**

In `web/messages/en.json`, in `board`, replace the `closedLater` line with:

```json
    "recentClosed": "Closed in the last 14 days",
    "allClosed": "Every closed ticket",
    "showAll": "Show all",
    "showRecent": "Last 14 days",
```

In `web/messages/id.json`, in `board`, replace the `closedLater` line with:

```json
    "recentClosed": "Ditutup 14 hari terakhir",
    "allClosed": "Semua tiket yang ditutup",
    "showAll": "Tampilkan semua",
    "showRecent": "14 hari terakhir",
```

- [ ] **Step 4: Build and look**

Run: `cd web && npm run build`
Expected: the build succeeds.

Then run `make up` and open a project's board. Expected:
- dragging a card onto Done opens the close dialog;
- Batal leaves the card in its column;
- confirming moves it into Done;
- choosing Cancelled in a card's status menu opens the dialog with "Apa yang diputuskan" and "Tidak diimplementasikan";
- the Done column says "Ditutup 14 hari terakhir", and "Tampilkan semua" adds `closed=all` to the URL while keeping the filters.

- [ ] **Step 5: Commit**

```bash
git add web/app/p web/messages
git commit -m "feat(web): close tickets from the board; closed columns show 14 days"
```

### Task 12: The node page

**Files:**
- Create: `web/app/p/[key]/modules/[nodeId]/page.tsx`, `Timeline.tsx`, `Behaviors.tsx`, `NodeDetails.tsx` (all in that folder)
- Modify: `web/app/p/[key]/modules/ModuleTree.tsx` (an "Open page" link)
- Modify: `web/app/t/[ticketKey]/TicketView.tsx` (menu chips open the node page)
- Modify: `web/messages/en.json`, `web/messages/id.json`

**Interfaces:**
- Consumes: `GET /nodes/{id}`, `GET /nodes/{id}/timeline`, `GET /nodes/{id}/behaviors` (Task 5); `NodeForm` and `ReadOnlyNode` from `app/p/[key]/modules/NodeForm.tsx` (Iteration 1); `TicketForm` `nodeId` through `/p/{key}/tickets/new?node_id=` (Task 9).
- Produces:
  - Route `/p/{key}/modules/{nodeId}` with `?tab=timeline|behaviors|details` and the timeline filters `client` (`core` or a client id), `type`, `from`, `to`, `sub` (`0` leaves sub-nodes out) and `limit`.
  - The timeline's sections are regions named "Sedang berjalan · N" and "Ditutup · N, terbaru dulu", each entry an `article`. Task 15 reads them.
  - Messages `nodePage.*` and `modules.openPage`.

- [ ] **Step 1: Write the page**

`web/app/p/[key]/modules/[nodeId]/page.tsx`:

```tsx
import { Fragment } from "react";
import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { ClientChip } from "@/components/Chips";
import Icon from "@/components/Icon";
import type { TicketType } from "@/lib/problem";
import { getProject, serverApi } from "@/lib/server-api";
import { one } from "@/lib/ticket-query";
import { button, chip, cx } from "@/lib/ui";
import Behaviors from "./Behaviors";
import NodeDetails from "./NodeDetails";
import Timeline from "./Timeline";

const tabs = ["timeline", "behaviors", "details"] as const;
type Tab = (typeof tabs)[number];

// The node page (FSD §7.4): a menu's or module's history, the behaviors each
// client has now, and its details. The tab, the filters and the page size live
// in the URL, so every view is shareable. Archived nodes keep their page (R-MR-4).
export default async function NodePage({
  params,
  searchParams,
}: {
  params: Promise<{ key: string; nodeId: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { key, nodeId } = await params;
  const values = one(await searchParams);
  const id = Number(nodeId);
  const project = await getProject(key);
  if (!project || !Number.isInteger(id)) notFound();
  const api = await serverApi();
  const { data: detail } = await api.GET("/nodes/{id}", { params: { path: { id } } });
  if (!detail || detail.project_key !== project.key) notFound();
  const t = await getTranslations("nodePage");
  const tm = await getTranslations("modules");
  const tab: Tab = tabs.includes(values.tab as Tab) ? (values.tab as Tab) : "timeline";
  const node = detail.node;
  const subNodes = values.sub === "0" ? false : undefined;
  const clientsOf = async () => (await api.GET("/projects/{key}/clients", { params: { path: { key } } })).data?.items ?? [];

  let content: React.ReactNode;
  if (tab === "timeline") {
    const limit = Math.min(Math.max(Number(values.limit) || 50, 1), 1000);
    const [clients, timeline] = await Promise.all([
      clientsOf(),
      api.GET("/nodes/{id}/timeline", {
        params: {
          path: { id },
          query: {
            sub_nodes: subNodes,
            client_id: values.client && values.client !== "core" ? Number(values.client) : undefined,
            core: values.client === "core" ? true : undefined,
            type: values.type as TicketType | undefined,
            from: values.from,
            to: values.to,
            limit,
          },
        },
      }),
    ]);
    content = (
      <Timeline
        items={timeline.data?.items ?? []}
        failed={Boolean(timeline.error)}
        more={Boolean(timeline.data?.next_cursor)}
        limit={limit}
        clients={clients}
        values={values}
      />
    );
  } else if (tab === "behaviors") {
    const { data } = await api.GET("/nodes/{id}/behaviors", { params: { path: { id }, query: { sub_nodes: subNodes } } });
    content = <Behaviors items={data?.items ?? []} />;
  } else {
    const canEdit = project.role === "admin";
    content = <NodeDetails projectKey={key} node={node} clients={canEdit ? await clientsOf() : []} canEdit={canEdit} />;
  }

  return (
    <>
      <div className="flex flex-col gap-2.5 border-b border-line bg-white px-4 pt-3.5 md:px-5">
        <nav aria-label={t("path")} className="flex flex-wrap items-center gap-1.5 text-[13px] text-muted">
          <Link href={`/p/${key}/modules`}>{t("modules")}</Link>
          {detail.path.map((p) => (
            <Fragment key={p.id}>
              <Icon name="chevronRight" className="size-3.5" />
              <Link href={`/p/${key}/modules/${p.id}`}>{p.name}</Link>
            </Fragment>
          ))}
          <Icon name="chevronRight" className="size-3.5" />
          <span className="text-ink">{node.name}</span>
        </nav>
        <div className="flex flex-wrap items-start gap-4">
          <div className="flex min-w-0 flex-col gap-2">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="text-2xl font-semibold leading-tight">{node.name}</h1>
              <span className="text-[13px] text-muted">{tm(node.type)}</span>
              {node.type === "menu" &&
                (node.client_specific ? (
                  node.clients.map((c) => <ClientChip key={c.id} client={c} coreLabel="" />)
                ) : (
                  <span className={cx(chip, "bg-well text-[#4A423C]")}>{tm("shared")}</span>
                ))}
              {node.code && <span className="font-mono text-xs text-muted">{node.code}</span>}
              {node.archived && <span className={cx(chip, "bg-well text-muted")}>{tm("archivedBadge")}</span>}
            </div>
            {(node.description || node.aliases.length > 0) && (
              <p className="max-w-[760px] text-sm leading-relaxed text-[#3D3632]">
                {node.description}{" "}
                {node.aliases.length > 0 && <span className="text-muted">{t("aliases", { aliases: node.aliases.join(", ") })}</span>}
              </p>
            )}
          </div>
          {project.role !== "viewer" && !node.archived && (
            <Link href={`/p/${key}/tickets/new?node_id=${node.id}`} className={cx(button.primary, "ml-auto")}>
              <Icon name="plus" />
              {t("newTicket")}
            </Link>
          )}
        </div>
        <nav aria-label={t("tabs")} className="flex gap-1">
          {tabs.map((to) => (
            <Link
              key={to}
              href={to === "timeline" ? "?" : `?tab=${to}`}
              aria-current={tab === to ? "page" : undefined}
              className={cx(
                "border-b-2 px-3 py-2 text-[13px] no-underline",
                tab === to ? "border-accent font-semibold text-ink hover:text-ink" : "border-transparent font-medium text-muted hover:text-ink",
              )}
            >
              {t(to)}
            </Link>
          ))}
        </nav>
      </div>
      <main className="flex flex-col gap-4 px-4 py-4 md:px-5">{content}</main>
    </>
  );
}
```

- [ ] **Step 2: Write the timeline**

`web/app/p/[key]/modules/[nodeId]/Timeline.tsx`:

```tsx
import Link from "next/link";
import { getLocale, getTranslations } from "next-intl/server";
import { ClientChip, StatusDot } from "@/components/Chips";
import { day } from "@/lib/format";
import type { Client, TimelineEntry } from "@/lib/problem";
import { button, chip, cx, field, sectionTitle } from "@/lib/ui";

const types = ["bug", "change_request", "feature"] as const;

// The rail beside an entry: a ring while the ticket is open, a dot once closed.
function Rail({ color, open }: { color: string; open?: boolean }) {
  return (
    <div className="flex flex-col items-center">
      <span
        className={open ? "mt-3.5 size-2 rounded-full border-2 bg-ground" : "mt-[15px] size-3 rounded-full"}
        style={open ? { borderColor: color } : { background: color }}
      />
      <span className="w-0.5 grow bg-line" />
    </div>
  );
}

// The Timeline tab (FSD §7.4, story 1): open tickets pinned under "In progress",
// then closed ones newest first by close date, each with what changed and why.
// The filters are a plain GET form, so the URL holds them.
export default async function Timeline({ items, failed, more, limit, clients, values }: {
  items: TimelineEntry[];
  failed: boolean;
  more: boolean;
  limit: number;
  clients: Client[];
  values: Record<string, string>;
}) {
  const t = await getTranslations("nodePage");
  const tt = await getTranslations("ticketTypes");
  const locale = await getLocale();
  const open = items.filter((it) => !it.closed_at);
  const closed = items.filter((it) => it.closed_at);
  const requester = (it: TimelineEntry) => `${it.requester.name}${it.requester.title ? ` (${it.requester.title})` : ""}`;
  const grid = "grid grid-cols-[88px_22px_minmax(0,1fr)] gap-x-3 md:grid-cols-[140px_22px_minmax(0,1fr)] md:gap-x-3.5";
  const label = "flex flex-col gap-1 text-xs text-muted";

  return (
    <>
      <form aria-label={t("filters")} className="flex flex-wrap items-end gap-2">
        <label className={label}>
          {t("client")}
          <select name="client" defaultValue={values.client ?? ""} className={field.compact}>
            <option value="">{t("allClients")}</option>
            <option value="core">{t("core")}</option>
            {clients.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
        </label>
        <label className={label}>
          {t("type")}
          <select name="type" defaultValue={values.type ?? ""} className={field.compact}>
            <option value="">{t("allTypes")}</option>
            {types.map((ty) => (
              <option key={ty} value={ty}>{tt(ty)}</option>
            ))}
          </select>
        </label>
        <label className={label}>
          {t("from")}
          <input type="date" name="from" defaultValue={values.from} className={field.compact} />
        </label>
        <label className={label}>
          {t("to")}
          <input type="date" name="to" defaultValue={values.to} className={field.compact} />
        </label>
        {/* Checked sends sub=1 before the hidden sub=0, and the page reads the first value; unchecked sends sub=0. */}
        <label className="flex h-8 items-center gap-1.5 text-[13px] text-ink">
          <input type="checkbox" name="sub" value="1" defaultChecked={values.sub !== "0"} className="size-4 accent-accent" />
          {t("subNodes")}
        </label>
        <input type="hidden" name="sub" value="0" />
        <button className={button.secondary}>{t("apply")}</button>
      </form>
      {failed ? (
        <p role="alert" className={field.error}>{t("filterError")}</p>
      ) : items.length === 0 ? (
        <p className="text-muted">{t("empty")}</p>
      ) : (
        <>
          {open.length > 0 && (
            <section aria-labelledby="open-title" className="flex flex-col gap-2">
              <h2 id="open-title" className={sectionTitle}>{t("open", { count: open.length })}</h2>
              <ol className="flex flex-col">
                {open.map((it) => (
                  <li key={it.key} className={grid}>
                    <div className="pt-3 text-right text-xs text-muted">{t("createdOn", { date: day(it.created_at, locale) })}</div>
                    <Rail color={it.status.color} open />
                    <article
                      aria-label={`${it.key} ${it.title}`}
                      className="mb-2 flex flex-wrap items-center gap-x-2.5 gap-y-1 rounded border border-line bg-white px-3.5 py-2.5 text-[13px]"
                    >
                      <Link href={`/t/${it.key}`} className="font-mono font-semibold">{it.key}</Link>
                      <Link href={`/t/${it.key}`} className="font-semibold text-ink no-underline hover:text-ink hover:underline">{it.title}</Link>
                      <ClientChip client={it.client} coreLabel={t("core")} />
                      <span className="text-muted">{requester(it)}</span>
                      <span className="ml-auto flex items-center gap-1.5">
                        <StatusDot color={it.status.color} />
                        {it.status.name}
                      </span>
                    </article>
                  </li>
                ))}
              </ol>
            </section>
          )}
          {closed.length > 0 && (
            <section aria-labelledby="closed-title" className="flex flex-col gap-2">
              <h2 id="closed-title" className={sectionTitle}>{t("closed", { count: closed.length })}</h2>
              <ol className="flex flex-col">
                {closed.map((it) => {
                  const implemented = it.decision?.outcome === "implemented";
                  return (
                    <li key={it.key} className={grid}>
                      <div className="flex flex-col items-end gap-0.5 pt-3">
                        <span className="text-sm font-semibold">{day(it.closed_at!, locale)}</span>
                        <span className="text-xs text-muted">{it.status.name}</span>
                      </div>
                      <Rail color={it.status.color} />
                      <article aria-label={`${it.key} ${it.title}`} className="mb-3 flex flex-col gap-2 rounded border border-line bg-white px-3.5 py-3">
                        <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
                          <Link href={`/t/${it.key}`} className="font-mono text-[13px] font-semibold">{it.key}</Link>
                          {it.decision && (
                            <span className={cx(chip, implemented ? "bg-ok-soft text-ok" : "bg-well text-[#4A423C]")}>
                              {implemented ? t("implemented") : t("rejected")}
                            </span>
                          )}
                          <ClientChip client={it.client} coreLabel={t("core")} />
                          <span>{tt(it.type)} · {t("requestedBy", { name: requester(it) })}</span>
                        </div>
                        <Link href={`/t/${it.key}`} className="text-[15px] font-semibold text-ink no-underline hover:text-ink hover:underline">
                          {it.title}
                        </Link>
                        {it.decision && (
                          <>
                            <div className="grid gap-x-5 gap-y-2 text-[13px] leading-normal md:grid-cols-2">
                              <div>
                                <h3 className="text-[11px] font-semibold uppercase tracking-[0.04em] text-muted">{implemented ? t("whatChanged") : t("whatDecided")}</h3>
                                <p className="mt-0.5 whitespace-pre-wrap">{it.decision.what_changed}</p>
                              </div>
                              <div>
                                <h3 className="text-[11px] font-semibold uppercase tracking-[0.04em] text-muted">{t("why")}</h3>
                                <p className="mt-0.5 whitespace-pre-wrap">{it.decision.why}</p>
                              </div>
                            </div>
                            {it.decision.alternatives && (
                              <details className="text-xs">
                                <summary className="cursor-pointer text-link">{t("alternatives")}</summary>
                                <p className="mt-1.5 whitespace-pre-wrap text-[13px]">{it.decision.alternatives}</p>
                              </details>
                            )}
                          </>
                        )}
                      </article>
                    </li>
                  );
                })}
              </ol>
            </section>
          )}
          {more && (
            <Link href={`?${new URLSearchParams({ ...values, limit: String(limit + 50) })}`} className={cx(button.secondary, "self-start")}>
              {t("loadMore")}
            </Link>
          )}
        </>
      )}
    </>
  );
}
```

- [ ] **Step 3: Write the Behaviors and Details tabs**

`web/app/p/[key]/modules/[nodeId]/Behaviors.tsx`:

```tsx
import Link from "next/link";
import { getLocale, getTranslations } from "next-intl/server";
import { day } from "@/lib/format";
import type { Behavior } from "@/lib/problem";
import { cx, panel, sectionTitle } from "@/lib/ui";

// The Behaviors tab (FSD §7.4, story 5): the decisions in force, "All clients"
// first, then each client in the user's scope, so QA checks what a client
// should get before filing a bug. The API returns them in that order.
export default async function Behaviors({ items }: { items: Behavior[] }) {
  const t = await getTranslations("nodePage");
  const locale = await getLocale();
  if (items.length === 0) return <p className="text-muted">{t("noBehaviors")}</p>;
  const groups: { name: string; items: Behavior[] }[] = [];
  for (const b of items) {
    const name = b.client?.name ?? t("allClients");
    const last = groups.at(-1);
    if (last?.name === name) last.items.push(b);
    else groups.push({ name, items: [b] });
  }
  return (
    <div className="flex max-w-4xl flex-col gap-5">
      {groups.map((g) => (
        <section key={g.name} aria-label={g.name} className="flex flex-col gap-2">
          <h2 className={sectionTitle}>{g.name}</h2>
          <ul className={cx(panel, "divide-y divide-line-soft")}>
            {g.items.map((b) => (
              <li key={b.key} className="flex flex-col gap-1.5 px-4 py-3">
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
                  <Link href={`/t/${b.key}`} className="font-mono text-[13px] font-semibold">{b.key}</Link>
                  <span className="text-[13px] font-semibold text-ink">{b.title}</span>
                  {b.closed_at && <span className="ml-auto">{day(b.closed_at, locale)}</span>}
                </div>
                <p className="whitespace-pre-wrap text-sm">{b.what_changed}</p>
                <p className="whitespace-pre-wrap text-[13px] text-muted">{t("because", { why: b.why })}</p>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}
```

`web/app/p/[key]/modules/[nodeId]/NodeDetails.tsx`:

```tsx
"use client";

import { useRouter } from "next/navigation";
import type { Client, Node } from "@/lib/problem";
import { cx, panel } from "@/lib/ui";
import NodeForm, { ReadOnlyNode } from "../NodeForm";

// The Details tab (FSD §7.4): project admins edit the node here; everyone else
// reads it. Archived nodes are read-only until restored in the tree (R-MR-4).
export default function NodeDetails({ projectKey, node, clients, canEdit }: { projectKey: string; node: Node; clients: Client[]; canEdit: boolean }) {
  const router = useRouter();
  return (
    <div className={cx(panel, "max-w-3xl p-4")}>
      {canEdit && !node.archived ? (
        <NodeForm projectKey={projectKey} node={node} clients={clients} onSaved={() => router.refresh()} />
      ) : (
        <ReadOnlyNode node={node} />
      )}
    </div>
  );
}
```

- [ ] **Step 4: Link to node pages**

In `web/app/p/[key]/modules/ModuleTree.tsx`, in `details`, replace

```tsx
    const path = <p className="text-xs text-muted">{pathOf(n)}</p>;
```

with

```tsx
    const path = (
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-xs text-muted">{pathOf(n)}</p>
        <Link href={`/p/${projectKey}/modules/${n.id}`} className={cx(button.quiet, "ml-auto")}>
          {t("openPage")}
          <Icon name="arrowRight" className="size-3.5" />
        </Link>
      </div>
    );
```

In `web/app/t/[ticketKey]/TicketView.tsx`, replace the menu chip `<li>` with a link to the node page (FSD §8.6):

```tsx
                        <li key={n.id}>
                          <Link
                            href={`/p/${ticket.project_key}/modules/${n.id}`}
                            className="inline-flex h-7 items-center gap-1.5 rounded border border-line bg-ground px-2.5 text-[13px] text-ink no-underline hover:border-field hover:text-ink"
                          >
                            <Icon name="screen" className="size-3.5 text-muted" />
                            {pathOf(n.id) || n.name}
                            {n.archived ? ` (${t("archived")})` : ""}
                          </Link>
                        </li>
```

- [ ] **Step 5: Add the strings**

In `web/messages/en.json`, in `modules`, below `archivedBadge`, add `"openPage": "Open page"` (with a comma after `archivedBadge`'s value), and add this namespace after `modules`:

```json
  "nodePage": {
    "path": "Path",
    "modules": "Modules",
    "tabs": "Node page",
    "timeline": "Timeline",
    "behaviors": "Behaviors by client",
    "details": "Details",
    "aliases": "Also called: {aliases}.",
    "newTicket": "New ticket for this menu",
    "filters": "Filter the timeline",
    "client": "Client",
    "allClients": "All clients",
    "core": "Core",
    "type": "Type",
    "allTypes": "All types",
    "from": "From",
    "to": "To",
    "subNodes": "Include sub-items",
    "apply": "Apply",
    "open": "In progress · {count}",
    "closed": "Closed · {count}, newest first",
    "createdOn": "Created {date}",
    "implemented": "Implemented",
    "rejected": "Rejected",
    "requestedBy": "requested by {name}",
    "whatChanged": "What changed",
    "whatDecided": "What was decided",
    "why": "Why",
    "alternatives": "Alternatives rejected",
    "empty": "No tickets linked yet. Link this menu from a ticket form.",
    "loadMore": "Load more",
    "filterError": "These filters could not be applied.",
    "noBehaviors": "No decisions in force yet. Closing tickets on this menu adds them.",
    "because": "Why: {why}"
  },
```

In `web/messages/id.json`: `"openPage": "Buka halaman"` in `modules`, and:

```json
  "nodePage": {
    "path": "Jejak",
    "modules": "Modul",
    "tabs": "Halaman menu",
    "timeline": "Linimasa",
    "behaviors": "Perilaku per klien",
    "details": "Rincian",
    "aliases": "Juga disebut: {aliases}.",
    "newTicket": "Tiket baru untuk menu ini",
    "filters": "Saring linimasa",
    "client": "Klien",
    "allClients": "Semua klien",
    "core": "Inti",
    "type": "Jenis",
    "allTypes": "Semua jenis",
    "from": "Dari",
    "to": "Sampai",
    "subNodes": "Sertakan sub-item",
    "apply": "Terapkan",
    "open": "Sedang berjalan · {count}",
    "closed": "Ditutup · {count}, terbaru dulu",
    "createdOn": "Dibuat {date}",
    "implemented": "Diimplementasikan",
    "rejected": "Ditolak",
    "requestedBy": "diminta oleh {name}",
    "whatChanged": "Apa yang berubah",
    "whatDecided": "Apa yang diputuskan",
    "why": "Mengapa",
    "alternatives": "Alternatif yang ditolak",
    "empty": "Belum ada tiket yang terhubung. Hubungkan menu ini dari formulir tiket.",
    "loadMore": "Muat lebih banyak",
    "filterError": "Saringan ini tidak dapat diterapkan.",
    "noBehaviors": "Belum ada keputusan yang berlaku. Menutup tiket pada menu ini akan menambahkannya.",
    "because": "Mengapa: {why}"
  },
```

- [ ] **Step 6: Build and look**

Run: `cd web && npm run build`
Expected: the build succeeds, and the route list shows `ƒ /p/[key]/modules/[nodeId]`.

Then run `make up`, open a module in the tree and follow "Buka halaman". Expected:
- the breadcrumb, the name, the scope chip and the tabs show, and the top bar's Modul tab is active;
- Linimasa lists open tickets under "Sedang berjalan" and closed ones under "Ditutup", newest first, each with what changed and why;
- unticking "Sertakan sub-item" and pressing Terapkan adds `sub=0` to the URL and leaves only the node's own tickets;
- "Tiket baru untuk menu ini" opens the form with that menu ticked;
- a menu chip on a ticket page opens its node page.

- [ ] **Step 7: Commit**

```bash
git add web/app/p web/app/t web/messages
git commit -m "feat(web): node page with timeline, behaviors by client and details"
```

### Task 13: Search in the top bar

**Files:**
- Create: `web/app/search/page.tsx`
- Modify: `web/app/TopBar.tsx` (the search box), `web/app/Header.tsx` (its comment)
- Modify: `web/messages/en.json`, `web/messages/id.json`

**Interfaces:**
- Consumes: `GET /search?q=` (Task 6), whose tickets list an exact key first.
- Produces:
  - The top bar's search box: `input type="search" name="q"`, labelled "Cari tiket atau menu", submitting to `/search`.
  - Route `/search?q=&kind=nodes|tickets`: a query that is a visible ticket's key redirects to `/t/{key}`; results link to node pages and tickets.
  - Messages `search.*`, `nav.search`, `nav.searchPlaceholder`.

- [ ] **Step 1: Put the box in the top bar**

In `web/app/TopBar.tsx`:

1. Add `import Form from "next/form";` below the `next/link` import.
2. Replace the comment above `TopBar` with:

```tsx
// The dark top bar of the Terakota design: the project switcher and the
// project's tabs, search, New ticket, the language switch and the account menu.
// Off project pages, system admins get their admin pages as tabs instead.
```

3. As the first child of `<div className="ml-auto flex h-13 items-center gap-2">`, add:

```tsx
          <Form action="/search" role="search">
            <label className="flex h-8 w-40 items-center gap-2 rounded border border-bar-line bg-bar-raised px-2.5 text-bar-muted focus-within:border-bar-accent sm:w-56 lg:w-80">
              <Icon name="search" />
              <input
                type="search"
                name="q"
                required
                aria-label={t("search")}
                placeholder={t("searchPlaceholder")}
                className="min-w-0 flex-1 bg-transparent text-[13px] text-white outline-none placeholder:text-bar-muted"
              />
            </label>
          </Form>
```

In `web/app/Header.tsx`, replace the comment with:

```tsx
// The top bar on every signed-in page (FSD §6.1). Ask joins it in Iteration 5.
```

- [ ] **Step 2: Write the results page**

`web/app/search/page.tsx`:

```tsx
import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { ClientChip, StatusDot } from "@/components/Chips";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import { serverApi } from "@/lib/server-api";
import { one } from "@/lib/ticket-query";
import { chip, cx, panel, sectionTitle } from "@/lib/ui";

// Marks the first place q occurs in text, ignoring case.
function Highlight({ text, q }: { text: string; q: string }) {
  const at = text.toLowerCase().indexOf(q.toLowerCase());
  if (q === "" || at < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, at)}
      <mark className="rounded-sm bg-[#FBE3C8] px-px text-inherit">{text.slice(at, at + q.length)}</mark>
      {text.slice(at + q.length)}
    </>
  );
}

// Search (FSD §6.1–6.2): the tickets and menus the user may open, across their
// projects. A query that is a visible ticket's key opens that ticket.
export default async function SearchPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { q = "", kind } = one(await searchParams);
  const query = q.trim();
  const t = await getTranslations("search");
  const tm = await getTranslations("modules");
  const api = await serverApi();
  const res = query.length >= 2 ? (await api.GET("/search", { params: { query: { q: query } } })).data : undefined;
  const tickets = res?.tickets ?? [];
  const nodes = res?.nodes ?? [];
  if (tickets[0]?.key === query.toUpperCase()) redirect(`/t/${tickets[0].key}`);
  const showNodes = kind !== "tickets";
  const showTickets = kind !== "nodes";
  const kinds: [string | undefined, string, number][] = [
    [undefined, t("all"), nodes.length + tickets.length],
    ["nodes", t("nodes"), nodes.length],
    ["tickets", t("tickets"), tickets.length],
  ];

  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{query ? t("title", { q: query }) : t("heading")}</h1>
        <p className="text-[13px] text-muted">{t("hint")}</p>
      </PageBar>
      <main className="flex max-w-[1040px] flex-col gap-4 px-4 py-4 md:px-5">
        {query.length < 2 ? (
          <p className="text-muted">{t("short")}</p>
        ) : nodes.length + tickets.length === 0 ? (
          <p className="text-muted">{t("none", { q: query })}</p>
        ) : (
          <>
            <nav aria-label={t("kinds")} className="flex gap-1 border-b border-line">
              {kinds.map(([k, label, count]) => (
                <Link
                  key={label}
                  href={`/search?${new URLSearchParams(k ? { q: query, kind: k } : { q: query })}`}
                  aria-current={kind === k ? "page" : undefined}
                  className={cx(
                    "flex items-center gap-1.5 border-b-2 px-3 py-2 text-[13px] no-underline",
                    kind === k ? "border-accent font-semibold text-ink hover:text-ink" : "border-transparent text-muted hover:text-ink",
                  )}
                >
                  {label}
                  <span className="font-mono text-[11px]">{count}</span>
                </Link>
              ))}
            </nav>
            {showNodes && nodes.length > 0 && (
              <section aria-labelledby="nodes-title" className="flex flex-col gap-2">
                <h2 id="nodes-title" className={sectionTitle}>{t("nodes")}</h2>
                <ul className={cx(panel, "divide-y divide-line-soft")}>
                  {nodes.map((n) => (
                    <li key={n.id}>
                      <Link
                        href={`/p/${n.project_key}/modules/${n.id}`}
                        className="flex items-center gap-2.5 px-3.5 py-2.5 text-ink no-underline hover:bg-paper hover:text-ink"
                      >
                        <Icon name={n.type === "menu" ? "screen" : "folder"} className="text-muted" />
                        <span className="flex min-w-0 flex-col gap-0.5">
                          <span className="text-sm font-semibold"><Highlight text={n.path.join(" › ")} q={query} /></span>
                          {(n.aliases.length > 0 || n.code) && (
                            <span className="text-xs text-muted">
                              {n.aliases.length > 0 && <Highlight text={t("aliases", { aliases: n.aliases.join(", ") })} q={query} />}
                              {n.aliases.length > 0 && n.code ? " · " : ""}
                              {n.code && <span className="font-mono"><Highlight text={n.code} q={query} /></span>}
                            </span>
                          )}
                        </span>
                        <span className="ml-auto text-xs text-muted">{tm(n.type)}</span>
                        <span className={cx(chip, "bg-ground font-mono text-[#4A423C]")}>{n.project_key}</span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </section>
            )}
            {showTickets && tickets.length > 0 && (
              <section aria-labelledby="tickets-title" className="flex flex-col gap-2">
                <h2 id="tickets-title" className={sectionTitle}>{t("tickets")}</h2>
                <ul className={cx(panel, "divide-y divide-line-soft")}>
                  {tickets.map((it) => (
                    <li key={it.key}>
                      <Link
                        href={`/t/${it.key}`}
                        className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3.5 py-2.5 text-ink no-underline hover:bg-paper hover:text-ink"
                      >
                        <span className="w-24 font-mono text-[13px] font-semibold text-link">{it.key}</span>
                        <span className="min-w-0 flex-1 text-sm font-semibold"><Highlight text={it.title} q={query} /></span>
                        <span className="flex items-center gap-1.5 text-xs">
                          <StatusDot color={it.status.color} />
                          {it.status.name}
                        </span>
                        <ClientChip client={it.client} coreLabel={t("core")} />
                        <span className={cx(chip, "bg-ground font-mono text-[#4A423C]")}>{it.project_key}</span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </section>
            )}
          </>
        )}
      </main>
    </>
  );
}
```

- [ ] **Step 3: Add the strings**

In `web/messages/en.json`, in `nav`, below `newTicket`, add:

```json
    "search": "Search tickets or menus",
    "searchPlaceholder": "Search, or type HRIS-231",
```

and add this namespace after `nav`:

```json
  "search": {
    "heading": "Search",
    "title": "Results for “{q}”",
    "hint": "Type a key such as HRIS-231 to open that ticket.",
    "kinds": "Result types",
    "all": "All",
    "nodes": "Menus and modules",
    "tickets": "Tickets",
    "none": "Nothing matches “{q}”.",
    "short": "Type at least 2 characters.",
    "aliases": "Also called: {aliases}",
    "core": "Core"
  },
```

In `web/messages/id.json`:

```json
    "search": "Cari tiket atau menu",
    "searchPlaceholder": "Cari tiket, menu, atau HRIS-231",
```

```json
  "search": {
    "heading": "Pencarian",
    "title": "Hasil untuk “{q}”",
    "hint": "Ketik kunci seperti HRIS-231 untuk langsung membuka tiketnya.",
    "kinds": "Jenis hasil",
    "all": "Semua",
    "nodes": "Menu dan modul",
    "tickets": "Tiket",
    "none": "Tidak ada yang cocok dengan “{q}”.",
    "short": "Ketik minimal 2 karakter.",
    "aliases": "Alias: {aliases}",
    "core": "Inti"
  },
```

- [ ] **Step 4: Build and look**

Run: `cd web && npm run build`
Expected: the build succeeds, and the route list shows `ƒ /search`.

Then run `make up`. Expected:
- typing "overt" in the top bar's box and pressing Enter shows "Hasil untuk “overt”" with the menu and ticket results, the matched part marked;
- a menu result opens its node page;
- typing a visible ticket's key in lower case, e.g. `hris-1`, opens that ticket;
- a key the user cannot see shows "Tidak ada yang cocok …".

- [ ] **Step 5: Commit**

```bash
git add web/app/search web/app/TopBar.tsx web/app/Header.tsx web/messages
git commit -m "feat(web): search in the top bar, results page and key jump"
```

### Task 14: Home

**Files:**
- Modify: `web/app/page.tsx` (rewrite)
- Modify: `web/app/TopBar.tsx` (New ticket off project pages)
- Modify: `web/messages/en.json`, `web/messages/id.json`

**Interfaces:**
- Consumes: `GET /me/tickets?view=&cursor=` and `GET /me/updates` (Task 7); `describeChange` (Task 10); `getProjects`, `getMe` (Iteration 2).
- Produces:
  - `/` with `?mine=overdue|week|incomplete` and `?cursor=`: the regions "Tiket saya", "Baru diperbarui" and "Proyek saya". The page bar keeps "Masuk sebagai {name}" and, for system admins, "Proyek baru".
  - The top bar's New ticket off project pages: a direct link with one project where the user is member or above, a menu of those projects with more.
  - Messages under `home` and `nav.chooseProject`.

- [ ] **Step 1: Rewrite Home**

Replace `web/app/page.tsx` with:

```tsx
import Link from "next/link";
import { redirect } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { ClientChip, PriorityChip, StatusDot, TypeIcon } from "@/components/Chips";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import { describeChange } from "@/lib/activity";
import { day } from "@/lib/format";
import { getMe, getProjects, serverApi } from "@/lib/server-api";
import { one } from "@/lib/ticket-query";
import { button, cx, panel, table } from "@/lib/ui";

const views = ["all", "overdue", "week", "incomplete"] as const;
type View = (typeof views)[number];

// Home (FSD §6.4): what I have to do and what changed in my projects. A work
// list, not a report, so no charts. The tab lives in the URL (/?mine=overdue).
export default async function Home({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const me = await getMe();
  if (!me) redirect("/login");
  const values = one(await searchParams);
  const view: View = views.includes(values.mine as View) ? (values.mine as View) : "all";
  const t = await getTranslations("home");
  const ta = await getTranslations("activity");
  const tTypes = await getTranslations("ticketTypes");
  const tPri = await getTranslations("priorities");
  const locale = await getLocale();
  const projects = await getProjects();
  const api = await serverApi();
  const [mine, updates] = await Promise.all([
    api.GET("/me/tickets", { params: { query: { view, cursor: values.cursor } } }),
    api.GET("/me/updates"),
  ]);
  const items = mine.data?.items ?? [];
  const counts = mine.data?.counts ?? { all: 0, overdue: 0, week: 0, incomplete: 0 };
  const perProject = new Map((mine.data?.projects ?? []).map((p) => [p.key, p.open]));
  const changes = updates.data?.items ?? [];
  const now = new Date();
  const today = now.toISOString().slice(0, 10);
  // Home renders on the server, so "now" is one moment for the whole page.
  const ago = (iso: string) => {
    const minutes = Math.floor((now.getTime() - Date.parse(iso)) / 60_000);
    if (minutes < 1) return t("ago.now");
    if (minutes < 60) return t("ago.minutes", { n: minutes });
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return t("ago.hours", { n: hours });
    const days = Math.floor(hours / 24);
    if (days === 1) return t("ago.yesterday");
    return days < 7 ? t("ago.days", { n: days }) : day(iso, locale);
  };

  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
        <p className="text-[13px] text-muted">{t("signedInAs", { name: me.name })}</p>
        {me.is_admin && (
          <Link href="/projects/new" className={`${button.primary} ml-auto`}>
            <Icon name="plus" />
            {t("newProject")}
          </Link>
        )}
      </PageBar>
      {projects.length === 0 ? (
        <main className="p-4 md:p-5">
          <p className="text-muted">{me.is_admin ? t("noProjectsAdmin") : t("noProjects")}</p>
        </main>
      ) : (
        <main className="grid items-start gap-5 p-4 md:p-5 xl:grid-cols-[minmax(0,1fr)_360px]">
          <section aria-labelledby="mine-title" className={cx(panel, "min-w-0 overflow-hidden")}>
            <div className="flex flex-wrap items-baseline gap-x-2.5 px-4 pt-3">
              <h2 id="mine-title" className="text-sm font-semibold">{t("mine")}</h2>
              <span className="text-[13px] text-muted">{t("mineHint")}</span>
            </div>
            <nav aria-label={t("mineViews")} className="flex gap-1 overflow-x-auto border-b border-line px-2">
              {views.map((v) => (
                <Link
                  key={v}
                  href={v === "all" ? "/" : `/?mine=${v}`}
                  aria-current={view === v ? "page" : undefined}
                  className={cx(
                    "flex shrink-0 items-center gap-1.5 border-b-2 px-2 pb-2 pt-2.5 text-[13px] no-underline",
                    view === v ? "border-accent font-semibold text-ink hover:text-ink" : "border-transparent text-muted hover:text-ink",
                  )}
                >
                  {t(`views.${v}`)}
                  <span
                    className={cx(
                      "font-mono text-[11px]",
                      v === "overdue" && counts.overdue > 0 && "font-semibold text-danger",
                      v === "incomplete" && counts.incomplete > 0 && "font-semibold text-warn",
                    )}
                  >
                    {counts[v]}
                  </span>
                </Link>
              ))}
            </nav>
            {items.length === 0 ? (
              <p className="px-4 py-6 text-sm text-muted">
                {counts.all === 0 ? (
                  <>
                    {t("noTickets")} <Link href={`/p/${projects[0].key}/board`}>{t("openBoard")}</Link>
                  </>
                ) : (
                  t("noneInView")
                )}
              </p>
            ) : (
              <div className="overflow-x-auto">
                <table className={table.table}>
                  <thead className={table.head}>
                    <tr>
                      <th className={cx(table.th, "pl-4")}>{t("columns.ticket")}</th>
                      <th className={table.th}>{t("columns.title")}</th>
                      <th className={table.th}>{t("columns.status")}</th>
                      <th className={table.th}>{t("columns.client")}</th>
                      <th className={table.th}>{t("columns.priority")}</th>
                      <th className={cx(table.th, "pr-4 text-right")}>{t("columns.due")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((it) => {
                      const overdue = Boolean(it.due_date && it.due_date < today);
                      return (
                        <tr key={it.key} className={cx(table.row, "hover:bg-paper")}>
                          <td className={cx(table.td, "whitespace-nowrap pl-4")}>
                            <span className="flex items-center gap-1.5">
                              <TypeIcon type={it.type} label={tTypes(it.type)} />
                              <Link href={`/t/${it.key}`} className="font-mono text-xs font-semibold">{it.key}</Link>
                              {(it.missing_reason || it.missing_menus) && (
                                <span role="img" title={t("missing")} aria-label={t("missing")} className="size-[7px] rounded-full bg-[#D97706]" />
                              )}
                            </span>
                          </td>
                          <td className={cx(table.td, "min-w-64")}>
                            <Link href={`/t/${it.key}`} className="font-medium text-ink no-underline hover:text-ink hover:underline">{it.title}</Link>
                            <span className="flex flex-wrap gap-x-1.5 text-xs text-muted">
                              {it.menu ? <span>{it.menu}</span> : <span className="font-medium text-warn">{t("menuMissing")}</span>}
                              {it.menu && it.missing_reason && (
                                <>
                                  <span aria-hidden="true">·</span>
                                  <span className="font-medium text-warn">{t("reasonMissing")}</span>
                                </>
                              )}
                            </span>
                          </td>
                          <td className={cx(table.td, "whitespace-nowrap")}>
                            <span className="flex items-center gap-1.5">
                              <StatusDot color={it.status.color} />
                              {it.status.name}
                            </span>
                          </td>
                          <td className={table.td}><ClientChip client={it.client} coreLabel={t("core")} /></td>
                          <td className={table.td}><PriorityChip priority={it.priority} label={tPri(it.priority)} /></td>
                          <td className={cx(table.td, "whitespace-nowrap pr-4 text-right", overdue ? "font-medium text-danger" : "text-muted")}>
                            {it.due_date ? (
                              <span className="inline-flex items-center gap-1">
                                {overdue && <Icon name="warning" className="size-3.5" />}
                                {overdue && <span className="sr-only">{t("overdue")}</span>}
                                {day(it.due_date, locale, it.due_date.slice(0, 4) !== today.slice(0, 4))}
                              </span>
                            ) : (
                              "–"
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
            {mine.data?.next_cursor && (
              <div className="border-t border-line-soft px-4 py-2.5">
                <Link href={`/?${new URLSearchParams({ ...(view === "all" ? {} : { mine: view }), cursor: mine.data.next_cursor })}`} className={button.secondary}>
                  {t("more")}
                </Link>
              </div>
            )}
          </section>
          <div className="flex flex-col gap-5">
            <section aria-labelledby="recent-title" className={panel}>
              <div className="flex items-baseline gap-2 border-b border-line px-4 pb-2.5 pt-3">
                <h2 id="recent-title" className="text-sm font-semibold">{t("recent")}</h2>
                <span className="text-[13px] text-muted">{t("recentHint")}</span>
              </div>
              {changes.length === 0 ? (
                <p className="px-4 py-4 text-sm text-muted">{t("noChanges")}</p>
              ) : (
                <ol className="divide-y divide-line-soft">
                  {changes.map((u) => (
                    <li key={u.key} className="flex flex-col gap-0.5 px-4 py-2.5">
                      <span className="flex min-w-0 items-baseline gap-2">
                        <Link href={`/t/${u.key}`} className="shrink-0 font-mono text-xs font-semibold">{u.key}</Link>
                        <Link href={`/t/${u.key}`} className="truncate text-[13px] font-medium text-ink no-underline hover:text-ink hover:underline">{u.title}</Link>
                      </span>
                      <span className="text-xs text-muted">
                        {u.change ? describeChange(ta, u.change, me.id) : t("changed")} · {ago(u.change?.at ?? u.updated_at)}
                      </span>
                    </li>
                  ))}
                </ol>
              )}
            </section>
            <section aria-labelledby="projects-title" className={panel}>
              <h2 id="projects-title" className="border-b border-line px-4 pb-2.5 pt-3 text-sm font-semibold">{t("projects")}</h2>
              <ul className="divide-y divide-line-soft">
                {projects.map((p) => (
                  <li key={p.id}>
                    <Link href={`/p/${p.key}/board`} className="flex items-center gap-2.5 px-4 py-2.5 text-ink no-underline hover:bg-paper hover:text-ink">
                      <span className="rounded-[3px] bg-accent-soft px-1.5 font-mono text-[11px] font-semibold leading-5 text-accent-strong">{p.key}</span>
                      <span className="min-w-0 truncate text-[13px] font-medium">{p.name}</span>
                      <span className="ml-auto shrink-0 text-xs text-muted">{t("yourTickets", { count: perProject.get(p.key) ?? 0 })}</span>
                      <Icon name="chevronRight" className="size-4 text-muted" />
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          </div>
        </main>
      )}
    </>
  );
}
```

- [ ] **Step 2: New ticket off project pages**

In `web/app/TopBar.tsx`:

1. Below `const project = …`, add:

```tsx
  // Off a project page, New ticket asks which project (FSD §6.1).
  const creatable = projects.filter((p) => p.role !== "viewer");
```

2. Replace the block

```tsx
          {project && project.role !== "viewer" && (
            <Link href={`/p/${project.key}/tickets/new`} aria-label={t("newTicket")} className={button.primary}>
              <Icon name="plus" />
              <span className="hidden sm:inline">{t("newTicket")}</span>
            </Link>
          )}
```

with

```tsx
          {project ? (
            project.role !== "viewer" && (
              <Link href={`/p/${project.key}/tickets/new`} aria-label={t("newTicket")} className={button.primary}>
                <Icon name="plus" />
                <span className="hidden sm:inline">{t("newTicket")}</span>
              </Link>
            )
          ) : creatable.length === 1 ? (
            <Link href={`/p/${creatable[0].key}/tickets/new`} aria-label={t("newTicket")} className={button.primary}>
              <Icon name="plus" />
              <span className="hidden sm:inline">{t("newTicket")}</span>
            </Link>
          ) : (
            creatable.length > 1 && (
              <Menu
                align="right"
                label={t("newTicket")}
                summaryClassName={button.primary}
                summary={
                  <>
                    <Icon name="plus" />
                    <span className="hidden sm:inline">{t("newTicket")}</span>
                    <Icon name="chevron" className="size-4" />
                  </>
                }
              >
                <p className="px-3 pb-1 pt-1.5 text-xs text-muted">{t("chooseProject")}</p>
                {creatable.map((p) => (
                  <Link
                    key={p.id}
                    href={`/p/${p.key}/tickets/new`}
                    className="flex items-baseline gap-2 px-3 py-2 text-sm text-ink no-underline hover:bg-paper hover:text-ink"
                  >
                    <span className="font-mono text-xs font-semibold text-muted">{p.key}</span>
                    {p.name}
                  </Link>
                ))}
              </Menu>
            )
          )}
```

- [ ] **Step 3: Add the strings**

In `web/messages/en.json`, replace the whole `home` namespace with:

```json
  "home": {
    "title": "Home",
    "signedInAs": "Signed in as {name}",
    "newProject": "New project",
    "noProjects": "You are not in any project yet. Ask your admin to add you.",
    "noProjectsAdmin": "No projects yet. Create the first one.",
    "mine": "My tickets",
    "mineHint": "Assigned to you and not closed",
    "mineViews": "Filter my tickets",
    "views": { "all": "All", "overdue": "Overdue", "week": "Next 7 days", "incomplete": "Missing details" },
    "columns": { "ticket": "Ticket", "title": "Title", "status": "Status", "client": "Client", "priority": "Priority", "due": "Due" },
    "missing": "Reason or menus missing",
    "menuMissing": "Menu not filled",
    "reasonMissing": "Reason not filled",
    "overdue": "Overdue",
    "core": "Core",
    "noTickets": "No tickets are assigned to you.",
    "openBoard": "Open the board",
    "noneInView": "No tickets here.",
    "more": "More tickets",
    "recent": "Recently updated",
    "recentHint": "in your projects",
    "noChanges": "Nothing has changed yet.",
    "changed": "Updated",
    "ago": {
      "now": "just now",
      "minutes": "{n, plural, one {# minute ago} other {# minutes ago}}",
      "hours": "{n, plural, one {# hour ago} other {# hours ago}}",
      "yesterday": "yesterday",
      "days": "{n} days ago"
    },
    "projects": "My projects",
    "yourTickets": "{count, plural, =0 {No tickets for you} one {# ticket for you} other {# tickets for you}}"
  },
```

and in `nav`, below `newTicket`, add `"chooseProject": "New ticket in",`.

In `web/messages/id.json`, replace `home` with:

```json
  "home": {
    "title": "Beranda",
    "signedInAs": "Masuk sebagai {name}",
    "newProject": "Proyek baru",
    "noProjects": "Anda belum tergabung dalam proyek. Minta admin menambahkan Anda.",
    "noProjectsAdmin": "Belum ada proyek. Buat proyek pertama.",
    "mine": "Tiket saya",
    "mineHint": "Ditugaskan kepada Anda dan belum ditutup",
    "mineViews": "Saring tiket saya",
    "views": { "all": "Semua", "overdue": "Terlambat", "week": "7 hari ke depan", "incomplete": "Perlu dilengkapi" },
    "columns": { "ticket": "Tiket", "title": "Judul", "status": "Status", "client": "Klien", "priority": "Prioritas", "due": "Tenggat" },
    "missing": "Alasan atau menu belum diisi",
    "menuMissing": "Menu belum diisi",
    "reasonMissing": "Alasan belum diisi",
    "overdue": "Terlambat",
    "core": "Inti",
    "noTickets": "Belum ada tiket yang ditugaskan kepada Anda.",
    "openBoard": "Buka papan",
    "noneInView": "Tidak ada tiket di sini.",
    "more": "Tiket berikutnya",
    "recent": "Baru diperbarui",
    "recentHint": "di proyek Anda",
    "noChanges": "Belum ada perubahan.",
    "changed": "Diperbarui",
    "ago": {
      "now": "baru saja",
      "minutes": "{n} menit lalu",
      "hours": "{n} jam lalu",
      "yesterday": "kemarin",
      "days": "{n} hari lalu"
    },
    "projects": "Proyek saya",
    "yourTickets": "{count, plural, =0 {belum ada tiket Anda} other {# tiket Anda}}"
  },
```

and in `nav`: `"chooseProject": "Tiket baru di",`.

- [ ] **Step 4: Build and look**

Run: `cd web && npm run build`
Expected: the build succeeds.

Then run `make up`, sign in as a member with assigned tickets and open `/`. Expected:
- "Tiket saya" lists them by due date, with the tab counts;
- "Terlambat" shows only overdue ones and keeps `?mine=overdue` in the URL;
- a ticket with no menu says "Menu belum diisi" and carries the amber dot;
- "Baru diperbarui" shows sentences such as "Anda menambahkan komentar · 3 menit lalu";
- "Proyek saya" links to each board;
- with more than one project, the top bar's "Tiket baru" opens a menu of projects.

- [ ] **Step 5: Commit**

```bash
git add web/app/page.tsx web/app/TopBar.tsx web/messages
git commit -m "feat(web): Home with my tickets, recent changes and my projects"
```

### Task 15: The Iteration 3 exit check

**Files:**
- Create: `web/e2e/decisions.spec.ts`
- Modify: `web/e2e/global-setup.ts` (one more admin)

**Interfaces:**
- Consumes: every screen of Tasks 9–14; the API of Tasks 3–7 for the setup.
- Produces: the end-to-end test for FSD §21 Iteration 3's exit check (story 1), plus AC-DC-1, AC-DC-2, AC-DC-4 and AC-TK-2 on real screens.

- [ ] **Step 1: Give the test its own admin**

In `web/e2e/global-setup.ts`, at the end of `globalSetup`, add:

```ts
  const decisions = createAdmin("Decision Admin");
  process.env.E2E_DECISION_ADMIN_EMAIL = decisions.email;
  process.env.E2E_DECISION_ADMIN_LINK = decisions.link;
```

- [ ] **Step 2: Write the test**

`web/e2e/decisions.spec.ts`:

```ts
import { expect, test, type Page } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// api returns a JSON caller that acts as the page's signed-in user.
function api(page: Page) {
  return async (method: string, path: string, data?: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: new URL(page.url()).origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.status() === 204 ? null : res.json();
  };
}

// FSD §21 Iteration 3 exit check: a developer sees a menu's history newest
// first (story 1). On the way, closes from the ticket page and from the board
// ask for the decision record (AC-DC-1, AC-DC-2, AC-DC-4, AC-TK-2), Home shows
// the developer's ticket, and search finds the menu and jumps to a key.
test("a developer sees a menu's history newest first", async ({ page, browser }) => {
  test.setTimeout(180_000);
  const run = Date.now().toString(36).toUpperCase(); // keys and names stay unique across runs on one stack
  const key = `D${run.slice(-6)}`;
  const clientName = `Arunika ${run}`;
  const rinaEmail = `rina-${run.toLowerCase()}@example.com`;
  const dimasEmail = `dimas-${run.toLowerCase()}@example.com`;
  const adminPassword = "e2e-decision-admin-passphrase-7";
  const rinaPassword = "e2e-rina-decision-passphrase-8";
  const dimasPassword = "e2e-dimas-dev-passphrase-9";

  // The admin sets the project up through the API; its screens have their own tests.
  await setPassword(page, process.env.E2E_DECISION_ADMIN_LINK!, adminPassword);
  await signIn(page, process.env.E2E_DECISION_ADMIN_EMAIL!, adminPassword);
  const asAdmin = api(page);
  const client = await asAdmin("POST", "/clients", { name: clientName });
  await asAdmin("POST", "/projects", { key, name: `HRIS ${run}` });
  await asAdmin("PUT", `/projects/${key}/clients`, { client_ids: [client.id] });
  const hr = await asAdmin("POST", `/projects/${key}/nodes`, { type: "module", name: "HR" });
  const attendance = await asAdmin("POST", `/projects/${key}/nodes`, { type: "module", name: "Attendance", parent_id: hr.id });
  const overtime = await asAdmin("POST", `/projects/${key}/nodes`, { type: "menu", name: "Overtime Approval", parent_id: attendance.id });
  const rina = await asAdmin("POST", "/admin/users", { name: "Rina PM", email: rinaEmail });
  const dimas = await asAdmin("POST", "/admin/users", { name: "Dimas Dev", email: dimasEmail });
  await asAdmin("PUT", `/projects/${key}/members`, {
    members: [
      { email: rinaEmail, role: "member", all_clients: true },
      { email: dimasEmail, role: "member", all_clients: true },
    ],
  });

  // Rina files three requests on Overtime Approval; the second has no reason yet.
  const pm = await (await browser.newContext()).newPage();
  await setPassword(pm, rina.setup_link.url, rinaPassword);
  await signIn(pm, rinaEmail, rinaPassword);
  const asRina = api(pm);
  const file = (title: string, fields: object) =>
    asRina("POST", `/projects/${key}/tickets`, { type: "change_request", title, client_id: client.id, node_ids: [overtime.id], ...fields });
  const inThreeDays = new Date(Date.now() + 3 * 86_400_000).toISOString().slice(0, 10);
  await file("Overtime cap of 40 hours a month", { reason: "Arunika's labor agreement caps overtime at 40 hours a month." });
  await file("Weekend overtime at double rate", {});
  await file("Skip supervisor approval for overtime", {
    reason: "Supervisors are often on leave; HR approves overtime directly.",
    assignee_id: dimas.user.id,
    due_date: inThreeDays,
  });

  // AC-DC-1 and AC-DC-2: closing from the ticket page asks for the decision record, prefilled.
  await pm.goto(`/t/${key}-1`);
  await pm.getByLabel(/^Status/).selectOption({ label: "Done" });
  const dialog = pm.getByRole("dialog", { name: `Tutup ${key}-1 sebagai Done` });
  const whatChanged = dialog.getByLabel("Apa yang berubah");
  await expect(whatChanged).toHaveValue("Overtime cap of 40 hours a month");
  await expect(dialog.getByLabel("Mengapa")).toHaveValue("Arunika's labor agreement caps overtime at 40 hours a month.");
  await whatChanged.fill("");
  await expect(dialog.getByRole("button", { name: "Tutup tiket" })).toBeDisabled();
  await expect(dialog.getByText("Wajib diisi")).toBeVisible();
  await whatChanged.fill("Payroll blocks overtime approvals above 40 hours a month.");
  await dialog.getByLabel("Alternatif yang ditolak").fill("A warning without a block, rejected because payroll still paid the hours.");
  await dialog.getByRole("button", { name: "Tutup tiket" }).click();
  const record = pm.getByRole("region", { name: "Catatan keputusan" });
  await expect(record).toContainText("Dikonfirmasi oleh Rina PM");
  await expect(record).toContainText("Payroll blocks overtime approvals above 40 hours a month.");

  // AC-TK-2: dropping a card on Done opens the dialog, and Batal leaves the card in To do.
  await pm.goto(`/p/${key}/board`);
  const todo = pm.getByRole("region", { name: "To do" });
  const done = pm.getByRole("region", { name: "Done" });
  const weekend = pm.getByRole("article").filter({ hasText: `${key}-2` });
  await weekend.dragTo(done);
  const dialog2 = pm.getByRole("dialog", { name: `Tutup ${key}-2 sebagai Done` });
  await dialog2.getByRole("button", { name: "Batal" }).click();
  await expect(dialog2).toHaveCount(0);
  await expect(todo).toContainText(`${key}-2`);

  // AC-DC-4: the ticket has no reason, so the dialog asks for one and flags a weak one.
  await weekend.dragTo(done);
  const reason = dialog2.getByLabel("Alasan");
  await reason.fill("sesuai permintaan klien");
  await expect(dialog2.getByText(/Jelaskan mengapa klien membutuhkannya/)).toBeVisible();
  await reason.fill("Arunika's new labor agreement pays weekend overtime at double rate.");
  await dialog2.getByLabel("Apa yang berubah").fill("Weekend overtime pays double for Arunika from October.");
  await dialog2.getByLabel("Mengapa").fill("The new labor agreement doubles weekend overtime pay.");
  await dialog2.getByRole("button", { name: "Tutup tiket" }).click();
  await expect(done).toContainText(`${key}-2`);

  // Dimas, a developer, starts from Home, where the assigned ticket waits under Tiket saya.
  const dev = await (await browser.newContext()).newPage();
  await setPassword(dev, dimas.setup_link.url, dimasPassword);
  await signIn(dev, dimasEmail, dimasPassword);
  await expect(dev.getByRole("region", { name: "Tiket saya" }).getByRole("link", { name: `${key}-3` })).toBeVisible();
  await expect(dev.getByRole("region", { name: "Baru diperbarui" })).toContainText(`${key}-2`);

  // Story 1: search finds the menu, and its page lists the history newest first.
  const search = dev.getByRole("searchbox", { name: "Cari tiket atau menu" });
  await search.fill("Overtime Appr");
  await search.press("Enter");
  await dev.getByRole("link", { name: /HR › Attendance › Overtime Approval/ }).click();
  await expect(dev.getByRole("heading", { level: 1, name: "Overtime Approval" })).toBeVisible();
  const open = dev.getByRole("region", { name: /^Sedang berjalan/ });
  await expect(open.getByRole("article")).toHaveCount(1);
  await expect(open).toContainText(`${key}-3`);
  const history = dev.getByRole("region", { name: /^Ditutup/ }).getByRole("article");
  await expect(history).toHaveCount(2);
  await expect(history.nth(0)).toContainText(`${key}-2`); // closed last, so listed first
  await expect(history.nth(0)).toContainText("Weekend overtime pays double for Arunika from October.");
  await expect(history.nth(0)).toContainText("The new labor agreement doubles weekend overtime pay.");
  await expect(history.nth(1)).toContainText(`${key}-1`);
  await expect(history.nth(1)).toContainText("Payroll blocks overtime approvals above 40 hours a month.");

  // Story 5: Behaviors by client lists the decisions in force under the client.
  await dev.getByRole("link", { name: "Perilaku per klien" }).click();
  await expect(dev.getByRole("region", { name: clientName })).toContainText(`${key}-1`);

  // A key typed in the search box opens its ticket (FSD §6.1).
  await search.fill(`${key.toLowerCase()}-1`);
  await search.press("Enter");
  await expect(dev).toHaveURL(new RegExp(`/t/${key}-1$`));
});
```

- [ ] **Step 3: Run the whole suite on a rebuilt stack**

Run: `make up && cd web && E2E_BASE_URL=http://localhost:8080 npx playwright test`
Expected: 4 passed (signin, registry, tickets and decisions). The four files sign in about ten times, under the limit of 20 a minute per IP. Setup links are single-use, so every run needs the fresh admins that `globalSetup` makes; `--repeat-each` fails by design.

- [ ] **Step 4: Commit**

```bash
git add web/e2e
git commit -m "test(e2e): a developer sees a menu's history newest first"
```

- [ ] **Step 5: Final checks and the FSD**

Run:

```bash
cd server && gofmt -l . && go vet ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...
cd ../web && npm run gen:api && git diff --exit-code lib/api-types.ts && npm run build
```

Expected: no gofmt output, no vet findings, `ok` for every Go package, no drift in `lib/api-types.ts`, and a clean build.

Then update FSD §21's "Progress" paragraph through the Claude Docs connector (never as a file): Iteration 3 is built and passes its exit check, and its repository plan is `docs/superpowers/plans/2026-09-24-iteration-3-decisions.md`, which lists the deliberate deviations. Keep the rest of the paragraph.
