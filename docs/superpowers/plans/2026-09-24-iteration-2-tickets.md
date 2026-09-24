# Muasal Iteration 2 — Tickets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A PM logs a client request in one form, with the client, who asked, the affected menus and the reason. The team then works it on a board, a list and a ticket page, with statuses, Internal comments, attachments and a full history. This is the exit check for FSD §21 Iteration 2 (story 3).

**Architecture:** Same stack and patterns as Iterations 0 and 1.
- **Tickets:** they live in PostgreSQL with composite foreign keys to their project's statuses and clients. So a ticket cannot point at another project's status or at a client the project lacks, a status in use cannot be deleted, and a client in use cannot be unlinked.
- **Default statuses:** a database trigger gives every new project its default statuses.
- **Ticket keys:** a per-project counter updated inside the create transaction numbers the keys without gaps.
- **Saving:** saves carry `If-Match`, and a stale version answers 412.
- **Attachments:** files are stored by SHA-256 on a volume and served by Go after the ticket's visibility check.
- **History:** comment edits, deletes, transitions and field updates are audit events on the ticket, and the Activity view interleaves them with comments.

**Tech Stack:** Unchanged; no new dependencies. The board uses native HTML drag-and-drop plus a status menu on every card, so keyboards work too. Forms use native elements.

**Spec:** Claude Docs "FSD — Muasal":
- §8.1–8.7 and §8.11: ticketing.
- §9.1–9.2: close rules, which arrive in Iteration 3.
- §5: visibility.
- §15.2: project key rules.
- §16: data model.
- §17: API.
- §21: the delivery plan.

## Global Constraints

- **Carried over:** everything in the Iteration 0 and Iteration 1 Global Constraints still holds, notably "hidden and missing look the same" (404) and "one transaction per mutation with its audit row".
- **Ticket key:** `<PROJECT>-<n>`. The number comes from a per-project counter incremented inside the create transaction, so keys have no gaps and never change. A project's key is fixed once it has a ticket (FSD §15.2).
- **Ticket fields (FSD §8.1):**
  - title: 5–200 characters;
  - type: `bug`, `change_request` (default) or `feature`;
  - priority: `low`, `medium` (default), `high` or `urgent`;
  - reason: up to 2,000 characters;
  - description: up to 50,000 characters.
- **Client:** a client linked to the project, or none, which means core work for all clients. It must be within the creator's scope (AC-TK-4).
- **Requested by:** a contact of the ticket's client, an internal contact, or a user. Without one, it defaults to the reporter.
- **Assignee:** a project member who is not a viewer and whose scope covers the ticket.
- **Affected menus:** live nodes of the project that the editor can see.
- **Statuses (R-TK-1, R-TK-2):**
  - categories `todo`, `in_progress`, `done`, `cancelled`;
  - new projects start with To do (the default), In progress, In review, Done and Cancelled;
  - exactly one default, and it is a To do status;
  - at least one Done and one Cancelled status;
  - names are unique per project, ignoring case.
- **Deleting a status (R-TK-4):** a status in use is deleted only together with a move of its tickets to another status of the same open-or-closed kind.
- **Closing:** any move into a Done or Cancelled status is a close (R-TK-3). Closing needs the decision record (§9.1), which arrives in Iteration 3. Until then the API answers 422 `close_unavailable`, and the board does not accept drops there.
- **Versions:** tickets carry `version`. `GET` returns `ETag: "<version>"`; `PUT` needs `If-Match` and answers 412 `stale` when the version moved on (AC-TK-5).
- **Comments:** Internal by default (AC-TK-10). Authors edit and delete their own. Every edit keeps the old text in the ticket's history (AC-TK-6). Deletes are soft, and only system admins still read the text.
- **Attachments (AC-TK-7):**
  - at most 25 MB per file;
  - types: images (png, jpg, gif, webp), PDF, Office and OpenDocument files, txt, csv, log, zip;
  - stored as `<ATTACHMENTS_DIR>/<sha256[:2]>/<sha256>`;
  - served with `nosniff`, images inline and everything else as a download.
- **History:** the ticket's history lists events with `entity = 'ticket'`: `create`, `update` (old and new values), `transition`, `comment_edit`, `comment_delete`, `attachment_add`, `attachment_delete`.
- **Linked nodes:** a node linked to a ticket is archived, never deleted (R-MR-4).
- **New error codes the web translates:**
  - `stale`, `close_unavailable`, `status_in_use`, `status_name_taken`, `project_key_fixed`;
  - `node_linked`, `parent_archived`;
  - `file_too_large`, `file_type_not_allowed`, `invalid_upload`.

## Deliberate Deviations from the FSD

**API shape**
- Ticket lists are per project: `GET /projects/{key}/tickets`, not `GET /tickets`. Editing is `PUT /tickets/{key}`, which replaces the editable fields, not PATCH. The ticket page always sends every field, and a full replacement avoids the null-versus-absent ambiguity of client, assignee and due date.
- List pagination uses an opaque offset cursor, capped at 1,000 rows a page. Activity is not paginated yet.
- The list filters are status, category, open-only, type, client or core, assignee or "only mine", node (with sub-nodes), text (title or key), and missing reason or menus. The requester and date-range filters come later.

**Deferred or simplified features**
- **Closing** waits for Iteration 3 (close dialog, decision records). So do the board's "last 14 days" for closed columns, `closed_at`, and the weak-reason hint (R-DC-8).
- **Ticket archive and restore** (R-AC-10), `source`, `external_ref`, CSV export and similar-ticket hints (P1) come later.
- **Text rendering:** descriptions and comments render as plain text with line breaks. Markdown rendering (react-markdown + rehype-sanitize), pasted images and @mentions come later.
- **The create form** is a page. The modal and the `c` shortcut come later, as do "recently used first" menus and requesters who are other users. A requester is you or a contact.
- **R-MR-10** shows the warning. The one-click "Add Client B to this menu" comes later.
- **Libraries and drag-and-drop:** dnd-kit and shadcn/ui are not added (see Tech Stack). The module tree keeps its buttons for moving.
- **Project audit search** (`GET /projects/{key}/audit`) moves to the admin audit log work.
- **The attachment limit** is fixed at 25 MB. The admin setting comes with the settings page.

## Prerequisites

- Iteration 1 merged to `main`; work on branch `feat/iteration-2`.
- `make testdb` for the Go tests; `make up` (rebuilt) for the end-to-end tests.

## File Structure

```text
server/
├── migrations/00003_tickets.sql                statuses (+ default-status trigger), tickets, ticket_nodes, comments, attachments
├── Dockerfile                                  /data/attachments owned by nonroot
├── internal/
│   ├── config/config.go                        ATTACHMENTS_DIR, 25 MB limit
│   ├── access/access.go                        Scope.Sees(clientID)
│   ├── db/queries/{statuses,tickets,comments,attachments}.sql (+ changes to nodes, contacts, memberships, projects, audit)
│   ├── db/tickets_test.go                      ticket constraint names and default statuses
│   └── httpapi/
│       ├── statuses.go (+ statuses_test.go)    statuses, assignees
│       ├── tickets.go (+ tickets_test.go)      create, read, update, transition
│       ├── ticket_list.go (+ ticket_list_test.go) filters, sort, cursor
│       ├── comments.go (+ comments_test.go)    comments and the Activity feed
│       ├── attachments.go (+ attachments_test.go) upload, download, delete
│       └── nodes.go, clients.go, contacts.go   archive/restore, tickets in use, internal contacts
deploy/compose.yaml                             an attachments volume for app
web/
├── components/TicketForm.tsx                   create and edit a ticket
├── components/TicketFilters.tsx                the filter bar of the board and the list
├── app/p/[key]/board/                          the board
├── app/p/[key]/tickets/                        the list; tickets/new is the create page
├── app/t/[ticketKey]/                          the ticket page, its activity and attachments
├── app/p/[key]/settings/StatusesForm.tsx       project statuses
└── e2e/tickets.spec.ts                         the Iteration 2 exit check
```

---

### Task 1: Ticket data layer

**Files:**
- Create: `server/migrations/00003_tickets.sql`
- Create: `server/internal/db/queries/statuses.sql`, `tickets.sql`, `comments.sql`, `attachments.sql`
- Modify: `server/internal/db/queries/audit.sql`, `memberships.sql`, `contacts.sql`, `nodes.sql`, `projects.sql`; `server/sqlc.yaml` (dates as `time.Time`)
- Modify: `server/internal/httpapi/clients.go` (the new `ListProjectClients` parameters)
- Test: `server/internal/db/tickets_test.go`

**Interfaces:**
- Consumes: the Iteration 1 schema and queries.
- Produces (package `db`):
  - Models: `Status{ID, ProjectID int64; Name, Category string; Position int32; Color string; IsDefault bool}`, `Ticket{ID, ProjectID, Number int64; Key, Type, Title, Description, Reason string; StatusID int64; ClientID, RequesterContactID, RequesterUserID *int64; ReporterID int64; AssigneeID *int64; Priority string; DueDate *time.Time; Version int32; CreatedAt, UpdatedAt time.Time}`, `Comment{ID, TicketID, AuthorID int64; Internal bool; Body string; CreatedAt time.Time; EditedAt, DeletedAt *time.Time}`, `Attachment{ID, TicketID, UploaderID int64; Filename, ContentType string; SizeBytes int64; Sha256 []byte; CreatedAt time.Time; DeletedAt *time.Time}`; `Project` gains `TicketSeq int64`.
  - Statuses: `ListStatuses(ctx, projectID) ([]Status, error)`, `GetStatus(ctx, id)`, `GetDefaultStatus(ctx, projectID)`, `ClearDefaultStatus(ctx, projectID) error`, `UpdateStatus(ctx, UpdateStatusParams{ID, ProjectID int64; Name, Category string; Position int32; Color string; IsDefault bool}) (int64, error)` (rows changed), `InsertStatus(ctx, InsertStatusParams{ProjectID int64; Name, Category string; Position int32; Color string; IsDefault bool}) (int64, error)`, `MoveTicketsToStatus(ctx, MoveTicketsToStatusParams{ToID, ProjectID, FromID int64}) error`, `DeleteStatusesExcept(ctx, DeleteStatusesExceptParams{ProjectID int64; KeepIds []int64}) error`.
  - Tickets:
    - `NextTicketNumber(ctx, projectID) (int64, error)`
    - `CreateTicket(ctx, CreateTicketParams{ProjectID, Number int64; Key, Type, Title, Description, Reason string; StatusID int64; ClientID, RequesterContactID, RequesterUserID *int64; ReporterID int64; AssigneeID *int64; Priority string; DueDate *time.Time}) (Ticket, error)`
    - `GetTicketByKey(ctx, key) (GetTicketByKeyRow{Ticket Ticket; Status Status; ProjectKey, ReporterName string; ClientName, RequesterContactName, RequesterContactTitle, RequesterUserName, AssigneeName *string}, error)`
    - `UpdateTicket(ctx, UpdateTicketParams{ID int64; Version int32; Type, Title, Description, Reason string; ClientID, RequesterContactID, RequesterUserID, AssigneeID *int64; Priority string; DueDate *time.Time}) (Ticket, error)`: `pgx.ErrNoRows` when the version is stale.
    - `SetTicketStatus(ctx, SetTicketStatusParams{ID, StatusID int64}) (Ticket, error)`
    - `ListTicketNodes(ctx, ticketID) ([]ListTicketNodesRow{ID int64; Name string; Archived bool}, error)`, `ClearTicketNodes(ctx, ticketID) error`, `AddTicketNodes(ctx, AddTicketNodesParams{TicketID int64; NodeIds []int64}) error`
    - `ListTickets(ctx, ListTicketsParams{…}) ([]ListTicketsRow, error)`
  - Comments: `CreateComment(ctx, CreateCommentParams{TicketID, AuthorID int64; Internal bool; Body string}) (Comment, error)`, `GetComment(ctx, id) (GetCommentRow{Comment Comment; TicketKey string}, error)`, `UpdateCommentBody(ctx, UpdateCommentBodyParams{ID int64; Body string}) (Comment, error)`, `DeleteComment(ctx, id) error`, `ListComments(ctx, ticketID) ([]ListCommentsRow{Comment Comment; AuthorName string}, error)`.
  - Attachments: `CreateAttachment(ctx, CreateAttachmentParams{TicketID, UploaderID int64; Filename, ContentType string; SizeBytes int64; Sha256 []byte}) (Attachment, error)`, `GetAttachment(ctx, id) (GetAttachmentRow{Attachment Attachment; TicketKey string}, error)`, `ListAttachments(ctx, ticketID) ([]ListAttachmentsRow{Attachment Attachment; UploaderName string}, error)`, `DeleteAttachment(ctx, id) error`.
  - Audit: `ListTicketEvents(ctx, ticketID) ([]ListTicketEventsRow{ID int64; OccurredAt time.Time; ActorID *int64; ActorName *string; Action string; Changes []byte}, error)`.
  - Members: `ListAssignees(ctx, projectID) ([]ListAssigneesRow{ID int64; Name string}, error)`.
  - Changed queries:
    - `ListProjectClients(ctx, ListProjectClientsParams{ProjectID int64; AllClients bool; ClientIds []int64})`
    - `ListContacts` gains `Internal bool`.
    - `ListNodes` gains `IncludeArchived bool`, and rows gain `Archived bool`.
    - `UpdateNode` gains `Archived *bool`.
    - New `CountLiveChildren(ctx, nodeID) (int64, error)`.
  - Constraint names: `tickets_status_same_project`, `tickets_client_linked`, `tickets_one_requester`, `statuses_default_uq`, `statuses_default_todo`, `statuses_name_uq`, `ticket_nodes_node_fk`.

- [ ] **Step 1: Write the migration**

`server/migrations/00003_tickets.sql`:

```sql
-- +goose Up
-- Statuses, tickets, their menus, comments and attachments (FSD §8, §16).
ALTER TABLE projects ADD COLUMN ticket_seq bigint NOT NULL DEFAULT 0; -- incremented inside the ticket create transaction

CREATE TABLE statuses (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects (id),
  name       text NOT NULL,
  category   text NOT NULL CHECK (category IN ('todo', 'in_progress', 'done', 'cancelled')),
  position   int NOT NULL,
  color      text NOT NULL DEFAULT '#6B7280',
  is_default boolean NOT NULL DEFAULT false,
  UNIQUE (project_id, id), -- lets a ticket point at a status of its own project
  CONSTRAINT statuses_default_todo CHECK (NOT is_default OR category = 'todo')
);
CREATE UNIQUE INDEX statuses_name_uq ON statuses (project_id, lower(name));
CREATE UNIQUE INDEX statuses_default_uq ON statuses (project_id) WHERE is_default; -- one default at most (R-TK-2)

-- R-TK-2: every project starts with these statuses; the trigger adds them to new projects.
-- +goose StatementBegin
CREATE FUNCTION insert_default_statuses(pid bigint) RETURNS void LANGUAGE sql AS $$
  INSERT INTO statuses (project_id, name, category, position, color, is_default)
  VALUES (pid, 'To do', 'todo', 0, '#6B7280', true),
         (pid, 'In progress', 'in_progress', 1, '#2563EB', false),
         (pid, 'In review', 'in_progress', 2, '#7C3AED', false),
         (pid, 'Done', 'done', 3, '#16A34A', false),
         (pid, 'Cancelled', 'cancelled', 4, '#9CA3AF', false);
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION projects_default_statuses() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM insert_default_statuses(NEW.id);
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER projects_default_statuses AFTER INSERT ON projects
  FOR EACH ROW EXECUTE FUNCTION projects_default_statuses();

SELECT insert_default_statuses(id) FROM projects;

CREATE TABLE tickets (
  id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id           bigint NOT NULL REFERENCES projects (id),
  number               bigint NOT NULL,
  key                  text NOT NULL UNIQUE,                 -- 'HRIS-231'
  type                 text NOT NULL CHECK (type IN ('bug', 'change_request', 'feature')),
  title                text NOT NULL,
  description          text NOT NULL DEFAULT '',
  reason               text NOT NULL DEFAULT '',
  status_id            bigint NOT NULL,
  client_id            bigint,                               -- NULL = all clients (core work)
  requester_contact_id bigint REFERENCES contacts (id),
  requester_user_id    bigint REFERENCES users (id),
  reporter_id          bigint NOT NULL REFERENCES users (id),
  assignee_id          bigint REFERENCES users (id),
  priority             text NOT NULL DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high', 'urgent')),
  due_date             date,
  version              int NOT NULL DEFAULT 1,              -- optimistic locking (If-Match)
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, number),
  -- A status of the ticket's own project; a status in use cannot be deleted (R-TK-4).
  CONSTRAINT tickets_status_same_project FOREIGN KEY (project_id, status_id) REFERENCES statuses (project_id, id),
  -- A client linked to the project; a client in use cannot be unlinked (FSD §15.2).
  CONSTRAINT tickets_client_linked FOREIGN KEY (project_id, client_id) REFERENCES project_clients,
  CONSTRAINT tickets_one_requester CHECK (num_nonnulls(requester_contact_id, requester_user_id) = 1)
);
CREATE INDEX tickets_scope_idx ON tickets (project_id, client_id, status_id);

CREATE TABLE ticket_nodes (
  ticket_id bigint NOT NULL REFERENCES tickets (id),
  node_id   bigint NOT NULL CONSTRAINT ticket_nodes_node_fk REFERENCES nodes (id), -- linked nodes are archived, never deleted (R-MR-4)
  PRIMARY KEY (ticket_id, node_id)
);
CREATE INDEX ticket_nodes_node_idx ON ticket_nodes (node_id);

CREATE TABLE comments (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ticket_id  bigint NOT NULL REFERENCES tickets (id),
  author_id  bigint NOT NULL REFERENCES users (id),
  internal   boolean NOT NULL DEFAULT true,                 -- Internal by default; false = client-safe (FSD §8.7)
  body       text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  edited_at  timestamptz,
  deleted_at timestamptz
);
CREATE INDEX comments_ticket_idx ON comments (ticket_id, created_at);

CREATE TABLE attachments (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ticket_id    bigint NOT NULL REFERENCES tickets (id),
  uploader_id  bigint NOT NULL REFERENCES users (id),
  filename     text NOT NULL,
  content_type text NOT NULL,
  size_bytes   bigint NOT NULL,
  sha256       bytea NOT NULL,                              -- the file is <ATTACHMENTS_DIR>/<hex[:2]>/<hex>
  created_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz
);
CREATE INDEX attachments_ticket_idx ON attachments (ticket_id);

-- +goose Down
DROP TABLE attachments;
DROP TABLE comments;
DROP TABLE ticket_nodes;
DROP TABLE tickets;
DROP TRIGGER projects_default_statuses ON projects;
DROP FUNCTION projects_default_statuses();
DROP FUNCTION insert_default_statuses(bigint);
DROP TABLE statuses;
ALTER TABLE projects DROP COLUMN ticket_seq;
```

In `server/sqlc.yaml`, add two overrides after the `timestamptz` ones, so dates read as `time.Time` like timestamps:

```yaml
          - db_type: "date"
            go_type: "time.Time"
          - db_type: "date"
            go_type:
              type: "time.Time"
              pointer: true
            nullable: true
```

- [ ] **Step 2: Write the new queries**

`server/internal/db/queries/statuses.sql`:

```sql
-- name: ListStatuses :many
SELECT * FROM statuses WHERE project_id = $1 ORDER BY position, id;

-- name: GetStatus :one
SELECT * FROM statuses WHERE id = $1;

-- name: GetDefaultStatus :one
SELECT * FROM statuses WHERE project_id = $1 AND is_default;

-- name: ClearDefaultStatus :exec
-- Runs before a statuses update, so the new default never meets the old one.
UPDATE statuses SET is_default = false WHERE project_id = $1;

-- name: UpdateStatus :execrows
UPDATE statuses SET name = $3, category = $4, position = $5, color = $6, is_default = $7
WHERE id = $1 AND project_id = $2;

-- name: InsertStatus :one
INSERT INTO statuses (project_id, name, category, position, color, is_default)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: MoveTicketsToStatus :exec
UPDATE tickets SET status_id = sqlc.arg('to_id'), version = version + 1, updated_at = now()
WHERE project_id = sqlc.arg('project_id') AND status_id = sqlc.arg('from_id');

-- name: DeleteStatusesExcept :exec
DELETE FROM statuses
WHERE project_id = sqlc.arg('project_id') AND NOT (id = ANY (sqlc.arg('keep_ids')::bigint[]));
```

`server/internal/db/queries/tickets.sql`:

```sql
-- name: NextTicketNumber :one
-- The row lock makes ticket numbers gapless and unique per project (FSD §8.1).
UPDATE projects SET ticket_seq = ticket_seq + 1 WHERE id = $1 RETURNING ticket_seq;

-- name: CreateTicket :one
INSERT INTO tickets (project_id, number, key, type, title, description, reason, status_id, client_id,
                     requester_contact_id, requester_user_id, reporter_id, assignee_id, priority, due_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: GetTicketByKey :one
SELECT sqlc.embed(t), sqlc.embed(s), p.key AS project_key, rp.name AS reporter_name, c.name AS client_name,
       rc.name AS requester_contact_name, rc.title AS requester_contact_title,
       ru.name AS requester_user_name, a.name AS assignee_name
FROM tickets t
JOIN statuses s ON s.id = t.status_id
JOIN projects p ON p.id = t.project_id
JOIN users rp ON rp.id = t.reporter_id
LEFT JOIN clients c ON c.id = t.client_id
LEFT JOIN contacts rc ON rc.id = t.requester_contact_id
LEFT JOIN users ru ON ru.id = t.requester_user_id
LEFT JOIN users a ON a.id = t.assignee_id
WHERE t.key = $1;

-- name: UpdateTicket :one
-- Optimistic locking: no row comes back when the version moved on (FSD §8.6).
UPDATE tickets SET type = $3, title = $4, description = $5, reason = $6, client_id = $7,
  requester_contact_id = $8, requester_user_id = $9, assignee_id = $10, priority = $11, due_date = $12,
  version = version + 1, updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: SetTicketStatus :one
UPDATE tickets SET status_id = $2, version = version + 1, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListTicketNodes :many
SELECT n.id, n.name, (n.archived_at IS NOT NULL)::boolean AS archived
FROM ticket_nodes tn
JOIN nodes n ON n.id = tn.node_id
WHERE tn.ticket_id = $1
ORDER BY lower(n.name), n.id;

-- name: ClearTicketNodes :exec
DELETE FROM ticket_nodes WHERE ticket_id = $1;

-- name: AddTicketNodes :exec
INSERT INTO ticket_nodes (ticket_id, node_id)
SELECT sqlc.arg('ticket_id')::bigint, unnest(sqlc.arg('node_ids')::bigint[]);

-- name: ListTickets :many
-- One project's tickets that the scope may see (R-AC-2, R-AC-3), for the list
-- and the board. A filter is off when its argument is NULL or false.
SELECT t.id, t.key, t.title, t.type, t.priority, t.due_date, t.status_id, t.client_id, c.name AS client_name,
       t.assignee_id, a.name AS assignee_name, coalesce(rc.name, ru.name, '')::text AS requester_name,
       t.reason = '' AS missing_reason, t.updated_at,
       ARRAY(SELECT n.name FROM ticket_nodes tn JOIN nodes n ON n.id = tn.node_id
             WHERE tn.ticket_id = t.id ORDER BY lower(n.name), n.id)::text[] AS node_names
FROM tickets t
JOIN statuses s ON s.id = t.status_id
LEFT JOIN clients c ON c.id = t.client_id
LEFT JOIN users a ON a.id = t.assignee_id
LEFT JOIN contacts rc ON rc.id = t.requester_contact_id
LEFT JOIN users ru ON ru.id = t.requester_user_id
WHERE t.project_id = sqlc.arg('project_id')
  AND (sqlc.arg('all_clients')::boolean OR t.client_id IS NULL OR t.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
  AND (sqlc.narg('status_id')::bigint IS NULL OR t.status_id = sqlc.narg('status_id')::bigint)
  AND (sqlc.narg('category')::text IS NULL OR s.category = sqlc.narg('category')::text)
  AND (NOT sqlc.arg('open_only')::boolean OR s.category IN ('todo', 'in_progress'))
  AND (sqlc.narg('type')::text IS NULL OR t.type = sqlc.narg('type')::text)
  AND (sqlc.narg('client_id')::bigint IS NULL OR t.client_id = sqlc.narg('client_id')::bigint)
  AND (NOT sqlc.arg('core_only')::boolean OR t.client_id IS NULL)
  AND (sqlc.narg('assignee_id')::bigint IS NULL OR t.assignee_id = sqlc.narg('assignee_id')::bigint)
  AND (sqlc.narg('node_ids')::bigint[] IS NULL OR EXISTS (
        SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id AND tn.node_id = ANY (sqlc.narg('node_ids')::bigint[])))
  AND (NOT sqlc.arg('missing_reason')::boolean OR t.reason = '')
  AND (NOT sqlc.arg('missing_menus')::boolean OR NOT EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id))
  AND (sqlc.arg('q')::text = '' OR t.title ILIKE '%' || sqlc.arg('q')::text || '%' OR t.key = upper(sqlc.arg('q')::text))
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'priority' THEN array_position(ARRAY['urgent', 'high', 'medium', 'low'], t.priority) END,
  CASE WHEN sqlc.arg('sort')::text IN ('priority', 'due') THEN t.due_date END NULLS LAST,
  CASE WHEN sqlc.arg('sort')::text = 'updated' THEN t.updated_at END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'created' THEN t.number END DESC,
  t.number
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');
```

`server/internal/db/queries/comments.sql`:

```sql
-- name: CreateComment :one
INSERT INTO comments (ticket_id, author_id, internal, body) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetComment :one
SELECT sqlc.embed(c), t.key AS ticket_key
FROM comments c JOIN tickets t ON t.id = c.ticket_id
WHERE c.id = $1;

-- name: UpdateCommentBody :one
UPDATE comments SET body = $2, edited_at = now() WHERE id = $1 AND deleted_at IS NULL RETURNING *;

-- name: DeleteComment :exec
UPDATE comments SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;

-- name: ListComments :many
SELECT sqlc.embed(c), u.name AS author_name
FROM comments c JOIN users u ON u.id = c.author_id
WHERE c.ticket_id = $1
ORDER BY c.created_at, c.id;
```

`server/internal/db/queries/attachments.sql`:

```sql
-- name: CreateAttachment :one
INSERT INTO attachments (ticket_id, uploader_id, filename, content_type, size_bytes, sha256)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAttachment :one
SELECT sqlc.embed(a), t.key AS ticket_key
FROM attachments a JOIN tickets t ON t.id = a.ticket_id
WHERE a.id = $1 AND a.deleted_at IS NULL;

-- name: ListAttachments :many
SELECT sqlc.embed(a), u.name AS uploader_name
FROM attachments a JOIN users u ON u.id = a.uploader_id
WHERE a.ticket_id = $1 AND a.deleted_at IS NULL
ORDER BY a.created_at, a.id;

-- name: DeleteAttachment :exec
UPDATE attachments SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;
```

- [ ] **Step 3: Change the existing queries**

Append to `server/internal/db/queries/audit.sql`:

```sql

-- name: ListTicketEvents :many
-- A ticket's history: every event recorded against it, oldest first (FSD §8.7).
SELECT e.id, e.occurred_at, e.actor_id, u.name AS actor_name, e.action, e.changes
FROM audit_events e
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.entity = 'ticket' AND e.entity_id = $1
ORDER BY e.id;
```

Append to `server/internal/db/queries/memberships.sql`:

```sql

-- name: ListAssignees :many
-- Who can own tickets: active members who are not viewers (FSD §8.1).
SELECT u.id, u.name
FROM memberships m JOIN users u ON u.id = m.user_id
WHERE m.project_id = $1 AND m.role IN ('admin', 'member') AND u.disabled_at IS NULL
ORDER BY lower(u.name), u.id;
```

In `server/internal/db/queries/projects.sql`, replace `ListProjectClients` so a member sees only the clients in their scope:

```sql
-- name: ListProjectClients :many
SELECT c.* FROM clients c
JOIN project_clients pc ON pc.client_id = c.id
WHERE pc.project_id = sqlc.arg('project_id')
  AND (sqlc.arg('all_clients')::boolean OR c.id = ANY (sqlc.arg('client_ids')::bigint[]))
ORDER BY lower(c.name), c.id;
```

In `server/internal/db/queries/contacts.sql`, replace `ListContacts`. It gains the `internal` filter, which the ticket form uses for internal people:

```sql
-- name: ListContacts :many
-- R-AC-6: a contact is visible when its client is in the user's scope in some
-- project; internal contacts (no client) are visible to every project member.
SELECT c.id, c.client_id, cl.name AS client_name, c.name, c.title, c.email, c.phone
FROM contacts c
LEFT JOIN clients cl ON cl.id = c.client_id
WHERE (sqlc.arg('is_admin')::boolean
       OR (c.client_id IS NULL AND EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = sqlc.arg('user_id')))
       OR c.client_id IN (SELECT pc.client_id
                          FROM memberships m
                          JOIN project_clients pc ON pc.project_id = m.project_id
                          WHERE m.user_id = sqlc.arg('user_id') AND m.all_clients
                          UNION ALL
                          SELECT mc.client_id FROM membership_clients mc WHERE mc.user_id = sqlc.arg('user_id')))
  AND (sqlc.narg('id')::bigint IS NULL OR c.id = sqlc.narg('id')::bigint)
  AND (sqlc.narg('client_id')::bigint IS NULL OR c.client_id = sqlc.narg('client_id')::bigint)
  AND (NOT sqlc.arg('internal')::boolean OR c.client_id IS NULL)
  AND c.name ILIKE '%' || sqlc.arg('q')::text || '%'
ORDER BY lower(c.name), c.id
LIMIT 50;
```

In `server/internal/db/queries/nodes.sql`:
- Replace `ListNodes`, so project admins can include archived nodes.
- Replace `UpdateNode`, so it can archive and restore.
- Add `CountLiveChildren`.

```sql
-- name: ListNodes :many
-- The project's tree as a flat list, siblings in position order. A
-- client-specific menu, and everything under it, is left out unless one of its
-- clients is in scope (R-AC-5); client_ids and client_names hold only in-scope
-- clients (R-MR-8). Archived nodes appear only with include_archived.
-- UNION, not UNION ALL, so a cycle could never loop forever.
WITH RECURSIVE visible AS (
  SELECT n.id FROM nodes n
  WHERE n.project_id = sqlc.arg('project_id') AND n.parent_id IS NULL
    AND (n.archived_at IS NULL OR sqlc.arg('include_archived')::boolean)
    AND (NOT n.client_specific OR sqlc.arg('all_clients')::boolean
         OR EXISTS (SELECT 1 FROM node_clients nc
                    WHERE nc.node_id = n.id AND nc.client_id = ANY (sqlc.arg('client_ids')::bigint[])))
  UNION
  SELECT n.id FROM nodes n
  JOIN visible v ON n.parent_id = v.id
  WHERE (n.archived_at IS NULL OR sqlc.arg('include_archived')::boolean)
    AND (NOT n.client_specific OR sqlc.arg('all_clients')::boolean
         OR EXISTS (SELECT 1 FROM node_clients nc
                    WHERE nc.node_id = n.id AND nc.client_id = ANY (sqlc.arg('client_ids')::bigint[])))
)
SELECT n.id, n.parent_id, n.type, n.name, n.code, n.aliases, n.description, n.client_specific, n.position,
       (n.archived_at IS NOT NULL)::boolean AS archived,
       coalesce(array_agg(c.id ORDER BY lower(c.name), c.id) FILTER (WHERE c.id IS NOT NULL), '{}')::bigint[] AS client_ids,
       coalesce(array_agg(c.name ORDER BY lower(c.name), c.id) FILTER (WHERE c.id IS NOT NULL), '{}')::text[] AS client_names
FROM visible v
JOIN nodes n ON n.id = v.id
LEFT JOIN node_clients nc ON nc.node_id = n.id
  AND (sqlc.arg('all_clients')::boolean OR nc.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
LEFT JOIN clients c ON c.id = nc.client_id
GROUP BY n.id
ORDER BY n.parent_id NULLS FIRST, n.position, n.id;
```

```sql
-- name: UpdateNode :one
-- NULL keeps a field; an empty code clears it; archived sets or clears archived_at.
UPDATE nodes SET
  name            = coalesce(sqlc.narg('name'), name),
  type            = coalesce(sqlc.narg('type'), type),
  code            = CASE WHEN sqlc.narg('code')::text IS NULL THEN code ELSE nullif(sqlc.narg('code')::text, '') END,
  aliases         = coalesce(sqlc.narg('aliases')::text[], aliases),
  description     = coalesce(sqlc.narg('description'), description),
  client_specific = coalesce(sqlc.narg('client_specific'), client_specific),
  archived_at     = CASE WHEN sqlc.narg('archived')::boolean IS NULL THEN archived_at
                         WHEN sqlc.narg('archived')::boolean THEN coalesce(archived_at, now())
                         ELSE NULL END
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: CountLiveChildren :one
SELECT count(*) FROM nodes WHERE parent_id = sqlc.arg('node_id')::bigint AND archived_at IS NULL;
```

`ListProjectClients` now takes the scope. In `server/internal/httpapi/clients.go`, make its three calls pass all clients, since only project admins call them in this task:

```go
q.ListProjectClients(ctx, db.ListProjectClientsParams{ProjectID: pc.project.ID, AllClients: true})
```

In `ListProjectClients`, the call is `s.q.ListProjectClients(r.Context(), db.ListProjectClientsParams{ProjectID: pc.project.ID, AllClients: true})`.

- [ ] **Step 4: Generate and build**

```bash
cd server && go generate ./... && go build ./... && go vet ./...
```

Expected: no output. If sqlc names a field differently from the Interfaces block, use the generated name in every later task.

- [ ] **Step 5: Write the database test**

`server/internal/db/tickets_test.go`:

```go
package db_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

// R-TK-2 and the ticket constraints the handlers turn into API problems.
func TestTicketRulesAreEnforcedByTheDatabase(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	u := must(q.CreateUser(ctx, db.CreateUserParams{Email: "u@example.com", Name: "U", Locale: "id", Timezone: "Asia/Jakarta"}))
	p := must(q.CreateProject(ctx, db.CreateProjectParams{Key: "HRIS", Name: "HRIS"}))
	pay := must(q.CreateProject(ctx, db.CreateProjectParams{Key: "PAY", Name: "PAY"}))

	statuses := must(q.ListStatuses(ctx, p.ID))
	var names []string
	for _, s := range statuses {
		names = append(names, s.Name)
	}
	if !slices.Equal(names, []string{"To do", "In progress", "In review", "Done", "Cancelled"}) || !statuses[0].IsDefault {
		t.Fatalf("default statuses: %+v", statuses)
	}
	a := must(q.CreateClient(ctx, db.CreateClientParams{Name: "Client A"}))
	b := must(q.CreateClient(ctx, db.CreateClientParams{Name: "Client B"}))
	check(q.LinkClients(ctx, db.LinkClientsParams{ProjectID: p.ID, ClientIds: []int64{a.ID}}))
	node := must(q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, Type: "menu", Name: "Leave"}))
	ticket := func(n int64, mutate func(*db.CreateTicketParams)) db.CreateTicketParams {
		params := db.CreateTicketParams{
			ProjectID: p.ID, Number: n, Key: fmt.Sprintf("HRIS-%d", n), Type: "bug", Title: "A ticket",
			StatusID: statuses[0].ID, RequesterUserID: &u.ID, ReporterID: u.ID, Priority: "medium",
		}
		if mutate != nil {
			mutate(&params)
		}
		return params
	}
	first := must(q.CreateTicket(ctx, ticket(must(q.NextTicketNumber(ctx, p.ID)), func(t *db.CreateTicketParams) { t.ClientID = &a.ID })))
	check(q.AddTicketNodes(ctx, db.AddTicketNodesParams{TicketID: first.ID, NodeIds: []int64{node.ID}}))
	if first.Number != 1 || first.Version != 1 {
		t.Fatalf("first ticket: %+v", first)
	}
	payStatus := must(q.ListStatuses(ctx, pay.ID))[0]

	for _, c := range []struct {
		constraint string
		op         func(q *db.Queries) error
	}{
		{"tickets_status_same_project", func(q *db.Queries) error {
			_, err := q.CreateTicket(ctx, ticket(2, func(t *db.CreateTicketParams) { t.StatusID = payStatus.ID }))
			return err
		}},
		{"tickets_client_linked", func(q *db.Queries) error { // B is not a client of HRIS
			_, err := q.CreateTicket(ctx, ticket(2, func(t *db.CreateTicketParams) { t.ClientID = &b.ID }))
			return err
		}},
		{"tickets_one_requester", func(q *db.Queries) error {
			_, err := q.CreateTicket(ctx, ticket(2, func(t *db.CreateTicketParams) { t.RequesterUserID = nil }))
			return err
		}},
		{"tickets_client_linked", func(q *db.Queries) error { // A is still used by a ticket
			return q.UnlinkClientsExcept(ctx, db.UnlinkClientsExceptParams{ProjectID: p.ID, ClientIds: []int64{}})
		}},
		{"tickets_status_same_project", func(q *db.Queries) error { // "To do" is still used by a ticket
			return q.DeleteStatusesExcept(ctx, db.DeleteStatusesExceptParams{ProjectID: p.ID, KeepIds: []int64{statuses[1].ID}})
		}},
		{"ticket_nodes_node_fk", func(q *db.Queries) error { return q.DeleteNode(ctx, node.ID) }},
		{"statuses_default_uq", func(q *db.Queries) error {
			_, err := q.InsertStatus(ctx, db.InsertStatusParams{ProjectID: p.ID, Name: "Backlog", Category: "todo", Position: 9, Color: "#000000", IsDefault: true})
			return err
		}},
		{"statuses_default_todo", func(q *db.Queries) error {
			_, err := q.InsertStatus(ctx, db.InsertStatusParams{ProjectID: pay.ID, Name: "Shipped", Category: "done", Position: 9, Color: "#000000", IsDefault: true})
			return err
		}},
		{"statuses_name_uq", func(q *db.Queries) error {
			_, err := q.InsertStatus(ctx, db.InsertStatusParams{ProjectID: p.ID, Name: "to do", Category: "todo", Position: 9, Color: "#000000"})
			return err
		}},
	} {
		tx, err := d.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = c.op(q.WithTx(tx))
		_ = tx.Rollback(ctx)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != c.constraint {
			t.Errorf("want a %s violation, got %v", c.constraint, err)
		}
	}
}
```

- [ ] **Step 6: Run the tests**

```bash
cd server && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...
```

Expected: `ok` for every package. The new test passes as soon as the migration is right, because this task adds data rules, not handlers. If a constraint case fails, fix the migration, not the test.

- [ ] **Step 7: Commit**

```bash
git add server
git commit -m "feat(server): ticket schema with default statuses and scoped queries"
```

### Task 2: Statuses, assignees, scoped project clients and the fixed key

**Files:**
- Modify: `api/openapi.yaml` (paths `/projects/{key}/statuses`, `/projects/{key}/assignees`; schemas `Ref`, `RefList`, `StatusCategory`, `Status`, `StatusList`, `StatusInput`, `StatusMove`, `StatusesUpdate`)
- Modify: `server/internal/access/access.go` (`Scope.Sees`), `server/internal/config/config.go` (attachment settings)
- Create: `server/internal/httpapi/statuses.go`
- Modify: `server/internal/httpapi/clients.go` (`ListProjectClients` for members, scoped), `server/internal/httpapi/projects.go` (fixed key)
- Modify: `server/internal/httpapi/seed_test.go` (`seedTicket`), `server/internal/httpapi/permission_test.go` (scoped client lists)
- Test: `server/internal/access/access_test.go`, `server/internal/config/config_test.go`, `server/internal/httpapi/statuses_test.go`

**Interfaces:**
- Consumes: Task 1's queries; `projectFor`, `changed`, `constraintOf`, `deref`, `orEmpty` (Iteration 1).
- Produces:
  - `(access.Scope) Sees(clientID *int64) bool`.
  - `config.Config` gains `AttachmentsDir string` (env `ATTACHMENTS_DIR`, default `/data/attachments`) and `AttachmentMaxBytes int64` (25 MiB).
  - Handlers `GetStatuses`, `SetStatuses`, `ListAssignees` (all `(w, r, key string)`).
  - API types `Ref{Id int64; Name string}`, `RefList`, `Status{Id int64; Name string; Category StatusCategory; Color string; Position int32; IsDefault bool}`.
  - `GET /projects/{key}/clients` now serves every member, limited to their scope.
  - `(e *env) seedTicket(p db.Project, reporter db.User, title string, client *db.Client, nodes ...db.Node) db.Ticket`.
  - Error codes: `status_in_use` (409), `status_name_taken` (409, and as a field code), `project_key_fixed` (field code).

- [ ] **Step 1: Extend the contract**

`api/openapi.yaml`, under `paths:`:

```yaml
  /projects/{key}/statuses:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    get:
      operationId: getStatuses
      tags: [projects]
      description: Every member; the board, the list and the ticket page need them.
      responses:
        "200":
          description: The project's statuses in order.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/StatusList" }
        default: { $ref: "#/components/responses/Problem" }
    put:
      operationId: setStatuses
      tags: [projects]
      description: Project admins only. Replaces the ordered list; tickets of a removed status move as `move_to` says.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/StatusesUpdate" }
      responses:
        "200":
          description: The statuses now.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/StatusList" }
        default: { $ref: "#/components/responses/Problem" }
  /projects/{key}/assignees:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    get:
      operationId: listAssignees
      tags: [projects]
      description: Members who can own tickets (not viewers).
      responses:
        "200":
          description: Assignable members by name.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/RefList" }
        default: { $ref: "#/components/responses/Problem" }
```

In the existing `/projects/{key}/clients` `get`, change the description to `Every member; members see only the clients in their scope.`

Under `components.schemas:`:

```yaml
    Ref:
      type: object
      required: [id, name]
      properties:
        id: { type: integer, format: int64 }
        name: { type: string }
    RefList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/Ref" }
    StatusCategory:
      type: string
      enum: [todo, in_progress, done, cancelled]
    Status:
      type: object
      required: [id, name, category, color, position, is_default]
      properties:
        id: { type: integer, format: int64 }
        name: { type: string }
        category: { $ref: "#/components/schemas/StatusCategory" }
        color: { type: string, example: "#2563EB" }
        position: { type: integer, format: int32 }
        is_default: { type: boolean }
    StatusList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/Status" }
    StatusInput:
      type: object
      required: [name, category, color]
      properties:
        id: { type: integer, format: int64, description: An existing status; omitted adds one. }
        name: { type: string, maxLength: 50 }
        category: { $ref: "#/components/schemas/StatusCategory" }
        color: { type: string, example: "#2563EB" }
        is_default: { type: boolean }
    StatusMove:
      type: object
      required: [from, to]
      properties:
        from: { type: integer, format: int64, description: A removed status. }
        to: { type: integer, format: int64, description: A kept status of the same open or closed kind. }
    StatusesUpdate:
      type: object
      required: [statuses]
      properties:
        statuses:
          type: array
          items: { $ref: "#/components/schemas/StatusInput" }
        move_to:
          type: array
          items: { $ref: "#/components/schemas/StatusMove" }
```

- [ ] **Step 2: Add `Scope.Sees` and the attachment settings**

In `server/internal/access/access.go`, add `"slices"` to the imports and this method after `Allows`:

```go
// Sees reports whether the scope covers a row of the client; a nil client is
// core work, which every member sees (R-AC-2, R-AC-3).
func (s Scope) Sees(clientID *int64) bool {
	return s.AllClients || clientID == nil || slices.Contains(s.ClientIDs, *clientID)
}
```

In `server/internal/config/config.go`, add two fields to `Config`:

```go
	AttachmentsDir     string // ATTACHMENTS_DIR, default /data/attachments
	AttachmentMaxBytes int64  // 25 MB per file (FSD §8.7); the admin setting comes later
```

In `Load`, set them right after `ListenAddr`:

```go
		AttachmentsDir:     getenv("ATTACHMENTS_DIR"),
		AttachmentMaxBytes: 25 << 20,
```

After the `ListenAddr` default, add:

```go
	if c.AttachmentsDir == "" {
		c.AttachmentsDir = "/data/attachments"
	}
```

- [ ] **Step 3: Write the failing tests**

Append to `server/internal/access/access_test.go`:

```go

func TestScopeSeesItsClientsAndCoreWork(t *testing.T) {
	a, b := int64(1), int64(2)
	scoped := access.Scope{Role: access.Member, ClientIDs: []int64{a}}
	all := access.Scope{Role: access.Member, AllClients: true}
	for _, c := range []struct {
		scope  access.Scope
		client *int64
		sees   bool
	}{
		{scoped, nil, true}, {scoped, &a, true}, {scoped, &b, false}, {all, &b, true},
	} {
		if got := c.scope.Sees(c.client); got != c.sees {
			t.Errorf("%+v sees %v: %v, want %v", c.scope, c.client, got, c.sees)
		}
	}
}
```

Append to `server/internal/config/config_test.go`:

```go

func TestAttachmentsDefaultToTheDataVolume(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x", "PUBLIC_URL": "http://localhost"}))
	if err != nil || c.AttachmentsDir != "/data/attachments" || c.AttachmentMaxBytes != 25<<20 {
		t.Fatalf("attachments: %q %d %v", c.AttachmentsDir, c.AttachmentMaxBytes, err)
	}
	c, _ = Load(env(map[string]string{"DATABASE_URL": "postgres://x", "PUBLIC_URL": "http://localhost", "ATTACHMENTS_DIR": "/srv/files"}))
	if c.AttachmentsDir != "/srv/files" {
		t.Fatalf("ATTACHMENTS_DIR: %q", c.AttachmentsDir)
	}
}
```

Append to `server/internal/httpapi/seed_test.go` (add `"fmt"` to its imports):

```go

// seedTicket files a ticket in the project's default status, requested by its reporter.
func (e *env) seedTicket(p db.Project, reporter db.User, title string, client *db.Client, nodes ...db.Node) db.Ticket {
	e.t.Helper()
	ctx := context.Background()
	status, err := e.q.GetDefaultStatus(ctx, p.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	n, err := e.q.NextTicketNumber(ctx, p.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	params := db.CreateTicketParams{
		ProjectID: p.ID, Number: n, Key: fmt.Sprintf("%s-%d", p.Key, n), Type: "change_request", Title: title,
		StatusID: status.ID, RequesterUserID: &reporter.ID, ReporterID: reporter.ID, Priority: "medium",
	}
	if client != nil {
		params.ClientID = &client.ID
	}
	t, err := e.q.CreateTicket(ctx, params)
	if err != nil {
		e.t.Fatal(err)
	}
	ids := make([]int64, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	if err := e.q.AddTicketNodes(ctx, db.AddTicketNodesParams{TicketID: t.ID, NodeIds: ids}); err != nil {
		e.t.Fatal(err)
	}
	return t
}
```

`server/internal/httpapi/statuses_test.go`:

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

func statusNames(list httpapi.StatusList) []string {
	names := make([]string, len(list.Items))
	for i, s := range list.Items {
		names[i] = s.Name
	}
	return names
}

func statusInput(s httpapi.Status) map[string]any {
	return map[string]any{"id": s.Id, "name": s.Name, "category": s.Category, "color": s.Color, "is_default": s.IsDefault}
}

func TestNewProjectsStartWithTheDefaultStatuses(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.signedIn("admin@example.com", true)
	e.call(admin, http.MethodPost, "/projects", map[string]any{"key": "HRIS", "name": "HRIS"}, nil)
	var list httpapi.StatusList
	if code := e.call(admin, http.MethodGet, "/projects/HRIS/statuses", nil, &list); code != http.StatusOK ||
		!slices.Equal(statusNames(list), []string{"To do", "In progress", "In review", "Done", "Cancelled"}) || !list.Items[0].IsDefault {
		t.Fatalf("statuses: %d %+v", code, list)
	}
}

func TestAdminEditsStatusesAndMovesTicketsOffRemovedOnes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.seedProject("HRIS")
	admin, au := e.signedIn("admin@example.com", true)
	var list httpapi.StatusList
	e.call(admin, http.MethodGet, "/projects/HRIS/statuses", nil, &list)
	todo, prog, review, done, cancelled := list.Items[0], list.Items[1], list.Items[2], list.Items[3], list.Items[4]
	tk := e.seedTicket(p, au, "Waiting for review", nil)
	if _, err := e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: tk.ID, StatusID: review.Id}); err != nil {
		t.Fatal(err)
	}
	var prob httpapi.Problem
	body := map[string]any{"statuses": []map[string]any{statusInput(todo), statusInput(prog), statusInput(done), statusInput(cancelled)}}
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/statuses", body, &prob); code != http.StatusConflict || prob.Code != "status_in_use" {
		t.Fatalf("remove a status in use: %d %+v", code, prob)
	}
	shipped := statusInput(done)
	shipped["name"] = "Shipped"
	body = map[string]any{
		"statuses": []map[string]any{statusInput(todo), {"name": "Blocked", "category": "in_progress", "color": "#DC2626"},
			statusInput(prog), shipped, statusInput(cancelled)},
		"move_to": []map[string]any{{"from": review.Id, "to": prog.Id}},
	}
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/statuses", body, &list); code != http.StatusOK ||
		!slices.Equal(statusNames(list), []string{"To do", "Blocked", "In progress", "Shipped", "Cancelled"}) {
		t.Fatalf("update: %d %+v", code, list)
	}
	moved, err := e.q.GetTicketByKey(ctx, tk.Key)
	if err != nil || moved.Ticket.StatusID != prog.Id || moved.Ticket.Version != 2 {
		t.Fatalf("ticket after the move: %+v %v", moved.Ticket, err)
	}
}

func TestStatusRules(t *testing.T) {
	e := newEnv(t)
	e.seedProject("HRIS")
	admin, _ := e.signedIn("admin@example.com", true)
	var list httpapi.StatusList
	e.call(admin, http.MethodGet, "/projects/HRIS/statuses", nil, &list)
	rows := func() []map[string]any {
		out := make([]map[string]any, len(list.Items))
		for i, s := range list.Items {
			out[i] = statusInput(s)
		}
		return out
	}
	noDefault, doneDefault, noCancelled, twins, badColor := rows(), rows(), rows()[:4], rows(), rows()
	noDefault[0]["is_default"] = false
	doneDefault[0]["is_default"], doneDefault[3]["is_default"] = false, true
	twins[1]["name"] = "to do"
	badColor[2]["color"] = "blue"
	for _, c := range []struct {
		body        map[string]any
		field, code string
	}{
		{map[string]any{"statuses": noDefault}, "statuses", "invalid"},
		{map[string]any{"statuses": doneDefault}, "statuses[3].is_default", "invalid"},
		{map[string]any{"statuses": noCancelled}, "statuses", "invalid"},
		{map[string]any{"statuses": twins}, "statuses[1].name", "status_name_taken"},
		{map[string]any{"statuses": badColor}, "statuses[2].color", "invalid"},
		{map[string]any{"statuses": append(rows()[:2], rows()[3:]...), // In review goes; its tickets may not land in Done
			"move_to": []map[string]any{{"from": list.Items[2].Id, "to": list.Items[3].Id}}}, "move_to[0]", "invalid"},
	} {
		var prob httpapi.Problem
		code := e.call(admin, http.MethodPut, "/projects/HRIS/statuses", c.body, &prob)
		if f := firstError(prob); code != http.StatusUnprocessableEntity || f.Field != c.field || f.Code != c.code {
			t.Errorf("%s: %d %+v", c.field, code, prob)
		}
	}
}

func TestMembersReadStatusesButOnlyAdminsEditThem(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	viewer, vu := e.signedIn("viewer@example.com", false)
	e.seedMember(vu, p, "viewer")
	var list httpapi.StatusList
	if code := e.call(viewer, http.MethodGet, "/projects/HRIS/statuses", nil, &list); code != http.StatusOK || len(list.Items) != 5 {
		t.Fatalf("viewer reads: %d", code)
	}
	body := map[string]any{"statuses": []map[string]any{statusInput(list.Items[0])}}
	if code := e.call(viewer, http.MethodPut, "/projects/HRIS/statuses", body, nil); code != http.StatusForbidden {
		t.Fatalf("viewer edits: %d", code)
	}
}

func TestProjectKeyIsFixedOnceTheProjectHasTickets(t *testing.T) {
	e := newEnv(t)
	e.seedProject("HRIS")
	admin, au := e.signedIn("admin@example.com", true)
	if code := e.call(admin, http.MethodPatch, "/projects/HRIS", map[string]any{"key": "HR"}, nil); code != http.StatusOK {
		t.Fatalf("rename before tickets: %d", code)
	}
	p, err := e.q.GetProjectByKey(context.Background(), "HR")
	if err != nil {
		t.Fatal(err)
	}
	e.seedTicket(p, au, "First ticket", nil)
	var prob httpapi.Problem
	if code := e.call(admin, http.MethodPatch, "/projects/HR", map[string]any{"key": "HX"}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Code != "project_key_fixed" {
		t.Fatalf("rename after a ticket: %d %+v", code, prob)
	}
	if code := e.call(admin, http.MethodPatch, "/projects/HR", map[string]any{"key": "hr", "name": "HR System"}, nil); code != http.StatusOK {
		t.Fatalf("same key, new name: %d", code)
	}
}

func TestMembersSeeProjectClientsInTheirScope(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	p := e.seedProject("HRIS", a, b)
	budi, bu := e.signedIn("budi@example.com", false)
	e.seedMember(bu, p, "member", b)
	var list httpapi.ClientList
	if code := e.call(budi, http.MethodGet, "/projects/HRIS/clients", nil, &list); code != http.StatusOK || len(list.Items) != 1 || list.Items[0].Name != "Client B" {
		t.Fatalf("scoped member: %d %+v", code, list)
	}
}

func TestAssigneesAreMembersWhoAreNotViewers(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	owner, ou := e.signedIn("owner@example.com", false)
	e.seedMember(ou, p, "admin")
	e.seedMember(e.seedUser("budi@example.com", pw, false), p, "member")
	e.seedMember(e.seedUser("vera@example.com", pw, false), p, "viewer")
	var list httpapi.RefList
	e.call(owner, http.MethodGet, "/projects/HRIS/assignees", nil, &list)
	var names []string
	for _, u := range list.Items {
		names = append(names, u.Name)
	}
	if !slices.Equal(names, []string{"budi@example.com", "owner@example.com"}) {
		t.Fatalf("assignees: %v", names)
	}
}
```

In `server/internal/httpapi/permission_test.go`, members now read the client list of their project, limited to their scope. Replace the `"/projects/HRIS/clients"` row of `TestPermissionSuiteReads` with:

```go
		{"/projects/HRIS/clients", map[string][]string{
			"admin": abc, "hana": abc, "ani": abc, "budi": {"Client B"}, "citra": {"Client A", "Client C"},
		}, map[string]int{"dodi": 404}},
```

- [ ] **Step 4: Run the tests to verify they fail**

```bash
cd server && go generate ./... && go vet ./... 2>&1 | head -5
```

Expected: FAIL. `*Server does not implement ServerInterface (missing method GetStatuses)`.

- [ ] **Step 5: Write the handlers**

`server/internal/httpapi/statuses.go`:

```go
package httpapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

var colorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func (s *Server) GetStatuses(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.q.ListStatuses(r.Context(), pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, StatusList{Items: toAPIStatuses(rows)})
}

// SetStatuses replaces the project's ordered statuses (R-TK-1, R-TK-2). Tickets
// of a removed status move as move_to says, in the same transaction (R-TK-4).
func (s *Server) SetStatuses(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in StatusesUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	before, err := s.q.ListStatuses(ctx, pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if fields := validateStatuses(in, before); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var rows []db.Status
	err = s.inTx(ctx, func(q *db.Queries) error {
		if err := q.ClearDefaultStatus(ctx, pc.project.ID); err != nil {
			return err
		}
		keep := make([]int64, 0, len(in.Statuses))
		for i, st := range in.Statuses {
			name, def := strings.TrimSpace(st.Name), deref(st.IsDefault)
			if st.Id != nil {
				if _, err := q.UpdateStatus(ctx, db.UpdateStatusParams{
					ID: *st.Id, ProjectID: pc.project.ID, Name: name, Category: string(st.Category), Position: int32(i), Color: st.Color, IsDefault: def,
				}); err != nil {
					return err
				}
				keep = append(keep, *st.Id)
				continue
			}
			id, err := q.InsertStatus(ctx, db.InsertStatusParams{
				ProjectID: pc.project.ID, Name: name, Category: string(st.Category), Position: int32(i), Color: st.Color, IsDefault: def,
			})
			if err != nil {
				return err
			}
			keep = append(keep, id)
		}
		for _, m := range deref(in.MoveTo) {
			if err := q.MoveTicketsToStatus(ctx, db.MoveTicketsToStatusParams{ProjectID: pc.project.ID, FromID: m.From, ToID: m.To}); err != nil {
				return err
			}
		}
		if err := q.DeleteStatusesExcept(ctx, db.DeleteStatusesExceptParams{ProjectID: pc.project.ID, KeepIds: keep}); err != nil {
			return err
		}
		var err error
		if rows, err = q.ListStatuses(ctx, pc.project.ID); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "project", pc.project.ID, "set_statuses",
			changed(map[string]any{"statuses": statusAudit(before)}, map[string]any{"statuses": statusAudit(rows)}))
	})
	switch constraintOf(err) {
	case "tickets_status_same_project":
		writeProblem(w, http.StatusConflict, "status_in_use", "Tickets still use a status you removed; choose where they move")
		return
	case "statuses_name_uq":
		writeProblem(w, http.StatusConflict, "status_name_taken", "Two statuses have the same name")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, StatusList{Items: toAPIStatuses(rows)})
}

// validateStatuses checks R-TK-2 and the moves of R-TK-4 against the current list.
func validateStatuses(in StatusesUpdate, before []db.Status) []FieldError {
	var f []FieldError
	old := map[int64]db.Status{}
	for _, st := range before {
		old[st.ID] = st
	}
	names := map[string]bool{}
	kept := map[int64]StatusCategory{}
	defaults, done, cancelled := 0, 0, 0
	for i, st := range in.Statuses {
		at := fmt.Sprintf("statuses[%d].", i)
		name := strings.ToLower(strings.TrimSpace(st.Name))
		switch {
		case name == "" || len(name) > 50:
			f = append(f, FieldError{Field: at + "name", Code: "required", Message: "Enter a name of at most 50 characters"})
		case names[name]:
			f = append(f, FieldError{Field: at + "name", Code: "status_name_taken", Message: "Two statuses have this name"})
		}
		names[name] = true
		if !st.Category.Valid() {
			f = append(f, FieldError{Field: at + "category", Code: "invalid", Message: "Choose to do, in progress, done or cancelled"})
		}
		if !colorRe.MatchString(st.Color) {
			f = append(f, FieldError{Field: at + "color", Code: "invalid", Message: "Use a color like #2563EB"})
		}
		if st.Id != nil {
			if _, ok := old[*st.Id]; !ok {
				f = append(f, FieldError{Field: at + "id", Code: "invalid", Message: "Unknown status"})
			}
			kept[*st.Id] = st.Category
		}
		if deref(st.IsDefault) {
			defaults++
			if st.Category != StatusCategoryTodo {
				f = append(f, FieldError{Field: at + "is_default", Code: "invalid", Message: "The default status is a To do status"})
			}
		}
		switch st.Category {
		case StatusCategoryDone:
			done++
		case StatusCategoryCancelled:
			cancelled++
		}
	}
	if defaults != 1 {
		f = append(f, FieldError{Field: "statuses", Code: "invalid", Message: "Choose exactly one default status"})
	}
	if done == 0 || cancelled == 0 {
		f = append(f, FieldError{Field: "statuses", Code: "invalid", Message: "Keep at least one Done and one Cancelled status"})
	}
	for i, m := range deref(in.MoveTo) {
		from, existed := old[m.From]
		_, stays := kept[m.From]
		to, isKept := kept[m.To]
		if !existed || stays || !isKept || closedCategory(from.Category) != closedCategory(string(to)) {
			f = append(f, FieldError{Field: fmt.Sprintf("move_to[%d]", i), Code: "invalid", Message: "Move tickets from a removed status to a kept one of the same kind"})
		}
	}
	return f
}

// closedCategory reports whether a status category closes tickets (R-TK-3).
func closedCategory(c string) bool { return c == string(StatusCategoryDone) || c == string(StatusCategoryCancelled) }

// ListAssignees lists who can own the project's tickets.
func (s *Server) ListAssignees(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.q.ListAssignees(r.Context(), pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]Ref, len(rows))
	for i, u := range rows {
		items[i] = Ref{Id: u.ID, Name: u.Name}
	}
	writeJSON(w, http.StatusOK, RefList{Items: items})
}

func statusAudit(rows []db.Status) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, st := range rows {
		out[i] = map[string]any{"name": st.Name, "category": st.Category, "color": st.Color, "default": st.IsDefault}
	}
	return out
}

func toAPIStatus(st db.Status) Status {
	return Status{Id: st.ID, Name: st.Name, Category: StatusCategory(st.Category), Color: st.Color, Position: st.Position, IsDefault: st.IsDefault}
}

func toAPIStatuses(rows []db.Status) []Status {
	items := make([]Status, len(rows))
	for i, st := range rows {
		items[i] = toAPIStatus(st)
	}
	return items
}
```

In `server/internal/httpapi/clients.go`, `ListProjectClients` now serves every member, limited to their scope:

```go
func (s *Server) ListProjectClients(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	// Members pick from these on the ticket form, so they see their scope only (AC-TK-4).
	rows, err := s.q.ListProjectClients(r.Context(), db.ListProjectClientsParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ClientList{Items: toAPIClients(rows)})
}
```

In `server/internal/httpapi/projects.go`, `UpdateProject` refuses a new key once the project has tickets. Put this right after the `validateProject` check:

```go
	if in.Key != nil && *in.Key != pc.project.Key && pc.project.TicketSeq > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "key", Code: "project_key_fixed", Message: "The key cannot change once the project has tickets"})
		return
	}
```

- [ ] **Step 6: Run the tests**

```bash
cd server && go generate ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...
cd ../web && npm run gen:api
```

Expected: `ok` for every Go package, including the updated permission suite.

- [ ] **Step 7: Commit**

```bash
git add api/openapi.yaml server web/lib/api-types.ts
git commit -m "feat(server): project statuses, assignees and scoped client lists"
```

### Task 3: Create, read, update and move tickets

**Files:**
- Modify: `api/openapi.yaml`:
  - paths: `POST /projects/{key}/tickets`, `/tickets/{key}` (GET, PUT), `POST /tickets/{key}/transition`;
  - schemas: `TicketType`, `Priority`, `TicketRequester`, `NodeRef`, `Attachment`, `Ticket`, `TicketCreate`, `TicketUpdate`, `TransitionRequest`.
- Create: `server/internal/httpapi/tickets.go`
- Test: `server/internal/httpapi/tickets_test.go`

**Interfaces:**
- Consumes: Task 1's ticket queries; `Scope.Sees`, `seedTicket`, `closedCategory`, `toAPIStatus` (Task 2); `projectFor`, `memberOf` (Iteration 1).
- Produces:
  - Handlers: `CreateTicket(w, r, key)`, `GetTicket(w, r, key)`, `UpdateTicket(w, r, key, params UpdateTicketParams)`, `TransitionTicket(w, r, key)`.
  - Helpers for later tasks:
    - `(s *Server) ticketFor(w, r, key, need string) (projectCtx, db.GetTicketByKeyRow, bool)` answers 404 for a missing or hidden ticket and 403 for a role below `need`.
    - `readTicket(ctx, q *db.Queries, key string) (Ticket, error)` and `ticketFromRow(ctx, q, row) (Ticket, error)`.
    - `toAPIAttachment(a db.Attachment, uploader string) Attachment`.
    - `etag(version int32) string`.
    - `(e *env) callWith(c, method, path string, headers map[string]string, body, out any) (int, http.Header)`.
    - `newHRIS(e *env) hrisWorld` (test world).
  - Error codes: `stale` (412) and `close_unavailable` (422).
  - Audit actions `create`, `update`, `transition` on entity `ticket`.

- [ ] **Step 1: Extend the contract**

`api/openapi.yaml`, under `paths:`:

```yaml
  /projects/{key}/tickets:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    post:
      operationId: createTicket
      tags: [tickets]
      description: Members and project admins. The ticket starts in the default status unless an open status_id is given.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/TicketCreate" }
      responses:
        "201":
          description: The new ticket; ETag carries its version.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Ticket" }
        default: { $ref: "#/components/responses/Problem" }
  /tickets/{key}:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string, example: HRIS-231 } }
    get:
      operationId: getTicket
      tags: [tickets]
      responses:
        "200":
          description: The ticket; ETag carries its version.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Ticket" }
        default: { $ref: "#/components/responses/Problem" }
    put:
      operationId: updateTicket
      tags: [tickets]
      description: Members and project admins. Replaces every editable field; a stale If-Match answers 412.
      parameters:
        - { name: If-Match, in: header, required: true, schema: { type: string, example: '"7"' } }
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/TicketUpdate" }
      responses:
        "200":
          description: The updated ticket; ETag carries its new version.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Ticket" }
        default: { $ref: "#/components/responses/Problem" }
  /tickets/{key}/transition:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    post:
      operationId: transitionTicket
      tags: [tickets]
      description: Members and project admins. Moves among open statuses; closing answers 422 close_unavailable until the close dialog ships.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/TransitionRequest" }
      responses:
        "200":
          description: The ticket in its new status.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Ticket" }
        default: { $ref: "#/components/responses/Problem" }
```

Under `components.schemas:`:

```yaml
    TicketType:
      type: string
      enum: [bug, change_request, feature]
    Priority:
      type: string
      enum: [low, medium, high, urgent]
    TicketRequester:
      type: object
      required: [kind, id, name, title]
      properties:
        kind: { type: string, enum: [contact, user] }
        id: { type: integer, format: int64 }
        name: { type: string }
        title: { type: string, nullable: true, example: HR Manager }
    NodeRef:
      type: object
      required: [id, name, archived]
      properties:
        id: { type: integer, format: int64 }
        name: { type: string }
        archived: { type: boolean }
    Attachment:
      type: object
      required: [id, filename, content_type, size_bytes, uploader, created_at]
      properties:
        id: { type: integer, format: int64 }
        filename: { type: string }
        content_type: { type: string }
        size_bytes: { type: integer, format: int64 }
        uploader: { $ref: "#/components/schemas/Ref" }
        created_at: { type: string, format: date-time }
    Ticket:
      type: object
      required: [id, key, project_key, title, type, description, reason, status, requester, reporter, priority, due_date, nodes, attachments, version, created_at, updated_at]
      properties:
        id: { type: integer, format: int64 }
        key: { type: string, example: HRIS-231 }
        project_key: { type: string }
        title: { type: string }
        type: { $ref: "#/components/schemas/TicketType" }
        description: { type: string }
        reason: { type: string }
        status: { $ref: "#/components/schemas/Status" }
        client: { $ref: "#/components/schemas/Ref" }
        requester: { $ref: "#/components/schemas/TicketRequester" }
        reporter: { $ref: "#/components/schemas/Ref" }
        assignee: { $ref: "#/components/schemas/Ref" }
        priority: { $ref: "#/components/schemas/Priority" }
        due_date: { type: string, format: date, nullable: true }
        nodes:
          type: array
          items: { $ref: "#/components/schemas/NodeRef" }
        attachments:
          type: array
          items: { $ref: "#/components/schemas/Attachment" }
        version: { type: integer, format: int32 }
        created_at: { type: string, format: date-time }
        updated_at: { type: string, format: date-time }
    TicketCreate:
      type: object
      required: [type, title, node_ids]
      properties:
        type: { $ref: "#/components/schemas/TicketType" }
        title: { type: string, maxLength: 200 }
        client_id: { type: integer, format: int64, description: Omitted for core work (all clients). }
        requester_contact_id: { type: integer, format: int64 }
        requester_user_id: { type: integer, format: int64, description: Without a requester the reporter is the requester. }
        node_ids:
          type: array
          items: { type: integer, format: int64 }
        reason: { type: string, maxLength: 2000 }
        description: { type: string, maxLength: 50000 }
        assignee_id: { type: integer, format: int64 }
        priority: { $ref: "#/components/schemas/Priority" }
        due_date: { type: string, format: date }
        status_id: { type: integer, format: int64, description: An open status; omitted means the project's default. }
    TicketUpdate:
      type: object
      description: Replaces every editable field; send the current value of each field you keep.
      required: [type, title, node_ids]
      properties:
        type: { $ref: "#/components/schemas/TicketType" }
        title: { type: string, maxLength: 200 }
        client_id: { type: integer, format: int64, description: Omitted for core work (all clients). }
        requester_contact_id: { type: integer, format: int64 }
        requester_user_id: { type: integer, format: int64, description: One of the two requester fields is required. }
        node_ids:
          type: array
          items: { type: integer, format: int64 }
        reason: { type: string, maxLength: 2000 }
        description: { type: string, maxLength: 50000 }
        assignee_id: { type: integer, format: int64 }
        priority: { $ref: "#/components/schemas/Priority" }
        due_date: { type: string, format: date }
    TransitionRequest:
      type: object
      required: [status_id]
      properties:
        status_id: { type: integer, format: int64 }
```

- [ ] **Step 2: Write the failing tests**

`server/internal/httpapi/tickets_test.go`:

```go
package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// callWith sends a request with extra headers and returns the status and the response headers.
func (e *env) callWith(c *http.Client, method, path string, headers map[string]string, body, out any) (int, http.Header) {
	e.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, e.url+"/api/v1"+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode, res.Header
}

// hrisWorld is project HRIS with two clients, a small tree and a PM scoped to Client A.
type hrisWorld struct {
	p      db.Project
	a, b   db.Client
	hr, ot db.Node // HR › Overtime Approval, a Client A menu
	secret db.Node // HR › Client B Report, hidden from the PM
	budi   int64   // Budi (HR Manager), a contact of Client A
	pm     *http.Client
	pmUser db.User
}

func newHRIS(e *env) hrisWorld {
	w := hrisWorld{a: e.seedClient("Client A"), b: e.seedClient("Client B")}
	w.p = e.seedProject("HRIS", w.a, w.b)
	w.hr = e.seedNode(w.p, nil, "module", "HR")
	w.ot = e.seedNode(w.p, &w.hr, "menu", "Overtime Approval", w.a)
	w.secret = e.seedNode(w.p, &w.hr, "menu", "Client B Report", w.b)
	title := "HR Manager"
	id, err := e.q.CreateContact(context.Background(), db.CreateContactParams{ClientID: &w.a.ID, Name: "Budi", Title: &title})
	if err != nil {
		e.t.Fatal(err)
	}
	w.budi = id
	w.pm, w.pmUser = e.signedIn("pm@example.com", false)
	e.seedMember(w.pmUser, w.p, "member", w.a)
	return w
}

// The Iteration 2 exit check at API level (story 3, AC-TK-3).
func TestPMLogsAClientRequestInOneRequest(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	reason := "Client A supervisors are often on leave; HR approves overtime directly."
	body := map[string]any{
		"client_id": w.a.ID, "requester_contact_id": w.budi, "title": "Skip supervisor approval for overtime",
		"node_ids": []int64{w.ot.ID}, "reason": reason, "type": "change_request",
	}
	var tk httpapi.Ticket
	code, h := e.callWith(w.pm, http.MethodPost, "/projects/HRIS/tickets", nil, body, &tk)
	if code != http.StatusCreated || tk.Key != "HRIS-1" || tk.Status.Name != "To do" || !tk.Status.IsDefault ||
		tk.Client == nil || tk.Client.Name != "Client A" || tk.Requester.Kind != httpapi.TicketRequesterKindContact ||
		tk.Requester.Name != "Budi" || tk.Requester.Title == nil || *tk.Requester.Title != "HR Manager" ||
		len(tk.Nodes) != 1 || tk.Nodes[0].Name != "Overtime Approval" || tk.Reporter.Name != "pm@example.com" ||
		tk.Priority != httpapi.PriorityMedium || h.Get("ETag") != `"1"` {
		t.Fatalf("create: %d %+v %q", code, tk, h.Get("ETag"))
	}
	var got httpapi.Ticket
	if code := e.call(w.pm, http.MethodGet, "/tickets/hris-1", nil, &got); code != http.StatusOK || got.Reason != reason {
		t.Fatalf("read: %d %+v", code, got)
	}
	events, err := e.q.ListTicketEvents(context.Background(), tk.Id)
	if err != nil || len(events) != 1 || events[0].Action != "create" {
		t.Fatalf("history: %+v %v", events, err)
	}
}

func TestTicketFieldRules(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	bayu := e.seedContact("Bayu", &w.b)
	viewer := e.seedUser("vera@example.com", pw, false)
	e.seedMember(viewer, w.p, "viewer")
	var statuses httpapi.StatusList
	e.call(w.pm, http.MethodGet, "/projects/HRIS/statuses", nil, &statuses)
	base := map[string]any{"title": "A proper title", "type": "bug", "node_ids": []int64{}}
	with := func(k string, v any) map[string]any {
		m := map[string]any{k: v}
		for kk, vv := range base {
			if kk != k {
				m[kk] = vv
			}
		}
		return m
	}
	for _, c := range []struct {
		body        map[string]any
		field, code string
	}{
		{with("title", "Hi"), "title", "invalid"},
		{with("type", "question"), "type", "invalid"},
		{with("client_id", w.b.ID), "client_id", "invalid"},                     // outside the PM's scope (AC-TK-4)
		{with("requester_contact_id", bayu), "requester_contact_id", "invalid"}, // a contact the PM cannot see
		{with("requester_contact_id", w.budi), "requester_contact_id", "invalid"}, // a Client A contact on core work
		{with("node_ids", []int64{w.secret.ID}), "node_ids", "invalid"},         // a menu hidden from the PM
		{with("assignee_id", viewer.ID), "assignee_id", "invalid"},              // viewers do not own tickets
		{with("status_id", statuses.Items[3].Id), "status_id", "invalid"},       // Done: tickets start open
	} {
		var p httpapi.Problem
		code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", c.body, &p)
		if f := firstError(p); code != http.StatusUnprocessableEntity || f.Field != c.field || f.Code != c.code {
			t.Errorf("%s: %d %+v", c.field, code, p)
		}
	}
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", base, nil); code != http.StatusCreated {
		t.Fatalf("core work by a scoped member (AC-TK-4): %d", code)
	}
}

func TestStaleSavesAreRefused(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	path := "/tickets/" + tk.Key
	edit := map[string]any{"title": "Overtime export for payroll", "type": "feature", "client_id": w.a.ID,
		"requester_user_id": w.pmUser.ID, "node_ids": []int64{w.ot.ID}}
	var out httpapi.Ticket
	code, h := e.callWith(w.pm, http.MethodPut, path, map[string]string{"If-Match": `"1"`}, edit, &out)
	if code != http.StatusOK || out.Version != 2 || h.Get("ETag") != `"2"` || out.Type != httpapi.TicketTypeFeature {
		t.Fatalf("first save: %d %+v", code, out)
	}
	var p httpapi.Problem
	if code, _ := e.callWith(w.pm, http.MethodPut, path, map[string]string{"If-Match": `"1"`}, edit, &p); code != http.StatusPreconditionFailed || p.Code != "stale" {
		t.Fatalf("stale save (AC-TK-5): %d %+v", code, p)
	}
	if code := e.call(w.pm, http.MethodPut, path, edit, nil); code != http.StatusBadRequest {
		t.Fatalf("no If-Match: %d", code)
	}
	delete(edit, "requester_user_id")
	if code, _ := e.callWith(w.pm, http.MethodPut, path, map[string]string{"If-Match": `"2"`}, edit, &p); code != http.StatusUnprocessableEntity || firstError(p).Field != "requester_contact_id" {
		t.Fatalf("a replacement names the requester: %d %+v", code, p)
	}
}

func TestUpdatesRecordOldAndNewValues(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	edit := map[string]any{"title": "Overtime export for payroll", "type": "change_request", "client_id": w.a.ID,
		"requester_user_id": w.pmUser.ID, "node_ids": []int64{}}
	if code, _ := e.callWith(w.pm, http.MethodPut, "/tickets/"+tk.Key, map[string]string{"If-Match": `"1"`}, edit, nil); code != http.StatusOK {
		t.Fatalf("save: %d", code)
	}
	events, err := e.q.ListTicketEvents(context.Background(), tk.ID)
	if err != nil || len(events) != 1 || events[0].Action != "update" {
		t.Fatalf("history: %+v %v", events, err)
	}
	var changes map[string]map[string]any
	if err := json.Unmarshal(events[0].Changes, &changes); err != nil ||
		changes["title"]["old"] != "Overtime export" || changes["title"]["new"] != "Overtime export for payroll" ||
		len(changes["menus"]["old"].([]any)) != 1 || len(changes["menus"]["new"].([]any)) != 0 || changes["client"] != nil {
		t.Fatalf("changes: %s", events[0].Changes)
	}
}

func TestTransitionsMoveBetweenOpenStatuses(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	viewer, vu := e.signedIn("vera@example.com", false)
	e.seedMember(vu, w.p, "viewer")
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	var st httpapi.StatusList
	e.call(w.pm, http.MethodGet, "/projects/HRIS/statuses", nil, &st)
	path := "/tickets/" + tk.Key + "/transition"
	var out httpapi.Ticket
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"status_id": st.Items[1].Id}, &out); code != http.StatusOK || out.Status.Name != "In progress" || out.Version != 2 {
		t.Fatalf("to In progress: %d %+v", code, out)
	}
	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"status_id": st.Items[3].Id}, &p); code != http.StatusUnprocessableEntity || p.Code != "close_unavailable" {
		t.Fatalf("to Done: %d %+v", code, p)
	}
	if code := e.call(viewer, http.MethodPost, path, map[string]any{"status_id": st.Items[0].Id}, nil); code != http.StatusForbidden {
		t.Fatalf("a viewer moves it: %d", code)
	}
	// AC-TK-1: the history shows "Status To do → In progress" with the member and time.
	events, _ := e.q.ListTicketEvents(context.Background(), tk.ID)
	var changes map[string]map[string]any
	if len(events) != 1 || events[0].Action != "transition" || json.Unmarshal(events[0].Changes, &changes) != nil ||
		changes["status"]["old"] != "To do" || changes["status"]["new"] != "In progress" ||
		events[0].ActorName == nil || *events[0].ActorName != "pm@example.com" {
		t.Fatalf("history: %+v", events)
	}
}

func TestHiddenTicketsLookMissing(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Client B payroll change", &w.b)
	outsider, _ := e.signedIn("out@example.com", false)
	for _, c := range []*http.Client{w.pm, outsider} {
		if code := e.call(c, http.MethodGet, "/tickets/"+tk.Key, nil, nil); code != http.StatusNotFound {
			t.Errorf("hidden ticket: %d", code)
		}
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd server && go generate ./... && go vet ./internal/httpapi/ 2>&1 | head -5
```

Expected: FAIL. `*Server does not implement ServerInterface (missing method CreateTicket)`.

- [ ] **Step 4: Write the handlers**

`server/internal/httpapi/tickets.go`:

```go
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

var errStale = errors.New("the ticket changed since it was read")

var clientIDField = FieldError{Field: "client_id", Code: "invalid", Message: "Choose a client of this project in your scope"}

// ticketInput is a create or an update request, whichever arrived.
type ticketInput struct {
	Type        TicketType
	Title       string
	ClientID    *int64
	ContactID   *int64
	UserID      *int64
	NodeIDs     []int64
	Reason      *string
	Description *string
	AssigneeID  *int64
	Priority    *Priority
	DueDate     *openapi_types.Date
}

func (s *Server) CreateTicket(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Member)
	if !ok {
		return
	}
	var in TicketCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	draft, nodeIDs, fields, err := s.checkTicket(ctx, pc, ticketInput{
		Type: in.Type, Title: in.Title, ClientID: in.ClientId, ContactID: in.RequesterContactId, UserID: in.RequesterUserId,
		NodeIDs: in.NodeIds, Reason: in.Reason, Description: in.Description, AssigneeID: in.AssigneeId,
		Priority: in.Priority, DueDate: in.DueDate,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	status, statusOK, err := s.startStatus(ctx, pc.project.ID, in.StatusId)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !statusOK {
		fields = append(fields, FieldError{Field: "status_id", Code: "invalid", Message: "Choose an open status of this project"})
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var out Ticket
	err = s.inTx(ctx, func(q *db.Queries) error {
		n, err := q.NextTicketNumber(ctx, pc.project.ID)
		if err != nil {
			return err
		}
		draft.Number, draft.Key, draft.StatusID = n, fmt.Sprintf("%s-%d", pc.project.Key, n), status
		created, err := q.CreateTicket(ctx, draft)
		if err != nil {
			return err
		}
		if err := q.AddTicketNodes(ctx, db.AddTicketNodesParams{TicketID: created.ID, NodeIds: nodeIDs}); err != nil {
			return err
		}
		if out, err = readTicket(ctx, q, created.Key); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", created.ID, "create", ticketAudit(out))
	})
	if constraintOf(err) == "tickets_client_linked" {
		ticketClientInvalid(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("ETag", etag(out.Version))
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) GetTicket(w http.ResponseWriter, r *http.Request, key string) {
	_, row, ok := s.ticketFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	out, err := ticketFromRow(r.Context(), s.q, row)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("ETag", etag(out.Version))
	writeJSON(w, http.StatusOK, out)
}

// UpdateTicket replaces a ticket's editable fields. If-Match carries the
// version the editor read; a stale one answers 412 (FSD §8.6, AC-TK-5).
func (s *Server) UpdateTicket(w http.ResponseWriter, r *http.Request, key string, params UpdateTicketParams) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	version, err := strconv.ParseInt(strings.Trim(params.IfMatch, `"`), 10, 32)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_parameter", "If-Match must carry the ticket's version, as its ETag gave it")
		return
	}
	var in TicketUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	draft, nodeIDs, fields, err := s.checkTicket(ctx, pc, ticketInput{
		Type: in.Type, Title: in.Title, ClientID: in.ClientId, ContactID: in.RequesterContactId, UserID: in.RequesterUserId,
		NodeIDs: in.NodeIds, Reason: in.Reason, Description: in.Description, AssigneeID: in.AssigneeId,
		Priority: in.Priority, DueDate: in.DueDate,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if in.RequesterContactId == nil && in.RequesterUserId == nil {
		fields = append(fields, FieldError{Field: "requester_contact_id", Code: "required", Message: "Say who asked for this"})
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var out Ticket
	err = s.inTx(ctx, func(q *db.Queries) error {
		before, err := ticketFromRow(ctx, q, row)
		if err != nil {
			return err
		}
		updated, err := q.UpdateTicket(ctx, db.UpdateTicketParams{
			ID: row.Ticket.ID, Version: int32(version), Type: draft.Type, Title: draft.Title, Description: draft.Description,
			Reason: draft.Reason, ClientID: draft.ClientID, RequesterContactID: draft.RequesterContactID,
			RequesterUserID: draft.RequesterUserID, AssigneeID: draft.AssigneeID, Priority: draft.Priority, DueDate: draft.DueDate,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errStale
		}
		if err != nil {
			return err
		}
		if err := q.ClearTicketNodes(ctx, updated.ID); err != nil {
			return err
		}
		if err := q.AddTicketNodes(ctx, db.AddTicketNodesParams{TicketID: updated.ID, NodeIds: nodeIDs}); err != nil {
			return err
		}
		if out, err = readTicket(ctx, q, updated.Key); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", updated.ID, "update",
			changed(ticketAudit(before), ticketAudit(out)))
	})
	switch {
	case errors.Is(err, errStale):
		writeProblem(w, http.StatusPreconditionFailed, "stale", "Someone updated this ticket a moment ago")
	case constraintOf(err) == "tickets_client_linked":
		ticketClientInvalid(w)
	case err != nil:
		s.fail(w, r, err)
	default:
		w.Header().Set("ETag", etag(out.Version))
		writeJSON(w, http.StatusOK, out)
	}
}

// TransitionTicket moves a ticket among open statuses (R-TK-3). Entering Done
// or Cancelled is a close, which needs the decision record of FSD §9.1.
func (s *Server) TransitionTicket(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
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
	if closedCategory(st.Category) {
		writeProblem(w, http.StatusUnprocessableEntity, "close_unavailable", "Closing a ticket needs its decision record, which is not available yet")
		return
	}
	var out Ticket
	err = s.inTx(ctx, func(q *db.Queries) error {
		if st.ID != row.Ticket.StatusID {
			if _, err := q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: row.Ticket.ID, StatusID: st.ID}); err != nil {
				return err
			}
			if err := audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", row.Ticket.ID, "transition",
				map[string]any{"status": map[string]any{"old": row.Status.Name, "new": st.Name}}); err != nil {
				return err
			}
		}
		var err error
		out, err = readTicket(ctx, q, row.Ticket.Key)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("ETag", etag(out.Version))
	writeJSON(w, http.StatusOK, out)
}

// ticketFor loads a ticket for the signed-in user. A ticket in a project the
// user does not belong to, or of a client outside their scope, answers 404 like
// a missing one (R-AC-3, R-AC-7); a role below need answers 403.
func (s *Server) ticketFor(w http.ResponseWriter, r *http.Request, key, need string) (projectCtx, db.GetTicketByKeyRow, bool) {
	u := s.requireUser(w, r)
	if u == nil {
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	ctx := r.Context()
	row, err := s.q.GetTicketByKey(ctx, strings.ToUpper(key))
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Ticket not found")
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	p, err := s.q.GetProjectByID(ctx, row.Ticket.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	pc, ok := s.memberOf(w, r, u, p)
	if !ok {
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	if !pc.scope.Sees(row.Ticket.ClientID) {
		writeProblem(w, http.StatusNotFound, "not_found", "Ticket not found")
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	if !pc.scope.Allows(need) {
		writeProblem(w, http.StatusForbidden, "forbidden", "Your project role does not allow this")
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	return pc, row, true
}

// checkTicket validates a ticket's fields against the project and the caller's
// scope (FSD §8.1). It returns the row to store, minus number, key and status,
// and the distinct menu ids.
func (s *Server) checkTicket(ctx context.Context, pc projectCtx, in ticketInput) (db.CreateTicketParams, []int64, []FieldError, error) {
	var f []FieldError
	t := db.CreateTicketParams{
		ProjectID: pc.project.ID, Type: string(in.Type), Title: strings.TrimSpace(in.Title),
		Description: deref(in.Description), Reason: strings.TrimSpace(deref(in.Reason)),
		ClientID: in.ClientID, ReporterID: pc.user.ID, AssigneeID: in.AssigneeID, Priority: string(PriorityMedium),
	}
	if n := utf8.RuneCountInString(t.Title); n < 5 || n > 200 {
		f = append(f, FieldError{Field: "title", Code: "invalid", Message: "Use 5 to 200 characters"})
	}
	if !in.Type.Valid() {
		f = append(f, FieldError{Field: "type", Code: "invalid", Message: "Choose bug, change request or feature"})
	}
	if in.Priority != nil {
		if !in.Priority.Valid() {
			f = append(f, FieldError{Field: "priority", Code: "invalid", Message: "Choose low, medium, high or urgent"})
		}
		t.Priority = string(*in.Priority)
	}
	if utf8.RuneCountInString(t.Reason) > 2000 {
		f = append(f, FieldError{Field: "reason", Code: "invalid", Message: "Use at most 2,000 characters"})
	}
	if utf8.RuneCountInString(t.Description) > 50000 {
		f = append(f, FieldError{Field: "description", Code: "invalid", Message: "Use at most 50,000 characters"})
	}
	if in.DueDate != nil {
		t.DueDate = &in.DueDate.Time
	}
	// The client is in the caller's scope; the database checks it is linked (AC-TK-4).
	if !pc.scope.Sees(in.ClientID) {
		f = append(f, clientIDField)
	}
	switch {
	case in.ContactID != nil && in.UserID != nil:
		f = append(f, FieldError{Field: "requester_contact_id", Code: "invalid", Message: "Choose a contact or a user, not both"})
	case in.ContactID != nil:
		rows, err := s.q.ListContacts(ctx, db.ListContactsParams{IsAdmin: pc.user.IsAdmin, UserID: pc.user.ID, ID: in.ContactID})
		if err != nil {
			return t, nil, nil, err
		}
		if len(rows) == 0 || (rows[0].ClientID != nil && (in.ClientID == nil || *rows[0].ClientID != *in.ClientID)) {
			f = append(f, FieldError{Field: "requester_contact_id", Code: "invalid", Message: "Choose a contact of this client or an internal one"})
		}
		t.RequesterContactID = in.ContactID
	default:
		uid := pc.user.ID
		if in.UserID != nil && *in.UserID != uid {
			uid = *in.UserID
			_, member, err := access.ForProject(ctx, s.q, &db.User{ID: uid}, pc.project.ID)
			if err != nil {
				return t, nil, nil, err
			}
			if !member {
				f = append(f, FieldError{Field: "requester_user_id", Code: "invalid", Message: "Choose a member of this project"})
			}
		}
		t.RequesterUserID = &uid
	}
	if in.AssigneeID != nil {
		sc, member, err := access.ForProject(ctx, s.q, &db.User{ID: *in.AssigneeID}, pc.project.ID)
		if err != nil {
			return t, nil, nil, err
		}
		if !member || !sc.Allows(access.Member) || !sc.Sees(in.ClientID) {
			f = append(f, FieldError{Field: "assignee_id", Code: "invalid", Message: "Choose a member who can see this ticket"})
		}
	}
	nodeIDs := slices.Compact(slices.Sorted(slices.Values(in.NodeIDs)))
	if len(nodeIDs) > 0 {
		visible, err := s.q.ListNodes(ctx, db.ListNodesParams{ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs)})
		if err != nil {
			return t, nil, nil, err
		}
		live := map[int64]bool{}
		for _, n := range visible {
			live[n.ID] = true
		}
		for _, id := range nodeIDs {
			if !live[id] {
				f = append(f, FieldError{Field: "node_ids", Code: "invalid", Message: "Choose menus and modules of this project"})
				break
			}
		}
	}
	return t, orEmpty(nodeIDs), f, nil
}

// startStatus picks a new ticket's status: the one asked for when it is an open
// status of the project, else the project's default.
func (s *Server) startStatus(ctx context.Context, projectID int64, id *int64) (int64, bool, error) {
	if id == nil {
		st, err := s.q.GetDefaultStatus(ctx, projectID)
		return st.ID, err == nil, err
	}
	st, err := s.q.GetStatus(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return st.ID, st.ProjectID == projectID && !closedCategory(st.Category), nil
}

// readTicket reads a ticket the way the API shows it.
func readTicket(ctx context.Context, q *db.Queries, key string) (Ticket, error) {
	row, err := q.GetTicketByKey(ctx, key)
	if err != nil {
		return Ticket{}, err
	}
	return ticketFromRow(ctx, q, row)
}

func ticketFromRow(ctx context.Context, q *db.Queries, row db.GetTicketByKeyRow) (Ticket, error) {
	t := row.Ticket
	nodes, err := q.ListTicketNodes(ctx, t.ID)
	if err != nil {
		return Ticket{}, err
	}
	files, err := q.ListAttachments(ctx, t.ID)
	if err != nil {
		return Ticket{}, err
	}
	out := Ticket{
		Id: t.ID, Key: t.Key, ProjectKey: row.ProjectKey, Title: t.Title, Type: TicketType(t.Type),
		Description: t.Description, Reason: t.Reason, Priority: Priority(t.Priority), Version: t.Version,
		Status: toAPIStatus(row.Status), Reporter: Ref{Id: t.ReporterID, Name: row.ReporterName},
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		Nodes: make([]NodeRef, len(nodes)), Attachments: make([]Attachment, len(files)),
	}
	if t.ClientID != nil {
		out.Client = &Ref{Id: *t.ClientID, Name: deref(row.ClientName)}
	}
	if t.AssigneeID != nil {
		out.Assignee = &Ref{Id: *t.AssigneeID, Name: deref(row.AssigneeName)}
	}
	if t.RequesterContactID != nil {
		out.Requester = TicketRequester{Kind: TicketRequesterKindContact, Id: *t.RequesterContactID,
			Name: deref(row.RequesterContactName), Title: row.RequesterContactTitle}
	} else {
		out.Requester = TicketRequester{Kind: TicketRequesterKindUser, Id: *t.RequesterUserID, Name: deref(row.RequesterUserName)}
	}
	if t.DueDate != nil {
		out.DueDate = &openapi_types.Date{Time: *t.DueDate}
	}
	for i, n := range nodes {
		out.Nodes[i] = NodeRef{Id: n.ID, Name: n.Name, Archived: n.Archived}
	}
	for i, a := range files {
		out.Attachments[i] = toAPIAttachment(a.Attachment, a.UploaderName)
	}
	return out, nil
}

// ticketAudit is what a ticket's history shows: names, as they read at the time.
func ticketAudit(t Ticket) map[string]any {
	menus := make([]string, len(t.Nodes))
	for i, n := range t.Nodes {
		menus[i] = n.Name
	}
	m := map[string]any{
		"title": t.Title, "type": string(t.Type), "priority": string(t.Priority), "reason": t.Reason,
		"description": t.Description, "status": t.Status.Name, "requester": t.Requester.Name, "menus": menus,
		"client": nil, "assignee": nil, "due_date": nil,
	}
	if t.Client != nil {
		m["client"] = t.Client.Name
	}
	if t.Assignee != nil {
		m["assignee"] = t.Assignee.Name
	}
	if t.DueDate != nil {
		m["due_date"] = t.DueDate.String()
	}
	return m
}

func toAPIAttachment(a db.Attachment, uploader string) Attachment {
	return Attachment{
		Id: a.ID, Filename: a.Filename, ContentType: a.ContentType, SizeBytes: a.SizeBytes,
		Uploader: Ref{Id: a.UploaderID, Name: uploader}, CreatedAt: a.CreatedAt,
	}
}

func ticketClientInvalid(w http.ResponseWriter) {
	writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", clientIDField)
}

func etag(version int32) string { return `"` + strconv.Itoa(int(version)) + `"` }
```

- [ ] **Step 5: Generate and run the tests**

```bash
cd server && go generate ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...
cd ../web && npm run gen:api
```

Expected: `ok` for every Go package.

- [ ] **Step 6: Commit**

```bash
git add api/openapi.yaml server web/lib/api-types.ts
git commit -m "feat(server): create, read, edit and move tickets"
```

### Task 4: The ticket list

**Files:**
- Modify: `api/openapi.yaml` (a `get` under `/projects/{key}/tickets`; schemas `TicketSummary`, `TicketPage`)
- Create: `server/internal/httpapi/ticket_list.go`
- Test: `server/internal/httpapi/ticket_list_test.go`

**Interfaces:**
- Consumes: `ListTickets` and `ListNodes` (Task 1); `projectFor`, `orEmpty`, `deref`, `ptr`; `newHRIS`, `seedTicket` (Tasks 2–3).
- Produces: handler `ListTickets(w, r, key string, params ListTicketsParams)`, used by the board and the list.
  - Query parameters: `status_id`, `category`, `open`, `type`, `client_id`, `core`, `assignee_id`, `mine`, `node_id` (with sub-nodes), `q`, `missing` (`reason` | `menus`), `sort` (`updated` default, `created`, `key`, `priority`, `due`), `limit` (1–1000, default 50), `cursor`.
  - Returns `TicketPage{Items []TicketSummary; NextCursor *string}`.

- [ ] **Step 1: Extend the contract**

In `api/openapi.yaml`, under the existing `/projects/{key}/tickets` path, add this operation next to `post`:

```yaml
    get:
      operationId: listTickets
      tags: [tickets]
      description: The project's tickets the caller may see, filtered and sorted; pages follow next_cursor.
      parameters:
        - { name: status_id, in: query, schema: { type: integer, format: int64 } }
        - { name: category, in: query, schema: { $ref: "#/components/schemas/StatusCategory" } }
        - { name: open, in: query, schema: { type: boolean }, description: Only To do and In progress statuses. }
        - { name: type, in: query, schema: { $ref: "#/components/schemas/TicketType" } }
        - { name: client_id, in: query, schema: { type: integer, format: int64 } }
        - { name: core, in: query, schema: { type: boolean }, description: Only core work (no client). }
        - { name: assignee_id, in: query, schema: { type: integer, format: int64 } }
        - { name: mine, in: query, schema: { type: boolean }, description: Only tickets assigned to the caller. }
        - { name: node_id, in: query, schema: { type: integer, format: int64 }, description: Tickets on this node or its sub-nodes. }
        - { name: q, in: query, schema: { type: string, maxLength: 200 }, description: Words in the title, or a ticket key. }
        - { name: missing, in: query, schema: { type: string, enum: [reason, menus] } }
        - { name: sort, in: query, schema: { type: string, enum: [updated, created, key, priority, due] } }
        - { name: limit, in: query, schema: { type: integer, format: int32, minimum: 1, maximum: 1000 } }
        - { name: cursor, in: query, schema: { type: string } }
      responses:
        "200":
          description: One page of tickets.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/TicketPage" }
        default: { $ref: "#/components/responses/Problem" }
```

Under `components.schemas:`:

```yaml
    TicketSummary:
      type: object
      required: [id, key, title, type, priority, due_date, status_id, requester_name, node_names, missing_reason, updated_at]
      properties:
        id: { type: integer, format: int64 }
        key: { type: string }
        title: { type: string }
        type: { $ref: "#/components/schemas/TicketType" }
        priority: { $ref: "#/components/schemas/Priority" }
        due_date: { type: string, format: date, nullable: true }
        status_id: { type: integer, format: int64 }
        client: { $ref: "#/components/schemas/Ref" }
        assignee: { $ref: "#/components/schemas/Ref" }
        requester_name: { type: string }
        node_names:
          type: array
          items: { type: string }
        missing_reason: { type: boolean }
        updated_at: { type: string, format: date-time }
    TicketPage:
      type: object
      required: [items, next_cursor]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/TicketSummary" }
        next_cursor: { type: string, nullable: true }
```

- [ ] **Step 2: Write the failing tests**

`server/internal/httpapi/ticket_list_test.go`:

```go
package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func ticketKeys(page httpapi.TicketPage) []string {
	keys := []string{}
	for _, t := range page.Items {
		keys = append(keys, t.Key)
	}
	return keys
}

func TestTicketListFiltersSortsAndScopes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	w := newHRIS(e)
	admin, au := e.signedIn("admin@example.com", true)
	leave := e.seedNode(w.p, &w.hr, "menu", "Leave Request")
	t1 := e.seedTicket(w.p, au, "Overtime rules for Client A", &w.a, w.ot) // HRIS-1
	t2 := e.seedTicket(w.p, au, "Leave balance on the payslip", nil, leave) // HRIS-2, core work
	e.seedTicket(w.p, au, "Client B payroll export", &w.b)                  // HRIS-3, hidden from the PM
	var st httpapi.StatusList
	e.call(admin, http.MethodGet, "/projects/HRIS/statuses", nil, &st)
	if _, err := e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: t1.ID, StatusID: st.Items[1].Id}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.q.UpdateTicket(ctx, db.UpdateTicketParams{
		ID: t2.ID, Version: 1, Type: "feature", Title: t2.Title, Reason: "Employees ask about it every month.",
		RequesterUserID: &au.ID, AssigneeID: &w.pmUser.ID, Priority: "urgent",
	}); err != nil {
		t.Fatal(err)
	}
	list := func(c *http.Client, query string) []string {
		t.Helper()
		var page httpapi.TicketPage
		if code := e.call(c, http.MethodGet, "/projects/HRIS/tickets"+query, nil, &page); code != http.StatusOK {
			t.Fatalf("%s: %d", query, code)
		}
		return ticketKeys(page)
	}
	for query, want := range map[string][]string{
		"":                                          {"HRIS-2", "HRIS-1"}, // latest update first; HRIS-3 is out of scope
		"?sort=key":                                 {"HRIS-1", "HRIS-2"},
		"?sort=priority":                            {"HRIS-2", "HRIS-1"},
		"?mine=true":                                {"HRIS-2"},
		"?core=true":                                {"HRIS-2"},
		fmt.Sprintf("?client_id=%d", w.a.ID):        {"HRIS-1"},
		"?type=feature":                             {"HRIS-2"},
		fmt.Sprintf("?status_id=%d", st.Items[1].Id): {"HRIS-1"},
		"?category=todo":                            {"HRIS-2"},
		fmt.Sprintf("?node_id=%d", w.hr.ID):         {"HRIS-2", "HRIS-1"}, // sub-nodes count (AC-MR-4)
		fmt.Sprintf("?node_id=%d", w.ot.ID):         {"HRIS-1"},
		"?missing=reason":                           {"HRIS-1"},
		"?missing=menus":                            {},
		"?q=payslip":                                {"HRIS-2"},
		"?q=hris-1":                                 {"HRIS-1"},
	} {
		if got := list(w.pm, query); !slices.Equal(got, want) {
			t.Errorf("%q: %v, want %v", query, got, want)
		}
	}
	if got := list(admin, ""); !slices.Equal(got, []string{"HRIS-2", "HRIS-1", "HRIS-3"}) {
		t.Errorf("admin: %v", got)
	}
	var page httpapi.TicketPage
	e.call(w.pm, http.MethodGet, "/projects/HRIS/tickets?sort=key", nil, &page)
	first, second := page.Items[0], page.Items[1]
	if first.Client == nil || first.Client.Name != "Client A" || !slices.Equal(first.NodeNames, []string{"Overtime Approval"}) ||
		!first.MissingReason || second.Assignee == nil || second.Assignee.Name != "pm@example.com" || second.MissingReason ||
		second.Priority != httpapi.PriorityUrgent {
		t.Fatalf("rows: %+v %+v", first, second)
	}
}

func TestTicketListPagesByCursor(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	admin, au := e.signedIn("admin@example.com", true)
	for i := 1; i <= 5; i++ {
		e.seedTicket(p, au, fmt.Sprintf("Ticket number %d", i), nil)
	}
	var got [][]string
	path := "/projects/HRIS/tickets?sort=key&limit=2"
	for {
		var page httpapi.TicketPage
		if code := e.call(admin, http.MethodGet, path, nil, &page); code != http.StatusOK {
			t.Fatalf("page: %d", code)
		}
		got = append(got, ticketKeys(page))
		if page.NextCursor == nil {
			break
		}
		path = "/projects/HRIS/tickets?sort=key&limit=2&cursor=" + *page.NextCursor
	}
	if fmt.Sprint(got) != "[[HRIS-1 HRIS-2] [HRIS-3 HRIS-4] [HRIS-5]]" {
		t.Fatalf("pages: %v", got)
	}
	if code := e.call(admin, http.MethodGet, "/projects/HRIS/tickets?cursor=nope", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("bad cursor: %d", code)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd server && go generate ./... && go vet ./internal/httpapi/ 2>&1 | head -5
```

Expected: FAIL. `*Server does not implement ServerInterface (missing method ListTickets)`.

- [ ] **Step 4: Write the handler**

`server/internal/httpapi/ticket_list.go`:

```go
package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

// ListTickets serves the list and the board (FSD §8.4, §8.5): one project's
// visible tickets, filtered and sorted in SQL.
// ponytail: an offset cursor; switch to keyset pages if deep pages get slow.
func (s *Server) ListTickets(w http.ResponseWriter, r *http.Request, key string, params ListTicketsParams) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	limit, offset := 50, 0
	if params.Limit != nil {
		limit = min(max(int(*params.Limit), 1), 1000)
	}
	if params.Cursor != nil {
		n, err := strconv.Atoi(*params.Cursor)
		if err != nil || n < 0 {
			writeProblem(w, http.StatusBadRequest, "invalid_parameter", "The cursor is not one this API gave")
			return
		}
		offset = n
	}
	filter := db.ListTicketsParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
		StatusID: params.StatusId, OpenOnly: deref(params.Open), ClientID: params.ClientId, CoreOnly: deref(params.Core),
		AssigneeID: params.AssigneeId, Q: strings.TrimSpace(deref(params.Q)), Sort: "updated",
		MissingReason: params.Missing != nil && *params.Missing == ListTicketsParamsMissingReason,
		MissingMenus:  params.Missing != nil && *params.Missing == ListTicketsParamsMissingMenus,
		Lim:           int32(limit + 1), Off: int32(offset),
	}
	if params.Category != nil {
		filter.Category = ptr(string(*params.Category))
	}
	if params.Type != nil {
		filter.Type = ptr(string(*params.Type))
	}
	if params.Sort != nil {
		filter.Sort = string(*params.Sort)
	}
	if deref(params.Mine) {
		filter.AssigneeID = &pc.user.ID
	}
	ctx := r.Context()
	if params.NodeId != nil {
		nodes, err := s.q.ListNodes(ctx, db.ListNodesParams{
			ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs), IncludeArchived: true,
		})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		filter.NodeIds = subtree(nodes, *params.NodeId) // the node and its sub-nodes (AC-MR-4)
	}
	rows, err := s.q.ListTickets(ctx, filter)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	page := TicketPage{Items: make([]TicketSummary, 0, len(rows))}
	if len(rows) > limit {
		rows = rows[:limit]
		page.NextCursor = ptr(strconv.Itoa(offset + limit))
	}
	for _, t := range rows {
		page.Items = append(page.Items, toTicketSummary(t))
	}
	writeJSON(w, http.StatusOK, page)
}

// subtree lists root and every node below it; an unknown root gives an empty,
// non-nil list, which matches no ticket.
func subtree(rows []db.ListNodesRow, root int64) []int64 {
	kids := map[int64][]int64{}
	found := false
	for _, n := range rows {
		found = found || n.ID == root
		if n.ParentID != nil {
			kids[*n.ParentID] = append(kids[*n.ParentID], n.ID)
		}
	}
	out := []int64{}
	if !found {
		return out
	}
	for stack := []int64{root}; len(stack) > 0; {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, id)
		stack = append(stack, kids[id]...)
	}
	return out
}

func toTicketSummary(t db.ListTicketsRow) TicketSummary {
	out := TicketSummary{
		Id: t.ID, Key: t.Key, Title: t.Title, Type: TicketType(t.Type), Priority: Priority(t.Priority),
		StatusId: t.StatusID, RequesterName: t.RequesterName, NodeNames: orEmpty(t.NodeNames),
		MissingReason: t.MissingReason, UpdatedAt: t.UpdatedAt,
	}
	if t.DueDate != nil {
		out.DueDate = &openapi_types.Date{Time: *t.DueDate}
	}
	if t.ClientID != nil {
		out.Client = &Ref{Id: *t.ClientID, Name: deref(t.ClientName)}
	}
	if t.AssigneeID != nil {
		out.Assignee = &Ref{Id: *t.AssigneeID, Name: deref(t.AssigneeName)}
	}
	return out
}
```

- [ ] **Step 5: Generate and run the tests**

```bash
cd server && go generate ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...
cd ../web && npm run gen:api
```

Expected: `ok` for every Go package.

- [ ] **Step 6: Commit**

```bash
git add api/openapi.yaml server web/lib/api-types.ts
git commit -m "feat(server): ticket list with filters, sorting and pages"
```

### Task 5: Comments and the Activity feed

**Files:**
- Modify: `api/openapi.yaml` (paths `/tickets/{key}/activity`, `/tickets/{key}/comments`, `/comments/{id}`; schemas `ActivityItem`, `ActivityList`, `CommentInput`, `CommentUpdate`)
- Create: `server/internal/httpapi/comments.go`
- Test: `server/internal/httpapi/comments_test.go`

**Interfaces:**
- Consumes: `ticketFor` (Task 3); `ListComments`, `ListTicketEvents` and the comment queries (Task 1).
- Produces: handlers `GetTicketActivity(w, r, key)`, `CreateComment(w, r, key)`, `UpdateComment(w, r, id int64)`, `DeleteComment(w, r, id int64)`.
  - `ActivityItem` fields: `kind` (`comment` | `event`), `at`, `actor`, and either the comment fields (`comment_id`, `internal`, `body` (absent when deleted, except for system admins), `deleted`, `edited`) or the event fields (`action`, `changes`).
  - Audit actions `comment_edit` (with `comment_id` and the old and new `body`) and `comment_delete`.

- [ ] **Step 1: Extend the contract**

`api/openapi.yaml`, under `paths:`:

```yaml
  /tickets/{key}/activity:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    get:
      operationId: getTicketActivity
      tags: [tickets]
      responses:
        "200":
          description: Comments and history, oldest first.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/ActivityList" }
        default: { $ref: "#/components/responses/Problem" }
  /tickets/{key}/comments:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    post:
      operationId: createComment
      tags: [tickets]
      description: Members and project admins. Internal unless internal is false.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/CommentInput" }
      responses:
        "201":
          description: The comment.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/ActivityItem" }
        default: { $ref: "#/components/responses/Problem" }
  /comments/{id}:
    parameters:
      - { name: id, in: path, required: true, schema: { type: integer, format: int64 } }
    patch:
      operationId: updateComment
      tags: [tickets]
      description: The author only. The history keeps the earlier text.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/CommentUpdate" }
      responses:
        "200":
          description: The comment.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/ActivityItem" }
        default: { $ref: "#/components/responses/Problem" }
    delete:
      operationId: deleteComment
      tags: [tickets]
      description: The author only. Readers then see "Comment deleted".
      responses:
        "204": { description: Deleted. }
        default: { $ref: "#/components/responses/Problem" }
```

Under `components.schemas:`:

```yaml
    ActivityItem:
      type: object
      required: [kind, at]
      properties:
        kind: { type: string, enum: [comment, event] }
        at: { type: string, format: date-time }
        actor: { $ref: "#/components/schemas/Ref" }
        comment_id: { type: integer, format: int64 }
        internal: { type: boolean }
        body: { type: string, description: A comment's text; absent when deleted, except for system admins. }
        deleted: { type: boolean }
        edited: { type: boolean }
        action: { type: string, example: transition }
        changes: { type: object, additionalProperties: true }
    ActivityList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/ActivityItem" }
    CommentInput:
      type: object
      required: [body]
      properties:
        body: { type: string, maxLength: 20000 }
        internal: { type: boolean, description: Defaults to true; false marks the comment client-safe. }
    CommentUpdate:
      type: object
      required: [body]
      properties:
        body: { type: string, maxLength: 20000 }
```

- [ ] **Step 2: Write the failing tests**

`server/internal/httpapi/comments_test.go`:

```go
package httpapi_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func activity(e *env, c *http.Client, key string) []httpapi.ActivityItem {
	e.t.Helper()
	var list httpapi.ActivityList
	if code := e.call(c, http.MethodGet, "/tickets/"+key+"/activity", nil, &list); code != http.StatusOK {
		e.t.Fatalf("activity: %d", code)
	}
	return list.Items
}

func TestCommentsDefaultToInternalAndEditsKeepTheirHistory(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	var c httpapi.ActivityItem
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Budi confirmed by phone."}, &c); code != http.StatusCreated ||
		c.Internal == nil || !*c.Internal {
		t.Fatalf("a comment is Internal by default (AC-TK-10): %d %+v", code, c)
	}
	path := fmt.Sprintf("/comments/%d", *c.CommentId)
	for _, body := range []string{"Budi confirmed by phone on Monday.", "Budi confirmed by phone on Monday, 9 am."} {
		if code := e.call(w.pm, http.MethodPatch, path, map[string]any{"body": body}, nil); code != http.StatusOK {
			t.Fatalf("edit: %d", code)
		}
	}
	// AC-TK-6: both earlier versions appear, with actor and time.
	var olds []string
	for _, it := range activity(e, w.pm, tk.Key) {
		if it.Action != nil && *it.Action == "comment_edit" {
			olds = append(olds, (*it.Changes)["body"].(map[string]any)["old"].(string))
			if it.Actor == nil || it.Actor.Name != "pm@example.com" {
				t.Fatalf("edit without its actor: %+v", it)
			}
		}
	}
	if !slices.Equal(olds, []string{"Budi confirmed by phone.", "Budi confirmed by phone on Monday."}) {
		t.Fatalf("earlier versions: %v", olds)
	}
	if code := e.call(w.pm, http.MethodDelete, path, nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	for _, reader := range []struct {
		c        *http.Client
		readsIt  bool
	}{{w.pm, false}, {admin, true}} {
		for _, it := range activity(e, reader.c, tk.Key) {
			if it.Kind == httpapi.ActivityItemKindComment && (!*it.Deleted || (it.Body != nil) != reader.readsIt) {
				t.Errorf("deleted comment: %+v", it)
			}
		}
	}
}

func TestOnlyAuthorsChangeTheirComments(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ani, au := e.signedIn("ani@example.com", false)
	e.seedMember(au, w.p, "member")
	vera, vu := e.signedIn("vera@example.com", false)
	e.seedMember(vu, w.p, "viewer")
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	var c httpapi.ActivityItem
	e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Client-safe note", "internal": false}, &c)
	if c.Internal == nil || *c.Internal {
		t.Fatalf("client-safe comment: %+v", c)
	}
	path := fmt.Sprintf("/comments/%d", *c.CommentId)
	if code := e.call(ani, http.MethodPatch, path, map[string]any{"body": "Not mine"}, nil); code != http.StatusForbidden {
		t.Errorf("another member edits: %d", code)
	}
	if code := e.call(ani, http.MethodDelete, path, nil, nil); code != http.StatusForbidden {
		t.Errorf("another member deletes: %d", code)
	}
	if code := e.call(vera, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Hello"}, nil); code != http.StatusForbidden {
		t.Errorf("a viewer comments: %d", code)
	}
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "   "}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("an empty comment: %d", code)
	}
	hidden := e.seedTicket(w.p, au, "Client B payroll change", &w.b)
	if code := e.call(w.pm, http.MethodGet, "/tickets/"+hidden.Key+"/activity", nil, nil); code != http.StatusNotFound {
		t.Errorf("a hidden ticket's activity: %d", code)
	}
}

func TestActivityInterleavesCommentsAndHistory(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	var st httpapi.StatusList
	e.call(w.pm, http.MethodGet, "/projects/HRIS/statuses", nil, &st)
	e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Starting now"}, nil)
	e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/transition", map[string]any{"status_id": st.Items[1].Id}, nil)
	e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Halfway"}, nil)
	var kinds []string
	for _, it := range activity(e, w.pm, tk.Key) {
		kind := string(it.Kind)
		if it.Action != nil {
			kind = *it.Action
		}
		kinds = append(kinds, kind)
	}
	if !slices.Equal(kinds, []string{"comment", "transition", "comment"}) {
		t.Fatalf("activity: %v", kinds)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd server && go generate ./... && go vet ./internal/httpapi/ 2>&1 | head -5
```

Expected: FAIL. `*Server does not implement ServerInterface (missing method CreateComment)`.

- [ ] **Step 4: Write the handlers**

`server/internal/httpapi/comments.go`:

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

// GetTicketActivity interleaves a ticket's comments and history, oldest first
// (FSD §8.7). A deleted comment keeps its place; only system admins read it.
func (s *Server) GetTicketActivity(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	ctx := r.Context()
	comments, err := s.q.ListComments(ctx, row.Ticket.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	events, err := s.q.ListTicketEvents(ctx, row.Ticket.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]ActivityItem, 0, len(comments)+len(events))
	for _, c := range comments {
		items = append(items, commentItem(c.Comment, c.AuthorName, pc.user.IsAdmin))
	}
	for _, ev := range events {
		item := ActivityItem{Kind: ActivityItemKindEvent, At: ev.OccurredAt, Action: &ev.Action}
		if ev.ActorID != nil {
			item.Actor = &Ref{Id: *ev.ActorID, Name: deref(ev.ActorName)}
		}
		var changes map[string]any
		if err := json.Unmarshal(ev.Changes, &changes); err != nil {
			s.fail(w, r, err)
			return
		}
		item.Changes = &changes
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].At.Before(items[j].At) })
	writeJSON(w, http.StatusOK, ActivityList{Items: items})
}

// CreateComment adds a comment, Internal unless marked client-safe (FSD §8.7,
// AC-TK-10). The comment row is its own record; edits and deletes are audited.
func (s *Server) CreateComment(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	var in CommentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	body, ok := commentBody(w, in.Body)
	if !ok {
		return
	}
	c, err := s.q.CreateComment(r.Context(), db.CreateCommentParams{
		TicketID: row.Ticket.ID, AuthorID: pc.user.ID, Internal: in.Internal == nil || *in.Internal, Body: body,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, commentItem(c, pc.user.Name, true))
}

// UpdateComment rewords the author's comment; the history keeps the earlier
// text (AC-TK-6).
func (s *Server) UpdateComment(w http.ResponseWriter, r *http.Request, id int64) {
	pc, c, ok := s.ownComment(w, r, id)
	if !ok {
		return
	}
	var in CommentUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	body, ok := commentBody(w, in.Body)
	if !ok {
		return
	}
	ctx := r.Context()
	var updated db.Comment
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if updated, err = q.UpdateCommentBody(ctx, db.UpdateCommentBodyParams{ID: id, Body: body}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", c.TicketID, "comment_edit",
			map[string]any{"comment_id": id, "body": map[string]any{"old": c.Body, "new": body}})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Comment not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, commentItem(updated, pc.user.Name, true))
}

// DeleteComment hides the author's comment: readers see "Comment deleted" and
// only system admins still read the text (FSD §8.7).
func (s *Server) DeleteComment(w http.ResponseWriter, r *http.Request, id int64) {
	pc, c, ok := s.ownComment(w, r, id)
	if !ok {
		return
	}
	ctx := r.Context()
	err := s.inTx(ctx, func(q *db.Queries) error {
		if err := q.DeleteComment(ctx, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", c.TicketID, "comment_delete",
			map[string]any{"comment_id": id})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ownComment loads a live comment for its author: 404 when the comment or its
// ticket is hidden or gone, 403 for anyone but the author.
func (s *Server) ownComment(w http.ResponseWriter, r *http.Request, id int64) (projectCtx, db.Comment, bool) {
	if s.requireUser(w, r) == nil {
		return projectCtx{}, db.Comment{}, false
	}
	row, err := s.q.GetComment(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.Comment.DeletedAt != nil) {
		writeProblem(w, http.StatusNotFound, "not_found", "Comment not found")
		return projectCtx{}, db.Comment{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.Comment{}, false
	}
	pc, _, ok := s.ticketFor(w, r, row.TicketKey, access.Member)
	if !ok {
		return projectCtx{}, db.Comment{}, false
	}
	if row.Comment.AuthorID != pc.user.ID {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only the author can change a comment")
		return projectCtx{}, db.Comment{}, false
	}
	return pc, row.Comment, true
}

func commentBody(w http.ResponseWriter, body string) (string, bool) {
	b := strings.TrimSpace(body)
	if b == "" || utf8.RuneCountInString(b) > 20000 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "body", Code: "required", Message: "Write a comment of at most 20,000 characters"})
		return "", false
	}
	return b, true
}

// commentItem shows a comment; a deleted one keeps its text only when
// showDeleted (system admins, or the author's own response).
func commentItem(c db.Comment, author string, showDeleted bool) ActivityItem {
	item := ActivityItem{
		Kind: ActivityItemKindComment, At: c.CreatedAt, Actor: &Ref{Id: c.AuthorID, Name: author},
		CommentId: &c.ID, Internal: &c.Internal, Deleted: ptr(c.DeletedAt != nil), Edited: ptr(c.EditedAt != nil),
	}
	if c.DeletedAt == nil || showDeleted {
		item.Body = &c.Body
	}
	return item
}
```

- [ ] **Step 5: Generate and run the tests**

```bash
cd server && go generate ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...
cd ../web && npm run gen:api
```

Expected: `ok` for every Go package.

- [ ] **Step 6: Commit**

```bash
git add api/openapi.yaml server web/lib/api-types.ts
git commit -m "feat(server): comments with the Internal flag and the Activity feed"
```

### Task 6: Attachments

**Files:**
- Modify: `api/openapi.yaml` (paths `/tickets/{key}/attachments`, `/attachments/{id}`)
- Create: `server/internal/httpapi/attachments.go`
- Modify: `server/internal/httpapi/auth_test.go` (`newEnv`: a temporary attachments directory and a 64 KiB limit)
- Modify: `server/Dockerfile`, `deploy/compose.yaml` (a volume for `/data/attachments`)
- Test: `server/internal/httpapi/attachments_test.go`

**Interfaces:**
- Consumes: `ticketFor`, `toAPIAttachment` (Task 3); `config.Config.AttachmentsDir` and `AttachmentMaxBytes` (Task 2); the attachment queries (Task 1).
- Produces:
  - Handlers `UploadAttachment(w, r, key)` (multipart field `file`), `DownloadAttachment(w, r, id int64)`, `DeleteAttachment(w, r, id int64)`.
  - Error codes `file_too_large` (413), `file_type_not_allowed` (415), `invalid_upload` (400).
  - Audit actions `attachment_add` and `attachment_delete`, each with `attachment_id` and `filename`.
  - Test helpers `upload(e, c, key, filename string, content []byte) (int, httpapi.Attachment, httpapi.Problem)` and `download(e, c, id int64) (int, http.Header, []byte)`.

- [ ] **Step 1: Extend the contract**

`api/openapi.yaml`, under `paths:`:

```yaml
  /tickets/{key}/attachments:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    post:
      operationId: uploadAttachment
      tags: [tickets]
      description: Members and project admins. Up to 25 MB; images, PDF, Office and OpenDocument files, text, CSV, logs and ZIP.
      requestBody:
        required: true
        content:
          multipart/form-data:
            schema:
              type: object
              required: [file]
              properties:
                file: { type: string, format: binary }
      responses:
        "201":
          description: The stored file.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Attachment" }
        default: { $ref: "#/components/responses/Problem" }
  /attachments/{id}:
    parameters:
      - { name: id, in: path, required: true, schema: { type: integer, format: int64 } }
    get:
      operationId: downloadAttachment
      tags: [tickets]
      description: Anyone who can see the ticket. Images display inline; other files download.
      responses:
        "200":
          description: The file.
          content:
            application/octet-stream:
              schema: { type: string, format: binary }
        default: { $ref: "#/components/responses/Problem" }
    delete:
      operationId: deleteAttachment
      tags: [tickets]
      description: The uploader or a project admin.
      responses:
        "204": { description: Removed from the ticket. }
        default: { $ref: "#/components/responses/Problem" }
```

- [ ] **Step 2: Give tests and the container a place for files**

In `server/internal/httpapi/auth_test.go`, `newEnv` builds its config with a temporary directory and a small limit, so tests of the limit stay fast:

```go
	cfg := config.Config{
		DatabaseURL: d.AppURL, PublicURL: origin, ListenAddr: ":0",
		AttachmentsDir: t.TempDir(), AttachmentMaxBytes: 64 << 10,
	}
```

`server/Dockerfile`: the build stage also creates the directory, and the final stage copies it with the nonroot owner, so the named volume starts writable:

```dockerfile
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/app && mkdir -p /out/data/attachments

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
COPY --from=build --chown=nonroot:nonroot /out/data /data
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app"]
CMD ["serve"]
```

In `deploy/compose.yaml`, give `app` the volume. It goes after `environment` and before `healthcheck`:

```yaml
    volumes: [attachments:/data/attachments]
```

And add it under the top-level `volumes:`:

```yaml
  attachments: {}
```

- [ ] **Step 3: Write the failing tests**

`server/internal/httpapi/attachments_test.go`:

```go
package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// upload posts one file the way a browser form does.
func upload(e *env, c *http.Client, key, filename string, content []byte) (int, httpapi.Attachment, httpapi.Problem) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write(content)
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, e.url+"/api/v1/tickets/"+key+"/attachments", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", origin)
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var a httpapi.Attachment
	var p httpapi.Problem
	_ = json.Unmarshal(raw, &a)
	_ = json.Unmarshal(raw, &p)
	return res.StatusCode, a, p
}

func download(e *env, c *http.Client, id int64) (int, http.Header, []byte) {
	e.t.Helper()
	res, err := c.Get(fmt.Sprintf("%s/api/v1/attachments/%d", e.url, id))
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, raw
}

func TestUploadDownloadAndDeleteAttachments(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	pdf := []byte("%PDF-1.7 overtime report")
	code, a, _ := upload(e, w.pm, tk.Key, "Laporan Lembur.pdf", pdf)
	if code != http.StatusCreated || a.Filename != "Laporan Lembur.pdf" || a.ContentType != "application/pdf" ||
		a.SizeBytes != int64(len(pdf)) || a.Uploader.Name != "pm@example.com" {
		t.Fatalf("upload: %d %+v", code, a)
	}
	code, h, got := download(e, w.pm, a.Id)
	if code != http.StatusOK || !bytes.Equal(got, pdf) || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment") ||
		h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Content-Type") != "application/pdf" {
		t.Fatalf("download: %d %v", code, h)
	}
	_, png, _ := upload(e, w.pm, tk.Key, "screen.png", []byte("\x89PNG not really"))
	if _, h, _ := download(e, w.pm, png.Id); !strings.HasPrefix(h.Get("Content-Disposition"), "inline") {
		t.Fatalf("images display inline: %v", h)
	}
	var detail httpapi.Ticket
	if e.call(w.pm, http.MethodGet, "/tickets/"+tk.Key, nil, &detail); len(detail.Attachments) != 2 {
		t.Fatalf("ticket lists its files: %+v", detail.Attachments)
	}
	if code := e.call(w.pm, http.MethodDelete, fmt.Sprintf("/attachments/%d", a.Id), nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _, _ := download(e, w.pm, a.Id); code != http.StatusNotFound {
		t.Fatalf("a removed file: %d", code)
	}
	events, err := e.q.ListTicketEvents(context.Background(), tk.ID)
	if err != nil || len(events) != 3 || events[0].Action != "attachment_add" || events[2].Action != "attachment_delete" {
		t.Fatalf("history: %+v %v", events, err)
	}
}

// AC-TK-7, against the 64 KiB limit newEnv sets.
func TestAttachmentLimits(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	if code, _, p := upload(e, w.pm, tk.Key, "big.zip", bytes.Repeat([]byte("x"), 65<<10)); code != http.StatusRequestEntityTooLarge || p.Code != "file_too_large" {
		t.Errorf("too large: %d %+v", code, p)
	}
	for _, name := range []string{"run.exe", "page.svg", "index.html"} {
		if code, _, p := upload(e, w.pm, tk.Key, name, []byte("<x/>")); code != http.StatusUnsupportedMediaType || p.Code != "file_type_not_allowed" {
			t.Errorf("%s: %d %+v", name, code, p)
		}
	}
}

func TestAttachmentAccessFollowsTheTicket(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	owner, ou := e.signedIn("owner@example.com", false)
	e.seedMember(ou, w.p, "admin")
	ani, au := e.signedIn("ani@example.com", false)
	e.seedMember(au, w.p, "member")
	vera, vu := e.signedIn("vera@example.com", false)
	e.seedMember(vu, w.p, "viewer")
	forB := e.seedTicket(w.p, ou, "Client B export", &w.b)
	_, b, _ := upload(e, owner, forB.Key, "b.csv", []byte("a,b"))
	if code, _, _ := download(e, w.pm, b.Id); code != http.StatusNotFound {
		t.Errorf("a file of a hidden ticket: %d", code)
	}
	core := e.seedTicket(w.p, ou, "Shared thing", nil)
	_, f, _ := upload(e, ani, core.Key, "notes.txt", []byte("hello"))
	if code, _, _ := upload(e, vera, core.Key, "v.txt", []byte("x")); code != http.StatusForbidden {
		t.Errorf("a viewer uploads: %d", code)
	}
	path := fmt.Sprintf("/attachments/%d", f.Id)
	for _, c := range []*http.Client{vera, w.pm} {
		if code := e.call(c, http.MethodDelete, path, nil, nil); code != http.StatusForbidden {
			t.Errorf("not the uploader: %d", code)
		}
	}
	if code := e.call(owner, http.MethodDelete, path, nil, nil); code != http.StatusNoContent {
		t.Errorf("a project admin removes it: %d", code)
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

```bash
cd server && go generate ./... && go vet ./internal/httpapi/ 2>&1 | head -5
```

Expected: FAIL. `*Server does not implement ServerInterface (missing method UploadAttachment)`.

- [ ] **Step 5: Write the handlers**

`server/internal/httpapi/attachments.go`:

```go
package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

var errTooLarge = errors.New("the file is larger than the limit")

// attachmentTypes are the files tickets accept (FSD §8.7), by extension. SVG
// and HTML stay out because they could run script when opened.
var attachmentTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
	".pdf":  "application/pdf",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ppt":  "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".odt":  "application/vnd.oasis.opendocument.text",
	".ods":  "application/vnd.oasis.opendocument.spreadsheet",
	".txt":  "text/plain; charset=utf-8",
	".log":  "text/plain; charset=utf-8",
	".csv":  "text/csv; charset=utf-8",
	".zip":  "application/zip",
}

// UploadAttachment stores the multipart field "file" under its SHA-256, so a
// file uploaded twice is kept once.
func (s *Server) UploadAttachment(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	limit := s.cfg.AttachmentMaxBytes
	r.Body = http.MaxBytesReader(w, r.Body, limit+1<<20) // the file plus the multipart framing
	part, err := filePart(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_upload", "Send the file as multipart/form-data in a field named file")
		return
	}
	defer part.Close()
	name := filepath.Base(part.FileName())
	ctype, allowed := attachmentTypes[strings.ToLower(filepath.Ext(name))]
	if !allowed {
		writeProblem(w, http.StatusUnsupportedMediaType, "file_type_not_allowed",
			"Attach images, PDF, Office documents, text, CSV, logs or ZIP files")
		return
	}
	sum, size, err := s.storeFile(part, limit)
	if errors.Is(err, errTooLarge) {
		writeProblem(w, http.StatusRequestEntityTooLarge, "file_too_large", fmt.Sprintf("File is larger than %d MB", limit>>20))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	var a db.Attachment
	err = s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if a, err = q.CreateAttachment(ctx, db.CreateAttachmentParams{
			TicketID: row.Ticket.ID, UploaderID: pc.user.ID, Filename: name, ContentType: ctype, SizeBytes: size, Sha256: sum,
		}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", row.Ticket.ID, "attachment_add",
			map[string]any{"attachment_id": a.ID, "filename": name})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAPIAttachment(a, pc.user.Name))
}

// DownloadAttachment serves a file after its ticket's visibility check (FSD
// §8.7): images inline, everything else as a download, never sniffed.
func (s *Server) DownloadAttachment(w http.ResponseWriter, r *http.Request, id int64) {
	_, a, ok := s.attachmentFor(w, r, id, access.Viewer)
	if !ok {
		return
	}
	f, err := os.Open(s.attachmentPath(a.Sha256))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()
	disposition := "attachment"
	if strings.HasPrefix(a.ContentType, "image/") {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": a.Filename}))
	w.Header().Set("Content-Length", strconv.FormatInt(a.SizeBytes, 10))
	_, _ = io.Copy(w, f)
}

// DeleteAttachment removes a file from its ticket; the uploader or a project
// admin may. The stored bytes stay, since another upload may share them.
func (s *Server) DeleteAttachment(w http.ResponseWriter, r *http.Request, id int64) {
	pc, a, ok := s.attachmentFor(w, r, id, access.Member)
	if !ok {
		return
	}
	if a.UploaderID != pc.user.ID && !pc.scope.Allows(access.Admin) {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only the uploader or a project admin can remove a file")
		return
	}
	ctx := r.Context()
	err := s.inTx(ctx, func(q *db.Queries) error {
		if err := q.DeleteAttachment(ctx, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", a.TicketID, "attachment_delete",
			map[string]any{"attachment_id": id, "filename": a.Filename})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// attachmentFor loads a live attachment through its ticket's checks.
func (s *Server) attachmentFor(w http.ResponseWriter, r *http.Request, id int64, need string) (projectCtx, db.Attachment, bool) {
	if s.requireUser(w, r) == nil {
		return projectCtx{}, db.Attachment{}, false
	}
	row, err := s.q.GetAttachment(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Attachment not found")
		return projectCtx{}, db.Attachment{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.Attachment{}, false
	}
	pc, _, ok := s.ticketFor(w, r, row.TicketKey, need)
	if !ok {
		return projectCtx{}, db.Attachment{}, false
	}
	return pc, row.Attachment, true
}

// filePart finds the multipart field named file.
func filePart(r *http.Request) (*multipart.Part, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	for {
		p, err := mr.NextPart()
		if err != nil {
			return nil, err // io.EOF: the form has no file field
		}
		if p.FormName() == "file" {
			return p, nil
		}
		_ = p.Close()
	}
}

// storeFile copies at most limit bytes into the attachments directory, named
// by their SHA-256. A longer file is errTooLarge and leaves nothing behind.
func (s *Server) storeFile(src io.Reader, limit int64) ([]byte, int64, error) {
	tmp, err := os.CreateTemp(s.cfg.AttachmentsDir, "upload-*")
	if err != nil {
		return nil, 0, err
	}
	defer os.Remove(tmp.Name()) // a no-op once the file is renamed into place
	defer tmp.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(src, limit+1))
	var tooBig *http.MaxBytesError
	if n > limit || errors.As(err, &tooBig) {
		return nil, 0, errTooLarge
	}
	if err != nil {
		return nil, 0, err
	}
	if err := tmp.Close(); err != nil {
		return nil, 0, err
	}
	sum := h.Sum(nil)
	path := s.attachmentPath(sum)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, 0, err
	}
	return sum, n, os.Rename(tmp.Name(), path)
}

// attachmentPath is <ATTACHMENTS_DIR>/<hex[:2]>/<hex> (FSD §8.7).
func (s *Server) attachmentPath(sum []byte) string {
	h := hex.EncodeToString(sum)
	return filepath.Join(s.cfg.AttachmentsDir, h[:2], h)
}
```

- [ ] **Step 6: Generate and run the tests**

```bash
cd server && go generate ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...
cd ../web && npm run gen:api
```

Expected: `ok` for every Go package.

- [ ] **Step 7: Commit**

```bash
git add api/openapi.yaml server deploy/compose.yaml web/lib/api-types.ts
git commit -m "feat(server): ticket attachments stored by SHA-256"
```

### Task 7: Archived nodes, clients held by tickets and internal contacts

**Files:**
- Modify: `api/openapi.yaml`:
  - `Node` gains `archived`, and `NodeUpdate` gains `archived`;
  - `listNodes` gains the query parameter `archived`, and `listContacts` gains `internal`.
- Modify: `server/internal/httpapi/nodes.go`, `clients.go`, `contacts.go`
- Test: `server/internal/httpapi/nodes_archive_test.go`

**Interfaces:**
- Consumes: `UpdateNode` with `Archived`, `ListNodes` with `IncludeArchived`, `CountLiveChildren`, `ListContacts` with `Internal` (Task 1).
- Produces:
  - `ListNodes(w, r, key string, params ListNodesParams)`: project admins may pass `archived=true`.
  - `PATCH /nodes/{id}` with `{"archived": true|false}` archives and restores.
  - `DELETE /nodes/{id}` on a linked node answers 409 `node_linked`.
  - Archiving a node with live sub-nodes answers 409 `node_has_children`. Restoring under an archived parent answers 409 `parent_archived`.
  - `PUT /projects/{key}/clients` also refuses to drop a client that tickets use (409 `client_in_use`).
  - `GET /contacts?internal=true` lists only internal people.

- [ ] **Step 1: Extend the contract**

In `api/openapi.yaml`:

1. In `components.schemas.Node`, add `archived` to `required` and to `properties`:

```yaml
        archived: { type: boolean }
```

2. In `components.schemas.NodeUpdate`, add:

```yaml
        archived: { type: boolean, description: Archive (true) or restore (false); a linked node is archived, never deleted. }
```

3. Under `get` of `/projects/{key}/nodes`, add:

```yaml
      parameters:
        - { name: archived, in: query, schema: { type: boolean }, description: Project admins only; also list archived nodes. }
```

4. Under `get` of `/contacts`, add to its `parameters`:

```yaml
        - { name: internal, in: query, schema: { type: boolean }, description: Only internal people (no client). }
```

- [ ] **Step 2: Write the failing tests**

`server/internal/httpapi/nodes_archive_test.go`:

```go
package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// R-MR-4, AC-MR-5: a linked node is archived, never deleted; it leaves the tree
// and pickers, and its tickets keep the link.
func TestLinkedNodesAreArchivedNotDeleted(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, au := e.signedIn("admin@example.com", true)
	tk := e.seedTicket(w.p, au, "Overtime rules", &w.a, w.ot)
	path := fmt.Sprintf("/nodes/%d", w.ot.ID)
	var p httpapi.Problem
	if code := e.call(admin, http.MethodDelete, path, nil, &p); code != http.StatusConflict || p.Code != "node_linked" {
		t.Fatalf("delete a linked node: %d %+v", code, p)
	}
	var n httpapi.Node
	if code := e.call(admin, http.MethodPatch, path, map[string]any{"archived": true}, &n); code != http.StatusOK || !n.Archived {
		t.Fatalf("archive: %d %+v", code, n)
	}
	var list httpapi.NodeList
	e.call(admin, http.MethodGet, "/projects/HRIS/nodes", nil, &list)
	if slices.Contains(nodeNames(list), "Overtime Approval") {
		t.Fatalf("an archived node in the tree: %v", nodeNames(list))
	}
	e.call(admin, http.MethodGet, "/projects/HRIS/nodes?archived=true", nil, &list)
	if i := slices.Index(nodeNames(list), "Overtime Approval"); i < 0 || !list.Items[i].Archived {
		t.Fatalf("an admin lists archived nodes: %+v", list.Items)
	}
	var detail httpapi.Ticket
	if e.call(admin, http.MethodGet, "/tickets/"+tk.Key, nil, &detail); len(detail.Nodes) != 1 || !detail.Nodes[0].Archived {
		t.Fatalf("the ticket keeps the link: %+v", detail.Nodes)
	}
	if code := e.call(admin, http.MethodPatch, path, map[string]any{"archived": false}, &n); code != http.StatusOK || n.Archived {
		t.Fatalf("restore: %d %+v", code, n)
	}
}

func TestArchiveRules(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	node := func(n db.Node) string { return fmt.Sprintf("/nodes/%d", n.ID) }
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPatch, node(w.hr), map[string]any{"archived": true}, &p); code != http.StatusConflict || p.Code != "node_has_children" {
		t.Fatalf("archive a node with live children: %d %+v", code, p)
	}
	for _, n := range []db.Node{w.ot, w.secret, w.hr} {
		if code := e.call(admin, http.MethodPatch, node(n), map[string]any{"archived": true}, nil); code != http.StatusOK {
			t.Fatalf("archive %s: %d", n.Name, code)
		}
	}
	if code := e.call(admin, http.MethodPatch, node(w.ot), map[string]any{"archived": false}, &p); code != http.StatusConflict || p.Code != "parent_archived" {
		t.Fatalf("restore under an archived parent: %d %+v", code, p)
	}
	var list httpapi.NodeList
	if e.call(w.pm, http.MethodGet, "/projects/HRIS/nodes?archived=true", nil, &list); len(list.Items) != 0 {
		t.Fatalf("members never list archived nodes: %+v", list.Items)
	}
	body := map[string]any{"title": "Uses an archived menu", "type": "bug", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID}
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", body, &p); code != http.StatusUnprocessableEntity || firstError(p).Field != "node_ids" {
		t.Fatalf("a new ticket on an archived menu: %d %+v", code, p)
	}
}

func TestClientsUsedByTicketsStayLinked(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, au := e.signedIn("admin@example.com", true)
	c := e.seedClient("Client C")
	if err := e.q.LinkClients(context.Background(), db.LinkClientsParams{ProjectID: w.p.ID, ClientIds: []int64{c.ID}}); err != nil {
		t.Fatal(err)
	}
	e.seedTicket(w.p, au, "Client C export", &c)
	var p httpapi.Problem
	body := map[string]any{"client_ids": []int64{w.a.ID, w.b.ID}}
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/clients", body, &p); code != http.StatusConflict || p.Code != "client_in_use" {
		t.Fatalf("unlink a client with tickets: %d %+v", code, p)
	}
}

func TestInternalContactsFilter(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	e.seedContact("Dewi", nil)
	var list httpapi.ContactList
	e.call(w.pm, http.MethodGet, "/contacts?internal=true", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Name != "Dewi" {
		t.Fatalf("internal contacts: %+v", list.Items)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd server && go generate ./... && go vet ./internal/httpapi/ 2>&1 | head -5
```

Expected: FAIL. `*Server does not implement ServerInterface (wrong type for method ListNodes)`, because the generated method now takes `ListNodesParams`.

- [ ] **Step 4: Change the handlers**

In `server/internal/httpapi/nodes.go`:

1. `ListNodes` takes the parameters, lets project admins include archived nodes, and reports `archived`:

```go
func (s *Server) ListNodes(w http.ResponseWriter, r *http.Request, key string, params ListNodesParams) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.q.ListNodes(r.Context(), db.ListNodesParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
		IncludeArchived: deref(params.Archived) && pc.scope.Allows(access.Admin), // archived nodes are for restoring (R-MR-4)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]Node, len(rows))
	for i, n := range rows {
		clients := make([]NodeClient, len(n.ClientIds))
		for j, id := range n.ClientIds {
			clients[j] = NodeClient{Id: id, Name: n.ClientNames[j]}
		}
		items[i] = Node{
			Id: n.ID, ParentId: n.ParentID, Type: NodeType(n.Type), Name: n.Name, Code: n.Code,
			Aliases: orEmpty(n.Aliases), Description: n.Description, ClientSpecific: n.ClientSpecific,
			Clients: clients, Position: n.Position, Archived: n.Archived,
		}
	}
	writeJSON(w, http.StatusOK, NodeList{Items: items})
}
```

2. In `UpdateNode`, check the archive rules right after the `len(fields) > 0` return:

```go
	if in.Archived != nil && *in.Archived && n.ArchivedAt == nil {
		live, err := s.q.CountLiveChildren(r.Context(), id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if live > 0 {
			writeProblem(w, http.StatusConflict, "node_has_children", "Archive or move its sub-items first")
			return
		}
	}
	if in.Archived != nil && !*in.Archived && n.ArchivedAt != nil && n.ParentID != nil {
		parent, err := s.q.GetNode(r.Context(), *n.ParentID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if parent.ArchivedAt != nil {
			writeProblem(w, http.StatusConflict, "parent_archived", "Restore its parent first")
			return
		}
	}
```

   Then pass `Archived: in.Archived` in its `db.UpdateNodeParams`.

3. In `toAPINode`, set `Archived: n.ArchivedAt != nil`. In `nodeAudit`, add `"archived": n.ArchivedAt != nil` to the map.

4. In `nodeConflict`, add a case:

```go
	case "ticket_nodes_node_fk":
		writeProblem(w, http.StatusConflict, "node_linked", "Tickets link to this item; archive it instead")
```

In `server/internal/httpapi/clients.go`, `SetProjectClients` treats a client held by tickets like one held by menus or scopes:

```go
	case "membership_clients_linked", "node_clients_linked", "tickets_client_linked":
		writeProblem(w, http.StatusConflict, "client_in_use", "A client you removed is still used by tickets, menus or member scopes in this project")
		return
```

In `server/internal/httpapi/contacts.go`, `ListContacts` passes `Internal: deref(params.Internal)` in its `db.ListContactsParams`.

- [ ] **Step 5: Generate and run the tests**

```bash
cd server && go generate ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...
cd ../web && npm run gen:api
```

Expected: `ok` for every Go package.

- [ ] **Step 6: Commit**

```bash
git add api/openapi.yaml server web/lib/api-types.ts
git commit -m "feat(server): archive linked nodes, keep clients that tickets use"
```

### Task 8: The permission suite covers tickets

**Files:**
- Replace: `server/internal/httpapi/permission_test.go`

**Interfaces:**
- Consumes: every endpoint of Tasks 2–7; `seedTicket`, `upload`, `callWith`.
- Produces: the suite for Iteration 2. It seeds four HRIS tickets (core, Client A, Client B, Client C) and one PAY ticket (Client C), plus a Client B attachment. It checks the lists, one ticket, its activity and its file as every user, and it checks the ticket, comment and attachment writes.

Tasks 2–7 already built this behavior. If a row fails, fix the handler, not the table.

- [ ] **Step 1: Replace the suite**

`server/internal/httpapi/permission_test.go`:

```go
package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// The permission suite (FSD §5.3, §21.1) seeds two projects, three clients and
// four scoped users, calls every endpoint as every user and fails on any row
// outside the user's scope, or missing from it. CI runs it on every change, so
// it blocks merges. New endpoints add rows to the tables below.
//
//	HRIS (clients A, B, C)                 PAY (clients A, C)
//	  hana   project admin                   citra  member, Client C
//	  ani    member, all clients             dodi   member, all clients
//	  budi   member, Client B
//	  citra  viewer, Clients A and C
//	plus admin, a system admin who belongs to no project.
//
//	Tickets: HRIS-1 core, HRIS-2 Client A, HRIS-3 Client B (with a file),
//	HRIS-4 Client C, PAY-1 Client C.

var suiteUsers = []string{"admin", "hana", "ani", "budi", "citra", "dodi"}

type world struct {
	as         map[string]*http.Client
	users      map[string]db.User
	clients    map[string]db.Client
	nodes      map[string]db.Node
	contacts   map[string]int64
	fileB      int64 // an attachment of HRIS-3
	inProgress int64 // HRIS's "In progress" status
}

func seedWorld(e *env) world {
	w := world{as: map[string]*http.Client{}, users: map[string]db.User{}, clients: map[string]db.Client{},
		nodes: map[string]db.Node{}, contacts: map[string]int64{}}
	for _, name := range []string{"A", "B", "C"} {
		w.clients[name] = e.seedClient("Client " + name)
	}
	a, b, c := w.clients["A"], w.clients["B"], w.clients["C"]
	hris := e.seedProject("HRIS", a, b, c)
	pay := e.seedProject("PAY", a, c)
	for _, name := range suiteUsers {
		w.as[name], w.users[name] = e.signedIn(name+"@example.com", name == "admin")
	}
	u := w.users
	e.seedMember(u["hana"], hris, "admin")
	e.seedMember(u["ani"], hris, "member")
	e.seedMember(u["budi"], hris, "member", b)
	e.seedMember(u["citra"], hris, "viewer", a, c)
	e.seedMember(u["citra"], pay, "member", c)
	e.seedMember(u["dodi"], pay, "member")

	node := func(p db.Project, parent, typ, name string, clients ...db.Client) {
		var up *db.Node
		if parent != "" {
			n := w.nodes[parent]
			up = &n
		}
		w.nodes[name] = e.seedNode(p, up, typ, name, clients...)
	}
	node(hris, "", "module", "HR")
	node(hris, "HR", "module", "Attendance")
	node(hris, "Attendance", "menu", "Overtime Approval", a)
	node(hris, "Overtime Approval", "menu", "OT Rules")
	node(hris, "Attendance", "menu", "Leave Request")
	node(hris, "HR", "menu", "Payslip Export", b, c)
	node(hris, "", "module", "Core")
	node(pay, "", "module", "Payroll")
	node(pay, "Payroll", "menu", "Run Payroll", c)

	w.contacts["Dewi"] = e.seedContact("Dewi", nil) // internal
	w.contacts["Andi"] = e.seedContact("Andi", &a)
	w.contacts["Bayu"] = e.seedContact("Bayu", &b)
	w.contacts["Cahya"] = e.seedContact("Cahya", &c)

	e.seedTicket(hris, u["hana"], "Core fix", nil)
	e.seedTicket(hris, u["hana"], "Client A request", &a)
	e.seedTicket(hris, u["hana"], "Client B request", &b)
	e.seedTicket(hris, u["hana"], "Client C request", &c)
	e.seedTicket(pay, u["dodi"], "PAY Client C request", &c)
	_, file, _ := upload(e, w.as["hana"], "HRIS-3", "b.txt", []byte("for Client B"))
	w.fileB = file.Id
	statuses, err := e.q.ListStatuses(context.Background(), hris.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	w.inProgress = statuses[1].ID
	return w
}

// listed returns the sorted keys (projects, tickets) or names of a list response.
func listed(e *env, c *http.Client, path string) ([]string, int) {
	var list struct {
		Items []struct{ Key, Name string } `json:"items"`
	}
	code := e.call(c, http.MethodGet, path, nil, &list)
	out := []string{}
	for _, it := range list.Items {
		if it.Key != "" {
			out = append(out, it.Key)
		} else {
			out = append(out, it.Name)
		}
	}
	slices.Sort(out)
	return out, code
}

func TestPermissionSuiteReads(t *testing.T) {
	e := newEnv(t)
	w := seedWorld(e)
	hrisTree := []string{"Attendance", "Core", "HR", "Leave Request", "OT Rules", "Overtime Approval", "Payslip Export"}
	payTree := []string{"Payroll", "Run Payroll"}
	allContacts := []string{"Andi", "Bayu", "Cahya", "Dewi"}
	abc := []string{"Client A", "Client B", "Client C"}
	hrisMembers := []string{"ani@example.com", "budi@example.com", "citra@example.com", "hana@example.com"}
	hrisTickets := []string{"HRIS-1", "HRIS-2", "HRIS-3", "HRIS-4"}
	assignees := []string{"ani@example.com", "budi@example.com", "hana@example.com"}
	statuses := []string{"Cancelled", "Done", "In progress", "In review", "To do"}
	for _, c := range []struct {
		path string
		see  map[string][]string // these users get exactly these rows
		deny map[string]int      // the others get this status
	}{
		{"/projects", map[string][]string{
			"admin": {"HRIS", "PAY"}, "hana": {"HRIS"}, "ani": {"HRIS"}, "budi": {"HRIS"}, "citra": {"HRIS", "PAY"}, "dodi": {"PAY"},
		}, nil},
		{"/projects/HRIS/nodes", map[string][]string{
			"admin": hrisTree, "hana": hrisTree, "ani": hrisTree, "citra": hrisTree,
			"budi": {"Attendance", "Core", "HR", "Leave Request", "Payslip Export"},
		}, map[string]int{"dodi": 404}},
		{"/projects/PAY/nodes", map[string][]string{"admin": payTree, "citra": payTree, "dodi": payTree},
			map[string]int{"hana": 404, "ani": 404, "budi": 404}},
		{"/contacts", map[string][]string{
			"admin": allContacts, "hana": allContacts, "ani": allContacts,
			"budi": {"Bayu", "Dewi"}, "citra": {"Andi", "Cahya", "Dewi"}, "dodi": {"Andi", "Cahya", "Dewi"},
		}, nil},
		{"/clients", map[string][]string{"admin": abc, "hana": abc},
			map[string]int{"ani": 403, "budi": 403, "citra": 403, "dodi": 403}},
		{"/projects/HRIS/clients", map[string][]string{
			"admin": abc, "hana": abc, "ani": abc, "budi": {"Client B"}, "citra": {"Client A", "Client C"},
		}, map[string]int{"dodi": 404}},
		{"/projects/HRIS/members", map[string][]string{"admin": hrisMembers, "hana": hrisMembers},
			map[string]int{"ani": 403, "budi": 403, "citra": 403, "dodi": 404}},
		{"/projects/HRIS/tickets", map[string][]string{
			"admin": hrisTickets, "hana": hrisTickets, "ani": hrisTickets,
			"budi": {"HRIS-1", "HRIS-3"}, "citra": {"HRIS-1", "HRIS-2", "HRIS-4"},
		}, map[string]int{"dodi": 404}},
		{"/projects/PAY/tickets", map[string][]string{"admin": {"PAY-1"}, "citra": {"PAY-1"}, "dodi": {"PAY-1"}},
			map[string]int{"hana": 404, "ani": 404, "budi": 404}},
		{"/projects/HRIS/assignees", map[string][]string{
			"admin": assignees, "hana": assignees, "ani": assignees, "budi": assignees, "citra": assignees,
		}, map[string]int{"dodi": 404}},
		{"/projects/HRIS/statuses", map[string][]string{
			"admin": statuses, "hana": statuses, "ani": statuses, "budi": statuses, "citra": statuses,
		}, map[string]int{"dodi": 404}},
	} {
		for _, user := range suiteUsers {
			got, code := listed(e, w.as[user], c.path)
			if want, ok := c.see[user]; ok {
				if code != http.StatusOK || !slices.Equal(got, want) {
					t.Errorf("GET %s as %s: %d %v, want %v", c.path, user, code, got, want)
				}
			} else if code != c.deny[user] {
				t.Errorf("GET %s as %s: %d, want %d", c.path, user, code, c.deny[user])
			}
		}
	}
	// Single things: the users who may open them get 200, everyone else 404.
	for path, see := range map[string][]string{
		"/projects/HRIS":                     {"admin", "hana", "ani", "budi", "citra"},
		"/tickets/HRIS-2":                    {"admin", "hana", "ani", "citra"},
		"/tickets/HRIS-2/activity":           {"admin", "hana", "ani", "citra"},
		"/tickets/HRIS-3":                    {"admin", "hana", "ani", "budi"},
		fmt.Sprintf("/attachments/%d", w.fileB): {"admin", "hana", "ani", "budi"},
	} {
		for _, user := range suiteUsers {
			want := http.StatusNotFound
			if slices.Contains(see, user) {
				want = http.StatusOK
			}
			if code := e.call(w.as[user], http.MethodGet, path, nil, nil); code != want {
				t.Errorf("GET %s as %s: %d, want %d", path, user, code, want)
			}
		}
	}
	// R-MR-8: a client-specific menu names only in-scope clients.
	for user, want := range map[string][]string{"ani": {"Client B", "Client C"}, "budi": {"Client B"}, "citra": {"Client C"}} {
		var list httpapi.NodeList
		e.call(w.as[user], http.MethodGet, "/projects/HRIS/nodes", nil, &list)
		for _, n := range list.Items {
			if n.Name != "Payslip Export" {
				continue
			}
			var got []string
			for _, c := range n.Clients {
				got = append(got, c.Name)
			}
			if !slices.Equal(got, want) {
				t.Errorf("Payslip Export clients as %s: %v, want %v", user, got, want)
			}
		}
	}
}

func TestPermissionSuiteWrites(t *testing.T) {
	e := newEnv(t)
	w := seedWorld(e)
	node := func(name string) string { return fmt.Sprintf("/nodes/%d", w.nodes[name].ID) }
	contact := func(name string) string { return fmt.Sprintf("/contacts/%d", w.contacts[name]) }
	client := func(name string) string { return fmt.Sprintf("/clients/%d", w.clients[name].ID) }
	menu := map[string]any{"type": "menu", "name": "X"}
	coreTicket := map[string]any{"title": "Core clean-up", "type": "bug", "node_ids": []int64{}}
	aTicket := map[string]any{"title": "Client A change", "type": "bug", "node_ids": []int64{}, "client_id": w.clients["A"].ID}
	edit := map[string]any{"title": "Taken over", "type": "bug", "node_ids": []int64{}, "requester_user_id": w.users["budi"].ID}
	ifMatch := map[string]string{"If-Match": `"1"`}
	for _, c := range []struct {
		user, method, path string
		headers            map[string]string
		body               any
		want               int
	}{
		{"hana", http.MethodPost, "/projects", nil, map[string]any{"key": "NEW", "name": "New"}, 403},
		{"ani", http.MethodPatch, "/projects/HRIS", nil, map[string]any{"name": "X"}, 403},
		{"dodi", http.MethodPatch, "/projects/HRIS", nil, map[string]any{"name": "X"}, 404},
		{"ani", http.MethodPut, "/projects/HRIS/clients", nil, map[string]any{"client_ids": []int64{}}, 403},
		{"ani", http.MethodPut, "/projects/HRIS/members", nil, map[string]any{"members": []any{}}, 403},
		{"ani", http.MethodPut, "/projects/HRIS/statuses", nil, map[string]any{"statuses": []any{}}, 403},
		{"hana", http.MethodPatch, client("A"), nil, map[string]any{"name": "X"}, 403},
		{"ani", http.MethodPost, "/clients", nil, map[string]any{"name": "X"}, 403},
		{"ani", http.MethodPost, "/projects/HRIS/nodes", nil, menu, 403},
		{"dodi", http.MethodPost, "/projects/HRIS/nodes", nil, menu, 404},
		{"ani", http.MethodPatch, node("Leave Request"), nil, map[string]any{"name": "X"}, 403},
		{"budi", http.MethodPatch, node("Overtime Approval"), nil, map[string]any{"name": "X"}, 404}, // hidden by scope
		{"budi", http.MethodDelete, node("OT Rules"), nil, nil, 404},                                 // under a hidden menu
		{"dodi", http.MethodDelete, node("Leave Request"), nil, nil, 404},
		{"budi", http.MethodPatch, contact("Andi"), nil, map[string]any{"name": "X"}, 404},
		{"citra", http.MethodPatch, contact("Andi"), nil, map[string]any{"name": "X"}, 403}, // a viewer for Client A
		{"budi", http.MethodPost, "/contacts", nil, map[string]any{"name": "X", "client_id": w.clients["A"].ID}, 422},
		{"dodi", http.MethodPost, "/contacts", nil, map[string]any{"name": "X", "client_id": w.clients["B"].ID}, 422}, // B is no PAY client
		{"budi", http.MethodPost, "/projects/HRIS/tickets", nil, aTicket, 422}, // Client A is outside budi's scope
		{"citra", http.MethodPost, "/projects/HRIS/tickets", nil, coreTicket, 403},
		{"dodi", http.MethodPost, "/projects/HRIS/tickets", nil, coreTicket, 404},
		{"budi", http.MethodPut, "/tickets/HRIS-2", ifMatch, edit, 404},
		{"citra", http.MethodPost, "/tickets/HRIS-2/transition", nil, map[string]any{"status_id": w.inProgress}, 403},
		{"dodi", http.MethodPost, "/tickets/HRIS-1/transition", nil, map[string]any{"status_id": w.inProgress}, 404},
		{"citra", http.MethodPost, "/tickets/HRIS-2/comments", nil, map[string]any{"body": "Seen"}, 403},
		{"budi", http.MethodPost, "/tickets/HRIS-2/comments", nil, map[string]any{"body": "Hi"}, 404},
		{"citra", http.MethodDelete, fmt.Sprintf("/attachments/%d", w.fileB), nil, nil, 404},
		// Allowed, as controls: the suite must not pass by refusing everything.
		{"hana", http.MethodPatch, node("Leave Request"), nil, map[string]any{"name": "Leave Requests"}, 200},
		{"citra", http.MethodPatch, contact("Cahya"), nil, map[string]any{"name": "Cahya", "client_id": w.clients["C"].ID}, 200},
		{"budi", http.MethodPost, "/projects/HRIS/tickets", nil, coreTicket, 201},
		{"ani", http.MethodPost, "/tickets/HRIS-3/comments", nil, map[string]any{"body": "Checked"}, 201},
		{"budi", http.MethodPost, "/tickets/HRIS-3/transition", nil, map[string]any{"status_id": w.inProgress}, 200},
	} {
		if code, _ := e.callWith(w.as[c.user], c.method, c.path, c.headers, c.body, nil); code != c.want {
			t.Errorf("%s %s as %s: %d, want %d", c.method, c.path, c.user, code, c.want)
		}
	}
}
```

- [ ] **Step 2: Run the suite**

```bash
cd server && gofmt -w internal/httpapi/permission_test.go && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./internal/httpapi/ -run PermissionSuite -v
```

Expected: `--- PASS: TestPermissionSuiteReads` and `--- PASS: TestPermissionSuiteWrites`.

- [ ] **Step 3: Check that the suite bites**

Break the ticket scope on purpose. In `ticket_list.go`, pass `AllClients: true` instead of `pc.scope.AllClients`. Run the suite: `GET /projects/HRIS/tickets as budi` must fail. Then restore the line with `git checkout -- server/internal/httpapi/ticket_list.go`.

- [ ] **Step 4: Commit**

```bash
git add server/internal/httpapi/permission_test.go
git commit -m "test(server): permission suite covers tickets, comments and attachments"
```

### Task 9: Web strings, types, project tabs and statuses

**Files:**
- Modify: `web/messages/en.json`, `web/messages/id.json` (merged from the two temporary files below)
- Modify: `web/lib/problem.ts` (ticket types); Create: `web/lib/format.ts`, `web/lib/ticket-query.ts`
- Modify: `web/app/p/[key]/layout.tsx` (tabs), `web/app/page.tsx` (projects open on the board)
- Create: `web/app/p/[key]/settings/StatusesForm.tsx`; Modify: `web/app/p/[key]/settings/page.tsx`

**Interfaces:**
- Consumes: the statuses API (Task 2).
- Produces:
  - Message namespaces `ticketFilters`, `ticketTypes`, `priorities`, `board`, `tickets`, `ticketForm`, `ticket`, `activity`, `attachments`, `statuses`, plus new keys in `project`, `modules` and `errors`.
  - Types `Status`, `Ticket`, `TicketSummary`, `TicketType`, `Priority`, `ActivityItem`, `Ref`, `Contact`.
  - `utc(iso)` and `fileSize(bytes)` from `@/lib/format`; `one(searchParams)` and `ticketQuery(values)` from `@/lib/ticket-query`.
  - Project tabs "Papan", "Tiket", "Modul" and "Pengaturan".

- [ ] **Step 1: Add the strings**

Create `web/messages/en.add.json`:

```json
{
  "project": { "board": "Board", "tickets": "Tickets" },
  "ticketTypes": { "bug": "Bug", "change_request": "Change request", "feature": "Feature" },
  "priorities": { "low": "Low", "medium": "Medium", "high": "High", "urgent": "Urgent" },
  "ticketFilters": {
    "label": "Filter tickets",
    "q": "Search",
    "qPlaceholder": "Title or key",
    "client": "Client",
    "allClients": "All clients",
    "core": "Core (all clients)",
    "type": "Type",
    "anyType": "Any type",
    "assignee": "Assignee",
    "anyone": "Anyone",
    "mine": "Only mine",
    "status": "Status",
    "anyStatus": "Any status",
    "missing": "Missing",
    "nothingMissing": "—",
    "missingReason": "No reason",
    "missingMenus": "No menus",
    "sort": "Sort",
    "sortUpdated": "Recently updated",
    "sortCreated": "Newest",
    "sortKey": "Key",
    "sortPriority": "Priority",
    "sortDue": "Due date",
    "apply": "Apply",
    "reset": "Reset"
  },
  "board": {
    "newTicket": "New ticket",
    "addHere": "Add a ticket to {status}",
    "missing": "Reason or menus missing",
    "closedLater": "Closing comes with the close dialog.",
    "moveTo": "Status of {key}",
    "noClient": "Core"
  },
  "tickets": {
    "new": "New ticket",
    "key": "Key",
    "title": "Title",
    "type": "Type",
    "status": "Status",
    "client": "Client",
    "core": "Core",
    "assignee": "Assignee",
    "requestedBy": "Requested by",
    "menus": "Menus",
    "priority": "Priority",
    "updated": "Updated",
    "due": "Due",
    "none": "No tickets match these filters.",
    "next": "Next page"
  },
  "ticketForm": {
    "newTitle": "New ticket",
    "editTitle": "Edit {key}",
    "viewersCannot": "Viewers cannot create tickets.",
    "client": "Client",
    "core": "All clients (core)",
    "requestedBy": "Requested by",
    "me": "Me",
    "contact": "A contact",
    "contactSelect": "Contact",
    "chooseContact": "Choose a contact",
    "addContact": "+ Add contact",
    "contactName": "Contact name",
    "contactTitle": "Title (optional)",
    "saveContact": "Add",
    "title": "Title",
    "menus": "Affected menus",
    "menusFilter": "Find a menu",
    "menusHint": "A ticket needs at least one menu before it can close.",
    "menuForOtherClients": "{menu} is marked for {clients} only.",
    "reason": "Reason",
    "reasonHint": "Why was this requested? A ticket needs a reason before it can close.",
    "type": "Type",
    "description": "Description",
    "more": "More",
    "assignee": "Assignee",
    "nobody": "Nobody",
    "priority": "Priority",
    "due": "Due date",
    "create": "Create",
    "createAnother": "Create and add another",
    "created": "{key} created.",
    "save": "Save",
    "cancel": "Cancel",
    "reload": "Reload"
  },
  "ticket": {
    "edit": "Edit",
    "status": "Status",
    "core": "All clients (core)",
    "assignee": "Assignee",
    "nobody": "Nobody",
    "due": "Due",
    "description": "Description",
    "reason": "Reason",
    "menus": "Affected menus",
    "none": "—",
    "archived": "archived",
    "requestedBy": "Requested by",
    "reporter": "Reporter",
    "created": "Created",
    "updated": "Updated",
    "missingClose": "Add a reason and at least one menu before closing."
  },
  "activity": {
    "title": "Activity",
    "all": "All",
    "comments": "Comments",
    "history": "History",
    "internal": "Internal",
    "clientSafe": "Client-safe",
    "deleted": "Comment deleted",
    "edited": "edited",
    "edit": "Edit",
    "delete": "Delete",
    "confirmDelete": "Delete this comment?",
    "save": "Save",
    "cancel": "Cancel",
    "placeholder": "Write a comment",
    "internalToggle": "Internal comment",
    "send": "Send",
    "system": "System",
    "earlier": "Earlier text",
    "details": "Details",
    "created": "{actor} created the ticket",
    "transition": "{actor} changed Status from {old} to {new}",
    "updated": "{actor} changed {fields}",
    "commentEdited": "{actor} edited a comment",
    "commentDeleted": "{actor} deleted a comment",
    "attachmentAdded": "{actor} attached {file}",
    "attachmentDeleted": "{actor} removed {file}",
    "fields": {
      "title": "Title",
      "type": "Type",
      "priority": "Priority",
      "reason": "Reason",
      "description": "Description",
      "client": "Client",
      "assignee": "Assignee",
      "due_date": "Due date",
      "requester": "Requested by",
      "menus": "Menus",
      "status": "Status"
    }
  },
  "attachments": {
    "title": "Attachments",
    "upload": "Attach a file",
    "remove": "Remove",
    "none": "No files yet.",
    "limit": "Up to 25 MB: images, PDF, Office files, text, CSV, logs or ZIP."
  },
  "statuses": {
    "title": "Statuses",
    "name": "Name",
    "category": "Category",
    "color": "Color",
    "default": "Default",
    "todo": "To do",
    "in_progress": "In progress",
    "done": "Done",
    "cancelled": "Cancelled",
    "add": "Add status",
    "remove": "Remove",
    "up": "Up",
    "down": "Down",
    "moveFrom": "Move the tickets of {name} to",
    "save": "Save statuses",
    "saved": "Saved"
  },
  "modules": {
    "archive": "Archive",
    "restore": "Restore",
    "showArchived": "Show archived",
    "hideArchived": "Hide archived",
    "archivedBadge": "Archived"
  },
  "errors": {
    "stale": "Someone updated this ticket a moment ago. Your text is still here: reload, then save again.",
    "close_unavailable": "Closing a ticket needs the close dialog, which is not available yet.",
    "status_in_use": "Tickets still use a status you removed. Choose where they move.",
    "status_name_taken": "Two statuses have the same name",
    "project_key_fixed": "The key cannot change once the project has tickets",
    "node_linked": "Tickets link to this item. Archive it instead.",
    "parent_archived": "Restore its parent first",
    "file_too_large": "File is larger than 25 MB",
    "file_type_not_allowed": "This file type cannot be attached",
    "invalid_upload": "The file could not be read. Try again."
  }
}
```

Create `web/messages/id.add.json`:

```json
{
  "project": { "board": "Papan", "tickets": "Tiket" },
  "ticketTypes": { "bug": "Bug", "change_request": "Permintaan perubahan", "feature": "Fitur" },
  "priorities": { "low": "Rendah", "medium": "Sedang", "high": "Tinggi", "urgent": "Mendesak" },
  "ticketFilters": {
    "label": "Saring tiket",
    "q": "Cari",
    "qPlaceholder": "Judul atau kunci",
    "client": "Klien",
    "allClients": "Semua klien",
    "core": "Inti (semua klien)",
    "type": "Jenis",
    "anyType": "Semua jenis",
    "assignee": "Penanggung jawab",
    "anyone": "Siapa saja",
    "mine": "Hanya milik saya",
    "status": "Status",
    "anyStatus": "Semua status",
    "missing": "Belum diisi",
    "nothingMissing": "—",
    "missingReason": "Tanpa alasan",
    "missingMenus": "Tanpa menu",
    "sort": "Urutkan",
    "sortUpdated": "Terakhir diubah",
    "sortCreated": "Terbaru",
    "sortKey": "Kunci",
    "sortPriority": "Prioritas",
    "sortDue": "Tenggat",
    "apply": "Terapkan",
    "reset": "Atur ulang"
  },
  "board": {
    "newTicket": "Tiket baru",
    "addHere": "Tambah tiket ke {status}",
    "missing": "Alasan atau menu belum diisi",
    "closedLater": "Penutupan hadir bersama dialog penutupan.",
    "moveTo": "Status {key}",
    "noClient": "Inti"
  },
  "tickets": {
    "new": "Tiket baru",
    "key": "Kunci",
    "title": "Judul",
    "type": "Jenis",
    "status": "Status",
    "client": "Klien",
    "core": "Inti",
    "assignee": "Penanggung jawab",
    "requestedBy": "Diminta oleh",
    "menus": "Menu",
    "priority": "Prioritas",
    "updated": "Diubah",
    "due": "Tenggat",
    "none": "Tidak ada tiket yang cocok dengan saringan ini.",
    "next": "Halaman berikutnya"
  },
  "ticketForm": {
    "newTitle": "Tiket baru",
    "editTitle": "Ubah {key}",
    "viewersCannot": "Pengamat tidak dapat membuat tiket.",
    "client": "Klien",
    "core": "Semua klien (inti)",
    "requestedBy": "Diminta oleh",
    "me": "Saya",
    "contact": "Seorang kontak",
    "contactSelect": "Kontak",
    "chooseContact": "Pilih kontak",
    "addContact": "+ Tambah kontak",
    "contactName": "Nama kontak",
    "contactTitle": "Jabatan (opsional)",
    "saveContact": "Tambah",
    "title": "Judul",
    "menus": "Menu terdampak",
    "menusFilter": "Cari menu",
    "menusHint": "Tiket perlu minimal satu menu sebelum dapat ditutup.",
    "menuForOtherClients": "{menu} ditandai khusus untuk {clients}.",
    "reason": "Alasan",
    "reasonHint": "Mengapa ini diminta? Tiket perlu alasan sebelum dapat ditutup.",
    "type": "Jenis",
    "description": "Deskripsi",
    "more": "Lainnya",
    "assignee": "Penanggung jawab",
    "nobody": "Belum ada",
    "priority": "Prioritas",
    "due": "Tenggat",
    "create": "Buat",
    "createAnother": "Buat dan tambah lagi",
    "created": "{key} dibuat.",
    "save": "Simpan",
    "cancel": "Batal",
    "reload": "Muat ulang"
  },
  "ticket": {
    "edit": "Ubah",
    "status": "Status",
    "core": "Semua klien (inti)",
    "assignee": "Penanggung jawab",
    "nobody": "Belum ada",
    "due": "Tenggat",
    "description": "Deskripsi",
    "reason": "Alasan",
    "menus": "Menu terdampak",
    "none": "—",
    "archived": "diarsipkan",
    "requestedBy": "Diminta oleh",
    "reporter": "Pelapor",
    "created": "Dibuat",
    "updated": "Diubah",
    "missingClose": "Tambahkan alasan dan minimal satu menu sebelum menutup."
  },
  "activity": {
    "title": "Aktivitas",
    "all": "Semua",
    "comments": "Komentar",
    "history": "Riwayat",
    "internal": "Internal",
    "clientSafe": "Aman untuk klien",
    "deleted": "Komentar dihapus",
    "edited": "diubah",
    "edit": "Ubah",
    "delete": "Hapus",
    "confirmDelete": "Hapus komentar ini?",
    "save": "Simpan",
    "cancel": "Batal",
    "placeholder": "Tulis komentar",
    "internalToggle": "Komentar internal",
    "send": "Kirim",
    "system": "Sistem",
    "earlier": "Teks sebelumnya",
    "details": "Rincian",
    "created": "{actor} membuat tiket",
    "transition": "{actor} mengubah Status dari {old} menjadi {new}",
    "updated": "{actor} mengubah {fields}",
    "commentEdited": "{actor} mengubah komentar",
    "commentDeleted": "{actor} menghapus komentar",
    "attachmentAdded": "{actor} melampirkan {file}",
    "attachmentDeleted": "{actor} menghapus lampiran {file}",
    "fields": {
      "title": "Judul",
      "type": "Jenis",
      "priority": "Prioritas",
      "reason": "Alasan",
      "description": "Deskripsi",
      "client": "Klien",
      "assignee": "Penanggung jawab",
      "due_date": "Tenggat",
      "requester": "Diminta oleh",
      "menus": "Menu",
      "status": "Status"
    }
  },
  "attachments": {
    "title": "Lampiran",
    "upload": "Lampirkan berkas",
    "remove": "Hapus",
    "none": "Belum ada berkas.",
    "limit": "Maksimal 25 MB: gambar, PDF, berkas Office, teks, CSV, log, atau ZIP."
  },
  "statuses": {
    "title": "Status",
    "name": "Nama",
    "category": "Kategori",
    "color": "Warna",
    "default": "Bawaan",
    "todo": "Akan dikerjakan",
    "in_progress": "Sedang dikerjakan",
    "done": "Selesai",
    "cancelled": "Dibatalkan",
    "add": "Tambah status",
    "remove": "Hapus",
    "up": "Naik",
    "down": "Turun",
    "moveFrom": "Pindahkan tiket {name} ke",
    "save": "Simpan status",
    "saved": "Tersimpan"
  },
  "modules": {
    "archive": "Arsipkan",
    "restore": "Pulihkan",
    "showArchived": "Tampilkan arsip",
    "hideArchived": "Sembunyikan arsip",
    "archivedBadge": "Diarsipkan"
  },
  "errors": {
    "stale": "Seseorang baru saja mengubah tiket ini. Teks Anda masih ada: muat ulang, lalu simpan lagi.",
    "close_unavailable": "Menutup tiket memerlukan dialog penutupan, yang belum tersedia.",
    "status_in_use": "Masih ada tiket dengan status yang dihapus. Pilih ke mana tiket itu dipindahkan.",
    "status_name_taken": "Dua status memakai nama yang sama",
    "project_key_fixed": "Kunci tidak dapat diubah setelah proyek memiliki tiket",
    "node_linked": "Item ini dipakai tiket. Arsipkan saja.",
    "parent_archived": "Pulihkan induknya terlebih dahulu",
    "file_too_large": "Berkas lebih besar dari 25 MB",
    "file_type_not_allowed": "Jenis berkas ini tidak dapat dilampirkan",
    "invalid_upload": "Berkas tidak dapat dibaca. Coba lagi."
  }
}
```

Merge both files into the catalogs, one namespace deep, then delete them:

```bash
cd web && node -e 'const fs=require("fs");for(const l of ["en","id"]){const p=`messages/${l}.json`,m=JSON.parse(fs.readFileSync(p)),add=JSON.parse(fs.readFileSync(`messages/${l}.add.json`));for(const k of Object.keys(add))m[k]={...(m[k]??{}),...add[k]};fs.writeFileSync(p,JSON.stringify(m,null,2)+"\n")}' && rm messages/en.add.json messages/id.add.json
```

- [ ] **Step 2: Add the types and helpers**

Append to `web/lib/problem.ts`:

```ts
export type Status = components["schemas"]["Status"];
export type Ticket = components["schemas"]["Ticket"];
export type TicketSummary = components["schemas"]["TicketSummary"];
export type TicketType = components["schemas"]["TicketType"];
export type Priority = components["schemas"]["Priority"];
export type ActivityItem = components["schemas"]["ActivityItem"];
export type Ref = components["schemas"]["Ref"];
export type Contact = components["schemas"]["Contact"];
```

`web/lib/format.ts`:

```ts
// Deterministic on server and browser, so hydration matches (profile timezones arrive in a later iteration).
export function utc(iso: string) {
  return `${iso.slice(0, 16).replace("T", " ")} UTC`;
}

export function fileSize(bytes: number) {
  return bytes < 1024 * 1024 ? `${Math.max(1, Math.round(bytes / 1024))} KB` : `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}
```

`web/lib/ticket-query.ts`:

```ts
import type { TicketType } from "./problem";

type SearchParams = Record<string, string | string[] | undefined>;

/** The first value of each search parameter, without empty ones. */
export function one(sp: SearchParams): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(sp)) {
    const first = Array.isArray(v) ? v[0] : v;
    if (first) out[k] = first;
  }
  return out;
}

/** Maps the filter bar's URL (client=core|id, assignee=me, ...) onto the ticket list API. */
export function ticketQuery(v: Record<string, string>) {
  return {
    q: v.q,
    client_id: v.client && v.client !== "core" ? Number(v.client) : undefined,
    core: v.client === "core" ? true : undefined,
    type: v.type as TicketType | undefined,
    mine: v.assignee === "me" ? true : undefined,
    status_id: v.status ? Number(v.status) : undefined,
    missing: v.missing as "reason" | "menus" | undefined,
    sort: v.sort as "updated" | "created" | "key" | "priority" | "due" | undefined,
    cursor: v.cursor,
  };
}
```

- [ ] **Step 3: Add the project tabs**

In `web/app/p/[key]/layout.tsx`, replace the `<nav>` with:

```tsx
      <nav aria-label={t("nav")} className="mt-4 flex gap-5 border-b text-sm">
        <Link href={`/p/${project.key}/board`} className="pb-2">{t("board")}</Link>
        <Link href={`/p/${project.key}/tickets`} className="pb-2">{t("tickets")}</Link>
        <Link href={`/p/${project.key}/modules`} className="pb-2">{t("modules")}</Link>
        {project.role === "admin" && (
          <Link href={`/p/${project.key}/settings`} className="pb-2">{t("settings")}</Link>
        )}
      </nav>
```

In `web/app/page.tsx`, projects open on their board: change `href={`/p/${p.key}/modules`}` to `href={`/p/${p.key}/board`}`.

- [ ] **Step 4: Edit statuses in the project settings**

`web/app/p/[key]/settings/StatusesForm.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Status } from "@/lib/problem";

type Row = Pick<Status, "name" | "category" | "color" | "is_default"> & { id?: number };

const toRow = ({ id, name, category, color, is_default }: Status): Row => ({ id, name, category, color, is_default });
const categories = ["todo", "in_progress", "done", "cancelled"] as const;

// The project's ordered statuses (R-TK-1, R-TK-2). The tickets of a removed
// status move to the chosen status in the same save (R-TK-4).
export default function StatusesForm({ projectKey, statuses }: { projectKey: string; statuses: Status[] }) {
  const t = useTranslations("statuses");
  const problemText = useProblemText();
  const router = useRouter();
  const [rows, setRows] = useState<Row[]>(() => statuses.map(toRow));
  const [moves, setMoves] = useState<Record<number, number>>({});
  const [status, setStatus] = useState("");
  const removed = statuses.filter((s) => !rows.some((r) => r.id === s.id));
  const kept = rows.filter((r): r is Row & { id: number } => r.id !== undefined);

  const update = (i: number, patch: Partial<Row>) =>
    setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : patch.is_default ? { ...r, is_default: false } : r)));
  const swap = (i: number, j: number) =>
    setRows((rs) => {
      const next = [...rs];
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });

  async function save() {
    const { data, error } = await api.PUT("/projects/{key}/statuses", {
      params: { path: { key: projectKey } },
      body: { statuses: rows, move_to: removed.filter((s) => moves[s.id]).map((s) => ({ from: s.id, to: moves[s.id] })) },
    });
    if (error) return setStatus(problemText(error));
    setRows(data.items.map(toRow));
    setMoves({});
    setStatus(t("saved"));
    router.refresh();
  }

  const input = "rounded border px-2 py-1";
  return (
    <section aria-labelledby="statuses-title" className="flex flex-col gap-4 rounded-lg border bg-white p-4">
      <h2 id="statuses-title" className="font-medium">{t("title")}</h2>
      <table className="w-full border-collapse text-left text-sm">
        <thead>
          <tr className="border-b">
            <th className="p-2">{t("name")}</th>
            <th className="p-2">{t("category")}</th>
            <th className="p-2">{t("color")}</th>
            <th className="p-2">{t("default")}</th>
            <th className="p-2" />
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={r.id ?? `new-${i}`} className="border-b">
              <td className="p-2">
                <input aria-label={t("name")} value={r.name} maxLength={50} onChange={(e) => update(i, { name: e.target.value })} className={input} />
              </td>
              <td className="p-2">
                <select aria-label={t("category")} value={r.category} onChange={(e) => update(i, { category: e.target.value as Row["category"] })} className={input}>
                  {categories.map((c) => (
                    <option key={c} value={c}>{t(c)}</option>
                  ))}
                </select>
              </td>
              <td className="p-2">
                <input type="color" aria-label={t("color")} value={r.color} onChange={(e) => update(i, { color: e.target.value.toUpperCase() })} />
              </td>
              <td className="p-2">
                <input type="radio" name="default-status" aria-label={t("default")} checked={r.is_default} onChange={() => update(i, { is_default: true })} />
              </td>
              <td className="flex gap-2 p-2">
                <button type="button" disabled={i === 0} onClick={() => swap(i, i - 1)} className="underline disabled:opacity-40">{t("up")}</button>
                <button type="button" disabled={i === rows.length - 1} onClick={() => swap(i, i + 1)} className="underline disabled:opacity-40">{t("down")}</button>
                <button type="button" onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))} className="underline">{t("remove")}</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <button
        type="button"
        onClick={() => setRows((rs) => [...rs, { name: "", category: "in_progress", color: "#6B7280", is_default: false }])}
        className="self-start rounded border px-3 py-1 text-sm"
      >
        {t("add")}
      </button>
      {removed.map((s) => (
        <label key={s.id} className="flex items-center gap-2 text-sm">
          {t("moveFrom", { name: s.name })}
          <select value={moves[s.id] ?? ""} onChange={(e) => setMoves((m) => ({ ...m, [s.id]: Number(e.target.value) }))} className={input}>
            <option value="">—</option>
            {kept.map((r) => (
              <option key={r.id} value={r.id}>{r.name}</option>
            ))}
          </select>
        </label>
      ))}
      <div className="flex items-center gap-3">
        <button type="button" onClick={save} className="rounded bg-neutral-900 px-4 py-2 text-white">{t("save")}</button>
        {status && <p role="status" className="text-sm">{status}</p>}
      </div>
    </section>
  );
}
```

In `web/app/p/[key]/settings/page.tsx`:
- Import `StatusesForm from "./StatusesForm"`.
- Fetch the statuses with the other lists: `const [all, linked, members, statuses] = await Promise.all([... , api.GET("/projects/{key}/statuses", path)]);`.
- Render `<StatusesForm projectKey={key} statuses={statuses.data?.items ?? []} />` right after `<ProjectForm … />`.

- [ ] **Step 5: Build**

```bash
cd web && npm run build
```

Expected: the build and its type check succeed.

- [ ] **Step 6: Commit**

```bash
git add web
git commit -m "feat(web): ticket strings, project tabs and status settings"
```

### Task 10: The ticket form and the create page

**Files:**
- Create: `web/components/TicketForm.tsx`
- Create: `web/app/p/[key]/tickets/new/page.tsx`

**Interfaces:**
- Consumes: `POST /projects/{key}/tickets`, `PUT /tickets/{key}` (Task 3); `/contacts` with `client_id` and `internal`, and `POST /contacts` (Tasks 5 and 7); `/projects/{key}/clients`, `/nodes` and `/assignees` (Tasks 2 and 7); strings and types (Task 9).
- Produces: `TicketForm({projectKey, clients, nodes, assignees, ticket?, statusId?, onSaved?, onCancel?})`. Without `ticket` it creates, and "Create" opens the new ticket. With `ticket` it edits under `If-Match`. Its accessible names (Indonesian) are:
  - the form "Tiket baru";
  - the fields "Klien" and "Kontak", the radios "Saya" and "Seorang kontak", and the button "+ Tambah kontak";
  - "Nama kontak", "Jabatan (opsional)" and the button "Tambah";
  - "Judul", "Cari menu", one checkbox per menu named by its path (e.g. `HR › Overtime Approval`), "Alasan", "Jenis" and "Deskripsi";
  - "Lainnya", with "Penanggung jawab", "Prioritas" and "Tenggat";
  - the buttons "Buat" and "Buat dan tambah lagi".

- [ ] **Step 1: Write the form**

`web/components/TicketForm.tsx`:

```tsx
"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import {
  problemKey, useProblemText,
  type Client, type Contact, type Node, type Priority, type Ref, type Ticket, type TicketType,
} from "@/lib/problem";

type Props = {
  projectKey: string;
  clients: Client[];
  nodes: Node[];
  assignees: Ref[];
  ticket?: Ticket; // edit this ticket; without it the form creates one
  statusId?: number; // the board column a new ticket starts in
  onSaved?: () => void;
  onCancel?: () => void;
};

const types: TicketType[] = ["change_request", "bug", "feature"];
const priorities: Priority[] = ["low", "medium", "high", "urgent"];
const lastClientKey = (projectKey: string) => `muasal:last-client:${projectKey}`;

// The one form a PM fills while the client is on the phone (FSD §8.3):
// Client → Requested by → Title → Affected menus → Reason → Type → Description, and More.
export default function TicketForm({ projectKey, clients, nodes, assignees, ticket, statusId, onSaved, onCancel }: Props) {
  const t = useTranslations("ticketForm");
  const tTypes = useTranslations("ticketTypes");
  const tPri = useTranslations("priorities");
  const problemText = useProblemText();
  const router = useRouter();
  // Editing keeps a user requester; a contact requester can switch to the reporter.
  const userRequester = ticket
    ? ticket.requester.kind === "user" ? { id: ticket.requester.id, name: ticket.requester.name } : ticket.reporter
    : undefined;
  const [clientId, setClientId] = useState<number | null>(ticket?.client?.id ?? null);
  const [requester, setRequester] = useState<"user" | "contact">(ticket?.requester.kind === "contact" ? "contact" : "user");
  const [contactId, setContactId] = useState<number | null>(ticket?.requester.kind === "contact" ? ticket.requester.id : null);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [adding, setAdding] = useState(false);
  const [newName, setNewName] = useState("");
  const [newTitle, setNewTitle] = useState("");
  const [nodeIds, setNodeIds] = useState<Set<number>>(() => new Set(ticket?.nodes.map((n) => n.id)));
  const [menuFilter, setMenuFilter] = useState("");
  const [error, setError] = useState("");
  const [stale, setStale] = useState(false);
  const [notice, setNotice] = useState("");

  // A new ticket starts with the client picked last time (FSD §8.3).
  useEffect(() => {
    if (ticket) return;
    try {
      const last = localStorage.getItem(lastClientKey(projectKey));
      if (last && last !== "core" && clients.some((c) => c.id === Number(last))) setClientId(Number(last));
    } catch {
      // storage is a convenience
    }
  }, [ticket, projectKey, clients]);

  // The contacts of the chosen client, plus internal people.
  useEffect(() => {
    let live = true;
    (async () => {
      const own = clientId === null ? [] : ((await api.GET("/contacts", { params: { query: { client_id: clientId } } })).data?.items ?? []);
      const internal = (await api.GET("/contacts", { params: { query: { internal: true } } })).data?.items ?? [];
      if (live) setContacts([...own, ...internal]);
    })();
    return () => {
      live = false;
    };
  }, [clientId]);

  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
  const pathOf = (n: Node) => {
    const names = [n.name];
    for (let p = n.parent_id === null ? undefined : byId.get(n.parent_id); p; p = p.parent_id === null ? undefined : byId.get(p.parent_id)) {
      names.unshift(p.name);
    }
    return names.join(" › ");
  };
  const q = menuFilter.trim().toLowerCase();
  const menuChoices = nodes.filter((n) => !q || [pathOf(n), n.code ?? "", ...n.aliases].some((s) => s.toLowerCase().includes(q)));
  // R-MR-10: warn, without blocking, when a chosen menu belongs to other clients only.
  const warnings = clientId === null ? [] : nodes.filter((n) => nodeIds.has(n.id) && n.client_specific && !n.clients.some((c) => c.id === clientId));

  const toggleNode = (id: number, on: boolean) =>
    setNodeIds((s) => {
      const next = new Set(s);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });

  async function addContact() {
    const { data, error } = await api.POST("/contacts", {
      body: { name: newName, title: newTitle || undefined, client_id: clientId ?? undefined },
    });
    if (error) return setError(problemText(error));
    setContacts((cs) => [...cs, data]);
    setContactId(data.id);
    setAdding(false);
    setNewName("");
    setNewTitle("");
    setError("");
  }

  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const another = (e.nativeEvent as SubmitEvent).submitter?.getAttribute("value") === "another";
    const formEl = e.currentTarget;
    const form = new FormData(formEl);
    if (requester === "contact" && contactId === null) return setError(t("chooseContact"));
    const due = String(form.get("due_date") ?? "");
    const assignee = String(form.get("assignee_id") ?? "");
    const body = {
      type: String(form.get("type")) as TicketType,
      title: String(form.get("title")),
      client_id: clientId ?? undefined,
      requester_contact_id: requester === "contact" ? (contactId ?? undefined) : undefined,
      requester_user_id: requester === "user" ? userRequester?.id : undefined,
      node_ids: [...nodeIds],
      reason: String(form.get("reason") ?? ""),
      description: String(form.get("description") ?? ""),
      assignee_id: assignee ? Number(assignee) : undefined,
      priority: String(form.get("priority")) as Priority,
      due_date: due || undefined,
    };
    if (ticket) {
      const { error } = await api.PUT("/tickets/{key}", {
        params: { path: { key: ticket.key }, header: { "If-Match": `"${ticket.version}"` } },
        body,
      });
      if (error) {
        setStale(problemKey(error) === "stale"); // AC-TK-5: the user's text stays in the form
        return setError(problemText(error));
      }
      onSaved?.();
      return;
    }
    const { data, error } = await api.POST("/projects/{key}/tickets", {
      params: { path: { key: projectKey } },
      body: { ...body, status_id: statusId },
    });
    if (error) return setError(problemText(error));
    try {
      localStorage.setItem(lastClientKey(projectKey), clientId === null ? "core" : String(clientId));
    } catch {
      // storage is a convenience
    }
    if (another) {
      formEl.reset();
      setNodeIds(new Set());
      setError("");
      setNotice(t("created", { key: data.key }));
      return;
    }
    router.push(`/t/${data.key}`);
  }

  const input = "rounded border px-3 py-2";
  return (
    <form aria-label={ticket ? t("editTitle", { key: ticket.key }) : t("newTitle")} onSubmit={submit} className="flex flex-col gap-4">
      <label className="flex flex-col gap-1 text-sm">
        {t("client")}
        <select
          value={clientId ?? ""}
          onChange={(e) => {
            setClientId(e.target.value ? Number(e.target.value) : null);
            setContactId(null);
          }}
          className={input}
        >
          <option value="">{t("core")}</option>
          {clients.map((c) => (
            <option key={c.id} value={c.id}>{c.name}</option>
          ))}
        </select>
      </label>
      <fieldset className="flex flex-col gap-2 text-sm">
        <legend className="mb-1">{t("requestedBy")}</legend>
        <label className="flex items-center gap-2">
          <input type="radio" name="requester" checked={requester === "user"} onChange={() => setRequester("user")} />
          {userRequester?.name ?? t("me")}
        </label>
        <label className="flex items-center gap-2">
          <input type="radio" name="requester" checked={requester === "contact"} onChange={() => setRequester("contact")} />
          {t("contact")}
        </label>
        {requester === "contact" && (
          <div className="ml-6 flex flex-col gap-2">
            <label className="flex flex-col gap-1">
              {t("contactSelect")}
              <select value={contactId ?? ""} onChange={(e) => setContactId(e.target.value ? Number(e.target.value) : null)} className={input}>
                <option value="">{t("chooseContact")}</option>
                {contacts.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                    {c.title ? ` (${c.title})` : ""}
                    {c.client_name ? ` · ${c.client_name}` : ""}
                  </option>
                ))}
              </select>
            </label>
            {adding ? (
              <div className="flex flex-wrap items-end gap-2">
                <label className="flex flex-col gap-1">
                  {t("contactName")}
                  <input value={newName} onChange={(e) => setNewName(e.target.value)} maxLength={200} className={input} />
                </label>
                <label className="flex flex-col gap-1">
                  {t("contactTitle")}
                  <input value={newTitle} onChange={(e) => setNewTitle(e.target.value)} maxLength={200} className={input} />
                </label>
                <button type="button" onClick={addContact} disabled={!newName.trim()} className="rounded border px-3 py-2">
                  {t("saveContact")}
                </button>
              </div>
            ) : (
              <button type="button" onClick={() => setAdding(true)} className="self-start underline">{t("addContact")}</button>
            )}
          </div>
        )}
      </fieldset>
      <label className="flex flex-col gap-1 text-sm">
        {t("title")}
        <input name="title" defaultValue={ticket?.title} required minLength={5} maxLength={200} className={input} />
      </label>
      <fieldset className="flex flex-col gap-2 text-sm">
        <legend className="mb-1">{t("menus")}</legend>
        <input value={menuFilter} onChange={(e) => setMenuFilter(e.target.value)} aria-label={t("menusFilter")} placeholder={t("menusFilter")} className={input} />
        <div className="flex max-h-48 flex-col gap-1 overflow-y-auto rounded border p-2">
          {menuChoices.map((n) => (
            <label key={n.id} className="flex items-center gap-2">
              <input type="checkbox" checked={nodeIds.has(n.id)} onChange={(e) => toggleNode(n.id, e.target.checked)} />
              {pathOf(n)}
            </label>
          ))}
        </div>
        <p className="text-xs text-neutral-500">{t("menusHint")}</p>
        {warnings.map((n) => (
          <p key={n.id} className="text-xs text-amber-700">
            {t("menuForOtherClients", { menu: n.name, clients: n.clients.map((c) => c.name).join(", ") })}
          </p>
        ))}
      </fieldset>
      <label className="flex flex-col gap-1 text-sm">
        {t("reason")}
        <textarea name="reason" defaultValue={ticket?.reason} maxLength={2000} rows={3} aria-describedby="reason-hint" className={input} />
      </label>
      <p id="reason-hint" className="-mt-3 text-xs text-neutral-500">{t("reasonHint")}</p>
      <label className="flex flex-col gap-1 text-sm">
        {t("type")}
        <select name="type" defaultValue={ticket?.type ?? "change_request"} className={input}>
          {types.map((ty) => (
            <option key={ty} value={ty}>{tTypes(ty)}</option>
          ))}
        </select>
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("description")}
        <textarea name="description" defaultValue={ticket?.description} maxLength={50000} rows={5} className={input} />
      </label>
      <details className="text-sm" open={Boolean(ticket?.assignee || ticket?.due_date)}>
        <summary className="cursor-pointer">{t("more")}</summary>
        <div className="mt-3 flex flex-wrap gap-4">
          <label className="flex flex-col gap-1">
            {t("assignee")}
            <select name="assignee_id" defaultValue={ticket?.assignee?.id ?? ""} className={input}>
              <option value="">{t("nobody")}</option>
              {assignees.map((a) => (
                <option key={a.id} value={a.id}>{a.name}</option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1">
            {t("priority")}
            <select name="priority" defaultValue={ticket?.priority ?? "medium"} className={input}>
              {priorities.map((p) => (
                <option key={p} value={p}>{tPri(p)}</option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1">
            {t("due")}
            <input type="date" name="due_date" defaultValue={ticket?.due_date ?? ""} className={input} />
          </label>
        </div>
      </details>
      {error && (
        <p role="alert" className="text-sm text-red-700">
          {error}{" "}
          {stale && (
            <button type="button" onClick={() => router.refresh()} className="underline">{t("reload")}</button>
          )}
        </p>
      )}
      {notice && <p role="status" className="text-sm">{notice}</p>}
      <div className="flex gap-3">
        {ticket ? (
          <>
            <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("save")}</button>
            <button type="button" onClick={onCancel} className="rounded border px-4 py-2">{t("cancel")}</button>
          </>
        ) : (
          <>
            <button value="create" className="rounded bg-neutral-900 px-4 py-2 text-white">{t("create")}</button>
            <button value="another" className="rounded border px-4 py-2">{t("createAnother")}</button>
          </>
        )}
      </div>
    </form>
  );
}
```

- [ ] **Step 2: Write the create page**

`web/app/p/[key]/tickets/new/page.tsx`:

```tsx
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import TicketForm from "@/components/TicketForm";
import { getProject, serverApi } from "@/lib/server-api";

export default async function NewTicketPage({
  params,
  searchParams,
}: {
  params: Promise<{ key: string }>;
  searchParams: Promise<{ status_id?: string }>;
}) {
  const { key } = await params;
  const { status_id } = await searchParams;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("ticketForm");
  if (project.role === "viewer") return <p>{t("viewersCannot")}</p>;
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, nodes, assignees] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/nodes", path),
    api.GET("/projects/{key}/assignees", path),
  ]);
  return (
    <div className="max-w-3xl rounded-lg border bg-white p-6">
      <h2 className="mb-4 text-xl font-semibold">{t("newTitle")}</h2>
      <TicketForm
        projectKey={key}
        clients={clients.data?.items ?? []}
        nodes={nodes.data?.items ?? []}
        assignees={assignees.data?.items ?? []}
        statusId={status_id ? Number(status_id) : undefined}
      />
    </div>
  );
}
```

- [ ] **Step 3: Build**

```bash
cd web && npm run build
```

Expected: the build succeeds and lists `/p/[key]/tickets/new`.

- [ ] **Step 4: Commit**

```bash
git add web/components web/app/p
git commit -m "feat(web): the ticket form and the create page"
```

### Task 11: The ticket page

**Files:**
- Create: `web/app/t/[ticketKey]/page.tsx`, `TicketView.tsx`, `Activity.tsx`, `Attachments.tsx`

**Interfaces:**
- Consumes: `GET /tickets/{key}`, the transition, activity, comment and attachment endpoints (Tasks 3, 5, 6); `TicketForm` (Task 10); `utc`, `fileSize` (Task 9).
- Produces the page `/t/{key}`, the target of every link to a ticket:
  - a header with the key, the title and a status menu (closing statuses disabled);
  - the missing-close-fields banner;
  - the description, reason and menu chips; requester, reporter and dates;
  - "Ubah", which opens `TicketForm`;
  - the Activity feed, with the tabs Semua/Komentar/Riwayat and a comment box "Tulis komentar" whose "Komentar internal" is checked by default, sent with "Kirim";
  - Attachments, with "Lampirkan berkas".

- [ ] **Step 1: Write the page and the ticket view**

`web/app/t/[ticketKey]/page.tsx`:

```tsx
import Link from "next/link";
import { notFound } from "next/navigation";
import { getMe, getProject, serverApi } from "@/lib/server-api";
import Activity from "./Activity";
import Attachments from "./Attachments";
import TicketView from "./TicketView";

// The ticket page (FSD §8.6): the target of every citation and link to a ticket.
export default async function TicketPage({ params }: { params: Promise<{ ticketKey: string }> }) {
  const { ticketKey } = await params;
  const api = await serverApi();
  const { data: ticket } = await api.GET("/tickets/{key}", { params: { path: { key: ticketKey } } });
  if (!ticket) notFound();
  const [me, project] = await Promise.all([getMe(), getProject(ticket.project_key)]);
  if (!me || !project) notFound();
  const path = { params: { path: { key: project.key } } };
  const [statuses, activity, clients, nodes, assignees] = await Promise.all([
    api.GET("/projects/{key}/statuses", path),
    api.GET("/tickets/{key}/activity", { params: { path: { key: ticket.key } } }),
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/nodes", path),
    api.GET("/projects/{key}/assignees", path),
  ]);
  const canEdit = project.role !== "viewer";
  return (
    <main className="mx-auto max-w-6xl p-6">
      <Link href={`/p/${project.key}/board`} className="text-sm text-neutral-600 underline">
        {project.key} · {project.name}
      </Link>
      <TicketView
        ticket={ticket}
        statuses={statuses.data?.items ?? []}
        clients={clients.data?.items ?? []}
        nodes={nodes.data?.items ?? []}
        assignees={assignees.data?.items ?? []}
        canEdit={canEdit}
      />
      <div className="mt-8 grid gap-8 md:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <Activity ticketKey={ticket.key} items={activity.data?.items ?? []} meId={me.id} canComment={canEdit} />
        <Attachments
          ticketKey={ticket.key}
          files={ticket.attachments}
          meId={me.id}
          isProjectAdmin={project.role === "admin"}
          canUpload={canEdit}
        />
      </div>
    </main>
  );
}
```

`web/app/t/[ticketKey]/TicketView.tsx`:

```tsx
"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import TicketForm from "@/components/TicketForm";
import { api } from "@/lib/api";
import { utc } from "@/lib/format";
import { useProblemText, type Client, type Node, type Ref, type Status, type Ticket } from "@/lib/problem";

type Props = { ticket: Ticket; statuses: Status[]; clients: Client[]; nodes: Node[]; assignees: Ref[]; canEdit: boolean };

const closing = (s: Status) => s.category === "done" || s.category === "cancelled";

export default function TicketView({ ticket, statuses, clients, nodes, assignees, canEdit }: Props) {
  const t = useTranslations("ticket");
  const tTypes = useTranslations("ticketTypes");
  const tPri = useTranslations("priorities");
  const problemText = useProblemText();
  const router = useRouter();
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState("");
  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
  const pathOf = (id: number) => {
    const names: string[] = [];
    for (let n = byId.get(id); n; n = n.parent_id === null ? undefined : byId.get(n.parent_id)) names.unshift(n.name);
    return names.join(" › ");
  };

  async function transition(statusId: number) {
    const { error } = await api.POST("/tickets/{key}/transition", {
      params: { path: { key: ticket.key } },
      body: { status_id: statusId },
    });
    if (error) return setError(problemText(error));
    setError("");
    router.refresh();
  }

  return (
    <div className="mt-2">
      <div className="flex flex-wrap items-baseline gap-3">
        <span className="font-mono text-neutral-500">{ticket.key}</span>
        <h1 className="text-2xl font-semibold">{ticket.title}</h1>
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-4 text-sm">
        <label className="flex items-center gap-2">
          {t("status")}
          <select
            value={ticket.status.id}
            disabled={!canEdit}
            onChange={(e) => transition(Number(e.target.value))}
            className="rounded border px-2 py-1"
          >
            {statuses.map((s) => (
              <option key={s.id} value={s.id} disabled={closing(s)}>{s.name}</option>
            ))}
          </select>
        </label>
        <span>{tTypes(ticket.type)}</span>
        <span className="rounded bg-neutral-100 px-2">{ticket.client?.name ?? t("core")}</span>
        <span>{tPri(ticket.priority)}</span>
        <span>{t("assignee")}: {ticket.assignee?.name ?? t("nobody")}</span>
        {ticket.due_date && <span>{t("due")}: {ticket.due_date}</span>}
        {canEdit && !editing && (
          <button type="button" onClick={() => setEditing(true)} className="underline">{t("edit")}</button>
        )}
      </div>
      {error && <p role="alert" className="mt-2 text-sm text-red-700">{error}</p>}
      {(!ticket.reason || ticket.nodes.length === 0) && (
        <p className="mt-3 rounded border border-amber-300 bg-amber-50 p-2 text-sm">{t("missingClose")}</p>
      )}
      {editing ? (
        <div className="mt-4 rounded-lg border bg-white p-4">
          <TicketForm
            projectKey={ticket.project_key}
            ticket={ticket}
            clients={clients}
            nodes={nodes}
            assignees={assignees}
            onSaved={() => {
              setEditing(false);
              router.refresh();
            }}
            onCancel={() => setEditing(false)}
          />
        </div>
      ) : (
        <div className="mt-6 grid gap-6 md:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
          <div className="flex flex-col gap-4">
            <section>
              <h2 className="text-sm font-medium text-neutral-500">{t("description")}</h2>
              <p className="whitespace-pre-wrap">{ticket.description || t("none")}</p>
            </section>
            <section>
              <h2 className="text-sm font-medium text-neutral-500">{t("reason")}</h2>
              <p className="whitespace-pre-wrap">{ticket.reason || t("none")}</p>
            </section>
            <section>
              <h2 className="text-sm font-medium text-neutral-500">{t("menus")}</h2>
              <ul className="mt-1 flex flex-wrap gap-2">
                {ticket.nodes.map((n) => (
                  <li key={n.id} title={pathOf(n.id)} className="rounded bg-neutral-100 px-2 py-0.5 text-sm">
                    {n.name}
                    {n.archived ? ` (${t("archived")})` : ""}
                  </li>
                ))}
              </ul>
            </section>
          </div>
          <dl className="grid grid-cols-[auto_1fr] content-start gap-x-3 gap-y-2 text-sm">
            <dt className="text-neutral-500">{t("requestedBy")}</dt>
            <dd>{ticket.requester.name}{ticket.requester.title ? ` (${ticket.requester.title})` : ""}</dd>
            <dt className="text-neutral-500">{t("reporter")}</dt>
            <dd>{ticket.reporter.name}</dd>
            <dt className="text-neutral-500">{t("created")}</dt>
            <dd>{utc(ticket.created_at)}</dd>
            <dt className="text-neutral-500">{t("updated")}</dt>
            <dd>{utc(ticket.updated_at)}</dd>
          </dl>
        </div>
      )}
    </div>
  );
}
```

`web/app/t/[ticketKey]/Activity.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { utc } from "@/lib/format";
import { useProblemText, type ActivityItem } from "@/lib/problem";

type Change = { old?: unknown; new?: unknown };
type Filter = "all" | "comments" | "history";

const shown = (v: unknown) =>
  v === null || v === undefined || v === "" ? "—" : Array.isArray(v) ? v.join(", ") || "—" : String(v);

// A ticket's comments and history, oldest first (FSD §8.7).
export default function Activity({ ticketKey, items, meId, canComment }: {
  ticketKey: string;
  items: ActivityItem[];
  meId: number;
  canComment: boolean;
}) {
  const t = useTranslations("activity");
  const problemText = useProblemText();
  const router = useRouter();
  const [filter, setFilter] = useState<Filter>("all");
  const [editing, setEditing] = useState<number | null>(null);
  const [error, setError] = useState("");
  const visible = items.filter((it) => filter === "all" || (filter === "comments") === (it.kind === "comment"));
  const field = (k: string) => (t.has(`fields.${k}`) ? t(`fields.${k}`) : k);

  async function send(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formEl = e.currentTarget;
    const form = new FormData(formEl);
    const { error } = await api.POST("/tickets/{key}/comments", {
      params: { path: { key: ticketKey } },
      body: { body: String(form.get("body")), internal: form.get("internal") === "on" },
    });
    if (error) return setError(problemText(error));
    formEl.reset();
    setError("");
    router.refresh();
  }

  async function saveEdit(e: React.FormEvent<HTMLFormElement>, id: number) {
    e.preventDefault();
    const { error } = await api.PATCH("/comments/{id}", {
      params: { path: { id } },
      body: { body: String(new FormData(e.currentTarget).get("body")) },
    });
    if (error) return setError(problemText(error));
    setEditing(null);
    setError("");
    router.refresh();
  }

  async function remove(id: number) {
    if (!window.confirm(t("confirmDelete"))) return;
    const { error } = await api.DELETE("/comments/{id}", { params: { path: { id } } });
    if (error) return setError(problemText(error));
    router.refresh();
  }

  function describe(it: ActivityItem) {
    const actor = it.actor?.name ?? t("system");
    const c = (it.changes ?? {}) as Record<string, unknown>;
    switch (it.action) {
      case "create":
        return t("created", { actor });
      case "transition": {
        const s = c.status as Change;
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
      default:
        return `${actor}: ${it.action}`;
    }
  }

  // Updates list their old and new values; comment edits keep the earlier text (AC-TK-6).
  function details(it: ActivityItem) {
    const c = (it.changes ?? {}) as Record<string, Change>;
    if (it.action === "update") {
      return (
        <details className="mt-1">
          <summary className="cursor-pointer text-xs">{t("details")}</summary>
          <ul className="mt-1 text-xs">
            {Object.entries(c).map(([k, v]) => (
              <li key={k}>{field(k)}: {shown(v.old)} → {shown(v.new)}</li>
            ))}
          </ul>
        </details>
      );
    }
    if (it.action === "comment_edit") {
      return (
        <details className="mt-1">
          <summary className="cursor-pointer text-xs">{t("earlier")}</summary>
          <p className="mt-1 whitespace-pre-wrap text-xs">{shown(c.body?.old)}</p>
        </details>
      );
    }
    return null;
  }

  const tab = (f: Filter, label: string) => (
    <button type="button" aria-pressed={filter === f} onClick={() => setFilter(f)} className={filter === f ? "font-semibold underline" : "hover:underline"}>
      {label}
    </button>
  );

  return (
    <section aria-labelledby="activity-title" className="flex flex-col gap-3">
      <div className="flex items-center gap-4">
        <h2 id="activity-title" className="font-medium">{t("title")}</h2>
        <div className="flex gap-3 text-sm">
          {tab("all", t("all"))}
          {tab("comments", t("comments"))}
          {tab("history", t("history"))}
        </div>
      </div>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <ol className="flex flex-col gap-2">
        {visible.map((it, i) =>
          it.kind === "comment" ? (
            <li key={`c${it.comment_id}`} className="rounded border bg-white p-3 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{it.actor?.name}</span>
                <span className="rounded bg-neutral-100 px-1.5 text-xs">{it.internal ? t("internal") : t("clientSafe")}</span>
                <span className="text-xs text-neutral-500">{utc(it.at)}{it.edited ? ` · ${t("edited")}` : ""}</span>
              </div>
              {it.deleted ? (
                <p className="mt-1 italic text-neutral-500">{t("deleted")}{it.body ? `: ${it.body}` : ""}</p>
              ) : editing === it.comment_id ? (
                <form onSubmit={(e) => saveEdit(e, it.comment_id!)} className="mt-2 flex flex-col gap-2">
                  <textarea name="body" defaultValue={it.body} required maxLength={20000} rows={3} aria-label={t("edit")} className="rounded border px-3 py-2" />
                  <div className="flex gap-2">
                    <button className="rounded bg-neutral-900 px-3 py-1 text-white">{t("save")}</button>
                    <button type="button" onClick={() => setEditing(null)} className="rounded border px-3 py-1">{t("cancel")}</button>
                  </div>
                </form>
              ) : (
                <p className="mt-1 whitespace-pre-wrap">{it.body}</p>
              )}
              {!it.deleted && canComment && it.actor?.id === meId && editing !== it.comment_id && (
                <div className="mt-2 flex gap-3 text-xs">
                  <button type="button" onClick={() => setEditing(it.comment_id!)} className="underline">{t("edit")}</button>
                  <button type="button" onClick={() => remove(it.comment_id!)} className="underline">{t("delete")}</button>
                </div>
              )}
            </li>
          ) : (
            <li key={`e${i}`} className="text-sm text-neutral-600">
              {describe(it)} · <span className="text-xs">{utc(it.at)}</span>
              {details(it)}
            </li>
          ),
        )}
      </ol>
      {canComment && (
        <form onSubmit={send} className="flex flex-col gap-2">
          <textarea name="body" required maxLength={20000} rows={3} aria-label={t("placeholder")} placeholder={t("placeholder")} className="rounded border px-3 py-2" />
          <div className="flex items-center gap-3 text-sm">
            <label className="flex items-center gap-2">
              <input type="checkbox" name="internal" defaultChecked />
              {t("internalToggle")}
            </label>
            <button className="ml-auto rounded bg-neutral-900 px-4 py-2 text-white">{t("send")}</button>
          </div>
        </form>
      )}
    </section>
  );
}
```

`web/app/t/[ticketKey]/Attachments.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { fileSize } from "@/lib/format";
import { useProblemText, type Ticket } from "@/lib/problem";

type Props = { ticketKey: string; files: Ticket["attachments"]; meId: number; isProjectAdmin: boolean; canUpload: boolean };

// A ticket's files (FSD §8.7). Go serves each download after the ticket's visibility check.
export default function Attachments({ ticketKey, files, meId, isProjectAdmin, canUpload }: Props) {
  const t = useTranslations("attachments");
  const problemText = useProblemText();
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function upload(e: React.ChangeEvent<HTMLInputElement>) {
    const input = e.currentTarget;
    const file = input.files?.[0];
    if (!file) return;
    const form = new FormData();
    form.append("file", file);
    setBusy(true);
    // openapi-fetch sends JSON, so the multipart upload uses fetch directly (same origin, same cookie).
    const res = await fetch(`/api/v1/tickets/${encodeURIComponent(ticketKey)}/attachments`, { method: "POST", body: form });
    setBusy(false);
    input.value = "";
    if (!res.ok) return setError(problemText(await res.json().catch(() => undefined)));
    setError("");
    router.refresh();
  }

  async function remove(id: number) {
    const { error } = await api.DELETE("/attachments/{id}", { params: { path: { id } } });
    if (error) return setError(problemText(error));
    router.refresh();
  }

  return (
    <section aria-labelledby="attachments-title" className="flex flex-col gap-3">
      <h2 id="attachments-title" className="font-medium">{t("title")}</h2>
      {files.length === 0 ? (
        <p className="text-sm text-neutral-600">{t("none")}</p>
      ) : (
        <ul className="flex flex-col gap-1 text-sm">
          {files.map((f) => (
            <li key={f.id} className="flex items-center gap-2">
              <a href={`/api/v1/attachments/${f.id}`} className="underline">{f.filename}</a>
              <span className="text-xs text-neutral-500">{fileSize(f.size_bytes)}</span>
              {canUpload && (f.uploader.id === meId || isProjectAdmin) && (
                <button type="button" onClick={() => remove(f.id)} className="ml-auto text-xs underline">{t("remove")}</button>
              )}
            </li>
          ))}
        </ul>
      )}
      {canUpload && (
        <label className="flex flex-col gap-1 text-sm">
          {t("upload")}
          <input type="file" onChange={upload} disabled={busy} />
        </label>
      )}
      <p className="text-xs text-neutral-500">{t("limit")}</p>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
    </section>
  );
}
```

- [ ] **Step 2: Build**

```bash
cd web && npm run build
```

Expected: the build succeeds and lists `/t/[ticketKey]`.

- [ ] **Step 3: Commit**

```bash
git add web/app/t
git commit -m "feat(web): the ticket page with activity and attachments"
```

### Task 12: The ticket list and the board

**Files:**
- Create: `web/components/TicketFilters.tsx`
- Create: `web/app/p/[key]/tickets/page.tsx`
- Create: `web/app/p/[key]/board/page.tsx`, `web/app/p/[key]/board/Board.tsx`

**Interfaces:**
- Consumes: `GET /projects/{key}/tickets` (Task 4); `POST /tickets/{key}/transition` (Task 3); `one`, `ticketQuery`, `utc` (Task 9).
- Produces:
  - `/p/{key}/tickets`: a table with the filter bar ("Saring tiket") and a "Halaman berikutnya" link.
  - `/p/{key}/board`: one column per status, each a region named by its status. Cards are `article`s that can be dragged onto open columns, and each card also has a status menu, "Status <key>", for keyboards. Closing columns take no drops.

- [ ] **Step 1: Write the filter bar**

`web/components/TicketFilters.tsx`:

```tsx
import { getTranslations } from "next-intl/server";
import type { Client, Status } from "@/lib/problem";

type Props = {
  action: string;
  values: Record<string, string>;
  clients: Client[];
  statuses?: Status[]; // the list filters by status; the board shows every status anyway
};

// The filter bar of the board and the list (FSD §8.4, §8.5). It is a GET form,
// so every view is a URL people can share.
export default async function TicketFilters({ action, values, clients, statuses }: Props) {
  const t = await getTranslations("ticketFilters");
  const tTypes = await getTranslations("ticketTypes");
  const input = "rounded border px-2 py-1";
  return (
    <form method="get" action={action} aria-label={t("label")} className="flex flex-wrap items-end gap-3 text-sm">
      <label className="flex flex-col gap-1">
        {t("q")}
        <input name="q" defaultValue={values.q} placeholder={t("qPlaceholder")} className={input} />
      </label>
      <label className="flex flex-col gap-1">
        {t("client")}
        <select name="client" defaultValue={values.client ?? ""} className={input}>
          <option value="">{t("allClients")}</option>
          <option value="core">{t("core")}</option>
          {clients.map((c) => (
            <option key={c.id} value={c.id}>{c.name}</option>
          ))}
        </select>
      </label>
      <label className="flex flex-col gap-1">
        {t("type")}
        <select name="type" defaultValue={values.type ?? ""} className={input}>
          <option value="">{t("anyType")}</option>
          {(["bug", "change_request", "feature"] as const).map((ty) => (
            <option key={ty} value={ty}>{tTypes(ty)}</option>
          ))}
        </select>
      </label>
      <label className="flex flex-col gap-1">
        {t("assignee")}
        <select name="assignee" defaultValue={values.assignee ?? ""} className={input}>
          <option value="">{t("anyone")}</option>
          <option value="me">{t("mine")}</option>
        </select>
      </label>
      {statuses && (
        <>
          <label className="flex flex-col gap-1">
            {t("status")}
            <select name="status" defaultValue={values.status ?? ""} className={input}>
              <option value="">{t("anyStatus")}</option>
              {statuses.map((s) => (
                <option key={s.id} value={s.id}>{s.name}</option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1">
            {t("missing")}
            <select name="missing" defaultValue={values.missing ?? ""} className={input}>
              <option value="">{t("nothingMissing")}</option>
              <option value="reason">{t("missingReason")}</option>
              <option value="menus">{t("missingMenus")}</option>
            </select>
          </label>
          <label className="flex flex-col gap-1">
            {t("sort")}
            <select name="sort" defaultValue={values.sort ?? "updated"} className={input}>
              <option value="updated">{t("sortUpdated")}</option>
              <option value="created">{t("sortCreated")}</option>
              <option value="key">{t("sortKey")}</option>
              <option value="priority">{t("sortPriority")}</option>
              <option value="due">{t("sortDue")}</option>
            </select>
          </label>
        </>
      )}
      <button className="rounded bg-neutral-900 px-3 py-1 text-white">{t("apply")}</button>
      <a href={action} className="underline">{t("reset")}</a>
    </form>
  );
}
```

- [ ] **Step 2: Write the list page**

`web/app/p/[key]/tickets/page.tsx`:

```tsx
import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import TicketFilters from "@/components/TicketFilters";
import { utc } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { one, ticketQuery } from "@/lib/ticket-query";

// The ticket list (FSD §8.5): filters and pages live in the URL.
export default async function TicketsPage({
  params,
  searchParams,
}: {
  params: Promise<{ key: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { key } = await params;
  const values = one(await searchParams);
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("tickets");
  const tTypes = await getTranslations("ticketTypes");
  const tPri = await getTranslations("priorities");
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, statuses, page] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/statuses", path),
    api.GET("/projects/{key}/tickets", { params: { path: { key }, query: ticketQuery(values) } }),
  ]);
  const statusName = new Map((statuses.data?.items ?? []).map((s) => [s.id, s.name]));
  const items = page.data?.items ?? [];
  const next = page.data?.next_cursor;
  const { cursor: _, ...kept } = values;
  const today = new Date().toISOString().slice(0, 10);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <TicketFilters action={`/p/${key}/tickets`} values={values} clients={clients.data?.items ?? []} statuses={statuses.data?.items ?? []} />
        {project.role !== "viewer" && (
          <Link href={`/p/${key}/tickets/new`} className="rounded bg-neutral-900 px-3 py-2 text-sm text-white">{t("new")}</Link>
        )}
      </div>
      {items.length === 0 ? (
        <p className="text-neutral-600">{t("none")}</p>
      ) : (
        <table className="w-full border-collapse bg-white text-left text-sm">
          <thead>
            <tr className="border-b">
              {(["key", "title", "type", "status", "client", "assignee", "requestedBy", "menus", "priority", "updated", "due"] as const).map((c) => (
                <th key={c} className="p-2">{t(c)}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {items.map((it) => (
              <tr key={it.id} className="border-b">
                <td className="p-2 font-mono"><Link href={`/t/${it.key}`} className="underline">{it.key}</Link></td>
                <td className="p-2">{it.title}</td>
                <td className="p-2">{tTypes(it.type)}</td>
                <td className="p-2">{statusName.get(it.status_id)}</td>
                <td className="p-2">{it.client?.name ?? t("core")}</td>
                <td className="p-2">{it.assignee?.name ?? "—"}</td>
                <td className="p-2">{it.requester_name}</td>
                <td className="p-2">
                  {it.node_names[0] ?? "—"}
                  {it.node_names.length > 1 ? ` +${it.node_names.length - 1}` : ""}
                </td>
                <td className="p-2">{tPri(it.priority)}</td>
                <td className="p-2">{utc(it.updated_at)}</td>
                <td className={`p-2 ${it.due_date && it.due_date < today ? "text-red-700" : ""}`}>{it.due_date ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {next && (
        <Link href={`?${new URLSearchParams({ ...kept, cursor: next })}`} className="self-end underline">{t("next")}</Link>
      )}
    </div>
  );
}
```

- [ ] **Step 3: Write the board**

`web/app/p/[key]/board/page.tsx`:

```tsx
import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import TicketFilters from "@/components/TicketFilters";
import { getProject, serverApi } from "@/lib/server-api";
import { one, ticketQuery } from "@/lib/ticket-query";
import Board from "./Board";

// The board (FSD §8.4): one column per status, cards by priority, due date, then key.
export default async function BoardPage({
  params,
  searchParams,
}: {
  params: Promise<{ key: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { key } = await params;
  const values = one(await searchParams);
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("board");
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, statuses, page] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/statuses", path),
    // ponytail: one page of up to 1,000 cards; per-column "Show more" comes with larger boards.
    api.GET("/projects/{key}/tickets", { params: { path: { key }, query: { ...ticketQuery(values), sort: "priority", limit: 1000 } } }),
  ]);
  const canEdit = project.role !== "viewer";
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <TicketFilters action={`/p/${key}/board`} values={values} clients={clients.data?.items ?? []} />
        {canEdit && (
          <Link href={`/p/${key}/tickets/new`} className="rounded bg-neutral-900 px-3 py-2 text-sm text-white">{t("newTicket")}</Link>
        )}
      </div>
      <Board
        projectKey={key}
        statuses={statuses.data?.items ?? []}
        tickets={page.data?.items ?? []}
        canEdit={canEdit}
        today={new Date().toISOString().slice(0, 10)}
      />
    </div>
  );
}
```

`web/app/p/[key]/board/Board.tsx`:

```tsx
"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Status, type TicketSummary } from "@/lib/problem";

type Props = { projectKey: string; statuses: Status[]; tickets: TicketSummary[]; canEdit: boolean; today: string };

const closing = (s: Status) => s.category === "done" || s.category === "cancelled";

// Cards move by native drag-and-drop or by their status menu, which keyboards
// reach too. A move shows at once and rolls back when the API refuses it (FSD §8.4).
export default function Board({ projectKey, statuses, tickets, canEdit, today }: Props) {
  const t = useTranslations("board");
  const tTypes = useTranslations("ticketTypes");
  const tPri = useTranslations("priorities");
  const problemText = useProblemText();
  const router = useRouter();
  const [items, setItems] = useState(tickets);
  const [dragging, setDragging] = useState<number | null>(null);
  const [error, setError] = useState("");
  useEffect(() => setItems(tickets), [tickets]); // fresh server data wins

  async function move(ticketId: number, statusId: number) {
    const card = items.find((x) => x.id === ticketId);
    if (!card || card.status_id === statusId) return;
    const before = items;
    setItems((xs) => xs.map((x) => (x.id === ticketId ? { ...x, status_id: statusId } : x)));
    const { error } = await api.POST("/tickets/{key}/transition", {
      params: { path: { key: card.key } },
      body: { status_id: statusId },
    });
    if (error) {
      setItems(before);
      return setError(problemText(error));
    }
    setError("");
    router.refresh();
  }

  return (
    <div className="flex flex-col gap-2">
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <div className="flex gap-3 overflow-x-auto pb-2">
        {statuses.map((s) => {
          const cards = items.filter((x) => x.status_id === s.id);
          const droppable = canEdit && !closing(s);
          return (
            <section
              key={s.id}
              aria-label={s.name}
              className="flex w-72 shrink-0 flex-col gap-2 rounded-lg bg-neutral-100 p-2"
              onDragOver={droppable ? (e) => e.preventDefault() : undefined}
              onDrop={
                droppable
                  ? (e) => {
                      e.preventDefault();
                      if (dragging !== null) move(dragging, s.id);
                      setDragging(null);
                    }
                  : undefined
              }
            >
              <h2 className="flex items-center gap-2 text-sm font-medium">
                <span className="h-2 w-2 rounded-full" style={{ background: s.color }} />
                {s.name}
                <span className="text-neutral-500">{cards.length}</span>
                {droppable && (
                  <Link href={`/p/${projectKey}/tickets/new?status_id=${s.id}`} aria-label={t("addHere", { status: s.name })} className="ml-auto px-1">+</Link>
                )}
              </h2>
              {closing(s) && <p className="text-xs text-neutral-500">{t("closedLater")}</p>}
              {cards.map((c) => (
                <article
                  key={c.id}
                  draggable={canEdit}
                  onDragStart={() => setDragging(c.id)}
                  onDragEnd={() => setDragging(null)}
                  className="rounded bg-white p-2 text-sm shadow-sm"
                >
                  <div className="flex items-center gap-2 text-xs text-neutral-500">
                    <span className="font-mono">{c.key}</span>
                    <span>{tTypes(c.type)}</span>
                    {(c.missing_reason || c.node_names.length === 0) && (
                      <span title={t("missing")} aria-label={t("missing")} className="text-amber-600">●</span>
                    )}
                    <span className="ml-auto">{tPri(c.priority)}</span>
                  </div>
                  <Link href={`/t/${c.key}`} className="mt-1 block font-medium hover:underline">{c.title}</Link>
                  <div className="mt-1 flex flex-wrap gap-2 text-xs">
                    <span className="rounded bg-neutral-100 px-1.5">{c.client?.name ?? t("noClient")}</span>
                    {c.assignee && <span>{c.assignee.name}</span>}
                    {c.due_date && <span className={c.due_date < today ? "text-red-700" : ""}>{c.due_date}</span>}
                  </div>
                  {canEdit && (
                    <select
                      aria-label={t("moveTo", { key: c.key })}
                      value={c.status_id}
                      onChange={(e) => move(c.id, Number(e.target.value))}
                      className="mt-2 w-full rounded border px-1 py-0.5 text-xs"
                    >
                      {statuses.map((o) => (
                        <option key={o.id} value={o.id} disabled={closing(o)}>{o.name}</option>
                      ))}
                    </select>
                  )}
                </article>
              ))}
            </section>
          );
        })}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Build**

```bash
cd web && npm run build
```

Expected: the build succeeds and lists `/p/[key]/board` and `/p/[key]/tickets`.

- [ ] **Step 5: Commit**

```bash
git add web/components web/app/p
git commit -m "feat(web): ticket list and board"
```

### Task 13: Archive and restore in the module tree

**Files:**
- Modify: `web/app/p/[key]/modules/page.tsx`, `web/app/p/[key]/modules/ModuleTree.tsx`

**Interfaces:**
- Consumes: `archived` on nodes and on `PATCH /nodes/{id}`, and `GET …/nodes?archived=true` (Task 7).
- Produces:
  - Project admins get "Arsipkan" next to "Hapus", and a "Tampilkan arsip" / "Sembunyikan arsip" toggle (`?archived=1`).
  - Archived nodes show an "Diarsipkan" badge and a "Pulihkan" button.
  - Deleting a linked node shows the `node_linked` message.

- [ ] **Step 1: Load archived nodes on request**

In `web/app/p/[key]/modules/page.tsx`:
- Take `searchParams: Promise<{ archived?: string }>` next to `params`.
- Compute `const showArchived = canEdit && (await searchParams).archived === "1";`.
- Fetch the nodes with `api.GET("/projects/{key}/nodes", { params: { path: { key }, query: { archived: showArchived || undefined } } })`.
- Pass `showArchived={showArchived}` to `ModuleTree`.

- [ ] **Step 2: Archive, restore and show archived nodes**

In `web/app/p/[key]/modules/ModuleTree.tsx`:

1. Add `import Link from "next/link";`. Add `showArchived: boolean` to `Props`, and destructure it.
2. In the tree header, after the "Tambah modul" button, add the toggle:

```tsx
          {canEdit && (
            <Link href={showArchived ? "?" : "?archived=1"} className="text-sm underline">
              {showArchived ? t("hideArchived") : t("showArchived")}
            </Link>
          )}
```

3. In `level()`, right after `{badge(n)}`, add:

```tsx
                {n.archived && <span className={chip}>{t("archivedBadge")}</span>}
```

4. At the top of `details(n)`, an archived node can only be restored:

```tsx
    if (n.archived) {
      return (
        <div className="flex flex-col gap-4">
          <p className="text-sm text-neutral-600">{pathOf(n)}</p>
          <ReadOnlyNode node={n} />
          {canEdit && (
            <button
              type="button"
              onClick={() => run(api.PATCH("/nodes/{id}", { params: { path: { id: n.id } }, body: { archived: false } }))}
              className={`${action} self-start`}
            >
              {t("restore")}
            </button>
          )}
        </div>
      );
    }
```

5. In the actions row, add "Archive" before the delete button (R-MR-4):

```tsx
              <button
                type="button"
                onClick={() => run(api.PATCH("/nodes/{id}", { params: { path: { id: n.id } }, body: { archived: true } }), () => setSelectedId(null))}
                className={action}
              >
                {t("archive")}
              </button>
```

6. The "Pindahkan ke" choices leave archived nodes out: `nodes.filter((x) => !blocked.has(x.id) && !x.archived)`.

- [ ] **Step 3: Build**

```bash
cd web && npm run build
```

Expected: the build succeeds.

- [ ] **Step 4: Commit**

```bash
git add web/app/p
git commit -m "feat(web): archive and restore menus in the module tree"
```

### Task 14: End-to-end exit check

**Files:**
- Modify: `web/e2e/global-setup.ts` (a third admin)
- Create: `web/e2e/tickets.spec.ts`

**Interfaces:**
- Consumes: the rebuilt stack and the accessible names of Tasks 9–12.
- Produces: `E2E_TICKET_ADMIN_EMAIL` and `E2E_TICKET_ADMIN_LINK`. `make e2e` then proves the three exit checks so far.

- [ ] **Step 1: Create a third admin in the global setup**

In `web/e2e/global-setup.ts`, after the `Tree Admin` lines, add:

```ts
  const tickets = createAdmin("Ticket Admin");
  process.env.E2E_TICKET_ADMIN_EMAIL = tickets.email;
  process.env.E2E_TICKET_ADMIN_LINK = tickets.link;
```

- [ ] **Step 2: Write the exit-check test**

`web/e2e/tickets.spec.ts`:

```ts
import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// FSD §21 Iteration 2 exit check: a PM logs a client request in one form (story 3),
// then works it: an Internal comment (AC-TK-10) and a move on the board (AC-TK-1).
test("a PM logs a client request in one form and moves it on the board", async ({ page, browser }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase(); // keys and names stay unique across runs on one stack
  const key = `T${run.slice(-6)}`;
  const clientName = `Klien A ${run}`;
  const pmEmail = `rina-${run.toLowerCase()}@example.com`;
  const adminPassword = "e2e-ticket-admin-passphrase-5";
  const pmPassword = "e2e-rina-pm-passphrase-6";

  // The admin sets the project up through the API; the screens for that have their own test.
  await setPassword(page, process.env.E2E_TICKET_ADMIN_LINK!, adminPassword);
  await signIn(page, process.env.E2E_TICKET_ADMIN_EMAIL!, adminPassword);
  const origin = new URL(page.url()).origin;
  const call = async (method: string, path: string, data: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.json();
  };
  const client = await call("POST", "/clients", { name: clientName });
  await call("POST", "/projects", { key, name: `HRIS ${run}` });
  await call("PUT", `/projects/${key}/clients`, { client_ids: [client.id] });
  const hr = await call("POST", `/projects/${key}/nodes`, { type: "module", name: "HR" });
  await call("POST", `/projects/${key}/nodes`, {
    type: "menu", name: "Overtime Approval", parent_id: hr.id, client_specific: true, client_ids: [client.id],
  });
  const pm = await call("POST", "/admin/users", { name: "Rina PM", email: pmEmail });
  await call("PUT", `/projects/${key}/members`, { members: [{ email: pmEmail, role: "member", all_clients: true }] });

  // Story 3: while Budi is on the phone, Rina logs his request in one form.
  const rina = await (await browser.newContext()).newPage();
  await setPassword(rina, pm.setup_link.url, pmPassword);
  await signIn(rina, pmEmail, pmPassword);
  await rina.goto(`/p/${key}/tickets/new`);
  const form = rina.getByRole("form", { name: "Tiket baru" });
  await form.getByLabel("Klien").selectOption({ label: clientName });
  await form.getByRole("radio", { name: "Seorang kontak" }).check();
  await form.getByRole("button", { name: "+ Tambah kontak" }).click();
  await form.getByLabel("Nama kontak").fill("Budi");
  await form.getByLabel("Jabatan (opsional)").fill("HR Manager");
  await form.getByRole("button", { name: "Tambah", exact: true }).click();
  await expect(form.getByLabel("Kontak", { exact: true })).not.toHaveValue("");
  await form.getByLabel("Judul").fill("Skip supervisor approval for overtime");
  await form.getByRole("checkbox", { name: "HR › Overtime Approval" }).check();
  await form.getByLabel("Alasan").fill("Client A supervisors are often on leave; HR approves overtime directly.");
  await form.getByRole("button", { name: "Buat", exact: true }).click();

  await expect(rina).toHaveURL(new RegExp(`/t/${key}-1$`));
  await expect(rina.getByRole("heading", { name: "Skip supervisor approval for overtime" })).toBeVisible();
  await expect(rina.getByText("Budi (HR Manager)")).toBeVisible();
  await expect(rina.getByRole("listitem").filter({ hasText: /^Overtime Approval$/ })).toBeVisible();
  await expect(rina.getByText("Tambahkan alasan dan minimal satu menu sebelum menutup.")).toHaveCount(0);

  // AC-TK-10: a new comment is Internal by default.
  await rina.getByLabel("Tulis komentar").fill("Budi confirmed by phone.");
  await rina.getByRole("button", { name: "Kirim" }).click();
  const comment = rina.getByRole("listitem").filter({ hasText: "Budi confirmed by phone." });
  await expect(comment.getByText("Internal", { exact: true })).toBeVisible();

  // AC-TK-1: dragging the card from To do to In progress records the move.
  await rina.goto(`/p/${key}/board`);
  const inProgress = rina.getByRole("region", { name: "In progress" });
  await rina.getByRole("article").filter({ hasText: `${key}-1` }).dragTo(inProgress);
  await expect(inProgress.getByRole("article")).toContainText(`${key}-1`);
  await rina.goto(`/t/${key}-1`);
  await expect(rina.getByText(/Rina PM mengubah Status dari To do menjadi In progress/)).toBeVisible();
});
```

- [ ] **Step 3: Run it on a rebuilt stack**

```bash
make up
cd web && E2E_BASE_URL=http://localhost:8080 npx playwright test
```

Use `make e2e` where the stack answers on port 80. Expected: `3 passed`.

- [ ] **Step 4: Run every check CI runs**

```bash
cd server && go generate ./... && git diff --exit-code && go vet ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...
cd ../web && npm run gen:api && git diff --exit-code && npm run build
```

Expected: no diff, no vet findings, `ok` for every Go package, and a successful web build.

- [ ] **Step 5: Record progress in the FSD**

Use the Claude Docs connector. In §21, replace the "Progress:" paragraph so it says:
- Iteration 1 is merged.
- Iteration 2 is built and passes its exit check (24 Sep 2026).
- Closing moves to Iteration 3, together with the close dialog and decision records.
- Ticket archive, markdown rendering and the create modal come later.
- The details are in `docs/superpowers/plans/2026-09-24-iteration-2-tickets.md`.

- [ ] **Step 6: Commit**

```bash
git add web/e2e
git commit -m "test(e2e): Iteration 2 exit check: a client request in one form"
```

## Exit check (FSD §21, Iteration 2)

- `make test` passes, including the ticket permission suite.
- On a stack rebuilt with `make up`, `make e2e` passes:
  1. A PM picks Client A, adds contact "Budi (HR Manager)" inline, ticks HR › Overtime Approval and gives a reason.
  2. Pressing "Create" opens `<KEY>-1`, whose page shows the requester and the menu.
  3. A new comment is Internal.
  4. Dragging the card to In progress shows in the history as "Status To do → In progress".
- CI is green on `server`, `web` and `e2e`.
