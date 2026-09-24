# Muasal Iteration 1 — Registry and Access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A system admin creates project HRIS, links clients, scopes members by client and builds the HRIS module tree, and a member scoped to one client sees only the menus in that scope. This is the exit check for FSD §21 Iteration 1.

**Architecture:** The same stack as Iteration 0. Each resource adds its slice of `api/openapi.yaml`, sqlc queries and std-http handlers. A small `access` package reads the caller's project scope from the database on every request. Visibility is decided in SQL: a recursive CTE hides client-specific menus, and what sits under them, from members whose client scope does not cover them. Composite foreign keys to `project_clients` stop a member scope or a menu from naming a client the project does not have, and stop a client from being unlinked while something still uses it. Next.js adds a top bar, a project list, project settings, a client admin page and the module tree editor.

**Tech Stack:** Unchanged from Iteration 0: Go 1.27.1, pgx v5, sqlc 1.31.1, goose 3.28, oapi-codegen 2.8.0; Next.js 16.3, React 19.3, next-intl 4.14, openapi-fetch, Tailwind 4, Playwright 1.63; PostgreSQL 18. No new dependencies.

**Spec:** Claude Docs "FSD — Muasal": §5 roles, permissions and visibility; §6 screens; §7.1–7.3 and §7.8 module registry; §15.2–15.3 projects, memberships, clients and contacts; §16 data model; §17 API; §21 delivery plan and permission suite.

## Global Constraints

- Everything in Iteration 0's Global Constraints still holds. That covers:
  - the module path;
  - RFC 9457 problems with stable codes;
  - the CSRF Origin check;
  - one transaction per mutation, together with its audit row;
  - `id` and `en` strings, with `id` as the default;
  - no ORM, no dependency-injection container, and no interface with a single implementation;
  - generated code committed and checked for drift in CI;
  - Next.js never writes data or decides access.
- Project key: unique and matching `^[A-Z][A-Z0-9]{1,9}$`, which means 2 to 10 capital letters or digits starting with a letter. The API upper-cases what it receives.
- Project roles: `admin`, `member`, `viewer`. System admins (`users.is_admin`) act as project admins with all clients in every project.
- Client scope (R-AC-1): a membership covers all clients, or a list of clients linked to the project. Project admins always cover all clients (R-AC-11, new).
- Visibility is decided in SQL on every request (R-AC-8):
  - A client-specific menu, and everything under it, is hidden unless one of its clients is in scope (R-AC-5).
  - A menu lists only in-scope client names (R-MR-8).
  - A contact is visible when its client is in scope in some membership. Internal contacts are visible to every project member (R-AC-6).
- Hidden and missing look the same (R-AC-7):
  - Answer 404 `not_found` for a project the user does not belong to, and for a node or contact outside their scope.
  - Answer 403 `forbidden` only when the user can already see the thing.
- Tree rules:
  - Node names are unique among live siblings, ignoring case.
  - Codes are unique per project (R-MR-2).
  - Only menus can be client-specific (R-MR-7).
  - A node with sub-nodes cannot be deleted.
- Auditing:
  - Audit events for project-level changes carry `project_id`.
  - Updates record `{"field": {"old": …, "new": …}}` (R-MR-5).
- New error codes, which the web translates: `project_key_taken`, `client_name_taken`, `client_in_use`, `client_not_linked`, `unknown_client`, `unknown_user`, `duplicate`, `admin_needs_all_clients`, `cannot_demote_self`, `node_name_taken`, `node_code_taken`, `node_has_children`, `node_cycle`, `client_specific_module`, `forbidden`, `not_found`.

## Deliberate Deviations from the FSD

**Data model**
- R-AC-11 (new): project admins always have all clients. They manage the tree, members and clients, so a scoped project admin would edit things they cannot see. A database CHECK enforces the rule.
- `membership_clients` and `node_clients` point to `project_clients` through composite foreign keys. The database then refuses any scope or menu that names a client the project lacks. It also refuses to unlink a client that is still used, which the API answers with 409 `client_in_use`. `node_clients` gains `project_id` for this, and it is deleted along with its node.
- `projects.ticket_seq`, `note_seq`, `doc_seq` and `archived_at` arrive with tickets (Iteration 2) and notes and documents (P1).
- The default statuses of a new project arrive with the `statuses` table in Iteration 2, which will backfill existing projects.

**Tree editing**
- No node archive yet. Nothing links to a node before tickets exist, so every node can be deleted (R-MR-4). Archive and restore arrive with ticket links in Iteration 2.
- Moving and reordering use a parent picker and "Move up" and "Move down" buttons, which also covers keyboard use (§6.3). Drag-and-drop arrives with dnd-kit and the board in Iteration 2.
- Linked-ticket counts, the node page (timeline, behaviors by client) and "Open node page" arrive with tickets in Iterations 2 and 3.
- The onboarding wizard (`/p/[key]/setup`) arrives with CSV import (P1). Until then, building the tree by hand starts from the module tree's empty state.

**API and access**
- Contacts ship as an API only. The inline "Add contact" in the ticket form (Iteration 2) is their first screen.
- `GET /clients` serves system admins and project admins. Members read clients through their project from Iteration 2 on.
- Members are added by email in `PUT /projects/{key}/members`, so no user-directory endpoint is exposed.
- `PATCH /contacts/{id}` replaces every field. This avoids the null-versus-absent ambiguity for `client_id`.
- The project audit search (`GET /projects/{key}/audit`) is Iteration 2. Iteration 1 starts tagging events with `project_id`.

**UI**
- shadcn/ui moves to Iteration 2, where the ticket form needs comboboxes. Iteration 1 uses native elements.

## Prerequisites

- Iteration 0 merged to `main`; work on branch `feat/iteration-1`.
- `make testdb` for the Go tests (`TEST_DATABASE_URL=postgres://owner:owner@localhost:55432/postgres?sslmode=disable`).
- `make up` for the end-to-end test.

## File Structure

```text
server/
├── migrations/00002_registry.sql            projects, clients, contacts, memberships, nodes
├── internal/
│   ├── access/access.go (+ access_test.go)  project roles, a user's scope in a project
│   ├── db/queries/{projects,clients,memberships,contacts,nodes}.sql   (+ generated *.sql.go)
│   ├── db/registry_test.go                  the constraint names the handlers rely on
│   └── httpapi/
│       ├── projects.go (+ projects_test.go) projects; projectFor/memberOf helpers
│       ├── clients.go  (+ clients_test.go)  clients and a project's linked clients
│       ├── members.go  (+ members_test.go)  memberships and client scopes
│       ├── contacts.go (+ contacts_test.go) contacts
│       ├── nodes.go    (+ nodes_test.go)    the module tree
│       ├── seed_test.go                     test seeders and helpers
│       └── permission_test.go               the permission suite (FSD §5.3)
web/
├── app/Header.tsx                           top bar
├── app/page.tsx                             home: my projects
├── app/projects/new/                        create a project (system admins)
├── app/admin/clients/                       client directory (system admins)
├── app/p/[key]/layout.tsx                   project header and tabs
├── app/p/[key]/settings/                    general, linked clients, members
├── app/p/[key]/modules/                     the module tree editor
└── e2e/{helpers,registry.spec}.ts           the Iteration 1 exit check
```

---

### Task 1: Registry data layer and the `access` package

**Files:**
- Create: `server/migrations/00002_registry.sql`
- Create: `server/internal/db/queries/projects.sql`, `clients.sql`, `memberships.sql`, `contacts.sql`, `nodes.sql`
- Generate: `server/internal/db/*.sql.go`, `models.go`
- Create: `server/internal/access/access.go`
- Test: `server/internal/access/access_test.go`, `server/internal/db/registry_test.go`

**Interfaces:**
- Consumes: `testdb.New` and the users queries from Iteration 0.
- Produces (sqlc, package `db`):
  - Models: `Project{ID int64; Key, Name, Description string; CreatedAt time.Time}`, `Client{ID int64; Name string; Code *string; Aliases []string; ArchivedAt *time.Time}`, `Node{ID, ProjectID int64; ParentID *int64; Type, Name string; Code *string; Aliases []string; Description string; ClientSpecific bool; Source string; Position int32; ArchivedAt *time.Time}`.
  - Projects: `CreateProject(ctx, CreateProjectParams{Key, Name, Description string}) (Project, error)`, `GetProjectByKey(ctx, key string)`, `GetProjectByID(ctx, id int64)`, `LockProject(ctx, id int64) error`, `ListProjects(ctx, ListProjectsParams{UserID int64; IsAdmin bool}) ([]ListProjectsRow{Project Project; Role *string}, error)`, `UpdateProject(ctx, UpdateProjectParams{Key, Name, Description *string; ID int64}) (Project, error)`.
  - Project clients: `ListProjectClients(ctx, projectID int64) ([]Client, error)`, `UnlinkClientsExcept(ctx, UnlinkClientsExceptParams{ProjectID int64; ClientIds []int64}) error`, `LinkClients(ctx, LinkClientsParams{ProjectID int64; ClientIds []int64}) error`.
  - Clients: `CreateClient(ctx, CreateClientParams{Name string; Code *string; Aliases []string}) (Client, error)`, `GetClient(ctx, id int64)`, `ListClients(ctx) ([]Client, error)`, `UpdateClient(ctx, UpdateClientParams{Name, Code *string; Aliases []string; Archived *bool; ID int64}) (Client, error)`.
  - Memberships:
    - `GetMembership(ctx, GetMembershipParams{UserID, ProjectID int64}) (GetMembershipRow{Role string; AllClients bool; ClientIds []int64}, error)`
    - `IsProjectAdminAnywhere(ctx, userID int64) (bool, error)`
    - `ListProjectMembers(ctx, projectID int64) ([]ListProjectMembersRow{UserID int64; Name, Email, Role string; AllClients bool; ClientIds []int64}, error)`
    - `DeleteMembershipsExcept(ctx, DeleteMembershipsExceptParams{ProjectID int64; UserIds []int64}) error`
    - `UpsertMembership(ctx, UpsertMembershipParams{UserID, ProjectID int64; Role string; AllClients bool}) error`
    - `ClearMembershipClients(ctx, ClearMembershipClientsParams{UserID, ProjectID int64}) error`
    - `AddMembershipClients(ctx, AddMembershipClientsParams{UserID, ProjectID int64; ClientIds []int64}) error`
  - Contacts: `ListContacts(ctx, ListContactsParams{IsAdmin bool; UserID int64; ID, ClientID *int64; Q string}) ([]ListContactsRow{ID int64; ClientID *int64; ClientName *string; Name string; Title, Email, Phone *string}, error)`, `CanEditContactsOf(ctx, CanEditContactsOfParams{UserID int64; ClientID *int64}) (bool, error)`, `CreateContact(ctx, CreateContactParams{ClientID *int64; Name string; Title, Email, Phone *string}) (int64, error)`, `UpdateContact(ctx, UpdateContactParams{ID int64; ClientID *int64; Name string; Title, Email, Phone *string}) error`.
  - Nodes:
    - `ListNodes(ctx, ListNodesParams{ProjectID int64; AllClients bool; ClientIds []int64}) ([]ListNodesRow{ID int64; ParentID *int64; Type, Name string; Code *string; Aliases []string; Description string; ClientSpecific bool; Position int32; ClientIds []int64; ClientNames []string}, error)`
    - `IsNodeVisible(ctx, IsNodeVisibleParams{ID int64; AllClients bool; ClientIds []int64}) (bool, error)`
    - `GetNode(ctx, id int64) (Node, error)`
    - `CreateNode(ctx, CreateNodeParams{ProjectID int64; ParentID *int64; Type, Name string; Code *string; Aliases []string; Description string; ClientSpecific bool}) (Node, error)`
    - `UpdateNode(ctx, UpdateNodeParams{Name, Type, Code *string; Aliases []string; Description *string; ClientSpecific *bool; ID int64}) (Node, error)`
    - `ListNodeClients(ctx, nodeID int64) ([]ListNodeClientsRow{ID int64; Name string}, error)`, `ClearNodeClients(ctx, nodeID int64) error`, `AddNodeClients(ctx, AddNodeClientsParams{NodeID, ProjectID int64; ClientIds []int64}) error`
    - `IsSelfOrDescendant(ctx, IsSelfOrDescendantParams{NodeID, CandidateID int64}) (bool, error)`, `ListSiblingIDs(ctx, ListSiblingIDsParams{ProjectID int64; ParentID *int64; ExcludeID int64}) ([]int64, error)`, `PlaceNodes(ctx, PlaceNodesParams{ParentID *int64; Ids []int64}) error`, `DeleteNode(ctx, id int64) error`.
  - Constraint names the handlers map to problems: `projects_key_format`, `memberships_admin_all_clients`, `membership_clients_linked`, `project_clients_client_fk`, `nodes_specific_menu`, `node_clients_linked`, `nodes_sibling_uq`, `nodes_code_uq`, `nodes_parent_fk`, `contacts_client_fk`.
- Produces (package `access`): constants `Viewer`, `Member`, `Admin`; `type Scope struct{Role string; AllClients bool; ClientIDs []int64}` with `(Scope) Allows(need string) bool`; `ForProject(ctx, q *db.Queries, u *db.User, projectID int64) (scope Scope, member bool, err error)`; `AdminsAnything(ctx, q *db.Queries, u *db.User) (bool, error)`.

- [ ] **Step 1: Write the migration**

`server/migrations/00002_registry.sql`:

```sql
-- +goose Up
-- Projects, clients, contacts, memberships and the module tree (FSD §7, §15.2, §15.3, §16).
CREATE TABLE projects (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  key         text NOT NULL UNIQUE CONSTRAINT projects_key_format CHECK (key ~ '^[A-Z][A-Z0-9]{1,9}$'),
  name        text NOT NULL,
  description text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE clients (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name        text NOT NULL,
  code        text,
  aliases     text[] NOT NULL DEFAULT '{}',
  archived_at timestamptz
);
CREATE UNIQUE INDEX clients_name_uq ON clients (lower(name));

CREATE TABLE project_clients (
  project_id bigint NOT NULL REFERENCES projects (id),
  client_id  bigint NOT NULL CONSTRAINT project_clients_client_fk REFERENCES clients (id),
  PRIMARY KEY (project_id, client_id)
);

CREATE TABLE contacts (
  id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  client_id bigint CONSTRAINT contacts_client_fk REFERENCES clients (id), -- NULL = internal person
  name      text NOT NULL,
  title     text,
  email     text,
  phone     text
);
CREATE INDEX contacts_client_idx ON contacts (client_id);

CREATE TABLE memberships (
  user_id     bigint NOT NULL REFERENCES users (id),
  project_id  bigint NOT NULL REFERENCES projects (id),
  role        text NOT NULL CHECK (role IN ('admin', 'member', 'viewer')),
  all_clients boolean NOT NULL DEFAULT true,
  PRIMARY KEY (user_id, project_id),
  -- Project admins manage the tree, members and clients, so they see every client (R-AC-11).
  CONSTRAINT memberships_admin_all_clients CHECK (role <> 'admin' OR all_clients)
);
CREATE INDEX memberships_project_idx ON memberships (project_id);

-- A scoped membership lists clients linked to its project (R-AC-1).
CREATE TABLE membership_clients (
  user_id    bigint NOT NULL,
  project_id bigint NOT NULL,
  client_id  bigint NOT NULL,
  PRIMARY KEY (user_id, project_id, client_id),
  FOREIGN KEY (user_id, project_id) REFERENCES memberships ON DELETE CASCADE,
  CONSTRAINT membership_clients_linked FOREIGN KEY (project_id, client_id) REFERENCES project_clients
);

CREATE TABLE nodes (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id      bigint NOT NULL REFERENCES projects (id),
  parent_id       bigint CONSTRAINT nodes_parent_fk REFERENCES nodes (id), -- NULL = top level
  type            text NOT NULL CHECK (type IN ('module', 'menu')),
  name            text NOT NULL,
  code            text,
  aliases         text[] NOT NULL DEFAULT '{}',
  description     text NOT NULL DEFAULT '',
  client_specific boolean NOT NULL DEFAULT false,
  source          text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'csv', 'ai_draft')),
  position        int NOT NULL DEFAULT 0,
  archived_at     timestamptz,
  CONSTRAINT nodes_specific_menu CHECK (type = 'menu' OR NOT client_specific) -- R-MR-7
);
CREATE UNIQUE INDEX nodes_sibling_uq ON nodes (project_id, coalesce(parent_id, 0), lower(name))
  WHERE archived_at IS NULL;
CREATE UNIQUE INDEX nodes_code_uq ON nodes (project_id, code) WHERE code IS NOT NULL;
CREATE INDEX nodes_parent_idx ON nodes (parent_id);

-- A client-specific menu names clients linked to its project (R-MR-7).
CREATE TABLE node_clients (
  node_id    bigint NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
  project_id bigint NOT NULL,
  client_id  bigint NOT NULL,
  PRIMARY KEY (node_id, client_id),
  CONSTRAINT node_clients_linked FOREIGN KEY (project_id, client_id) REFERENCES project_clients
);

-- +goose Down
DROP TABLE node_clients;
DROP TABLE nodes;
DROP TABLE membership_clients;
DROP TABLE memberships;
DROP TABLE contacts;
DROP TABLE project_clients;
DROP TABLE clients;
DROP TABLE projects;
```

`migrate.Up` grants the app role access to every table after migrating, so the new tables need no grants of their own.

- [ ] **Step 2: Write the queries**

`server/internal/db/queries/projects.sql`:

```sql
-- name: CreateProject :one
INSERT INTO projects (key, name, description)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetProjectByKey :one
SELECT * FROM projects WHERE key = $1;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = $1;

-- name: LockProject :exec
-- Serializes tree moves inside one project, so two moves cannot build a cycle.
SELECT id FROM projects WHERE id = $1 FOR UPDATE;

-- name: ListProjects :many
-- System admins see every project; everyone else sees the projects they belong to.
SELECT sqlc.embed(p), m.role
FROM projects p
LEFT JOIN memberships m ON m.project_id = p.id AND m.user_id = sqlc.arg('user_id')
WHERE sqlc.arg('is_admin')::boolean OR m.user_id IS NOT NULL
ORDER BY p.key;

-- name: UpdateProject :one
UPDATE projects SET
  key         = coalesce(sqlc.narg('key'), key),
  name        = coalesce(sqlc.narg('name'), name),
  description = coalesce(sqlc.narg('description'), description)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ListProjectClients :many
SELECT c.* FROM clients c
JOIN project_clients pc ON pc.client_id = c.id
WHERE pc.project_id = $1
ORDER BY lower(c.name), c.id;

-- name: UnlinkClientsExcept :exec
DELETE FROM project_clients
WHERE project_id = sqlc.arg('project_id') AND NOT (client_id = ANY (sqlc.arg('client_ids')::bigint[]));

-- name: LinkClients :exec
INSERT INTO project_clients (project_id, client_id)
SELECT sqlc.arg('project_id')::bigint, unnest(sqlc.arg('client_ids')::bigint[])
ON CONFLICT DO NOTHING;
```

`server/internal/db/queries/clients.sql`:

```sql
-- name: CreateClient :one
INSERT INTO clients (name, code, aliases)
VALUES (sqlc.arg('name'), sqlc.narg('code'), coalesce(sqlc.narg('aliases')::text[], '{}'))
RETURNING *;

-- name: GetClient :one
SELECT * FROM clients WHERE id = $1;

-- name: ListClients :many
SELECT * FROM clients ORDER BY lower(name), id;

-- name: UpdateClient :one
-- NULL keeps a field; an empty code clears it.
UPDATE clients SET
  name        = coalesce(sqlc.narg('name'), name),
  code        = CASE WHEN sqlc.narg('code')::text IS NULL THEN code ELSE nullif(sqlc.narg('code')::text, '') END,
  aliases     = coalesce(sqlc.narg('aliases')::text[], aliases),
  archived_at = CASE WHEN sqlc.narg('archived')::boolean IS NULL THEN archived_at
                     WHEN sqlc.narg('archived')::boolean THEN coalesce(archived_at, now())
                     ELSE NULL END
WHERE id = sqlc.arg('id')
RETURNING *;
```

`server/internal/db/queries/memberships.sql`:

```sql
-- name: GetMembership :one
SELECT m.role, m.all_clients,
       coalesce(array_agg(mc.client_id ORDER BY mc.client_id) FILTER (WHERE mc.client_id IS NOT NULL), '{}')::bigint[] AS client_ids
FROM memberships m
LEFT JOIN membership_clients mc ON mc.user_id = m.user_id AND mc.project_id = m.project_id
WHERE m.user_id = $1 AND m.project_id = $2
GROUP BY m.role, m.all_clients;

-- name: IsProjectAdminAnywhere :one
SELECT EXISTS (SELECT 1 FROM memberships WHERE user_id = $1 AND role = 'admin');

-- name: ListProjectMembers :many
SELECT u.id AS user_id, u.name, u.email, m.role, m.all_clients,
       coalesce(array_agg(mc.client_id ORDER BY mc.client_id) FILTER (WHERE mc.client_id IS NOT NULL), '{}')::bigint[] AS client_ids
FROM memberships m
JOIN users u ON u.id = m.user_id
LEFT JOIN membership_clients mc ON mc.user_id = m.user_id AND mc.project_id = m.project_id
WHERE m.project_id = $1
GROUP BY u.id, u.name, u.email, m.role, m.all_clients
ORDER BY lower(u.name), u.id;

-- name: DeleteMembershipsExcept :exec
DELETE FROM memberships
WHERE project_id = sqlc.arg('project_id') AND NOT (user_id = ANY (sqlc.arg('user_ids')::bigint[]));

-- name: UpsertMembership :exec
INSERT INTO memberships (user_id, project_id, role, all_clients)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, project_id) DO UPDATE SET role = excluded.role, all_clients = excluded.all_clients;

-- name: ClearMembershipClients :exec
DELETE FROM membership_clients WHERE user_id = $1 AND project_id = $2;

-- name: AddMembershipClients :exec
INSERT INTO membership_clients (user_id, project_id, client_id)
SELECT sqlc.arg('user_id')::bigint, sqlc.arg('project_id')::bigint, unnest(sqlc.arg('client_ids')::bigint[]);
```

`server/internal/db/queries/contacts.sql`:

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
  AND c.name ILIKE '%' || sqlc.arg('q')::text || '%'
ORDER BY lower(c.name), c.id
LIMIT 50;

-- name: CanEditContactsOf :one
-- Members and project admins keep the contacts of clients in their scope, and
-- internal contacts (client NULL) in any project they belong to.
SELECT EXISTS (
  SELECT 1 FROM memberships m
  WHERE m.user_id = sqlc.arg('user_id') AND m.role IN ('admin', 'member')
    AND (sqlc.narg('client_id')::bigint IS NULL
         OR (m.all_clients AND EXISTS (SELECT 1 FROM project_clients pc
                                       WHERE pc.project_id = m.project_id
                                         AND pc.client_id = sqlc.narg('client_id')::bigint))
         OR EXISTS (SELECT 1 FROM membership_clients mc
                    WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id
                      AND mc.client_id = sqlc.narg('client_id')::bigint))
);

-- name: CreateContact :one
INSERT INTO contacts (client_id, name, title, email, phone)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: UpdateContact :exec
UPDATE contacts SET client_id = $2, name = $3, title = $4, email = $5, phone = $6
WHERE id = $1;
```

`server/internal/db/queries/nodes.sql`:

```sql
-- name: ListNodes :many
-- The project's live tree as a flat list, siblings in position order. A
-- client-specific menu, and everything under it, is left out unless one of its
-- clients is in scope (R-AC-5); client_ids and client_names hold only in-scope
-- clients (R-MR-8). UNION, not UNION ALL, so a cycle could never loop forever.
WITH RECURSIVE visible AS (
  SELECT n.id FROM nodes n
  WHERE n.project_id = sqlc.arg('project_id') AND n.parent_id IS NULL AND n.archived_at IS NULL
    AND (NOT n.client_specific OR sqlc.arg('all_clients')::boolean
         OR EXISTS (SELECT 1 FROM node_clients nc
                    WHERE nc.node_id = n.id AND nc.client_id = ANY (sqlc.arg('client_ids')::bigint[])))
  UNION
  SELECT n.id FROM nodes n
  JOIN visible v ON n.parent_id = v.id
  WHERE n.archived_at IS NULL
    AND (NOT n.client_specific OR sqlc.arg('all_clients')::boolean
         OR EXISTS (SELECT 1 FROM node_clients nc
                    WHERE nc.node_id = n.id AND nc.client_id = ANY (sqlc.arg('client_ids')::bigint[])))
)
SELECT n.id, n.parent_id, n.type, n.name, n.code, n.aliases, n.description, n.client_specific, n.position,
       coalesce(array_agg(c.id ORDER BY lower(c.name), c.id) FILTER (WHERE c.id IS NOT NULL), '{}')::bigint[] AS client_ids,
       coalesce(array_agg(c.name ORDER BY lower(c.name), c.id) FILTER (WHERE c.id IS NOT NULL), '{}')::text[] AS client_names
FROM visible v
JOIN nodes n ON n.id = v.id
LEFT JOIN node_clients nc ON nc.node_id = n.id
  AND (sqlc.arg('all_clients')::boolean OR nc.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
LEFT JOIN clients c ON c.id = nc.client_id
GROUP BY n.id
ORDER BY n.parent_id NULLS FIRST, n.position, n.id;

-- name: IsNodeVisible :one
-- Whether the node and every node above it are visible to this scope (R-AC-5).
WITH RECURSIVE up AS (
  SELECT s.id, s.parent_id, s.client_specific FROM nodes s WHERE s.id = sqlc.arg('id')
  UNION
  SELECT n.id, n.parent_id, n.client_specific FROM nodes n JOIN up ON n.id = up.parent_id
)
SELECT NOT EXISTS (
  SELECT 1 FROM up
  WHERE up.client_specific AND NOT sqlc.arg('all_clients')::boolean
    AND NOT EXISTS (SELECT 1 FROM node_clients nc
                    WHERE nc.node_id = up.id AND nc.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
);

-- name: GetNode :one
SELECT * FROM nodes WHERE id = $1;

-- name: CreateNode :one
-- New nodes go last among their siblings.
INSERT INTO nodes (project_id, parent_id, type, name, code, aliases, description, client_specific, position)
VALUES (sqlc.arg('project_id'), sqlc.narg('parent_id'), sqlc.arg('type'), sqlc.arg('name'), sqlc.narg('code'),
        coalesce(sqlc.narg('aliases')::text[], '{}'), sqlc.arg('description'), sqlc.arg('client_specific'),
        (SELECT coalesce(max(s.position) + 1, 0) FROM nodes s
         WHERE s.project_id = sqlc.arg('project_id')
           AND s.parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::bigint))
RETURNING *;

-- name: UpdateNode :one
-- NULL keeps a field; an empty code clears it.
UPDATE nodes SET
  name            = coalesce(sqlc.narg('name'), name),
  type            = coalesce(sqlc.narg('type'), type),
  code            = CASE WHEN sqlc.narg('code')::text IS NULL THEN code ELSE nullif(sqlc.narg('code')::text, '') END,
  aliases         = coalesce(sqlc.narg('aliases')::text[], aliases),
  description     = coalesce(sqlc.narg('description'), description),
  client_specific = coalesce(sqlc.narg('client_specific'), client_specific)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ListNodeClients :many
SELECT c.id, c.name FROM node_clients nc
JOIN clients c ON c.id = nc.client_id
WHERE nc.node_id = $1
ORDER BY lower(c.name), c.id;

-- name: ClearNodeClients :exec
DELETE FROM node_clients WHERE node_id = $1;

-- name: AddNodeClients :exec
INSERT INTO node_clients (node_id, project_id, client_id)
SELECT sqlc.arg('node_id')::bigint, sqlc.arg('project_id')::bigint, unnest(sqlc.arg('client_ids')::bigint[]);

-- name: IsSelfOrDescendant :one
-- Whether candidate is the node itself or anywhere below it: moving there would build a cycle.
WITH RECURSIVE below AS (
  SELECT s.id FROM nodes s WHERE s.id = sqlc.arg('node_id')
  UNION
  SELECT n.id FROM nodes n JOIN below b ON n.parent_id = b.id
)
SELECT EXISTS (SELECT 1 FROM below WHERE below.id = sqlc.arg('candidate_id')::bigint);

-- name: ListSiblingIDs :many
SELECT id FROM nodes
WHERE project_id = sqlc.arg('project_id')
  AND parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::bigint
  AND id <> sqlc.arg('exclude_id') AND archived_at IS NULL
ORDER BY position, id;

-- name: PlaceNodes :exec
-- Puts the listed nodes, in this order, under one parent.
UPDATE nodes
SET parent_id = sqlc.narg('parent_id')::bigint,
    position  = array_position(sqlc.arg('ids')::bigint[], id) - 1
WHERE id = ANY (sqlc.arg('ids')::bigint[]);

-- name: DeleteNode :exec
DELETE FROM nodes WHERE id = $1;
```

- [ ] **Step 3: Generate and build**

```bash
cd server && go generate ./... && go build ./...
```

Expected: no output. `internal/db` gains `projects.sql.go`, `clients.sql.go`, `memberships.sql.go`, `contacts.sql.go`, `nodes.sql.go`, and `models.go` gains `Project`, `Client`, `Contact`, `Membership`, `MembershipClient`, `Node`, `NodeClient`, `ProjectClient`. If a generated name differs from the Interfaces block (sqlc spells `client_ids` as `ClientIds`), use the generated name everywhere below.

- [ ] **Step 4: Write the failing tests**

`server/internal/db/registry_test.go`:

```go
package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func ptr[T any](v T) *T { return &v }

// The handlers turn these constraint names into API problems, so the names are
// part of the contract: each case must fail on exactly the named constraint.
func TestRegistryRulesAreEnforcedByTheDatabase(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	u := must(q.CreateUser(ctx, db.CreateUserParams{Email: "u@example.com", Name: "U", Locale: "id", Timezone: "Asia/Jakarta"}))
	p := must(q.CreateProject(ctx, db.CreateProjectParams{Key: "HRIS", Name: "HRIS"}))
	a := must(q.CreateClient(ctx, db.CreateClientParams{Name: "Client A"}))
	b := must(q.CreateClient(ctx, db.CreateClientParams{Name: "Client B"}))
	check(q.LinkClients(ctx, db.LinkClientsParams{ProjectID: p.ID, ClientIds: []int64{a.ID}}))
	check(q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: u.ID, ProjectID: p.ID, Role: "member"}))
	check(q.AddMembershipClients(ctx, db.AddMembershipClientsParams{UserID: u.ID, ProjectID: p.ID, ClientIds: []int64{a.ID}}))
	hr := must(q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, Type: "module", Name: "HR"}))
	must(q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, ParentID: &hr.ID, Type: "menu", Name: "Leave", Code: ptr("HR.LV")}))

	for _, c := range []struct {
		constraint string
		op         func(q *db.Queries) error
	}{
		{"projects_key_format", func(q *db.Queries) error {
			_, err := q.CreateProject(ctx, db.CreateProjectParams{Key: "hris2", Name: "x"})
			return err
		}},
		{"memberships_admin_all_clients", func(q *db.Queries) error {
			return q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: u.ID, ProjectID: p.ID, Role: "admin"})
		}},
		{"membership_clients_linked", func(q *db.Queries) error { // B is not linked to HRIS
			return q.AddMembershipClients(ctx, db.AddMembershipClientsParams{UserID: u.ID, ProjectID: p.ID, ClientIds: []int64{b.ID}})
		}},
		{"membership_clients_linked", func(q *db.Queries) error { // A is still in a member scope
			return q.UnlinkClientsExcept(ctx, db.UnlinkClientsExceptParams{ProjectID: p.ID, ClientIds: []int64{}})
		}},
		{"project_clients_client_fk", func(q *db.Queries) error {
			return q.LinkClients(ctx, db.LinkClientsParams{ProjectID: p.ID, ClientIds: []int64{999999}})
		}},
		{"nodes_specific_menu", func(q *db.Queries) error {
			_, err := q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, Type: "module", Name: "Payroll", ClientSpecific: true})
			return err
		}},
		{"node_clients_linked", func(q *db.Queries) error {
			return q.AddNodeClients(ctx, db.AddNodeClientsParams{NodeID: hr.ID, ProjectID: p.ID, ClientIds: []int64{b.ID}})
		}},
		{"nodes_sibling_uq", func(q *db.Queries) error { // names are unique among siblings, ignoring case
			_, err := q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, Type: "module", Name: "hr"})
			return err
		}},
		{"nodes_code_uq", func(q *db.Queries) error {
			_, err := q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, Type: "menu", Name: "Other", Code: ptr("HR.LV")})
			return err
		}},
		{"nodes_parent_fk", func(q *db.Queries) error { return q.DeleteNode(ctx, hr.ID) }},
		{"contacts_client_fk", func(q *db.Queries) error {
			_, err := q.CreateContact(ctx, db.CreateContactParams{ClientID: ptr(int64(999999)), Name: "Ghost"})
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

`server/internal/access/access_test.go`:

```go
package access_test

import (
	"context"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

func TestRolesRankViewerMemberAdmin(t *testing.T) {
	for _, c := range []struct {
		role, need string
		ok         bool
	}{
		{access.Viewer, access.Viewer, true},
		{access.Viewer, access.Member, false},
		{access.Member, access.Member, true},
		{access.Member, access.Admin, false},
		{access.Admin, access.Viewer, true},
		{"", access.Viewer, false},
	} {
		if got := (access.Scope{Role: c.role}).Allows(c.need); got != c.ok {
			t.Errorf("%q allows %q = %v, want %v", c.role, c.need, got, c.ok)
		}
	}
}

func TestForProjectReadsTheMembership(t *testing.T) {
	d := testdb.New(t)
	q := db.New(d.Pool)
	ctx := context.Background()
	user := func(email string, admin bool) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{Email: email, Name: email, IsAdmin: admin, Locale: "id", Timezone: "Asia/Jakarta"})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	p, err := q.CreateProject(ctx, db.CreateProjectParams{Key: "HRIS", Name: "HRIS"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := q.CreateClient(ctx, db.CreateClientParams{Name: "Client B"})
	if err != nil {
		t.Fatal(err)
	}
	admin, all, scoped, outsider := user("admin@example.com", true), user("all@example.com", false),
		user("scoped@example.com", false), user("out@example.com", false)
	for _, err := range []error{
		q.LinkClients(ctx, db.LinkClientsParams{ProjectID: p.ID, ClientIds: []int64{b.ID}}),
		q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: all.ID, ProjectID: p.ID, Role: access.Member, AllClients: true}),
		q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: scoped.ID, ProjectID: p.ID, Role: access.Viewer}),
		q.AddMembershipClients(ctx, db.AddMembershipClientsParams{UserID: scoped.ID, ProjectID: p.ID, ClientIds: []int64{b.ID}}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		name   string
		u      db.User
		want   access.Scope
		member bool
	}{
		{"system admin", admin, access.Scope{Role: access.Admin, AllClients: true}, true},
		{"all clients", all, access.Scope{Role: access.Member, AllClients: true}, true},
		{"scoped", scoped, access.Scope{Role: access.Viewer, ClientIDs: []int64{b.ID}}, true},
		{"outsider", outsider, access.Scope{}, false},
	} {
		got, member, err := access.ForProject(ctx, q, &c.u, p.ID)
		if err != nil || member != c.member || got.Role != c.want.Role || got.AllClients != c.want.AllClients ||
			!slices.Equal(got.ClientIDs, c.want.ClientIDs) {
			t.Errorf("%s: %+v member=%v err=%v", c.name, got, member, err)
		}
	}
	if ok, err := access.AdminsAnything(ctx, q, &all); err != nil || ok {
		t.Errorf("a member is no admin: %v %v", ok, err)
	}
}
```

- [ ] **Step 5: Run the tests to verify they fail**

```bash
TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./internal/db/ ./internal/access/
```

Expected: `internal/db` passes (the queries exist), while `internal/access` fails to compile: `no required module provides package .../internal/access` or `undefined: access.Scope`. If `internal/db` fails on a constraint name, fix the migration, not the test.

- [ ] **Step 6: Write the `access` package**

`server/internal/access/access.go`:

```go
// Package access decides what a user may see and do in a project (FSD §5).
package access

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/db"
)

// Project roles, lowest first (FSD §5.1).
const (
	Viewer = "viewer"
	Member = "member"
	Admin  = "admin"
)

var rank = map[string]int{Viewer: 1, Member: 2, Admin: 3}

// Scope is one user's standing in one project: a role and the clients they may
// see there (R-AC-1). ClientIDs matters only when AllClients is false.
type Scope struct {
	Role       string
	AllClients bool
	ClientIDs  []int64
}

// Allows reports whether the scope's role is at least need.
func (s Scope) Allows(need string) bool { return rank[s.Role] >= rank[need] }

// ForProject reads u's scope in a project fresh from the database, so a changed
// scope applies on the user's next request (R-AC-8). System admins act as
// project admins with all clients everywhere; member is false when u does not
// belong to the project.
func ForProject(ctx context.Context, q *db.Queries, u *db.User, projectID int64) (scope Scope, member bool, err error) {
	if u.IsAdmin {
		return Scope{Role: Admin, AllClients: true}, true, nil
	}
	m, err := q.GetMembership(ctx, db.GetMembershipParams{UserID: u.ID, ProjectID: projectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Scope{}, false, nil
	}
	if err != nil {
		return Scope{}, false, err
	}
	return Scope{Role: m.Role, AllClients: m.AllClients, ClientIDs: m.ClientIds}, true, nil
}

// AdminsAnything reports whether u is a system admin or a project admin
// somewhere; both may list and create clients (FSD §5.1).
func AdminsAnything(ctx context.Context, q *db.Queries, u *db.User) (bool, error) {
	if u.IsAdmin {
		return true, nil
	}
	return q.IsProjectAdminAnywhere(ctx, u.ID)
}
```

- [ ] **Step 7: Run the tests to verify they pass**

```bash
TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...
```

Expected: `ok` for every package, including `internal/db` and `internal/access`.

- [ ] **Step 8: Commit**

```bash
git add server/migrations/00002_registry.sql server/internal/db server/internal/access
git commit -m "feat(server): registry schema, queries and the access package"
```

### Task 2: Projects API

**Files:**
- Modify: `api/openapi.yaml` (paths `/projects`, `/projects/{key}`; schemas `ProjectRole`, `Project`, `ProjectList`, `ProjectCreate`, `ProjectUpdate`)
- Modify: `server/internal/httpapi/oapi-codegen.yaml` (prefix enum constants with their type)
- Modify: `server/internal/db/queries/audit.sql`, `server/internal/httpapi/server.go`
- Create: `server/internal/httpapi/projects.go`
- Create: `server/internal/httpapi/seed_test.go`
- Test: `server/internal/httpapi/projects_test.go`
- Generate: `server/internal/httpapi/api.gen.go`, `server/internal/db/audit.sql.go`, `web/lib/api-types.ts`

**Interfaces:**
- Consumes: Task 1's queries and `access`.
- Produces:
  - Handlers: `ListProjects`, `CreateProject`, `GetProject(w, r, key string)`, `UpdateProject(w, r, key string)`.
  - Helpers for later tasks:
    - `type projectCtx struct{user *db.User; project db.Project; scope access.Scope}`
    - `(s *Server) projectFor(w, r, key, need string) (projectCtx, bool)` answers 404 for a missing project or a non-member, and 403 when the role is below `need`.
    - `(s *Server) memberOf(w, r, u *db.User, p db.Project) (projectCtx, bool)`
    - `auditMeta.inProject(id int64) auditMeta`
    - `changed(before, after map[string]any) map[string]any`
    - `constraintOf(err error) string`, `deref[T](*T) T`, `orEmpty[T]([]T) []T`
  - Enum constants are now prefixed: `ProjectRoleAdmin`, `ProjectRoleMember`, `ProjectRoleViewer`, and later `NodeTypeModule`, `NodeTypeMenu`. The Iteration 0 constants become `LocaleId` and `LocaleEn`; nothing uses them.
  - Test helpers in `seed_test.go`:
    - `(e *env) signedIn(email string, admin bool) (*http.Client, db.User)`
    - `seedProject(key string, clients ...db.Client) db.Project`
    - `seedClient(name string) db.Client`
    - `seedMember(u db.User, p db.Project, role string, clients ...db.Client)`: no clients means all clients.
    - `seedNode(p db.Project, parent *db.Node, typ, name string, clients ...db.Client) db.Node`: clients make it client-specific.
    - `seedContact(name string, client *db.Client) int64`
    - `firstError(p httpapi.Problem) httpapi.FieldError`

- [ ] **Step 1: Extend the contract**

In `server/internal/httpapi/oapi-codegen.yaml`, add at the end:

```yaml
compatibility:
  always-prefix-enum-values: true
```

In `api/openapi.yaml`, add under `paths:` (after `/admin/users/{id}/setup-link`):

```yaml
  /projects:
    get:
      operationId: listProjects
      tags: [projects]
      responses:
        "200":
          description: The projects the user belongs to; system admins see every project.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/ProjectList" }
        default: { $ref: "#/components/responses/Problem" }
    post:
      operationId: createProject
      tags: [projects]
      description: System admins only.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/ProjectCreate" }
      responses:
        "201":
          description: The new project.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Project" }
        default: { $ref: "#/components/responses/Problem" }
  /projects/{key}:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    get:
      operationId: getProject
      tags: [projects]
      description: Members only; everyone else gets 404.
      responses:
        "200":
          description: The project and the caller's role in it.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Project" }
        default: { $ref: "#/components/responses/Problem" }
    patch:
      operationId: updateProject
      tags: [projects]
      description: Project admins only.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/ProjectUpdate" }
      responses:
        "200":
          description: The updated project.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Project" }
        default: { $ref: "#/components/responses/Problem" }
```

And under `components.schemas:`:

```yaml
    ProjectRole:
      type: string
      enum: [admin, member, viewer]
    Project:
      type: object
      required: [id, key, name, description, role, created_at]
      properties:
        id: { type: integer, format: int64 }
        key: { type: string, example: HRIS }
        name: { type: string }
        description: { type: string }
        role: { $ref: "#/components/schemas/ProjectRole" }
        created_at: { type: string, format: date-time }
    ProjectList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/Project" }
    ProjectCreate:
      type: object
      required: [key, name]
      properties:
        key: { type: string, maxLength: 10, description: "2–10 capital letters or digits, starting with a letter; lower case is accepted" }
        name: { type: string, maxLength: 200 }
        description: { type: string, maxLength: 2000 }
    ProjectUpdate:
      type: object
      properties:
        key: { type: string, maxLength: 10 }
        name: { type: string, maxLength: 200 }
        description: { type: string, maxLength: 2000 }
```

In `server/internal/db/queries/audit.sql`, replace `InsertAuditEvent` so events can carry their project:

```sql
-- name: InsertAuditEvent :exec
INSERT INTO audit_events (actor_id, via, entity, entity_id, project_id, action, changes, request_id, ip)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
```

- [ ] **Step 2: Write the test helpers**

`server/internal/httpapi/seed_test.go`:

```go
package httpapi_test

import (
	"context"
	"net/http"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// The seeders write straight through the queries, so a test depends only on
// the handlers it exercises.

// signedIn seeds a user with the shared test password and signs them in.
func (e *env) signedIn(email string, admin bool) (*http.Client, db.User) {
	e.t.Helper()
	u := e.seedUser(email, pw, admin)
	c := e.client()
	if code, _ := login(e, c, email, pw); code != http.StatusOK {
		e.t.Fatalf("sign in %s: %d", email, code)
	}
	return c, u
}

func (e *env) seedProject(key string, clients ...db.Client) db.Project {
	e.t.Helper()
	ctx := context.Background()
	p, err := e.q.CreateProject(ctx, db.CreateProjectParams{Key: key, Name: key})
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.q.LinkClients(ctx, db.LinkClientsParams{ProjectID: p.ID, ClientIds: clientIDs(clients)}); err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *env) seedClient(name string) db.Client {
	e.t.Helper()
	c, err := e.q.CreateClient(context.Background(), db.CreateClientParams{Name: name})
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

// seedMember adds u to p with role; without clients the membership covers all clients.
func (e *env) seedMember(u db.User, p db.Project, role string, clients ...db.Client) {
	e.t.Helper()
	ctx := context.Background()
	err := e.q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: u.ID, ProjectID: p.ID, Role: role, AllClients: len(clients) == 0})
	if err == nil {
		err = e.q.AddMembershipClients(ctx, db.AddMembershipClientsParams{UserID: u.ID, ProjectID: p.ID, ClientIds: clientIDs(clients)})
	}
	if err != nil {
		e.t.Fatal(err)
	}
}

// seedNode adds a node under parent (nil = top level); clients make it a client-specific menu.
func (e *env) seedNode(p db.Project, parent *db.Node, typ, name string, clients ...db.Client) db.Node {
	e.t.Helper()
	ctx := context.Background()
	params := db.CreateNodeParams{ProjectID: p.ID, Type: typ, Name: name, ClientSpecific: len(clients) > 0}
	if parent != nil {
		params.ParentID = &parent.ID
	}
	n, err := e.q.CreateNode(ctx, params)
	if err == nil {
		err = e.q.AddNodeClients(ctx, db.AddNodeClientsParams{NodeID: n.ID, ProjectID: p.ID, ClientIds: clientIDs(clients)})
	}
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

// seedContact adds a contact of client, or an internal one when client is nil.
func (e *env) seedContact(name string, client *db.Client) int64 {
	e.t.Helper()
	params := db.CreateContactParams{Name: name}
	if client != nil {
		params.ClientID = &client.ID
	}
	id, err := e.q.CreateContact(context.Background(), params)
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func clientIDs(clients []db.Client) []int64 {
	ids := make([]int64, len(clients))
	for i, c := range clients {
		ids[i] = c.ID
	}
	return ids
}

// firstError returns a problem's first field error, or a zero one.
func firstError(p httpapi.Problem) httpapi.FieldError {
	if p.Errors == nil || len(*p.Errors) == 0 {
		return httpapi.FieldError{}
	}
	return (*p.Errors)[0]
}
```

- [ ] **Step 3: Write the failing tests**

`server/internal/httpapi/projects_test.go`:

```go
package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func TestAdminCreatesAProjectAndMembersSeeOnlyTheirProjects(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.signedIn("admin@example.com", true)
	var p httpapi.Project
	code := e.call(admin, http.MethodPost, "/projects", map[string]any{"key": " hris ", "name": " HRIS ", "description": "Human resources"}, &p)
	if code != http.StatusCreated || p.Key != "HRIS" || p.Name != "HRIS" || p.Role != httpapi.ProjectRoleAdmin {
		t.Fatalf("create: %d %+v", code, p)
	}
	pay := e.seedProject("PAY")
	budi, bu := e.signedIn("budi@example.com", false)
	e.seedMember(bu, pay, "viewer")
	var list httpapi.ProjectList
	if code := e.call(budi, http.MethodGet, "/projects", nil, &list); code != http.StatusOK ||
		len(list.Items) != 1 || list.Items[0].Key != "PAY" || list.Items[0].Role != httpapi.ProjectRoleViewer {
		t.Fatalf("budi's projects: %d %+v", code, list)
	}
	if code := e.call(budi, http.MethodGet, "/projects/HRIS", nil, nil); code != http.StatusNotFound {
		t.Fatalf("a non-member must get 404, got %d", code)
	}
	if code := e.call(admin, http.MethodGet, "/projects", nil, &list); code != http.StatusOK || len(list.Items) != 2 {
		t.Fatalf("a system admin sees every project: %d %+v", code, list)
	}
	if code := e.call(budi, http.MethodPost, "/projects", map[string]any{"key": "NEW", "name": "New"}, nil); code != http.StatusForbidden {
		t.Fatalf("only system admins create projects, got %d", code)
	}
}

func TestProjectKeyRules(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.signedIn("admin@example.com", true)
	for _, key := range []string{"H", "1HR", "HR-IS", "ABCDEFGHIJK"} {
		var p httpapi.Problem
		if code := e.call(admin, http.MethodPost, "/projects", map[string]any{"key": key, "name": "X"}, &p); code != http.StatusUnprocessableEntity || firstError(p).Field != "key" {
			t.Errorf("key %q: %d %+v", key, code, p)
		}
	}
	e.call(admin, http.MethodPost, "/projects", map[string]any{"key": "HRIS", "name": "HRIS"}, nil)
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPost, "/projects", map[string]any{"key": "HRIS", "name": "Again"}, &p); code != http.StatusConflict || p.Code != "project_key_taken" {
		t.Fatalf("duplicate key: %d %+v", code, p)
	}
}

func TestOnlyProjectAdminsEditAProjectAndEditsAreAudited(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	member, mu := e.signedIn("member@example.com", false)
	e.seedMember(mu, p, "member")
	owner, ou := e.signedIn("owner@example.com", false)
	e.seedMember(ou, p, "admin")
	var prob httpapi.Problem
	if code := e.call(member, http.MethodPatch, "/projects/HRIS", map[string]any{"name": "Nope"}, &prob); code != http.StatusForbidden || prob.Code != "forbidden" {
		t.Fatalf("member edit: %d %+v", code, prob)
	}
	var out httpapi.Project
	if code := e.call(owner, http.MethodPatch, "/projects/HRIS", map[string]any{"name": "HR System", "key": "hrs"}, &out); code != http.StatusOK ||
		out.Key != "HRS" || out.Name != "HR System" || out.Role != httpapi.ProjectRoleAdmin {
		t.Fatalf("admin edit: %d %+v", code, out)
	}
	events, err := e.q.ListAuditEvents(context.Background(), db.ListAuditEventsParams{Entity: "project", EntityID: p.ID})
	if err != nil || len(events) != 1 || events[0].ProjectID == nil || *events[0].ProjectID != p.ID {
		t.Fatalf("audit: %+v %v", events, err)
	}
	var changes map[string]map[string]any
	if err := json.Unmarshal(events[0].Changes, &changes); err != nil ||
		changes["name"]["old"] != "HRIS" || changes["name"]["new"] != "HR System" || changes["description"] != nil {
		t.Fatalf("changes: %s", events[0].Changes)
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

```bash
cd server && go generate ./... && go vet ./internal/httpapi/
```

Expected: FAIL. `*Server does not implement ServerInterface (missing method CreateProject)`, and the tests do not compile: `undefined: httpapi.Project`.

- [ ] **Step 5: Extend the server helpers**

In `server/internal/httpapi/server.go`, replace `auditMeta` and `audit`, and add the helpers after `ptr`. The imports gain `errors`, `reflect` and `github.com/jackc/pgx/v5/pgconn`.

```go
// auditMeta records where a change came from.
type auditMeta struct {
	via       string
	requestID *string
	ip        *netip.Addr
	projectID *int64 // lets project admins search their project's history (FSD §5.1)
}

// inProject tags the event with its project.
func (m auditMeta) inProject(id int64) auditMeta {
	m.projectID = &id
	return m
}
```

```go
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
		ActorID: actorID, Via: m.via, Entity: entity, EntityID: entityID, ProjectID: m.projectID,
		Action: action, Changes: b, RequestID: m.requestID, Ip: m.ip,
	})
}
```

```go
// changed keeps the fields whose values differ, as {"field": {"old": …, "new": …}} (FSD §16).
func changed(before, after map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range after {
		if !reflect.DeepEqual(before[k], v) {
			out[k] = map[string]any{"old": before[k], "new": v}
		}
	}
	return out
}

// constraintOf names the database constraint err violated, or "".
func constraintOf(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

func deref[T any](p *T) (v T) {
	if p != nil {
		v = *p
	}
	return v
}

// orEmpty keeps a JSON array [] instead of null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
```

- [ ] **Step 6: Write the handlers**

`server/internal/httpapi/projects.go`:

```go
package httpapi

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

var projectKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)

// projectCtx is a request's project and the caller's standing in it.
type projectCtx struct {
	user    *db.User
	project db.Project
	scope   access.Scope
}

// projectFor resolves {key} for the signed-in user. A missing project and one
// the user does not belong to both answer 404, so projects never reveal
// themselves (R-AC-7); a role below need answers 403.
func (s *Server) projectFor(w http.ResponseWriter, r *http.Request, key, need string) (projectCtx, bool) {
	u := s.requireUser(w, r)
	if u == nil {
		return projectCtx{}, false
	}
	p, err := s.q.GetProjectByKey(r.Context(), key)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Project not found")
		return projectCtx{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, false
	}
	pc, ok := s.memberOf(w, r, u, p)
	if ok && !pc.scope.Allows(need) {
		writeProblem(w, http.StatusForbidden, "forbidden", "Your project role does not allow this")
		return projectCtx{}, false
	}
	return pc, ok
}

// memberOf reads u's scope in p and answers 404 when u is not a member.
func (s *Server) memberOf(w http.ResponseWriter, r *http.Request, u *db.User, p db.Project) (projectCtx, bool) {
	scope, member, err := access.ForProject(r.Context(), s.q, u, p.ID)
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, false
	}
	if !member {
		writeProblem(w, http.StatusNotFound, "not_found", "Project not found")
		return projectCtx{}, false
	}
	return projectCtx{user: u, project: p, scope: scope}, true
}

func (s *Server) ListProjects(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := s.q.ListProjects(r.Context(), db.ListProjectsParams{UserID: u.ID, IsAdmin: u.IsAdmin})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]Project, len(rows))
	for i, row := range rows {
		role := access.Admin
		if !u.IsAdmin {
			role = *row.Role
		}
		items[i] = toAPIProject(row.Project, role)
	}
	writeJSON(w, http.StatusOK, ProjectList{Items: items})
}

func (s *Server) CreateProject(w http.ResponseWriter, r *http.Request) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}
	var in ProjectCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	key := strings.ToUpper(strings.TrimSpace(in.Key))
	if fields := validateProject(&key, &in.Name, in.Description); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	ctx := r.Context()
	var p db.Project
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if p, err = q.CreateProject(ctx, db.CreateProjectParams{
			Key: key, Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(deref(in.Description)),
		}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(p.ID), &admin.ID, "project", p.ID, "create", projectAudit(p))
	})
	if isUniqueViolation(err) {
		projectKeyTaken(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAPIProject(p, access.Admin))
}

func (s *Server) GetProject(w http.ResponseWriter, r *http.Request, key string) {
	if pc, ok := s.projectFor(w, r, key, access.Viewer); ok {
		writeJSON(w, http.StatusOK, toAPIProject(pc.project, pc.scope.Role))
	}
}

func (s *Server) UpdateProject(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in ProjectUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Key != nil {
		in.Key = ptr(strings.ToUpper(strings.TrimSpace(*in.Key)))
	}
	if fields := validateProject(in.Key, in.Name, in.Description); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	ctx := r.Context()
	var updated db.Project
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if updated, err = q.UpdateProject(ctx, db.UpdateProjectParams{
			ID: pc.project.ID, Key: in.Key, Name: trimmed(in.Name), Description: trimmed(in.Description),
		}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "project", pc.project.ID, "update",
			changed(projectAudit(pc.project), projectAudit(updated)))
	})
	if isUniqueViolation(err) {
		projectKeyTaken(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIProject(updated, pc.scope.Role))
}

func validateProject(key, name, description *string) []FieldError {
	var f []FieldError
	if key != nil && !projectKeyRe.MatchString(*key) {
		f = append(f, FieldError{Field: "key", Code: "invalid", Message: "Use 2 to 10 capital letters or digits, starting with a letter"})
	}
	if name != nil {
		if n := strings.TrimSpace(*name); n == "" || len(n) > 200 {
			f = append(f, FieldError{Field: "name", Code: "required", Message: "Enter a name of at most 200 characters"})
		}
	}
	if description != nil && len(*description) > 2000 {
		f = append(f, FieldError{Field: "description", Code: "invalid", Message: "Use at most 2,000 characters"})
	}
	return f
}

func projectKeyTaken(w http.ResponseWriter) {
	const msg = "Another project already uses this key"
	writeProblem(w, http.StatusConflict, "project_key_taken", msg, FieldError{Field: "key", Code: "project_key_taken", Message: msg})
}

func projectAudit(p db.Project) map[string]any {
	return map[string]any{"key": p.Key, "name": p.Name, "description": p.Description}
}

func toAPIProject(p db.Project, role string) Project {
	return Project{Id: p.ID, Key: p.Key, Name: p.Name, Description: p.Description, Role: ProjectRole(role), CreatedAt: p.CreatedAt}
}
```

- [ ] **Step 7: Generate and run the tests**

```bash
cd server && go generate ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...
cd ../web && npm run gen:api
```

Expected: `ok` for every Go package. `web/lib/api-types.ts` gains the project paths.

- [ ] **Step 8: Commit**

```bash
git add api/openapi.yaml server web/lib/api-types.ts
git commit -m "feat(server): projects API with project-tagged audit events"
```

### Task 3: Clients and a project's linked clients

**Files:**
- Modify: `api/openapi.yaml` (paths `/clients`, `/clients/{id}`, `/projects/{key}/clients`; schemas `Client`, `ClientList`, `ClientCreate`, `ClientUpdate`, `ProjectClientsUpdate`)
- Create: `server/internal/httpapi/clients.go`
- Test: `server/internal/httpapi/clients_test.go`
- Generate: `server/internal/httpapi/api.gen.go`, `web/lib/api-types.ts`

**Interfaces:**
- Consumes: `projectFor`, `changed`, `constraintOf`, `deref`, `orEmpty`, the seeders (Task 2); `access.AdminsAnything` (Task 1).
- Produces:
  - Handlers: `ListClients`, `CreateClient`, `UpdateClient(w, r, id int64)`, `ListProjectClients(w, r, key string)`, `SetProjectClients(w, r, key string)`.
  - Helpers for later tasks: `cleanAliases([]string) ([]string, bool)`, `nonEmpty(*string) *string`, `aliasesError FieldError`.
  - Error codes: `client_name_taken` (409), `client_in_use` (409), `unknown_client` (422 field code).

- [ ] **Step 1: Extend the contract**

`api/openapi.yaml`, under `paths:`:

```yaml
  /projects/{key}/clients:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    get:
      operationId: listProjectClients
      tags: [projects]
      description: Project admins only.
      responses:
        "200":
          description: The clients linked to the project.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/ClientList" }
        default: { $ref: "#/components/responses/Problem" }
    put:
      operationId: setProjectClients
      tags: [projects]
      description: Project admins only. Replaces the linked clients; removing one that menus or member scopes use answers 409 client_in_use.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/ProjectClientsUpdate" }
      responses:
        "200":
          description: The clients now linked.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/ClientList" }
        default: { $ref: "#/components/responses/Problem" }
  /clients:
    get:
      operationId: listClients
      tags: [clients]
      description: System admins and project admins.
      responses:
        "200":
          description: Every client, archived ones included.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/ClientList" }
        default: { $ref: "#/components/responses/Problem" }
    post:
      operationId: createClient
      tags: [clients]
      description: System admins and project admins.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/ClientCreate" }
      responses:
        "201":
          description: The new client.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Client" }
        default: { $ref: "#/components/responses/Problem" }
  /clients/{id}:
    parameters:
      - { name: id, in: path, required: true, schema: { type: integer, format: int64 } }
    patch:
      operationId: updateClient
      tags: [clients]
      description: System admins only. An empty code clears it.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/ClientUpdate" }
      responses:
        "200":
          description: The updated client.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Client" }
        default: { $ref: "#/components/responses/Problem" }
```

Under `components.schemas:`:

```yaml
    Client:
      type: object
      required: [id, name, code, aliases, archived]
      properties:
        id: { type: integer, format: int64 }
        name: { type: string }
        code: { type: string, nullable: true, example: CLA }
        aliases:
          type: array
          items: { type: string }
        archived: { type: boolean }
    ClientList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/Client" }
    ClientCreate:
      type: object
      required: [name]
      properties:
        name: { type: string, maxLength: 200 }
        code: { type: string, maxLength: 20 }
        aliases:
          type: array
          maxItems: 20
          items: { type: string, maxLength: 100 }
    ClientUpdate:
      type: object
      properties:
        name: { type: string, maxLength: 200 }
        code: { type: string, maxLength: 20 }
        aliases:
          type: array
          maxItems: 20
          items: { type: string, maxLength: 100 }
        archived: { type: boolean }
    ProjectClientsUpdate:
      type: object
      required: [client_ids]
      properties:
        client_ids:
          type: array
          items: { type: integer, format: int64 }
```

- [ ] **Step 2: Write the failing tests**

`server/internal/httpapi/clients_test.go`:

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

func TestSystemAdminsManageClients(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.signedIn("admin@example.com", true)
	var c httpapi.Client
	code := e.call(admin, http.MethodPost, "/clients", map[string]any{"name": " Client A ", "code": "CLA", "aliases": []string{" PT Alfa ", ""}}, &c)
	if code != http.StatusCreated || c.Name != "Client A" || c.Code == nil || *c.Code != "CLA" ||
		!slices.Equal(c.Aliases, []string{"PT Alfa"}) || c.Archived {
		t.Fatalf("create: %d %+v", code, c)
	}
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPost, "/clients", map[string]any{"name": "client a"}, &p); code != http.StatusConflict || p.Code != "client_name_taken" {
		t.Fatalf("duplicate name: %d %+v", code, p)
	}
	path := fmt.Sprintf("/clients/%d", c.Id)
	if code := e.call(admin, http.MethodPatch, path, map[string]any{"archived": true, "code": ""}, &c); code != http.StatusOK ||
		!c.Archived || c.Code != nil || c.Name != "Client A" || len(c.Aliases) != 1 {
		t.Fatalf("archive: %d %+v", code, c)
	}
	events, err := e.q.ListAuditEvents(context.Background(), db.ListAuditEventsParams{Entity: "client", EntityID: c.Id})
	if err != nil || len(events) != 2 || events[1].Action != "update" {
		t.Fatalf("audit: %+v %v", events, err)
	}
}

func TestProjectAdminsCreateClientsButOnlySystemAdminsEditThem(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	owner, ou := e.signedIn("owner@example.com", false)
	e.seedMember(ou, p, "admin")
	member, mu := e.signedIn("member@example.com", false)
	e.seedMember(mu, p, "member")
	var c httpapi.Client
	if code := e.call(owner, http.MethodPost, "/clients", map[string]any{"name": "Client B"}, &c); code != http.StatusCreated {
		t.Fatalf("project admin creates: %d", code)
	}
	if code := e.call(owner, http.MethodGet, "/clients", nil, nil); code != http.StatusOK {
		t.Fatalf("project admin lists: %d", code)
	}
	if code := e.call(owner, http.MethodPatch, fmt.Sprintf("/clients/%d", c.Id), map[string]any{"name": "X"}, nil); code != http.StatusForbidden {
		t.Fatalf("project admin renames: %d", code)
	}
	if code := e.call(member, http.MethodGet, "/clients", nil, nil); code != http.StatusForbidden {
		t.Fatalf("member lists: %d", code)
	}
	if code := e.call(member, http.MethodPost, "/clients", map[string]any{"name": "Client C"}, nil); code != http.StatusForbidden {
		t.Fatalf("member creates: %d", code)
	}
}

func TestLinkingClientsToAProject(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	p := e.seedProject("HRIS")
	admin, _ := e.signedIn("admin@example.com", true)
	var list httpapi.ClientList
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/clients", map[string]any{"client_ids": []int64{a.ID, b.ID}}, &list); code != http.StatusOK || len(list.Items) != 2 {
		t.Fatalf("link: %d %+v", code, list)
	}
	budi, bu := e.signedIn("budi@example.com", false)
	e.seedMember(bu, p, "member", b) // a member scope now uses Client B
	var prob httpapi.Problem
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/clients", map[string]any{"client_ids": []int64{a.ID}}, &prob); code != http.StatusConflict || prob.Code != "client_in_use" {
		t.Fatalf("unlink a client in use: %d %+v", code, prob)
	}
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/clients", map[string]any{"client_ids": []int64{a.ID, b.ID, 999999}}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Code != "unknown_client" {
		t.Fatalf("unknown client: %d %+v", code, prob)
	}
	if code := e.call(budi, http.MethodGet, "/projects/HRIS/clients", nil, nil); code != http.StatusForbidden {
		t.Fatalf("a member reads the links: %d", code)
	}
	if code := e.call(admin, http.MethodGet, "/projects/HRIS/clients", nil, &list); code != http.StatusOK || len(list.Items) != 2 {
		t.Fatalf("failed updates must change nothing: %d %+v", code, list)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd server && go generate ./... && go vet ./internal/httpapi/
```

Expected: FAIL. `*Server does not implement ServerInterface (missing method CreateClient)`.

- [ ] **Step 4: Write the handlers**

`server/internal/httpapi/clients.go`:

```go
package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

var aliasesError = FieldError{Field: "aliases", Code: "invalid", Message: "Use at most 20 aliases of at most 100 characters each"}

// clientAdmin lets system admins and project admins through: both list and
// create clients to link them to projects (FSD §5.1).
func (s *Server) clientAdmin(w http.ResponseWriter, r *http.Request) *db.User {
	u := s.requireUser(w, r)
	if u == nil {
		return nil
	}
	ok, err := access.AdminsAnything(r.Context(), s.q, u)
	if err != nil {
		s.fail(w, r, err)
		return nil
	}
	if !ok {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only admins can manage clients")
		return nil
	}
	return u
}

func (s *Server) ListClients(w http.ResponseWriter, r *http.Request) {
	if s.clientAdmin(w, r) == nil {
		return
	}
	rows, err := s.q.ListClients(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ClientList{Items: toAPIClients(rows)})
}

func (s *Server) CreateClient(w http.ResponseWriter, r *http.Request) {
	u := s.clientAdmin(w, r)
	if u == nil {
		return
	}
	var in ClientCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	aliases, aliasesOK := cleanAliases(deref(in.Aliases))
	if fields := validateClient(&in.Name, in.Code, aliasesOK); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	ctx := r.Context()
	var c db.Client
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if c, err = q.CreateClient(ctx, db.CreateClientParams{Name: strings.TrimSpace(in.Name), Code: nonEmpty(in.Code), Aliases: aliases}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &u.ID, "client", c.ID, "create", clientAudit(c))
	})
	if isUniqueViolation(err) {
		clientNameTaken(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAPIClient(c))
}

// UpdateClient renames or archives a client. Clients are shared by every
// project, so only system admins change them.
func (s *Server) UpdateClient(w http.ResponseWriter, r *http.Request, id int64) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}
	var in ClientUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	var aliases []string
	aliasesOK := true
	if in.Aliases != nil {
		aliases, aliasesOK = cleanAliases(*in.Aliases)
	}
	if fields := validateClient(in.Name, in.Code, aliasesOK); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	ctx := r.Context()
	var updated db.Client
	err := s.inTx(ctx, func(q *db.Queries) error {
		before, err := q.GetClient(ctx, id)
		if err != nil {
			return err
		}
		if updated, err = q.UpdateClient(ctx, db.UpdateClientParams{
			ID: id, Name: trimmed(in.Name), Code: trimmed(in.Code), Aliases: aliases, Archived: in.Archived,
		}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &admin.ID, "client", id, "update", changed(clientAudit(before), clientAudit(updated)))
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeProblem(w, http.StatusNotFound, "not_found", "Client not found")
	case isUniqueViolation(err):
		clientNameTaken(w)
	case err != nil:
		s.fail(w, r, err)
	default:
		writeJSON(w, http.StatusOK, toAPIClient(updated))
	}
}

func (s *Server) ListProjectClients(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	rows, err := s.q.ListProjectClients(r.Context(), pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ClientList{Items: toAPIClients(rows)})
}

// SetProjectClients replaces the project's client links (FSD §15.2). The
// database refuses to unlink a client that menus or member scopes still use.
func (s *Server) SetProjectClients(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in ProjectClientsUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.ClientIds == nil {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "client_ids", Code: "required", Message: "List the project's clients"})
		return
	}
	ctx := r.Context()
	var rows []db.Client
	err := s.inTx(ctx, func(q *db.Queries) error {
		before, err := q.ListProjectClients(ctx, pc.project.ID)
		if err != nil {
			return err
		}
		if err := q.UnlinkClientsExcept(ctx, db.UnlinkClientsExceptParams{ProjectID: pc.project.ID, ClientIds: in.ClientIds}); err != nil {
			return err
		}
		if err := q.LinkClients(ctx, db.LinkClientsParams{ProjectID: pc.project.ID, ClientIds: in.ClientIds}); err != nil {
			return err
		}
		if rows, err = q.ListProjectClients(ctx, pc.project.ID); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "project", pc.project.ID, "set_clients",
			changed(map[string]any{"client_ids": idsOf(before)}, map[string]any{"client_ids": idsOf(rows)}))
	})
	switch constraintOf(err) {
	case "membership_clients_linked", "node_clients_linked":
		writeProblem(w, http.StatusConflict, "client_in_use", "A client you removed is still used by menus or member scopes in this project")
		return
	case "project_clients_client_fk":
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "client_ids", Code: "unknown_client", Message: "One of these clients does not exist"})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ClientList{Items: toAPIClients(rows)})
}

func validateClient(name, code *string, aliasesOK bool) []FieldError {
	var f []FieldError
	if name != nil {
		if n := strings.TrimSpace(*name); n == "" || len(n) > 200 {
			f = append(f, FieldError{Field: "name", Code: "required", Message: "Enter a name of at most 200 characters"})
		}
	}
	if code != nil && len(strings.TrimSpace(*code)) > 20 {
		f = append(f, FieldError{Field: "code", Code: "invalid", Message: "Use at most 20 characters"})
	}
	if !aliasesOK {
		f = append(f, aliasesError)
	}
	return f
}

// cleanAliases trims aliases and drops empty ones; ok is false past 20
// aliases or 100 characters. The result is never nil, so an empty list clears.
func cleanAliases(in []string) (out []string, ok bool) {
	out = []string{}
	for _, a := range in {
		if a = strings.TrimSpace(a); a == "" {
			continue
		}
		if len(a) > 100 {
			return nil, false
		}
		out = append(out, a)
	}
	return out, len(out) <= 20
}

// nonEmpty trims s and turns "" into nil.
func nonEmpty(s *string) *string {
	if s == nil {
		return nil
	}
	if t := strings.TrimSpace(*s); t != "" {
		return &t
	}
	return nil
}

func clientNameTaken(w http.ResponseWriter) {
	const msg = "A client with this name already exists"
	writeProblem(w, http.StatusConflict, "client_name_taken", msg, FieldError{Field: "name", Code: "client_name_taken", Message: msg})
}

func clientAudit(c db.Client) map[string]any {
	return map[string]any{"name": c.Name, "code": c.Code, "aliases": c.Aliases, "archived": c.ArchivedAt != nil}
}

func idsOf(clients []db.Client) []int64 {
	ids := make([]int64, len(clients))
	for i, c := range clients {
		ids[i] = c.ID
	}
	return ids
}

func toAPIClient(c db.Client) Client {
	return Client{Id: c.ID, Name: c.Name, Code: c.Code, Aliases: orEmpty(c.Aliases), Archived: c.ArchivedAt != nil}
}

func toAPIClients(rows []db.Client) []Client {
	items := make([]Client, len(rows))
	for i, c := range rows {
		items[i] = toAPIClient(c)
	}
	return items
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
git commit -m "feat(server): clients and project client links"
```

### Task 4: Project members and client scopes

**Files:**
- Modify: `api/openapi.yaml` (path `/projects/{key}/members`; schemas `Member`, `MemberList`, `MemberInput`, `MembersUpdate`)
- Create: `server/internal/httpapi/members.go`
- Test: `server/internal/httpapi/members_test.go`
- Generate: `server/internal/httpapi/api.gen.go`, `web/lib/api-types.ts`

**Interfaces:**
- Consumes: `projectFor`, `changed`, `constraintOf`, `deref`, `orEmpty`, the seeders (Task 2).
- Produces:
  - Handlers: `ListProjectMembers(w, r, key string)`, `SetProjectMembers(w, r, key string)`.
  - Error codes (422 field codes): `admin_needs_all_clients`, `unknown_user`, `duplicate`, `cannot_demote_self`, `client_not_linked`.

- [ ] **Step 1: Extend the contract**

`api/openapi.yaml`, under `paths:`:

```yaml
  /projects/{key}/members:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    get:
      operationId: listProjectMembers
      tags: [projects]
      description: Project admins only.
      responses:
        "200":
          description: The project's members and their client scopes.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/MemberList" }
        default: { $ref: "#/components/responses/Problem" }
    put:
      operationId: setProjectMembers
      tags: [projects]
      description: Project admins only. Replaces every membership of the project at once.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/MembersUpdate" }
      responses:
        "200":
          description: The members now.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/MemberList" }
        default: { $ref: "#/components/responses/Problem" }
```

Under `components.schemas:`:

```yaml
    Member:
      type: object
      required: [user_id, name, email, role, all_clients, client_ids]
      properties:
        user_id: { type: integer, format: int64 }
        name: { type: string }
        email: { type: string }
        role: { $ref: "#/components/schemas/ProjectRole" }
        all_clients: { type: boolean }
        client_ids:
          type: array
          items: { type: integer, format: int64 }
    MemberList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/Member" }
    MemberInput:
      type: object
      required: [email, role, all_clients]
      properties:
        email: { type: string, maxLength: 320 }
        role: { $ref: "#/components/schemas/ProjectRole" }
        all_clients: { type: boolean, description: Project admins always have all clients. }
        client_ids:
          type: array
          description: The clients a scoped member sees; ignored with all_clients.
          items: { type: integer, format: int64 }
    MembersUpdate:
      type: object
      required: [members]
      properties:
        members:
          type: array
          items: { $ref: "#/components/schemas/MemberInput" }
```

- [ ] **Step 2: Write the failing tests**

`server/internal/httpapi/members_test.go`:

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

func TestProjectAdminSetsMembersAndScopes(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	p := e.seedProject("HRIS", a, b)
	owner, ou := e.signedIn("owner@example.com", false)
	e.seedMember(ou, p, "admin")
	e.seedUser("budi@example.com", pw, false)
	e.seedUser("ani@example.com", pw, false)
	body := map[string]any{"members": []map[string]any{
		{"email": "owner@example.com", "role": "admin", "all_clients": true},
		{"email": "BUDI@example.com", "role": "member", "all_clients": false, "client_ids": []int64{b.ID}},
		{"email": "ani@example.com", "role": "viewer", "all_clients": true, "client_ids": []int64{a.ID}},
	}}
	var list httpapi.MemberList
	if code := e.call(owner, http.MethodPut, "/projects/HRIS/members", body, &list); code != http.StatusOK || len(list.Items) != 3 {
		t.Fatalf("set: %d %+v", code, list)
	}
	byEmail := map[string]httpapi.Member{}
	for _, m := range list.Items {
		byEmail[m.Email] = m
	}
	if m := byEmail["budi@example.com"]; m.Role != httpapi.ProjectRoleMember || m.AllClients || !slices.Equal(m.ClientIds, []int64{b.ID}) {
		t.Fatalf("budi: %+v", m)
	}
	if m := byEmail["ani@example.com"]; !m.AllClients || len(m.ClientIds) != 0 {
		t.Fatalf("all clients ignores the list: %+v", m)
	}
	events, err := e.q.ListAuditEvents(context.Background(), db.ListAuditEventsParams{Entity: "project", EntityID: p.ID})
	if err != nil || len(events) != 1 || events[0].Action != "set_members" {
		t.Fatalf("audit: %+v %v", events, err)
	}
}

func TestMemberUpdatesAreValidated(t *testing.T) {
	e := newEnv(t)
	a, other := e.seedClient("Client A"), e.seedClient("Client Z")
	p := e.seedProject("HRIS", a)
	owner, ou := e.signedIn("owner@example.com", false)
	e.seedMember(ou, p, "admin")
	e.seedUser("budi@example.com", pw, false)
	self := map[string]any{"email": "owner@example.com", "role": "admin", "all_clients": true}
	for _, c := range []struct {
		members     []map[string]any
		field, code string
	}{
		{[]map[string]any{self, {"email": "budi@example.com", "role": "admin", "all_clients": false}}, "members[1].all_clients", "admin_needs_all_clients"},
		{[]map[string]any{self, {"email": "nobody@example.com", "role": "member", "all_clients": true}}, "members[1].email", "unknown_user"},
		{[]map[string]any{self, self}, "members[1].email", "duplicate"},
		{[]map[string]any{self, {"email": "budi@example.com", "role": "owner", "all_clients": true}}, "members[1].role", "invalid"},
		{[]map[string]any{{"email": "owner@example.com", "role": "member", "all_clients": true}}, "members", "cannot_demote_self"},
		{[]map[string]any{self, {"email": "budi@example.com", "role": "member", "all_clients": false, "client_ids": []int64{other.ID}}}, "members", "client_not_linked"},
	} {
		var prob httpapi.Problem
		code := e.call(owner, http.MethodPut, "/projects/HRIS/members", map[string]any{"members": c.members}, &prob)
		if f := firstError(prob); code != http.StatusUnprocessableEntity || f.Field != c.field || f.Code != c.code {
			t.Errorf("%s: %d %+v", c.code, code, prob)
		}
	}
}

// R-AC-8: a changed scope applies on the user's next request.
func TestAScopeChangeAppliesOnTheNextRequest(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	admin, _ := e.signedIn("admin@example.com", true)
	budi, bu := e.signedIn("budi@example.com", false)
	e.seedMember(bu, p, "member")
	if code := e.call(budi, http.MethodGet, "/projects/HRIS", nil, nil); code != http.StatusOK {
		t.Fatalf("before: %d", code)
	}
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/members", map[string]any{"members": []any{}}, nil); code != http.StatusOK {
		t.Fatalf("remove everyone: %d", code)
	}
	if code := e.call(budi, http.MethodGet, "/projects/HRIS", nil, nil); code != http.StatusNotFound {
		t.Fatalf("after: %d", code)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd server && go generate ./... && go vet ./internal/httpapi/
```

Expected: FAIL. `*Server does not implement ServerInterface (missing method ListProjectMembers)`.

- [ ] **Step 4: Write the handlers**

`server/internal/httpapi/members.go`:

```go
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

func (s *Server) ListProjectMembers(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	rows, err := s.q.ListProjectMembers(r.Context(), pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, MemberList{Items: toAPIMembers(rows)})
}

// SetProjectMembers replaces the project's members and their client scopes in
// one transaction, for one member or many at once (FSD §15.2).
func (s *Server) SetProjectMembers(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in MembersUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Members == nil {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "members", Code: "required", Message: "List the project's members"})
		return
	}
	ctx := r.Context()
	members, fields, err := s.resolveMembers(ctx, in.Members)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A project admin cannot lock themselves out; system admins keep access anyway.
	if !pc.user.IsAdmin && !keepsAdmin(members, pc.user.ID) {
		fields = append(fields, FieldError{Field: "members", Code: "cannot_demote_self", Message: "You cannot remove your own project admin role"})
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	pid := pc.project.ID
	var rows []db.ListProjectMembersRow
	err = s.inTx(ctx, func(q *db.Queries) error {
		before, err := q.ListProjectMembers(ctx, pid)
		if err != nil {
			return err
		}
		userIDs := make([]int64, len(members))
		for i, m := range members {
			userIDs[i] = m.userID
		}
		if err := q.DeleteMembershipsExcept(ctx, db.DeleteMembershipsExceptParams{ProjectID: pid, UserIds: userIDs}); err != nil {
			return err
		}
		for _, m := range members {
			if err := q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: m.userID, ProjectID: pid, Role: m.role, AllClients: m.all}); err != nil {
				return err
			}
			if err := q.ClearMembershipClients(ctx, db.ClearMembershipClientsParams{UserID: m.userID, ProjectID: pid}); err != nil {
				return err
			}
			if err := q.AddMembershipClients(ctx, db.AddMembershipClientsParams{UserID: m.userID, ProjectID: pid, ClientIds: orEmpty(m.clients)}); err != nil {
				return err
			}
		}
		if rows, err = q.ListProjectMembers(ctx, pid); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pid), &pc.user.ID, "project", pid, "set_members",
			changed(map[string]any{"members": memberAudit(before)}, map[string]any{"members": memberAudit(rows)}))
	})
	if constraintOf(err) == "membership_clients_linked" {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "members", Code: "client_not_linked", Message: "Scopes can list only the project's clients"})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, MemberList{Items: toAPIMembers(rows)})
}

// member is one validated entry of a members update.
type member struct {
	userID  int64
	role    string
	all     bool
	clients []int64
}

// resolveMembers validates the entries and looks their users up by email.
func (s *Server) resolveMembers(ctx context.Context, in []MemberInput) ([]member, []FieldError, error) {
	var out []member
	var fields []FieldError
	seen := map[int64]bool{}
	for i, m := range in {
		at := fmt.Sprintf("members[%d].", i)
		if !m.Role.Valid() {
			fields = append(fields, FieldError{Field: at + "role", Code: "invalid", Message: "Choose admin, member or viewer"})
			continue
		}
		if m.Role == ProjectRoleAdmin && !m.AllClients {
			fields = append(fields, FieldError{Field: at + "all_clients", Code: "admin_needs_all_clients", Message: "Project admins always see all clients"})
			continue
		}
		u, err := s.q.GetUserByEmail(ctx, strings.TrimSpace(m.Email))
		if errors.Is(err, pgx.ErrNoRows) {
			fields = append(fields, FieldError{Field: at + "email", Code: "unknown_user", Message: "No user has this email"})
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if seen[u.ID] {
			fields = append(fields, FieldError{Field: at + "email", Code: "duplicate", Message: "This user is listed twice"})
			continue
		}
		seen[u.ID] = true
		entry := member{userID: u.ID, role: string(m.Role), all: m.AllClients}
		if !m.AllClients {
			entry.clients = deref(m.ClientIds)
		}
		out = append(out, entry)
	}
	return out, fields, nil
}

func keepsAdmin(members []member, userID int64) bool {
	for _, m := range members {
		if m.userID == userID && m.role == access.Admin {
			return true
		}
	}
	return false
}

func memberAudit(rows []db.ListProjectMembersRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, m := range rows {
		out[i] = map[string]any{"user_id": m.UserID, "role": m.Role, "all_clients": m.AllClients, "client_ids": orEmpty(m.ClientIds)}
	}
	return out
}

func toAPIMembers(rows []db.ListProjectMembersRow) []Member {
	items := make([]Member, len(rows))
	for i, m := range rows {
		items[i] = Member{
			UserId: m.UserID, Name: m.Name, Email: m.Email, Role: ProjectRole(m.Role),
			AllClients: m.AllClients, ClientIds: orEmpty(m.ClientIds),
		}
	}
	return items
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
git commit -m "feat(server): project members with client scopes"
```

### Task 5: Contacts

**Files:**
- Modify: `api/openapi.yaml` (paths `/contacts`, `/contacts/{id}`; schemas `Contact`, `ContactList`, `ContactInput`)
- Create: `server/internal/httpapi/contacts.go`
- Test: `server/internal/httpapi/contacts_test.go`
- Generate: `server/internal/httpapi/api.gen.go`, `web/lib/api-types.ts`

**Interfaces:**
- Consumes: `nonEmpty`, `changed`, `constraintOf`, `deref` (Tasks 2–3); `validEmail` (Iteration 0); the seeders.
- Produces: handlers `ListContacts(w, r, params ListContactsParams)` (query `q`, `client_id`), `CreateContact`, `UpdateContact(w, r, id int64)`; the ticket form in Iteration 2 uses them.

- [ ] **Step 1: Extend the contract**

`api/openapi.yaml`, under `paths:`:

```yaml
  /contacts:
    get:
      operationId: listContacts
      tags: [contacts]
      description: Contacts of clients in the caller's scope plus internal contacts, at most 50, for type-ahead.
      parameters:
        - { name: q, in: query, required: false, schema: { type: string, maxLength: 200 } }
        - { name: client_id, in: query, required: false, schema: { type: integer, format: int64 } }
      responses:
        "200":
          description: Matching contacts, by name.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/ContactList" }
        default: { $ref: "#/components/responses/Problem" }
    post:
      operationId: createContact
      tags: [contacts]
      description: Members and project admins, for clients in their scope.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/ContactInput" }
      responses:
        "201":
          description: The new contact.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Contact" }
        default: { $ref: "#/components/responses/Problem" }
  /contacts/{id}:
    parameters:
      - { name: id, in: path, required: true, schema: { type: integer, format: int64 } }
    patch:
      operationId: updateContact
      tags: [contacts]
      description: Replaces every field; an omitted client_id makes the contact internal.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/ContactInput" }
      responses:
        "200":
          description: The updated contact.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Contact" }
        default: { $ref: "#/components/responses/Problem" }
```

Under `components.schemas:`:

```yaml
    Contact:
      type: object
      required: [id, client_id, client_name, name, title, email, phone]
      properties:
        id: { type: integer, format: int64 }
        client_id: { type: integer, format: int64, nullable: true }
        client_name: { type: string, nullable: true }
        name: { type: string }
        title: { type: string, nullable: true }
        email: { type: string, nullable: true }
        phone: { type: string, nullable: true }
    ContactList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/Contact" }
    ContactInput:
      type: object
      required: [name]
      properties:
        client_id: { type: integer, format: int64, description: Omitted for internal people. }
        name: { type: string, maxLength: 200 }
        title: { type: string, maxLength: 200 }
        email: { type: string, maxLength: 320 }
        phone: { type: string, maxLength: 50 }
```

- [ ] **Step 2: Write the failing tests**

`server/internal/httpapi/contacts_test.go`:

```go
package httpapi_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func TestContactsFollowTheClientScope(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	p := e.seedProject("HRIS", a, b)
	e.seedContact("Dewi", nil)
	e.seedContact("Andi", &a)
	bayu := e.seedContact("Bayu", &b)
	budi, bu := e.signedIn("budi@example.com", false)
	e.seedMember(bu, p, "member", b)
	outsider, _ := e.signedIn("out@example.com", false)

	names := func(c *http.Client, query string) []string {
		var list httpapi.ContactList
		if code := e.call(c, http.MethodGet, "/contacts"+query, nil, &list); code != http.StatusOK {
			t.Fatalf("list %s: %d", query, code)
		}
		out := []string{}
		for _, it := range list.Items {
			out = append(out, it.Name)
		}
		return out
	}
	if got := names(budi, ""); !slices.Equal(got, []string{"Bayu", "Dewi"}) {
		t.Fatalf("scoped to B: %v", got)
	}
	if got := names(budi, "?q=ba"); !slices.Equal(got, []string{"Bayu"}) {
		t.Fatalf("search: %v", got)
	}
	if got := names(budi, fmt.Sprintf("?client_id=%d", a.ID)); len(got) != 0 {
		t.Fatalf("an out-of-scope filter finds nothing: %v", got)
	}
	if got := names(outsider, ""); len(got) != 0 {
		t.Fatalf("no project, no contacts: %v", got)
	}

	var c httpapi.Contact
	body := map[string]any{"name": "Budi Santoso", "client_id": b.ID, "title": "HR Manager", "email": "budi@client-b.example"}
	if code := e.call(budi, http.MethodPost, "/contacts", body, &c); code != http.StatusCreated ||
		c.ClientName == nil || *c.ClientName != "Client B" || c.Title == nil || *c.Title != "HR Manager" {
		t.Fatalf("create: %d %+v", code, c)
	}
	var prob httpapi.Problem
	if code := e.call(budi, http.MethodPost, "/contacts", map[string]any{"name": "X", "client_id": a.ID}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Field != "client_id" {
		t.Fatalf("out-of-scope client: %d %+v", code, prob)
	}
	if code := e.call(budi, http.MethodPost, "/contacts", map[string]any{"name": "X", "email": "not-an-email"}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Field != "email" {
		t.Fatalf("bad email: %d %+v", code, prob)
	}
	// PATCH replaces every field: without client_id, Bayu becomes internal and stays visible.
	if code := e.call(budi, http.MethodPatch, fmt.Sprintf("/contacts/%d", bayu), map[string]any{"name": "Bayu"}, &c); code != http.StatusOK || c.ClientId != nil {
		t.Fatalf("make internal: %d %+v", code, c)
	}
}

func TestViewersReadContactsButDoNotWriteThem(t *testing.T) {
	e := newEnv(t)
	a := e.seedClient("Client A")
	p := e.seedProject("HRIS", a)
	andi := e.seedContact("Andi", &a)
	viewer, vu := e.signedIn("viewer@example.com", false)
	e.seedMember(vu, p, "viewer")
	if code := e.call(viewer, http.MethodPost, "/contacts", map[string]any{"name": "X"}, nil); code != http.StatusForbidden {
		t.Fatalf("viewer creates: %d", code)
	}
	if code := e.call(viewer, http.MethodPatch, fmt.Sprintf("/contacts/%d", andi), map[string]any{"name": "Andi", "client_id": a.ID}, nil); code != http.StatusForbidden {
		t.Fatalf("viewer edits: %d", code)
	}
	if code := e.call(viewer, http.MethodPatch, "/contacts/999999", map[string]any{"name": "X"}, nil); code != http.StatusNotFound {
		t.Fatalf("missing contact: %d", code)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd server && go generate ./... && go vet ./internal/httpapi/
```

Expected: FAIL. `*Server does not implement ServerInterface (missing method CreateContact)`.

- [ ] **Step 4: Write the handlers**

`server/internal/httpapi/contacts.go`:

```go
package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/db"
)

// ListContacts searches the contacts the user may see (R-AC-6).
func (s *Server) ListContacts(w http.ResponseWriter, r *http.Request, params ListContactsParams) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := s.q.ListContacts(r.Context(), db.ListContactsParams{
		IsAdmin: u.IsAdmin, UserID: u.ID, ClientID: params.ClientId, Q: strings.TrimSpace(deref(params.Q)),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]Contact, len(rows))
	for i, c := range rows {
		items[i] = toAPIContact(c)
	}
	writeJSON(w, http.StatusOK, ContactList{Items: items})
}

func (s *Server) CreateContact(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	var in ContactInput
	if !decodeJSON(w, r, &in) || !s.contactInputOK(w, r, u, in) {
		return
	}
	ctx := r.Context()
	var out Contact
	err := s.inTx(ctx, func(q *db.Queries) error {
		id, err := q.CreateContact(ctx, contactParams(in))
		if err != nil {
			return err
		}
		if out, err = readContact(ctx, q, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &u.ID, "contact", id, "create", contactAudit(out))
	})
	if constraintOf(err) == "contacts_client_fk" {
		contactClientInvalid(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) UpdateContact(w http.ResponseWriter, r *http.Request, id int64) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	rows, err := s.q.ListContacts(ctx, db.ListContactsParams{IsAdmin: u.IsAdmin, UserID: u.ID, ID: &id})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(rows) == 0 { // missing, or outside the user's scope (R-AC-7)
		writeProblem(w, http.StatusNotFound, "not_found", "Contact not found")
		return
	}
	may, err := s.mayEditContactsOf(ctx, u, rows[0].ClientID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !may {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only members can edit contacts")
		return
	}
	var in ContactInput
	if !decodeJSON(w, r, &in) || !s.contactInputOK(w, r, u, in) {
		return
	}
	before := toAPIContact(rows[0])
	var out Contact
	err = s.inTx(ctx, func(q *db.Queries) error {
		p := contactParams(in)
		if err := q.UpdateContact(ctx, db.UpdateContactParams{
			ID: id, ClientID: p.ClientID, Name: p.Name, Title: p.Title, Email: p.Email, Phone: p.Phone,
		}); err != nil {
			return err
		}
		var err error
		if out, err = readContact(ctx, q, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &u.ID, "contact", id, "update", changed(contactAudit(before), contactAudit(out)))
	})
	if constraintOf(err) == "contacts_client_fk" {
		contactClientInvalid(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// contactInputOK validates a contact and checks that u may keep contacts of its
// client: members and project admins where the client is in scope, and for
// internal contacts, members of any project (FSD §5.1, §15.3).
func (s *Server) contactInputOK(w http.ResponseWriter, r *http.Request, u *db.User, in ContactInput) bool {
	if fields := validateContact(in); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return false
	}
	may, err := s.mayEditContactsOf(r.Context(), u, in.ClientId)
	switch {
	case err != nil:
		s.fail(w, r, err)
	case may:
		return true
	case in.ClientId != nil: // an out-of-scope client looks the same as a missing one
		contactClientInvalid(w)
	default:
		writeProblem(w, http.StatusForbidden, "forbidden", "Only members can add contacts")
	}
	return false
}

func (s *Server) mayEditContactsOf(ctx context.Context, u *db.User, clientID *int64) (bool, error) {
	if u.IsAdmin {
		return true, nil
	}
	return s.q.CanEditContactsOf(ctx, db.CanEditContactsOfParams{UserID: u.ID, ClientID: clientID})
}

// readContact reads a contact with its client name inside the write's transaction.
func readContact(ctx context.Context, q *db.Queries, id int64) (Contact, error) {
	rows, err := q.ListContacts(ctx, db.ListContactsParams{IsAdmin: true, ID: &id})
	if err != nil {
		return Contact{}, err
	}
	if len(rows) == 0 {
		return Contact{}, pgx.ErrNoRows
	}
	return toAPIContact(rows[0]), nil
}

func contactClientInvalid(w http.ResponseWriter) {
	writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
		FieldError{Field: "client_id", Code: "invalid", Message: "Choose a client you work with"})
}

func validateContact(in ContactInput) []FieldError {
	var f []FieldError
	if n := strings.TrimSpace(in.Name); n == "" || len(n) > 200 {
		f = append(f, FieldError{Field: "name", Code: "required", Message: "Enter a name of at most 200 characters"})
	}
	if t := nonEmpty(in.Title); t != nil && len(*t) > 200 {
		f = append(f, FieldError{Field: "title", Code: "invalid", Message: "Use at most 200 characters"})
	}
	if e := nonEmpty(in.Email); e != nil && !validEmail(*e) {
		f = append(f, FieldError{Field: "email", Code: "invalid", Message: "Enter a valid email address"})
	}
	if p := nonEmpty(in.Phone); p != nil && len(*p) > 50 {
		f = append(f, FieldError{Field: "phone", Code: "invalid", Message: "Use at most 50 characters"})
	}
	return f
}

func contactParams(in ContactInput) db.CreateContactParams {
	return db.CreateContactParams{
		ClientID: in.ClientId, Name: strings.TrimSpace(in.Name),
		Title: nonEmpty(in.Title), Email: nonEmpty(in.Email), Phone: nonEmpty(in.Phone),
	}
}

func contactAudit(c Contact) map[string]any {
	return map[string]any{"client_id": c.ClientId, "name": c.Name, "title": c.Title, "email": c.Email, "phone": c.Phone}
}

func toAPIContact(c db.ListContactsRow) Contact {
	return Contact{
		Id: c.ID, ClientId: c.ClientID, ClientName: c.ClientName, Name: c.Name,
		Title: c.Title, Email: c.Email, Phone: c.Phone,
	}
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
git commit -m "feat(server): contacts scoped by client"
```

### Task 6: The module tree

**Files:**
- Modify: `api/openapi.yaml` (paths `/projects/{key}/nodes`, `/nodes/{id}`; schemas `NodeType`, `NodeClient`, `Node`, `NodeList`, `NodeCreate`, `NodeUpdate`, `NodeMove`)
- Create: `server/internal/httpapi/nodes.go`
- Test: `server/internal/httpapi/nodes_test.go`
- Generate: `server/internal/httpapi/api.gen.go`, `web/lib/api-types.ts`

**Interfaces:**
- Consumes: `projectFor`, `memberOf`, `cleanAliases`, `aliasesError`, `nonEmpty`, `changed`, `constraintOf`, `deref`, `orEmpty` (Tasks 2–3); Task 1's node queries.
- Produces:
  - Handlers: `ListNodes(w, r, key string)`, `CreateNode(w, r, key string)`, `UpdateNode(w, r, id int64)`, `DeleteNode(w, r, id int64)`.
  - `(s *Server) nodeFor(w, r, id int64, need string) (projectCtx, db.Node, bool)`: answers 404 for a node that is missing, in another project, or hidden by scope; 403 when the role is below `need`. The node page (Iteration 3) reuses it.
  - Error codes: `node_name_taken` and `node_code_taken` (409), `node_has_children` (409), and the 422 field codes `node_cycle`, `client_specific_module`, `client_not_linked`.

- [ ] **Step 1: Extend the contract**

`api/openapi.yaml`, under `paths:`:

```yaml
  /projects/{key}/nodes:
    parameters:
      - { name: key, in: path, required: true, schema: { type: string } }
    get:
      operationId: listNodes
      tags: [nodes]
      description: The tree as a flat list. Client-specific menus outside the caller's scope, and what is under them, are left out.
      responses:
        "200":
          description: Live nodes; siblings in position order.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/NodeList" }
        default: { $ref: "#/components/responses/Problem" }
    post:
      operationId: createNode
      tags: [nodes]
      description: Project admins only. The node goes last among its siblings.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/NodeCreate" }
      responses:
        "201":
          description: The new node.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Node" }
        default: { $ref: "#/components/responses/Problem" }
  /nodes/{id}:
    parameters:
      - { name: id, in: path, required: true, schema: { type: integer, format: int64 } }
    patch:
      operationId: updateNode
      tags: [nodes]
      description: Project admins only. Edits fields, and moves the node when `move` is present.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/NodeUpdate" }
      responses:
        "200":
          description: The updated node.
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Node" }
        default: { $ref: "#/components/responses/Problem" }
    delete:
      operationId: deleteNode
      tags: [nodes]
      description: Project admins only. A node with sub-nodes answers 409 node_has_children.
      responses:
        "204": { description: Deleted. }
        default: { $ref: "#/components/responses/Problem" }
```

Under `components.schemas:`:

```yaml
    NodeType:
      type: string
      enum: [module, menu]
    NodeClient:
      type: object
      required: [id, name]
      properties:
        id: { type: integer, format: int64 }
        name: { type: string }
    Node:
      type: object
      required: [id, parent_id, type, name, code, aliases, description, client_specific, clients, position]
      properties:
        id: { type: integer, format: int64 }
        parent_id: { type: integer, format: int64, nullable: true }
        type: { $ref: "#/components/schemas/NodeType" }
        name: { type: string }
        code: { type: string, nullable: true, example: HR.ATT.OT_APPROVAL }
        aliases:
          type: array
          items: { type: string }
        description: { type: string }
        client_specific: { type: boolean }
        clients:
          type: array
          description: The node's clients within the caller's scope.
          items: { $ref: "#/components/schemas/NodeClient" }
        position: { type: integer, format: int32 }
    NodeList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: "#/components/schemas/Node" }
    NodeCreate:
      type: object
      required: [type, name]
      properties:
        parent_id: { type: integer, format: int64 }
        type: { $ref: "#/components/schemas/NodeType" }
        name: { type: string, maxLength: 200 }
        code: { type: string, maxLength: 100 }
        aliases:
          type: array
          maxItems: 20
          items: { type: string, maxLength: 100 }
        description: { type: string, maxLength: 5000 }
        client_specific: { type: boolean }
        client_ids:
          type: array
          items: { type: integer, format: int64 }
    NodeUpdate:
      type: object
      properties:
        name: { type: string, maxLength: 200 }
        type: { $ref: "#/components/schemas/NodeType" }
        code: { type: string, maxLength: 100, description: An empty code clears it. }
        aliases:
          type: array
          maxItems: 20
          items: { type: string, maxLength: 100 }
        description: { type: string, maxLength: 5000 }
        client_specific: { type: boolean, description: When present, client_ids replaces the node's clients. }
        client_ids:
          type: array
          items: { type: integer, format: int64 }
        move: { $ref: "#/components/schemas/NodeMove" }
    NodeMove:
      type: object
      properties:
        parent_id: { type: integer, format: int64, nullable: true, description: Omitted or null moves to the top level. }
        position: { type: integer, format: int32, minimum: 0, description: Index among the new siblings; omitted puts the node last. }
```

- [ ] **Step 2: Write the failing tests**

`server/internal/httpapi/nodes_test.go`:

```go
package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func nodeNames(list httpapi.NodeList) []string {
	names := make([]string, len(list.Items))
	for i, n := range list.Items {
		names[i] = n.Name
	}
	return names
}

// The Iteration 1 exit check at API level (AC-MR-1, AC-MR-2).
func TestAdminBuildsTheTreeAndScopedMembersSeeTheirMenus(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	p := e.seedProject("HRIS", a, b)
	admin, _ := e.signedIn("admin@example.com", true)
	create := func(parent *httpapi.Node, typ, name string, clients ...int64) httpapi.Node {
		t.Helper()
		body := map[string]any{"type": typ, "name": name}
		if parent != nil {
			body["parent_id"] = parent.Id
		}
		if len(clients) > 0 {
			body["client_specific"] = true
			body["client_ids"] = clients
		}
		var n httpapi.Node
		if code := e.call(admin, http.MethodPost, "/projects/HRIS/nodes", body, &n); code != http.StatusCreated {
			t.Fatalf("create %s: %d", name, code)
		}
		return n
	}
	hr := create(nil, "module", "HR")
	att := create(&hr, "module", "Attendance")
	ot := create(&att, "menu", "Overtime Approval", a.ID)
	create(&ot, "menu", "OT Rules")
	create(&att, "menu", "Leave Request")
	if ot.ParentId == nil || *ot.ParentId != att.Id || len(ot.Clients) != 1 || ot.Clients[0].Name != "Client A" {
		t.Fatalf("overtime: %+v", ot)
	}

	scopedB, ub := e.signedIn("budi@example.com", false)
	e.seedMember(ub, p, "member", b)
	allClients, ua := e.signedIn("ani@example.com", false)
	e.seedMember(ua, p, "viewer")

	var list httpapi.NodeList
	if code := e.call(scopedB, http.MethodGet, "/projects/HRIS/nodes", nil, &list); code != http.StatusOK {
		t.Fatalf("list: %d", code)
	}
	if got := nodeNames(list); !slices.Equal(got, []string{"HR", "Attendance", "Leave Request"}) {
		t.Fatalf("scoped to Client B sees %v", got)
	}
	e.call(allClients, http.MethodGet, "/projects/HRIS/nodes", nil, &list)
	if got := nodeNames(list); !slices.Equal(got, []string{"HR", "Attendance", "Overtime Approval", "Leave Request", "OT Rules"}) {
		t.Fatalf("all clients sees %v", got)
	}
	if c := list.Items[2].Clients; len(c) != 1 || c[0].Name != "Client A" {
		t.Fatalf("badge: %+v", c)
	}
}

func TestNodeNamesAndCodesAreUnique(t *testing.T) {
	e := newEnv(t)
	e.seedProject("HRIS")
	admin, _ := e.signedIn("admin@example.com", true)
	var hr httpapi.Node
	e.call(admin, http.MethodPost, "/projects/HRIS/nodes", map[string]any{"type": "module", "name": "HR", "code": "HR"}, &hr)
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPost, "/projects/HRIS/nodes", map[string]any{"type": "module", "name": "hr"}, &p); code != http.StatusConflict || p.Code != "node_name_taken" {
		t.Fatalf("sibling name: %d %+v", code, p)
	}
	if code := e.call(admin, http.MethodPost, "/projects/HRIS/nodes", map[string]any{"type": "menu", "name": "Payroll", "code": "HR"}, &p); code != http.StatusConflict || p.Code != "node_code_taken" {
		t.Fatalf("code: %d %+v", code, p)
	}
	if code := e.call(admin, http.MethodPost, "/projects/HRIS/nodes", map[string]any{"type": "menu", "name": "HR", "parent_id": hr.Id}, nil); code != http.StatusCreated {
		t.Fatalf("the same name under another parent: %d", code)
	}
}

func TestClientScopeRulesForMenus(t *testing.T) {
	e := newEnv(t)
	a, other := e.seedClient("Client A"), e.seedClient("Client Z")
	e.seedProject("HRIS", a)
	pay := e.seedNode(e.seedProject("PAY"), nil, "module", "Payroll")
	admin, _ := e.signedIn("admin@example.com", true)
	for _, c := range []struct {
		body        map[string]any
		field, code string
	}{
		{map[string]any{"type": "module", "name": "M", "client_specific": true, "client_ids": []int64{a.ID}}, "client_specific", "client_specific_module"},
		{map[string]any{"type": "menu", "name": "M", "client_specific": true}, "client_ids", "required"},
		{map[string]any{"type": "menu", "name": "M", "client_specific": true, "client_ids": []int64{other.ID}}, "client_ids", "client_not_linked"},
		{map[string]any{"type": "screen", "name": "M"}, "type", "invalid"},
		{map[string]any{"type": "menu", "name": " "}, "name", "required"},
		{map[string]any{"type": "menu", "name": "M", "parent_id": pay.ID}, "parent_id", "invalid"}, // a parent in another project
	} {
		var p httpapi.Problem
		code := e.call(admin, http.MethodPost, "/projects/HRIS/nodes", c.body, &p)
		if f := firstError(p); code != http.StatusUnprocessableEntity || f.Field != c.field || f.Code != c.code {
			t.Errorf("%v: %d %+v", c.body, code, p)
		}
	}
}

func TestEditingANodeRecordsOldAndNewValues(t *testing.T) {
	e := newEnv(t)
	a := e.seedClient("Client A")
	p := e.seedProject("HRIS", a)
	ot := e.seedNode(p, nil, "menu", "Overtime")
	admin, _ := e.signedIn("admin@example.com", true)
	path := fmt.Sprintf("/nodes/%d", ot.ID)
	var n httpapi.Node
	body := map[string]any{"name": "Overtime Approval", "code": "HR.OT", "aliases": []string{" Persetujuan Lembur ", ""},
		"client_specific": true, "client_ids": []int64{a.ID}}
	if code := e.call(admin, http.MethodPatch, path, body, &n); code != http.StatusOK || n.Name != "Overtime Approval" ||
		n.Code == nil || *n.Code != "HR.OT" || !slices.Equal(n.Aliases, []string{"Persetujuan Lembur"}) || len(n.Clients) != 1 {
		t.Fatalf("edit: %d %+v", code, n)
	}
	var prob httpapi.Problem
	if code := e.call(admin, http.MethodPatch, path, map[string]any{"type": "module"}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Code != "client_specific_module" {
		t.Fatalf("a client-specific menu cannot become a module: %d %+v", code, prob)
	}
	if code := e.call(admin, http.MethodPatch, path, map[string]any{"type": "module", "client_specific": false, "code": ""}, &n); code != http.StatusOK ||
		n.Type != httpapi.NodeTypeModule || n.Code != nil || len(n.Clients) != 0 {
		t.Fatalf("to a shared module: %d %+v", code, n)
	}
	events, err := e.q.ListAuditEvents(context.Background(), db.ListAuditEventsParams{Entity: "node", EntityID: ot.ID})
	if err != nil || len(events) != 2 {
		t.Fatalf("audit: %+v %v", events, err)
	}
	var changes map[string]map[string]any
	if err := json.Unmarshal(events[0].Changes, &changes); err != nil ||
		changes["name"]["old"] != "Overtime" || changes["name"]["new"] != "Overtime Approval" {
		t.Fatalf("changes: %s", events[0].Changes)
	}
}

func TestMovingAndDeletingNodes(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	hr := e.seedNode(p, nil, "module", "HR")
	att := e.seedNode(p, &hr, "module", "Attendance")
	leave := e.seedNode(p, &att, "menu", "Leave Request")
	e.seedNode(p, &att, "menu", "Overtime")
	admin, _ := e.signedIn("admin@example.com", true)
	patch := func(id int64, body map[string]any, out any) int {
		return e.call(admin, http.MethodPatch, fmt.Sprintf("/nodes/%d", id), body, out)
	}
	var moved httpapi.Node
	if code := patch(leave.ID, map[string]any{"move": map[string]any{"parent_id": hr.ID, "position": 0}}, &moved); code != http.StatusOK ||
		moved.ParentId == nil || *moved.ParentId != hr.ID || moved.Position != 0 {
		t.Fatalf("move: %d %+v", code, moved)
	}
	var list httpapi.NodeList
	e.call(admin, http.MethodGet, "/projects/HRIS/nodes", nil, &list)
	if got := nodeNames(list); !slices.Equal(got, []string{"HR", "Leave Request", "Attendance", "Overtime"}) {
		t.Fatalf("after the move: %v", got)
	}
	var prob httpapi.Problem
	if code := patch(hr.ID, map[string]any{"move": map[string]any{"parent_id": att.ID}}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Code != "node_cycle" {
		t.Fatalf("cycle: %d %+v", code, prob)
	}
	if code := patch(att.ID, map[string]any{"move": map[string]any{"parent_id": nil}}, &moved); code != http.StatusOK || moved.ParentId != nil {
		t.Fatalf("to the top level: %d %+v", code, moved)
	}
	// R-MR-5: the audit log keeps the old and the new parent.
	events, err := e.q.ListAuditEvents(context.Background(), db.ListAuditEventsParams{Entity: "node", EntityID: leave.ID})
	if err != nil || len(events) != 1 {
		t.Fatalf("audit: %+v %v", events, err)
	}
	var changes map[string]map[string]any
	if err := json.Unmarshal(events[0].Changes, &changes); err != nil ||
		changes["parent_id"]["old"] != float64(att.ID) || changes["parent_id"]["new"] != float64(hr.ID) {
		t.Fatalf("changes: %s", events[0].Changes)
	}
	if code := e.call(admin, http.MethodDelete, fmt.Sprintf("/nodes/%d", att.ID), nil, &prob); code != http.StatusConflict || prob.Code != "node_has_children" {
		t.Fatalf("delete a parent: %d %+v", code, prob)
	}
	if code := e.call(admin, http.MethodDelete, fmt.Sprintf("/nodes/%d", leave.ID), nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete a leaf: %d", code)
	}
}

func TestOnlyProjectAdminsChangeTheTree(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	p := e.seedProject("HRIS", a, b)
	secret := e.seedNode(p, nil, "menu", "Client A Report", a)
	open := e.seedNode(p, nil, "menu", "Leave")
	member, mu := e.signedIn("member@example.com", false)
	e.seedMember(mu, p, "member", b)
	outsider, _ := e.signedIn("out@example.com", false)
	for _, c := range []struct {
		who  *http.Client
		id   int64
		want int
	}{
		{member, open.ID, http.StatusForbidden},  // visible, but members do not edit the tree
		{member, secret.ID, http.StatusNotFound}, // hidden by the client scope, so it looks missing (R-AC-7)
		{outsider, open.ID, http.StatusNotFound},
		{member, 999999, http.StatusNotFound},
	} {
		if code := e.call(c.who, http.MethodPatch, fmt.Sprintf("/nodes/%d", c.id), map[string]any{"name": "X"}, nil); code != c.want {
			t.Errorf("node %d: %d, want %d", c.id, code, c.want)
		}
	}
	if code := e.call(member, http.MethodPost, "/projects/HRIS/nodes", map[string]any{"type": "menu", "name": "X"}, nil); code != http.StatusForbidden {
		t.Errorf("member creates: %d", code)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd server && go generate ./... && go vet ./internal/httpapi/
```

Expected: FAIL. `*Server does not implement ServerInterface (missing method CreateNode)`.

- [ ] **Step 4: Write the handlers**

`server/internal/httpapi/nodes.go`:

```go
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

var (
	errNodeCycle = errors.New("a node cannot move under itself")
	errBadParent = errors.New("the parent is not a live node of this project")
)

// ListNodes returns the project's tree as a flat list with parent ids (FSD §7.1).
func (s *Server) ListNodes(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.q.ListNodes(r.Context(), db.ListNodesParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
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
			Clients: clients, Position: n.Position,
		}
	}
	writeJSON(w, http.StatusOK, NodeList{Items: items})
}

func (s *Server) CreateNode(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in NodeCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	aliases, aliasesOK := cleanAliases(deref(in.Aliases))
	specific := deref(in.ClientSpecific)
	fields := validateNode(&in.Name, &in.Type, in.Code, aliasesOK, in.Description)
	fields = append(fields, clientScopeErrors(string(in.Type), specific, in.ClientSpecific, in.ClientIds)...)
	if in.ParentId != nil {
		parent, err := s.q.GetNode(ctx, *in.ParentId)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			s.fail(w, r, err)
			return
		}
		if err != nil || parent.ProjectID != pc.project.ID || parent.ArchivedAt != nil {
			fields = append(fields, FieldError{Field: "parent_id", Code: "invalid", Message: "Choose a parent in this project"})
		}
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var out Node
	err := s.inTx(ctx, func(q *db.Queries) error {
		n, err := q.CreateNode(ctx, db.CreateNodeParams{
			ProjectID: pc.project.ID, ParentID: in.ParentId, Type: string(in.Type), Name: strings.TrimSpace(in.Name),
			Code: nonEmpty(in.Code), Aliases: aliases, Description: strings.TrimSpace(deref(in.Description)),
			ClientSpecific: specific,
		})
		if err != nil {
			return err
		}
		if specific {
			if err := q.AddNodeClients(ctx, db.AddNodeClientsParams{NodeID: n.ID, ProjectID: pc.project.ID, ClientIds: *in.ClientIds}); err != nil {
				return err
			}
		}
		clients, err := q.ListNodeClients(ctx, n.ID)
		if err != nil {
			return err
		}
		out = toAPINode(n, clients)
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "node", n.ID, "create", nodeAudit(n, clients))
	})
	if nodeConflict(w, err) {
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) UpdateNode(w http.ResponseWriter, r *http.Request, id int64) {
	pc, n, ok := s.nodeFor(w, r, id, access.Admin)
	if !ok {
		return
	}
	var in NodeUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	var aliases []string
	aliasesOK := true
	if in.Aliases != nil {
		aliases, aliasesOK = cleanAliases(*in.Aliases)
	}
	typ, specific := n.Type, n.ClientSpecific
	if in.Type != nil {
		typ = string(*in.Type)
	}
	if in.ClientSpecific != nil {
		specific = *in.ClientSpecific
	}
	fields := validateNode(in.Name, in.Type, in.Code, aliasesOK, in.Description)
	fields = append(fields, clientScopeErrors(typ, specific, in.ClientSpecific, in.ClientIds)...)
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	ctx := r.Context()
	var out Node
	err := s.inTx(ctx, func(q *db.Queries) error {
		before, err := q.ListNodeClients(ctx, id)
		if err != nil {
			return err
		}
		updated, err := q.UpdateNode(ctx, db.UpdateNodeParams{
			ID: id, Name: trimmed(in.Name), Type: (*string)(in.Type), Code: trimmed(in.Code), Aliases: aliases,
			Description: trimmed(in.Description), ClientSpecific: in.ClientSpecific,
		})
		if err != nil {
			return err
		}
		if in.ClientSpecific != nil {
			if err := q.ClearNodeClients(ctx, id); err != nil {
				return err
			}
			if *in.ClientSpecific {
				if err := q.AddNodeClients(ctx, db.AddNodeClientsParams{NodeID: id, ProjectID: n.ProjectID, ClientIds: *in.ClientIds}); err != nil {
					return err
				}
			}
		}
		if in.Move != nil {
			if updated, err = moveNode(ctx, q, updated, in.Move); err != nil {
				return err
			}
		}
		after, err := q.ListNodeClients(ctx, id)
		if err != nil {
			return err
		}
		out = toAPINode(updated, after)
		return audit(ctx, q, webMeta(r).inProject(n.ProjectID), &pc.user.ID, "node", id, "update",
			changed(nodeAudit(n, before), nodeAudit(updated, after)))
	})
	if errors.Is(err, errNodeCycle) || errors.Is(err, errBadParent) {
		f := FieldError{Field: "move.parent_id", Code: "invalid", Message: "Choose a parent in this project"}
		if errors.Is(err, errNodeCycle) {
			f.Code, f.Message = "node_cycle", "An item cannot move under itself"
		}
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", f)
		return
	}
	if nodeConflict(w, err) {
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// DeleteNode removes a node that has no sub-nodes. Before tickets exist nothing
// links to a node, so every node can be deleted (R-MR-4).
func (s *Server) DeleteNode(w http.ResponseWriter, r *http.Request, id int64) {
	pc, n, ok := s.nodeFor(w, r, id, access.Admin)
	if !ok {
		return
	}
	ctx := r.Context()
	err := s.inTx(ctx, func(q *db.Queries) error {
		clients, err := q.ListNodeClients(ctx, id)
		if err != nil {
			return err
		}
		if err := q.DeleteNode(ctx, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(n.ProjectID), &pc.user.ID, "node", id, "delete", nodeAudit(n, clients))
	})
	if nodeConflict(w, err) {
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// nodeFor loads a node for the signed-in user. A node in a project the user
// does not belong to, or one hidden by their client scope, answers 404 like a
// missing one (R-AC-5, R-AC-7); a role below need answers 403.
func (s *Server) nodeFor(w http.ResponseWriter, r *http.Request, id int64, need string) (projectCtx, db.Node, bool) {
	u := s.requireUser(w, r)
	if u == nil {
		return projectCtx{}, db.Node{}, false
	}
	ctx := r.Context()
	n, err := s.q.GetNode(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Not found")
		return projectCtx{}, db.Node{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.Node{}, false
	}
	p, err := s.q.GetProjectByID(ctx, n.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.Node{}, false
	}
	pc, ok := s.memberOf(w, r, u, p)
	if !ok {
		return projectCtx{}, db.Node{}, false
	}
	visible, err := s.q.IsNodeVisible(ctx, db.IsNodeVisibleParams{ID: id, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs)})
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.Node{}, false
	}
	if !visible {
		writeProblem(w, http.StatusNotFound, "not_found", "Not found")
		return projectCtx{}, db.Node{}, false
	}
	if !pc.scope.Allows(need) {
		writeProblem(w, http.StatusForbidden, "forbidden", "Your project role does not allow this")
		return projectCtx{}, db.Node{}, false
	}
	return pc, n, true
}

// moveNode puts n under move.parent_id (nil = top level) at move.position
// (default last) and renumbers its new siblings. The project row lock makes
// concurrent moves take turns, so none of them can build a cycle (R-MR-5).
func moveNode(ctx context.Context, q *db.Queries, n db.Node, move *NodeMove) (db.Node, error) {
	if err := q.LockProject(ctx, n.ProjectID); err != nil {
		return n, err
	}
	if move.ParentId != nil {
		parent, err := q.GetNode(ctx, *move.ParentId)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (parent.ProjectID != n.ProjectID || parent.ArchivedAt != nil)) {
			return n, errBadParent
		}
		if err != nil {
			return n, err
		}
		cycle, err := q.IsSelfOrDescendant(ctx, db.IsSelfOrDescendantParams{NodeID: n.ID, CandidateID: parent.ID})
		if err != nil {
			return n, err
		}
		if cycle {
			return n, errNodeCycle
		}
	}
	siblings, err := q.ListSiblingIDs(ctx, db.ListSiblingIDsParams{ProjectID: n.ProjectID, ParentID: move.ParentId, ExcludeID: n.ID})
	if err != nil {
		return n, err
	}
	at := len(siblings)
	if move.Position != nil {
		at = min(max(int(*move.Position), 0), len(siblings))
	}
	if err := q.PlaceNodes(ctx, db.PlaceNodesParams{ParentID: move.ParentId, Ids: slices.Insert(siblings, at, n.ID)}); err != nil {
		return n, err
	}
	return q.GetNode(ctx, n.ID)
}

func validateNode(name *string, typ *NodeType, code *string, aliasesOK bool, description *string) []FieldError {
	var f []FieldError
	if name != nil {
		if n := strings.TrimSpace(*name); n == "" || len(n) > 200 {
			f = append(f, FieldError{Field: "name", Code: "required", Message: "Enter a name of at most 200 characters"})
		}
	}
	if typ != nil && !typ.Valid() {
		f = append(f, FieldError{Field: "type", Code: "invalid", Message: "Choose module or menu"})
	}
	if code != nil && len(strings.TrimSpace(*code)) > 100 {
		f = append(f, FieldError{Field: "code", Code: "invalid", Message: "Use at most 100 characters"})
	}
	if !aliasesOK {
		f = append(f, aliasesError)
	}
	if description != nil && len(*description) > 5000 {
		f = append(f, FieldError{Field: "description", Code: "invalid", Message: "Use at most 5,000 characters"})
	}
	return f
}

// clientScopeErrors checks R-MR-7: only menus are client-specific, and a
// request that makes a menu client-specific names at least one client.
func clientScopeErrors(typ string, specific bool, set *bool, ids *[]int64) []FieldError {
	switch {
	case specific && typ == string(NodeTypeModule):
		return []FieldError{{Field: "client_specific", Code: "client_specific_module", Message: "Only menus can be client-specific"}}
	case set != nil && *set && (ids == nil || len(*ids) == 0):
		return []FieldError{{Field: "client_ids", Code: "required", Message: "Choose at least one client"}}
	}
	return nil
}

// nodeConflict answers the constraint errors of node writes and reports
// whether it wrote a response.
func nodeConflict(w http.ResponseWriter, err error) bool {
	switch constraintOf(err) {
	case "nodes_sibling_uq":
		const msg = "Another item under the same parent has this name"
		writeProblem(w, http.StatusConflict, "node_name_taken", msg, FieldError{Field: "name", Code: "node_name_taken", Message: msg})
	case "nodes_code_uq":
		const msg = "Another item in this project has this code"
		writeProblem(w, http.StatusConflict, "node_code_taken", msg, FieldError{Field: "code", Code: "node_code_taken", Message: msg})
	case "node_clients_linked":
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "client_ids", Code: "client_not_linked", Message: "Link this client to the project first"})
	case "nodes_parent_fk":
		writeProblem(w, http.StatusConflict, "node_has_children", "Delete or move its sub-items first")
	default:
		return false
	}
	return true
}

func toAPINode(n db.Node, clients []db.ListNodeClientsRow) Node {
	out := Node{
		Id: n.ID, ParentId: n.ParentID, Type: NodeType(n.Type), Name: n.Name, Code: n.Code,
		Aliases: orEmpty(n.Aliases), Description: n.Description, ClientSpecific: n.ClientSpecific,
		Clients: make([]NodeClient, len(clients)), Position: n.Position,
	}
	for i, c := range clients {
		out.Clients[i] = NodeClient{Id: c.ID, Name: c.Name}
	}
	return out
}

func nodeAudit(n db.Node, clients []db.ListNodeClientsRow) map[string]any {
	ids := make([]int64, len(clients))
	for i, c := range clients {
		ids[i] = c.ID
	}
	return map[string]any{
		"parent_id": n.ParentID, "position": n.Position, "type": n.Type, "name": n.Name, "code": n.Code,
		"aliases": n.Aliases, "description": n.Description, "client_specific": n.ClientSpecific, "client_ids": ids,
	}
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
git commit -m "feat(server): module tree with client-scoped visibility"
```

### Task 7: The permission suite

**Files:**
- Test: `server/internal/httpapi/permission_test.go`

**Interfaces:**
- Consumes: every endpoint from Tasks 2–6 and the seeders.
- Produces: `seedWorld(e *env) world` and the read and write tables. Later iterations add a row for each list, read, search, export, summary and Ask endpoint as it ships (FSD §5.3, §21.1).

This task adds tests for behavior that Tasks 2–6 already built. If a row fails, the handler is wrong, not the table: fix the handler.

- [ ] **Step 1: Write the suite**

`server/internal/httpapi/permission_test.go`:

```go
package httpapi_test

import (
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

var suiteUsers = []string{"admin", "hana", "ani", "budi", "citra", "dodi"}

type world struct {
	as       map[string]*http.Client
	clients  map[string]db.Client
	nodes    map[string]db.Node
	contacts map[string]int64
}

func seedWorld(e *env) world {
	w := world{as: map[string]*http.Client{}, clients: map[string]db.Client{}, nodes: map[string]db.Node{}, contacts: map[string]int64{}}
	for _, name := range []string{"A", "B", "C"} {
		w.clients[name] = e.seedClient("Client " + name)
	}
	a, b, c := w.clients["A"], w.clients["B"], w.clients["C"]
	hris := e.seedProject("HRIS", a, b, c)
	pay := e.seedProject("PAY", a, c)
	u := map[string]db.User{}
	for _, name := range suiteUsers {
		w.as[name], u[name] = e.signedIn(name+"@example.com", name == "admin")
	}
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
	return w
}

// listed returns the sorted keys (projects) or names of a list response.
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
		{"/projects/HRIS/clients", map[string][]string{"admin": abc, "hana": abc},
			map[string]int{"ani": 403, "budi": 403, "citra": 403, "dodi": 404}},
		{"/projects/HRIS/members", map[string][]string{"admin": hrisMembers, "hana": hrisMembers},
			map[string]int{"ani": 403, "budi": 403, "citra": 403, "dodi": 404}},
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
	for _, user := range suiteUsers {
		want := http.StatusOK
		if user == "dodi" {
			want = http.StatusNotFound
		}
		if code := e.call(w.as[user], http.MethodGet, "/projects/HRIS", nil, nil); code != want {
			t.Errorf("GET /projects/HRIS as %s: %d, want %d", user, code, want)
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
	for _, c := range []struct {
		user, method, path string
		body               any
		want               int
	}{
		{"hana", http.MethodPost, "/projects", map[string]any{"key": "NEW", "name": "New"}, 403},
		{"ani", http.MethodPatch, "/projects/HRIS", map[string]any{"name": "X"}, 403},
		{"dodi", http.MethodPatch, "/projects/HRIS", map[string]any{"name": "X"}, 404},
		{"ani", http.MethodPut, "/projects/HRIS/clients", map[string]any{"client_ids": []int64{}}, 403},
		{"ani", http.MethodPut, "/projects/HRIS/members", map[string]any{"members": []any{}}, 403},
		{"hana", http.MethodPatch, client("A"), map[string]any{"name": "X"}, 403},
		{"ani", http.MethodPost, "/clients", map[string]any{"name": "X"}, 403},
		{"ani", http.MethodPost, "/projects/HRIS/nodes", menu, 403},
		{"dodi", http.MethodPost, "/projects/HRIS/nodes", menu, 404},
		{"ani", http.MethodPatch, node("Leave Request"), map[string]any{"name": "X"}, 403},
		{"budi", http.MethodPatch, node("Overtime Approval"), map[string]any{"name": "X"}, 404}, // hidden by scope
		{"budi", http.MethodDelete, node("OT Rules"), nil, 404},                                 // under a hidden menu
		{"dodi", http.MethodDelete, node("Leave Request"), nil, 404},
		{"budi", http.MethodPatch, contact("Andi"), map[string]any{"name": "X"}, 404},
		{"citra", http.MethodPatch, contact("Andi"), map[string]any{"name": "X"}, 403}, // a viewer for Client A
		{"budi", http.MethodPost, "/contacts", map[string]any{"name": "X", "client_id": w.clients["A"].ID}, 422},
		{"dodi", http.MethodPost, "/contacts", map[string]any{"name": "X", "client_id": w.clients["B"].ID}, 422}, // B is no PAY client
		// Allowed, as controls: the suite must not pass by refusing everything.
		{"hana", http.MethodPatch, node("Leave Request"), map[string]any{"name": "Leave Requests"}, 200},
		{"citra", http.MethodPatch, contact("Cahya"), map[string]any{"name": "Cahya", "client_id": w.clients["C"].ID}, 200},
	} {
		if code := e.call(w.as[c.user], c.method, c.path, c.body, nil); code != c.want {
			t.Errorf("%s %s as %s: %d, want %d", c.method, c.path, c.user, code, c.want)
		}
	}
}
```

- [ ] **Step 2: Run the suite**

```bash
cd server && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./internal/httpapi/ -run PermissionSuite -v
```

Expected: `--- PASS: TestPermissionSuiteReads` and `--- PASS: TestPermissionSuiteWrites`.

- [ ] **Step 3: Commit**

```bash
git add server/internal/httpapi/permission_test.go
git commit -m "test(server): permission suite for projects, clients, contacts and the tree"
```

### Task 8: Web shell: strings, top bar, project list and new project

**Files:**
- Modify: `web/messages/en.json`, `web/messages/id.json` (every Iteration 1 string)
- Modify: `web/lib/server-api.ts` (`getMe` cached per request, `getProject`), `web/lib/problem.ts` (types, `useProblemText`)
- Create: `web/app/Header.tsx`; Modify: `web/app/layout.tsx`
- Replace: `web/app/page.tsx`
- Create: `web/app/projects/new/page.tsx`, `web/app/projects/new/NewProjectForm.tsx`

**Interfaces:**
- Consumes: `/me`, `/projects`, `/projects/{key}` (Task 2).
- Produces:
  - `getMe()` and `getProject(key)`, both wrapped in React `cache`, so a layout and its page share one API call.
  - Types `Project`, `Client`, `Member`, `Node`, `ProjectRole` and `useProblemText(): (p?: Problem) => string` from `@/lib/problem`.
  - Message namespaces `nav`, `home`, `newProject`, `project`, `settings`, `clients`, `modules`, and new `errors` keys.
  - Accessible names the e2e test uses: link "Muasal"; links "Pengguna", "Klien" and "Proyek baru"; the fields "Kunci" and "Nama"; the button "Buat proyek".

The home page loses its own profile, users and sign-out links, because the top bar now has them. The Iteration 0 test clicks "Pengguna", which must stay a single link on the page.

- [ ] **Step 1: Add the strings**

Replace `web/messages/en.json`. It keeps every Iteration 0 key except `home.profile`, `home.users` and `home.signOut`, which move to `nav`:

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
  "nav": {
    "label": "Main",
    "users": "Users",
    "clients": "Clients",
    "profile": "Profile",
    "signOut": "Sign out"
  },
  "home": {
    "signedInAs": "Signed in as {name}",
    "projects": "Projects",
    "newProject": "New project",
    "noProjects": "You are not in any project yet. Ask your admin to add you.",
    "noProjectsAdmin": "No projects yet. Create the first one."
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
  "newProject": {
    "title": "New project",
    "key": "Key",
    "keyHint": "2–10 capital letters or digits, e.g. HRIS. Ticket keys start with it.",
    "name": "Name",
    "description": "Description",
    "create": "Create project",
    "adminsOnly": "Only system admins can create projects."
  },
  "project": {
    "nav": "Project",
    "modules": "Modules",
    "settings": "Settings"
  },
  "settings": {
    "adminsOnly": "Only project admins can change settings.",
    "general": "General",
    "key": "Key",
    "name": "Name",
    "description": "Description",
    "save": "Save",
    "saved": "Saved",
    "clientsTitle": "Project clients",
    "clientsHint": "Menus and member scopes can use only these clients.",
    "saveClients": "Save clients",
    "noClients": "No clients yet. Create one below.",
    "archived": "archived",
    "newClient": "New client",
    "createClient": "Create client",
    "clientCreated": "{name} created. Save the clients to link it.",
    "membersTitle": "Members",
    "member": "User",
    "role": "Role",
    "roleAdmin": "Project admin",
    "roleMember": "Member",
    "roleViewer": "Viewer",
    "scope": "Client scope",
    "allClients": "All clients",
    "someClients": "Selected clients",
    "remove": "Remove",
    "email": "Email",
    "addMember": "Add member",
    "saveMembers": "Save members",
    "noMembers": "No members yet."
  },
  "clients": {
    "title": "Clients",
    "name": "Name",
    "code": "Code",
    "aliases": "Aliases (comma-separated)",
    "aliasesTitle": "Aliases",
    "status": "Status",
    "active": "Active",
    "archived": "Archived",
    "create": "Create client",
    "edit": "Edit",
    "editTitle": "Edit {name}",
    "save": "Save",
    "cancel": "Cancel",
    "archive": "Archive",
    "restore": "Restore",
    "adminsOnly": "Only system admins can manage clients."
  },
  "modules": {
    "tree": "Module tree",
    "details": "Details",
    "filter": "Filter",
    "filterPlaceholder": "Name, alias or code",
    "addTop": "Add module",
    "addChild": "Add child",
    "module": "Module",
    "menu": "Menu",
    "shared": "Shared",
    "clientSpecific": "Client-specific",
    "hasClientSpecific": "Has client-specific menus",
    "newTitle": "New module or menu",
    "editTitle": "Edit {name}",
    "under": "Under {path}",
    "name": "Name",
    "type": "Type",
    "code": "Code",
    "aliases": "Aliases (comma-separated)",
    "aliasesTitle": "Aliases",
    "description": "Description",
    "scope": "Client scope",
    "noLinkedClients": "Link clients to this project in Settings first.",
    "save": "Save",
    "cancel": "Cancel",
    "moveUp": "Move up",
    "moveDown": "Move down",
    "moveTo": "Move to",
    "topLevel": "(Top level)",
    "move": "Move",
    "delete": "Delete",
    "confirmDelete": "Delete {name}?",
    "expand": "Expand {name}",
    "collapse": "Collapse {name}",
    "empty": "No modules yet.",
    "emptyAdmin": "No modules yet. Add the first module to start the tree.",
    "pick": "Select a module or menu to see its details."
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
    "cannot_change_self": "You cannot disable yourself or remove your own admin role",
    "forbidden": "You do not have access to this.",
    "not_found": "Not found.",
    "project_key_taken": "Another project already uses this key",
    "client_name_taken": "A client with this name already exists",
    "client_in_use": "A client you removed is still used by menus or member scopes",
    "client_not_linked": "Link this client to the project first",
    "unknown_client": "One of these clients does not exist",
    "unknown_user": "No user has this email",
    "duplicate": "This user is listed twice",
    "admin_needs_all_clients": "Project admins always see all clients",
    "cannot_demote_self": "You cannot remove your own project admin role",
    "node_name_taken": "Another item under the same parent has this name",
    "node_code_taken": "Another item in this project has this code",
    "node_has_children": "Delete or move its sub-items first",
    "node_cycle": "An item cannot move under itself",
    "client_specific_module": "Only menus can be client-specific"
  }
}
```

Replace `web/messages/id.json`:

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
  "nav": {
    "label": "Utama",
    "users": "Pengguna",
    "clients": "Klien",
    "profile": "Profil",
    "signOut": "Keluar"
  },
  "home": {
    "signedInAs": "Masuk sebagai {name}",
    "projects": "Proyek",
    "newProject": "Proyek baru",
    "noProjects": "Anda belum tergabung dalam proyek. Minta admin menambahkan Anda.",
    "noProjectsAdmin": "Belum ada proyek. Buat proyek pertama."
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
  "newProject": {
    "title": "Proyek baru",
    "key": "Kunci",
    "keyHint": "2–10 huruf kapital atau angka, mis. HRIS. Kunci tiket diawali dengan ini.",
    "name": "Nama",
    "description": "Deskripsi",
    "create": "Buat proyek",
    "adminsOnly": "Hanya admin sistem yang dapat membuat proyek."
  },
  "project": {
    "nav": "Proyek",
    "modules": "Modul",
    "settings": "Pengaturan"
  },
  "settings": {
    "adminsOnly": "Hanya admin proyek yang dapat mengubah pengaturan.",
    "general": "Umum",
    "key": "Kunci",
    "name": "Nama",
    "description": "Deskripsi",
    "save": "Simpan",
    "saved": "Tersimpan",
    "clientsTitle": "Klien proyek",
    "clientsHint": "Menu dan cakupan anggota hanya dapat memakai klien ini.",
    "saveClients": "Simpan klien",
    "noClients": "Belum ada klien. Buat di bawah ini.",
    "archived": "diarsipkan",
    "newClient": "Klien baru",
    "createClient": "Buat klien",
    "clientCreated": "{name} dibuat. Simpan klien untuk menautkannya.",
    "membersTitle": "Anggota",
    "member": "Pengguna",
    "role": "Peran",
    "roleAdmin": "Admin proyek",
    "roleMember": "Anggota",
    "roleViewer": "Pengamat",
    "scope": "Cakupan klien",
    "allClients": "Semua klien",
    "someClients": "Klien tertentu",
    "remove": "Hapus",
    "email": "Email",
    "addMember": "Tambah anggota",
    "saveMembers": "Simpan anggota",
    "noMembers": "Belum ada anggota."
  },
  "clients": {
    "title": "Klien",
    "name": "Nama",
    "code": "Kode",
    "aliases": "Alias (pisahkan dengan koma)",
    "aliasesTitle": "Alias",
    "status": "Status",
    "active": "Aktif",
    "archived": "Diarsipkan",
    "create": "Buat klien",
    "edit": "Ubah",
    "editTitle": "Ubah {name}",
    "save": "Simpan",
    "cancel": "Batal",
    "archive": "Arsipkan",
    "restore": "Pulihkan",
    "adminsOnly": "Hanya admin sistem yang dapat mengelola klien."
  },
  "modules": {
    "tree": "Pohon modul",
    "details": "Detail",
    "filter": "Saring",
    "filterPlaceholder": "Nama, alias, atau kode",
    "addTop": "Tambah modul",
    "addChild": "Tambah anak",
    "module": "Modul",
    "menu": "Menu",
    "shared": "Bersama",
    "clientSpecific": "Khusus klien",
    "hasClientSpecific": "Ada menu khusus klien",
    "newTitle": "Modul atau menu baru",
    "editTitle": "Ubah {name}",
    "under": "Di bawah {path}",
    "name": "Nama",
    "type": "Jenis",
    "code": "Kode",
    "aliases": "Alias (pisahkan dengan koma)",
    "aliasesTitle": "Alias",
    "description": "Deskripsi",
    "scope": "Cakupan klien",
    "noLinkedClients": "Tautkan klien ke proyek ini di Pengaturan terlebih dahulu.",
    "save": "Simpan",
    "cancel": "Batal",
    "moveUp": "Naik",
    "moveDown": "Turun",
    "moveTo": "Pindahkan ke",
    "topLevel": "(Tingkat atas)",
    "move": "Pindahkan",
    "delete": "Hapus",
    "confirmDelete": "Hapus {name}?",
    "expand": "Buka {name}",
    "collapse": "Tutup {name}",
    "empty": "Belum ada modul.",
    "emptyAdmin": "Belum ada modul. Tambahkan modul pertama untuk mulai menyusun pohon.",
    "pick": "Pilih modul atau menu untuk melihat detailnya."
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
    "cannot_change_self": "Anda tidak dapat menonaktifkan diri sendiri atau mencabut peran admin Anda",
    "forbidden": "Anda tidak memiliki akses untuk ini.",
    "not_found": "Tidak ditemukan.",
    "project_key_taken": "Kunci ini sudah dipakai proyek lain",
    "client_name_taken": "Klien dengan nama ini sudah ada",
    "client_in_use": "Klien yang dihapus masih dipakai menu atau cakupan anggota",
    "client_not_linked": "Tautkan klien ini ke proyek terlebih dahulu",
    "unknown_client": "Salah satu klien ini tidak ada",
    "unknown_user": "Tidak ada pengguna dengan email ini",
    "duplicate": "Pengguna ini tercantum dua kali",
    "admin_needs_all_clients": "Admin proyek selalu melihat semua klien",
    "cannot_demote_self": "Anda tidak dapat mencabut peran admin proyek Anda sendiri",
    "node_name_taken": "Sudah ada item dengan nama ini di induk yang sama",
    "node_code_taken": "Kode ini sudah dipakai item lain di proyek ini",
    "node_has_children": "Hapus atau pindahkan sub-itemnya terlebih dahulu",
    "node_cycle": "Item tidak dapat dipindahkan ke bawah dirinya sendiri",
    "client_specific_module": "Hanya menu yang dapat dikhususkan untuk klien"
  }
}
```

- [ ] **Step 2: Share per-request reads and error text**

Replace `getMe` in `web/lib/server-api.ts` and add `getProject`. Add `import { cache } from "react";` to the imports.

```ts
/** The signed-in user, or undefined when the session is missing or has ended. One API call per request. */
export const getMe = cache(async () => {
  const { data } = await (await serverApi()).GET("/me");
  return data;
});

/** A project the user belongs to, or undefined: missing and hidden projects look the same. */
export const getProject = cache(async (key: string) => {
  const { data } = await (await serverApi()).GET("/projects/{key}", { params: { path: { key } } });
  return data;
});
```

Replace `web/lib/problem.ts`:

```ts
import { useTranslations } from "next-intl";
import type { components } from "./api-types";

export type User = components["schemas"]["User"];
export type Problem = components["schemas"]["Problem"];
export type Project = components["schemas"]["Project"];
export type ProjectRole = components["schemas"]["ProjectRole"];
export type Client = components["schemas"]["Client"];
export type Member = components["schemas"]["Member"];
export type Node = components["schemas"]["Node"];

/** The translation key for an API error: the first field error's code, else the problem code. */
export function problemKey(p?: Problem): string {
  return p?.errors?.[0]?.code ?? p?.code ?? "generic";
}

/** Turns an API problem into a sentence in the user's language. */
export function useProblemText() {
  const t = useTranslations("errors");
  return (p?: Problem) => {
    const key = problemKey(p);
    return t.has(key) ? t(key) : t("generic");
  };
}
```

- [ ] **Step 3: Add the top bar**

`web/app/Header.tsx`:

```tsx
import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { getMe } from "@/lib/server-api";
import SignOutButton from "./SignOutButton";

// The top bar on every signed-in page (FSD §6.1). Search, Ask and New ticket join it in later iterations.
export default async function Header() {
  const me = await getMe();
  if (!me) return null;
  const t = await getTranslations("nav");
  return (
    <header className="border-b bg-white">
      <nav aria-label={t("label")} className="mx-auto flex max-w-6xl items-center gap-5 px-6 py-3 text-sm">
        <Link href="/" className="font-semibold">Muasal</Link>
        {me.is_admin && <Link href="/admin/users">{t("users")}</Link>}
        {me.is_admin && <Link href="/admin/clients">{t("clients")}</Link>}
        <span className="ml-auto" />
        <Link href="/settings/profile">{t("profile")}</Link>
        <SignOutButton label={t("signOut")} />
      </nav>
    </header>
  );
}
```

In `web/app/layout.tsx`, add `import Header from "./Header";` and render it first inside the provider:

```tsx
        <NextIntlClientProvider>
          <Header />
          {children}
        </NextIntlClientProvider>
```

- [ ] **Step 4: List the user's projects on the home page**

Replace `web/app/page.tsx`:

```tsx
import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getMe, serverApi } from "@/lib/server-api";

export default async function Home() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("home");
  const { data } = await (await serverApi()).GET("/projects");
  const projects = data?.items ?? [];
  return (
    <main className="mx-auto max-w-4xl p-8">
      <p className="text-neutral-600">{t("signedInAs", { name: me.name })}</p>
      <div className="mt-6 flex items-center justify-between">
        <h1 className="text-2xl font-semibold">{t("projects")}</h1>
        {me.is_admin && (
          <Link className="rounded bg-neutral-900 px-4 py-2 text-sm text-white" href="/projects/new">
            {t("newProject")}
          </Link>
        )}
      </div>
      {projects.length === 0 ? (
        <p className="mt-4 text-neutral-600">{me.is_admin ? t("noProjectsAdmin") : t("noProjects")}</p>
      ) : (
        <ul className="mt-4 divide-y rounded-lg border bg-white">
          {projects.map((p) => (
            <li key={p.id}>
              <Link className="flex items-baseline gap-3 p-4 hover:bg-neutral-50" href={`/p/${p.key}/modules`}>
                <span className="font-mono text-sm text-neutral-500">{p.key}</span>
                <span>{p.name}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </main>
  );
}
```

- [ ] **Step 5: Add the new project page**

`web/app/projects/new/page.tsx`:

```tsx
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getMe } from "@/lib/server-api";
import NewProjectForm from "./NewProjectForm";

export default async function NewProjectPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("newProject");
  if (!me.is_admin) return <main className="p-8">{t("adminsOnly")}</main>;
  return (
    <main className="mx-auto max-w-xl p-8">
      <h1 className="mb-6 text-2xl font-semibold">{t("title")}</h1>
      <NewProjectForm />
    </main>
  );
}
```

`web/app/projects/new/NewProjectForm.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText } from "@/lib/problem";

export default function NewProjectForm() {
  const t = useTranslations("newProject");
  const problemText = useProblemText();
  const router = useRouter();
  const [error, setError] = useState("");

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const { data, error } = await api.POST("/projects", {
      body: { key: String(form.get("key")), name: String(form.get("name")), description: String(form.get("description")) },
    });
    if (error) return setError(problemText(error));
    router.push(`/p/${data.key}/settings`); // next: link clients and add members
  }

  const input = "rounded border px-3 py-2";
  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      <label className="flex flex-col gap-1 text-sm">
        {t("key")}
        <input name="key" required maxLength={10} aria-describedby="key-hint" className={`${input} font-mono uppercase`} />
      </label>
      <p id="key-hint" className="-mt-2 text-xs text-neutral-500">{t("keyHint")}</p>
      <label className="flex flex-col gap-1 text-sm">
        {t("name")}
        <input name="name" required maxLength={200} className={input} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("description")}
        <textarea name="description" maxLength={2000} rows={3} className={input} />
      </label>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("create")}</button>
    </form>
  );
}
```

- [ ] **Step 6: Build**

```bash
cd web && npm run build
```

Expected: the build succeeds and lists the routes `/`, `/projects/new`, `/admin/users`, `/login`, `/settings/profile` and `/setup/[token]`.

- [ ] **Step 7: Commit**

```bash
git add web
git commit -m "feat(web): top bar, project list and new project page"
```

### Task 9: Client directory page

**Files:**
- Create: `web/app/admin/clients/page.tsx`, `web/app/admin/clients/ClientsAdmin.tsx`

**Interfaces:**
- Consumes: `/clients`, `/clients/{id}` (Task 3); `useProblemText`, `Client` (Task 8).
- Produces: the page `/admin/clients` for system admins. The e2e test uses the field "Nama", the button "Buat klien" and a table cell with each client's name.

- [ ] **Step 1: Write the page**

`web/app/admin/clients/page.tsx`:

```tsx
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getMe, serverApi } from "@/lib/server-api";
import ClientsAdmin from "./ClientsAdmin";

export default async function ClientsPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("clients");
  if (!me.is_admin) return <main className="p-8">{t("adminsOnly")}</main>;
  const { data } = await (await serverApi()).GET("/clients");
  return (
    <main className="mx-auto max-w-5xl p-8">
      <h1 className="mb-6 text-2xl font-semibold">{t("title")}</h1>
      <ClientsAdmin clients={data?.items ?? []} />
    </main>
  );
}
```

`web/app/admin/clients/ClientsAdmin.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Client } from "@/lib/problem";

type ClientPatch = { name?: string; code?: string; aliases?: string[]; archived?: boolean };

const aliasList = (v: FormDataEntryValue | null) => String(v ?? "").split(",");

export default function ClientsAdmin({ clients }: { clients: Client[] }) {
  const t = useTranslations("clients");
  const problemText = useProblemText();
  const router = useRouter();
  const [editing, setEditing] = useState<number | null>(null);
  const [error, setError] = useState("");

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formEl = e.currentTarget;
    const form = new FormData(formEl);
    const { error } = await api.POST("/clients", {
      body: { name: String(form.get("name")), code: String(form.get("code")), aliases: aliasList(form.get("aliases")) },
    });
    if (error) return setError(problemText(error));
    setError("");
    formEl.reset();
    router.refresh();
  }

  async function update(c: Client, body: ClientPatch) {
    const { error } = await api.PATCH("/clients/{id}", { params: { path: { id: c.id } }, body });
    if (error) return setError(problemText(error));
    setError("");
    setEditing(null);
    router.refresh();
  }

  const input = "rounded border px-3 py-2";
  return (
    <div className="flex flex-col gap-6">
      <form aria-label={t("create")} onSubmit={create} className="flex flex-wrap items-end gap-3 rounded-lg border bg-white p-4">
        <label className="flex flex-col gap-1 text-sm">
          {t("name")}
          <input name="name" required maxLength={200} className={input} />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          {t("code")}
          <input name="code" maxLength={20} className={`${input} w-28 font-mono`} />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          {t("aliases")}
          <input name="aliases" className={input} />
        </label>
        <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("create")}</button>
      </form>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <table className="w-full border-collapse bg-white text-left text-sm">
        <thead>
          <tr className="border-b">
            <th className="p-2">{t("name")}</th>
            <th className="p-2">{t("code")}</th>
            <th className="p-2">{t("aliasesTitle")}</th>
            <th className="p-2">{t("status")}</th>
            <th className="p-2" />
          </tr>
        </thead>
        <tbody>
          {clients.map((c) =>
            editing === c.id ? (
              <tr key={c.id} className="border-b">
                <td colSpan={5} className="p-2">
                  <form
                    aria-label={t("editTitle", { name: c.name })}
                    className="flex flex-wrap items-end gap-3"
                    onSubmit={(e) => {
                      e.preventDefault();
                      const form = new FormData(e.currentTarget);
                      update(c, { name: String(form.get("name")), code: String(form.get("code")), aliases: aliasList(form.get("aliases")) });
                    }}
                  >
                    <label className="flex flex-col gap-1 text-sm">
                      {t("name")}
                      <input name="name" defaultValue={c.name} required maxLength={200} className={input} />
                    </label>
                    <label className="flex flex-col gap-1 text-sm">
                      {t("code")}
                      <input name="code" defaultValue={c.code ?? ""} maxLength={20} className={`${input} w-28 font-mono`} />
                    </label>
                    <label className="flex flex-col gap-1 text-sm">
                      {t("aliases")}
                      <input name="aliases" defaultValue={c.aliases.join(", ")} className={input} />
                    </label>
                    <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("save")}</button>
                    <button type="button" onClick={() => setEditing(null)} className="rounded border px-4 py-2">{t("cancel")}</button>
                  </form>
                </td>
              </tr>
            ) : (
              <tr key={c.id} className="border-b">
                <td className="p-2">{c.name}</td>
                <td className="p-2 font-mono">{c.code ?? ""}</td>
                <td className="p-2">{c.aliases.join(", ")}</td>
                <td className="p-2">{c.archived ? t("archived") : t("active")}</td>
                <td className="flex gap-3 p-2">
                  <button type="button" className="underline" onClick={() => setEditing(c.id)}>{t("edit")}</button>
                  <button type="button" className="underline" onClick={() => update(c, { archived: !c.archived })}>
                    {c.archived ? t("restore") : t("archive")}
                  </button>
                </td>
              </tr>
            ),
          )}
        </tbody>
      </table>
    </div>
  );
}
```

- [ ] **Step 2: Build**

```bash
cd web && npm run build
```

Expected: the build succeeds and lists `/admin/clients`.

- [ ] **Step 3: Commit**

```bash
git add web/app/admin/clients
git commit -m "feat(web): client directory for system admins"
```

### Task 10: Project layout and settings

**Files:**
- Create: `web/app/p/[key]/layout.tsx`
- Create: `web/app/p/[key]/settings/page.tsx`, `ProjectForm.tsx`, `ProjectClients.tsx`, `ProjectMembers.tsx`

**Interfaces:**
- Consumes:
  - From Task 8: `getProject`, `serverApi`, `useProblemText` and the types.
  - The endpoints `/projects/{key}`, `/clients` and `/projects/{key}/clients` (Task 3), and `/projects/{key}/members` (Task 4).
- Produces:
  - The project header, with the tabs "Modul" and "Pengaturan" (the latter for project admins only).
  - The settings page, with forms named "Umum", "Klien proyek", "Klien baru" and "Tambah anggota", and the region "Anggota".
  - Each member row has the selects "Peran" and "Cakupan klien", with the options `all` and `some`, plus one checkbox per linked client when the scope is `some`.
  - The buttons "Simpan klien", "Tambah anggota" and "Simpan anggota".

- [ ] **Step 1: Write the project layout**

`web/app/p/[key]/layout.tsx`:

```tsx
import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getProject } from "@/lib/server-api";

// Shared by every project page. Projects the user does not belong to answer 404.
export default async function ProjectLayout({ children, params }: { children: React.ReactNode; params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("project");
  return (
    <div className="mx-auto max-w-6xl p-6">
      <div className="flex items-baseline gap-3">
        <span className="font-mono text-sm text-neutral-500">{project.key}</span>
        <h1 className="text-2xl font-semibold">{project.name}</h1>
      </div>
      <nav aria-label={t("nav")} className="mt-4 flex gap-5 border-b text-sm">
        <Link href={`/p/${project.key}/modules`} className="pb-2">{t("modules")}</Link>
        {project.role === "admin" && (
          <Link href={`/p/${project.key}/settings`} className="pb-2">{t("settings")}</Link>
        )}
      </nav>
      <div className="mt-6">{children}</div>
    </div>
  );
}
```

- [ ] **Step 2: Write the settings page**

`web/app/p/[key]/settings/page.tsx`:

```tsx
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getProject, serverApi } from "@/lib/server-api";
import ProjectClients from "./ProjectClients";
import ProjectForm from "./ProjectForm";
import ProjectMembers from "./ProjectMembers";

export default async function SettingsPage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("settings");
  if (project.role !== "admin") return <p>{t("adminsOnly")}</p>;
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [all, linked, members] = await Promise.all([
    api.GET("/clients"),
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/members", path),
  ]);
  const linkedClients = linked.data?.items ?? [];
  return (
    <div className="flex flex-col gap-8">
      <ProjectForm project={project} />
      <ProjectClients projectKey={key} all={all.data?.items ?? []} linked={linkedClients} />
      <ProjectMembers projectKey={key} members={members.data?.items ?? []} clients={linkedClients} />
    </div>
  );
}
```

`web/app/p/[key]/settings/ProjectForm.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Project } from "@/lib/problem";

export default function ProjectForm({ project }: { project: Project }) {
  const t = useTranslations("settings");
  const problemText = useProblemText();
  const router = useRouter();
  const [status, setStatus] = useState("");

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const { data, error } = await api.PATCH("/projects/{key}", {
      params: { path: { key: project.key } },
      body: { key: String(form.get("key")), name: String(form.get("name")), description: String(form.get("description")) },
    });
    if (error) return setStatus(problemText(error));
    setStatus(t("saved"));
    if (data.key !== project.key) router.replace(`/p/${data.key}/settings`); // the key is in every project URL
    else router.refresh();
  }

  const input = "rounded border px-3 py-2";
  return (
    <form aria-label={t("general")} onSubmit={onSubmit} className="flex flex-col gap-4 rounded-lg border bg-white p-4">
      <h2 className="font-medium">{t("general")}</h2>
      <label className="flex flex-col gap-1 text-sm">
        {t("key")}
        <input name="key" defaultValue={project.key} required maxLength={10} className={`${input} font-mono uppercase`} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("name")}
        <input name="name" defaultValue={project.name} required maxLength={200} className={input} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("description")}
        <textarea name="description" defaultValue={project.description} maxLength={2000} rows={3} className={input} />
      </label>
      {status && <p role="status" className="text-sm">{status}</p>}
      <button className="self-start rounded bg-neutral-900 px-4 py-2 text-white">{t("save")}</button>
    </form>
  );
}
```

`web/app/p/[key]/settings/ProjectClients.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Client } from "@/lib/problem";

// Which clients the project serves. Project admins may also create a client
// here, since /admin/clients is for system admins (FSD §5.1).
export default function ProjectClients({ projectKey, all, linked }: { projectKey: string; all: Client[]; linked: Client[] }) {
  const t = useTranslations("settings");
  const problemText = useProblemText();
  const router = useRouter();
  const [checked, setChecked] = useState(() => new Set(linked.map((c) => c.id)));
  const [status, setStatus] = useState("");
  const choices = all.filter((c) => !c.archived || checked.has(c.id));

  function toggle(id: number, on: boolean) {
    setChecked((s) => {
      const next = new Set(s);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  async function save(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const { error } = await api.PUT("/projects/{key}/clients", {
      params: { path: { key: projectKey } },
      body: { client_ids: [...checked] },
    });
    if (error) return setStatus(problemText(error));
    setStatus(t("saved"));
    router.refresh(); // member scopes offer the new links
  }

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formEl = e.currentTarget;
    const { data, error } = await api.POST("/clients", { body: { name: String(new FormData(formEl).get("name")) } });
    if (error) return setStatus(problemText(error));
    formEl.reset();
    toggle(data.id, true);
    setStatus(t("clientCreated", { name: data.name }));
    router.refresh();
  }

  return (
    <section aria-labelledby="clients-title" className="flex flex-col gap-4 rounded-lg border bg-white p-4">
      <h2 id="clients-title" className="font-medium">{t("clientsTitle")}</h2>
      <p className="text-sm text-neutral-600">{t("clientsHint")}</p>
      <form aria-label={t("clientsTitle")} onSubmit={save} className="flex flex-col gap-3">
        {choices.length === 0 && <p className="text-sm text-neutral-600">{t("noClients")}</p>}
        <div className="flex flex-wrap gap-4">
          {choices.map((c) => (
            <label key={c.id} className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={checked.has(c.id)} onChange={(e) => toggle(c.id, e.target.checked)} />
              {c.name}
              {c.archived && <span className="text-xs text-neutral-500">({t("archived")})</span>}
            </label>
          ))}
        </div>
        <button className="self-start rounded bg-neutral-900 px-4 py-2 text-white">{t("saveClients")}</button>
        {status && <p role="status" className="text-sm">{status}</p>}
      </form>
      <form aria-label={t("newClient")} onSubmit={create} className="flex items-end gap-3 border-t pt-4">
        <label className="flex flex-col gap-1 text-sm">
          {t("newClient")}
          <input name="name" required maxLength={200} className="rounded border px-3 py-2" />
        </label>
        <button className="rounded border px-4 py-2">{t("createClient")}</button>
      </form>
    </section>
  );
}
```

`web/app/p/[key]/settings/ProjectMembers.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Client, type Member, type ProjectRole } from "@/lib/problem";

type Row = { email: string; name: string; role: ProjectRole; all_clients: boolean; client_ids: number[] };

const toRow = (m: Member): Row => ({ email: m.email, name: m.name, role: m.role, all_clients: m.all_clients, client_ids: m.client_ids });

// Edits the whole member list locally and saves it in one request (FSD §15.2: one by one or in bulk).
export default function ProjectMembers({ projectKey, members, clients }: { projectKey: string; members: Member[]; clients: Client[] }) {
  const t = useTranslations("settings");
  const problemText = useProblemText();
  const [rows, setRows] = useState<Row[]>(() => members.map(toRow));
  const [status, setStatus] = useState("");

  const update = (i: number, patch: Partial<Row>) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)));

  function add(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const email = String(new FormData(e.currentTarget).get("email")).trim();
    setRows((rs) => [...rs, { email, name: email, role: "member", all_clients: true, client_ids: [] }]);
    e.currentTarget.reset();
  }

  async function save() {
    const body = {
      members: rows.map((r) => {
        const all = r.role === "admin" || r.all_clients; // project admins always see every client
        return { email: r.email, role: r.role, all_clients: all, client_ids: all ? [] : r.client_ids };
      }),
    };
    const { data, error } = await api.PUT("/projects/{key}/members", { params: { path: { key: projectKey } }, body });
    if (error) return setStatus(problemText(error));
    setRows(data.items.map(toRow));
    setStatus(t("saved"));
  }

  const input = "rounded border px-2 py-1";
  return (
    <section aria-labelledby="members-title" className="flex flex-col gap-4 rounded-lg border bg-white p-4">
      <h2 id="members-title" className="font-medium">{t("membersTitle")}</h2>
      {rows.length === 0 ? (
        <p className="text-sm text-neutral-600">{t("noMembers")}</p>
      ) : (
        <table className="w-full border-collapse text-left text-sm">
          <thead>
            <tr className="border-b">
              <th className="p-2">{t("member")}</th>
              <th className="p-2">{t("role")}</th>
              <th className="p-2">{t("scope")}</th>
              <th className="p-2" />
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={r.email} className="border-b align-top">
                <td className="p-2">
                  {r.name}
                  <div className="text-xs text-neutral-500">{r.email}</div>
                </td>
                <td className="p-2">
                  <select aria-label={t("role")} value={r.role} onChange={(e) => update(i, { role: e.target.value as ProjectRole })} className={input}>
                    <option value="admin">{t("roleAdmin")}</option>
                    <option value="member">{t("roleMember")}</option>
                    <option value="viewer">{t("roleViewer")}</option>
                  </select>
                </td>
                <td className="p-2">
                  {r.role === "admin" ? (
                    <span>{t("allClients")}</span>
                  ) : (
                    <div className="flex flex-col gap-2">
                      <select
                        aria-label={t("scope")}
                        value={r.all_clients ? "all" : "some"}
                        onChange={(e) => update(i, { all_clients: e.target.value === "all" })}
                        className={input}
                      >
                        <option value="all">{t("allClients")}</option>
                        <option value="some">{t("someClients")}</option>
                      </select>
                      {!r.all_clients &&
                        clients.map((c) => (
                          <label key={c.id} className="flex items-center gap-2">
                            <input
                              type="checkbox"
                              checked={r.client_ids.includes(c.id)}
                              onChange={(e) =>
                                update(i, { client_ids: e.target.checked ? [...r.client_ids, c.id] : r.client_ids.filter((id) => id !== c.id) })
                              }
                            />
                            {c.name}
                          </label>
                        ))}
                    </div>
                  )}
                </td>
                <td className="p-2">
                  <button type="button" className="underline" onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))}>
                    {t("remove")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <form aria-label={t("addMember")} onSubmit={add} className="flex items-end gap-3">
        <label className="flex flex-col gap-1 text-sm">
          {t("email")}
          <input name="email" type="email" required className="rounded border px-3 py-2" />
        </label>
        <button className="rounded border px-4 py-2">{t("addMember")}</button>
      </form>
      <div className="flex items-center gap-3">
        <button type="button" onClick={save} className="rounded bg-neutral-900 px-4 py-2 text-white">{t("saveMembers")}</button>
        {status && <p role="status" className="text-sm">{status}</p>}
      </div>
    </section>
  );
}
```

- [ ] **Step 3: Build and try it**

```bash
cd web && npm run build
```

Expected: the build succeeds and lists `/p/[key]/settings`. With `make up` running, sign in as the demo admin and create a project. The settings page should open with the three sections.

- [ ] **Step 4: Commit**

```bash
git add web/app/p
git commit -m "feat(web): project settings for clients and members"
```

### Task 11: The module tree screen

**Files:**
- Create: `web/app/p/[key]/modules/page.tsx`, `ModuleTree.tsx`, `NodeForm.tsx`

**Interfaces:**
- Consumes: `/projects/{key}/nodes`, `/nodes/{id}` (Task 6), `/projects/{key}/clients` (Task 3), the project layout (Task 10) and `useProblemText` (Task 8).
- Produces the screen described in FSD §7.3:
  - A tree region, with the button "Tambah modul" and one button per node, named after the node.
  - A details region. It holds the form "Modul atau menu baru", with the fields "Nama" and "Jenis", the radios "Bersama" and "Khusus klien", one checkbox per linked client, and the button "Simpan".
  - The actions "Tambah anak", "Naik", "Turun", "Hapus" and "Pindahkan ke".

- [ ] **Step 1: Write the page**

`web/app/p/[key]/modules/page.tsx`:

```tsx
import { notFound } from "next/navigation";
import { getProject, serverApi } from "@/lib/server-api";
import ModuleTree from "./ModuleTree";

export default async function ModulesPage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const canEdit = project.role === "admin";
  const nodes = (await api.GET("/projects/{key}/nodes", path)).data?.items ?? [];
  // Only project admins pick clients for menus; the link list is theirs to read.
  const clients = canEdit ? ((await api.GET("/projects/{key}/clients", path)).data?.items ?? []) : [];
  return <ModuleTree projectKey={key} nodes={nodes} clients={clients} canEdit={canEdit} />;
}
```

- [ ] **Step 2: Write the node form and the read-only view**

`web/app/p/[key]/modules/NodeForm.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Client, type Node } from "@/lib/problem";

type Props = {
  projectKey: string;
  node?: Node; // edit this node; without it the form creates one
  parentId?: number;
  parentPath?: string;
  clients: Client[];
  onSaved: (id: number) => void;
  onCancel?: () => void;
};

// Creates or edits a module or menu. The client scope applies to menus only (R-MR-7).
export default function NodeForm({ projectKey, node, parentId, parentPath, clients, onSaved, onCancel }: Props) {
  const t = useTranslations("modules");
  const problemText = useProblemText();
  const [type, setType] = useState<Node["type"]>(node?.type ?? (parentId === undefined ? "module" : "menu"));
  const [specific, setSpecific] = useState(node?.client_specific ?? false);
  const [error, setError] = useState("");

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const clientSpecific = type === "menu" && specific;
    const body = {
      name: String(form.get("name")),
      type,
      code: String(form.get("code")),
      aliases: String(form.get("aliases")).split(","),
      description: String(form.get("description")),
      client_specific: clientSpecific,
      client_ids: clientSpecific ? form.getAll("client_ids").map(Number) : [],
    };
    if (node) {
      const { data, error } = await api.PATCH("/nodes/{id}", { params: { path: { id: node.id } }, body });
      if (error) return setError(problemText(error));
      onSaved(data.id);
    } else {
      const { data, error } = await api.POST("/projects/{key}/nodes", {
        params: { path: { key: projectKey } },
        body: { ...body, parent_id: parentId },
      });
      if (error) return setError(problemText(error));
      onSaved(data.id);
    }
  }

  const input = "rounded border px-3 py-2";
  const title = node ? t("editTitle", { name: node.name }) : t("newTitle");
  return (
    <form aria-label={title} onSubmit={onSubmit} className="flex flex-col gap-3">
      <h2 className="font-medium">{title}</h2>
      {parentPath && <p className="text-sm text-neutral-600">{t("under", { path: parentPath })}</p>}
      <label className="flex flex-col gap-1 text-sm">
        {t("name")}
        <input name="name" defaultValue={node?.name} required maxLength={200} className={input} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("type")}
        <select name="type" value={type} onChange={(e) => setType(e.target.value === "menu" ? "menu" : "module")} className={input}>
          <option value="module">{t("module")}</option>
          <option value="menu">{t("menu")}</option>
        </select>
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("code")}
        <input name="code" defaultValue={node?.code ?? ""} maxLength={100} className={`${input} font-mono`} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("aliases")}
        <input name="aliases" defaultValue={node?.aliases.join(", ")} className={input} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("description")}
        <textarea name="description" defaultValue={node?.description} maxLength={5000} rows={3} className={input} />
      </label>
      {type === "menu" && (
        <fieldset className="flex flex-col gap-2 text-sm">
          <legend className="mb-1">{t("scope")}</legend>
          <label className="flex items-center gap-2">
            <input type="radio" name="scope" checked={!specific} onChange={() => setSpecific(false)} />
            {t("shared")}
          </label>
          <label className="flex items-center gap-2">
            <input type="radio" name="scope" checked={specific} onChange={() => setSpecific(true)} />
            {t("clientSpecific")}
          </label>
          {specific &&
            (clients.length === 0 ? (
              <p className="text-neutral-600">{t("noLinkedClients")}</p>
            ) : (
              clients.map((c) => (
                <label key={c.id} className="ml-6 flex items-center gap-2">
                  <input type="checkbox" name="client_ids" value={c.id} defaultChecked={node?.clients.some((x) => x.id === c.id)} />
                  {c.name}
                </label>
              ))
            ))}
        </fieldset>
      )}
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <div className="flex gap-3">
        <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("save")}</button>
        {onCancel && (
          <button type="button" onClick={onCancel} className="rounded border px-4 py-2">
            {t("cancel")}
          </button>
        )}
      </div>
    </form>
  );
}

// What members and viewers see of a node; editing is for project admins.
export function ReadOnlyNode({ node }: { node: Node }) {
  const t = useTranslations("modules");
  return (
    <div className="flex flex-col gap-3">
      <h2 className="font-medium">{node.name}</h2>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
        <dt className="text-neutral-500">{t("type")}</dt>
        <dd>{t(node.type)}</dd>
        <dt className="text-neutral-500">{t("code")}</dt>
        <dd className="font-mono">{node.code ?? "—"}</dd>
        <dt className="text-neutral-500">{t("aliasesTitle")}</dt>
        <dd>{node.aliases.join(", ") || "—"}</dd>
        <dt className="text-neutral-500">{t("description")}</dt>
        <dd className="whitespace-pre-wrap">{node.description || "—"}</dd>
      </dl>
    </div>
  );
}
```

- [ ] **Step 3: Write the tree**

`web/app/p/[key]/modules/ModuleTree.tsx`:

```tsx
"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Client, type Node, type Problem } from "@/lib/problem";
import NodeForm, { ReadOnlyNode } from "./NodeForm";

type Children = Map<number | null, Node[]>;

// ancestorsOf lists n's parents, nearest first.
function ancestorsOf(n: Node, byId: Map<number, Node>): Node[] {
  const out: Node[] = [];
  for (let p = n.parent_id === null ? undefined : byId.get(n.parent_id); p; p = p.parent_id === null ? undefined : byId.get(p.parent_id)) {
    out.push(p);
  }
  return out;
}

// subtree holds id and every node below it: the places a node cannot move to.
function subtree(id: number, children: Children): Set<number> {
  const out = new Set([id]);
  const stack = [id];
  while (stack.length > 0) {
    for (const c of children.get(stack.pop()!) ?? []) {
      out.add(c.id);
      stack.push(c.id);
    }
  }
  return out;
}

type Props = { projectKey: string; nodes: Node[]; clients: Client[]; canEdit: boolean };

// The module tree screen (FSD §7.3): the tree on the left, the selected node on the right.
// ponytail: moves use a parent picker and up/down buttons; drag-and-drop arrives with dnd-kit and the board.
export default function ModuleTree({ projectKey, nodes, clients, canEdit }: Props) {
  const t = useTranslations("modules");
  const problemText = useProblemText();
  const router = useRouter();
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [creatingUnder, setCreatingUnder] = useState<number | null | undefined>(undefined); // undefined: not creating
  const [filter, setFilter] = useState("");
  const [collapsed, setCollapsed] = useState<Set<number>>(() => new Set());
  const [error, setError] = useState("");

  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
  const children = useMemo(() => {
    const m: Children = new Map();
    for (const n of nodes) m.set(n.parent_id, [...(m.get(n.parent_id) ?? []), n]);
    return m;
  }, [nodes]);
  // R-MR-9: a module above a visible client-specific menu gets a badge.
  const hasSpecific = useMemo(() => {
    const out = new Set<number>();
    for (const n of nodes) if (n.client_specific) for (const p of ancestorsOf(n, byId)) out.add(p.id);
    return out;
  }, [nodes, byId]);
  // The filter keeps nodes whose name, alias or code matches, plus their ancestors.
  const shown = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return null;
    const keep = new Set<number>();
    for (const n of nodes) {
      if (![n.name, n.code ?? "", ...n.aliases].some((s) => s.toLowerCase().includes(q))) continue;
      keep.add(n.id);
      for (const p of ancestorsOf(n, byId)) keep.add(p.id);
    }
    return keep;
  }, [filter, nodes, byId]);

  const pathOf = (n: Node) => [...ancestorsOf(n, byId).reverse(), n].map((x) => x.name).join(" › ");
  const selected = selectedId === null ? undefined : byId.get(selectedId);

  function select(id: number | null) {
    setSelectedId(id);
    setCreatingUnder(undefined);
    setError("");
  }

  function toggle(id: number) {
    setCollapsed((s) => {
      const next = new Set(s);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function run(call: Promise<{ error?: Problem }>, then?: () => void) {
    const { error } = await call;
    if (error) return setError(problemText(error));
    setError("");
    then?.();
    router.refresh();
  }

  const chip = "rounded bg-neutral-100 px-2 py-0.5 text-xs";
  function badge(n: Node) {
    if (n.type === "module") return hasSpecific.has(n.id) ? <span className={chip}>{t("hasClientSpecific")}</span> : null;
    return <span className={chip}>{n.client_specific ? n.clients.map((c) => c.name).join(", ") : t("shared")}</span>;
  }

  function level(parentId: number | null, depth: number): React.ReactNode {
    const list = (children.get(parentId) ?? []).filter((n) => shown === null || shown.has(n.id));
    if (list.length === 0) return null;
    return (
      <ul>
        {list.map((n) => {
          const hasKids = (children.get(n.id)?.length ?? 0) > 0;
          const open = shown !== null || !collapsed.has(n.id);
          return (
            <li key={n.id}>
              {/* R-MR-1: indentation stops growing after six levels. */}
              <div className="flex items-center gap-2 py-1" style={{ paddingLeft: `${Math.min(depth, 6) * 1.25}rem` }}>
                {hasKids ? (
                  <button
                    type="button"
                    aria-expanded={open}
                    aria-label={t(open ? "collapse" : "expand", { name: n.name })}
                    onClick={() => toggle(n.id)}
                    className="w-5 text-neutral-500"
                  >
                    {open ? "▾" : "▸"}
                  </button>
                ) : (
                  <span className="w-5" />
                )}
                <button
                  type="button"
                  onClick={() => select(n.id)}
                  aria-current={n.id === selectedId ? "true" : undefined}
                  className={n.id === selectedId ? "font-semibold underline" : "hover:underline"}
                >
                  {n.name}
                </button>
                <span className="text-xs text-neutral-500">{t(n.type)}</span>
                {badge(n)}
              </div>
              {open && level(n.id, depth + 1)}
            </li>
          );
        })}
      </ul>
    );
  }

  const action = "rounded border px-3 py-1 disabled:opacity-40";
  function details(n: Node) {
    const siblings = children.get(n.parent_id) ?? [];
    const index = siblings.findIndex((s) => s.id === n.id);
    const blocked = subtree(n.id, children);
    const move = (parentId: number | null, position?: number) =>
      run(api.PATCH("/nodes/{id}", { params: { path: { id: n.id } }, body: { move: { parent_id: parentId, position } } }));
    const remove = () => {
      if (window.confirm(t("confirmDelete", { name: n.name }))) {
        run(api.DELETE("/nodes/{id}", { params: { path: { id: n.id } } }), () => setSelectedId(null));
      }
    };
    return (
      <div className="flex flex-col gap-4">
        <p className="text-sm text-neutral-600">{pathOf(n)}</p>
        {!canEdit ? (
          <ReadOnlyNode node={n} />
        ) : (
          <>
            <NodeForm
              key={n.id}
              projectKey={projectKey}
              node={n}
              clients={clients}
              onSaved={() => {
                setError("");
                router.refresh();
              }}
            />
            <div className="flex flex-wrap gap-2 border-t pt-4 text-sm">
              <button type="button" onClick={() => { setCreatingUnder(n.id); setError(""); }} className={action}>
                {t("addChild")}
              </button>
              <button type="button" disabled={index <= 0} onClick={() => move(n.parent_id, index - 1)} className={action}>
                {t("moveUp")}
              </button>
              <button type="button" disabled={index === siblings.length - 1} onClick={() => move(n.parent_id, index + 1)} className={action}>
                {t("moveDown")}
              </button>
              <button type="button" onClick={remove} className={`${action} border-red-300 text-red-700`}>
                {t("delete")}
              </button>
            </div>
            <form
              key={`move-${n.id}`}
              aria-label={t("moveTo")}
              onSubmit={(e) => {
                e.preventDefault();
                const v = String(new FormData(e.currentTarget).get("parent"));
                move(v === "" ? null : Number(v));
              }}
              className="flex items-end gap-2 text-sm"
            >
              <label className="flex flex-col gap-1">
                {t("moveTo")}
                <select name="parent" defaultValue={n.parent_id ?? ""} className="rounded border px-2 py-1">
                  <option value="">{t("topLevel")}</option>
                  {nodes
                    .filter((x) => !blocked.has(x.id))
                    .map((x) => (
                      <option key={x.id} value={x.id}>{pathOf(x)}</option>
                    ))}
                </select>
              </label>
              <button className={action}>{t("move")}</button>
            </form>
          </>
        )}
      </div>
    );
  }

  return (
    <div className="grid gap-6 md:grid-cols-2">
      <section aria-label={t("tree")} className="rounded-lg border bg-white p-4">
        <div className="mb-3 flex items-end gap-3">
          <label className="flex flex-1 flex-col gap-1 text-sm">
            {t("filter")}
            <input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t("filterPlaceholder")} className="rounded border px-3 py-2" />
          </label>
          {canEdit && (
            <button type="button" onClick={() => { select(null); setCreatingUnder(null); }} className="rounded bg-neutral-900 px-3 py-2 text-sm text-white">
              {t("addTop")}
            </button>
          )}
        </div>
        {nodes.length === 0 ? <p className="text-sm text-neutral-600">{canEdit ? t("emptyAdmin") : t("empty")}</p> : level(null, 0)}
      </section>
      <section aria-label={t("details")} className="rounded-lg border bg-white p-4">
        {error && <p role="alert" className="mb-3 text-sm text-red-700">{error}</p>}
        {creatingUnder !== undefined ? (
          <NodeForm
            key={`new-${creatingUnder}`}
            projectKey={projectKey}
            parentId={creatingUnder ?? undefined}
            parentPath={creatingUnder === null ? undefined : pathOf(byId.get(creatingUnder)!)}
            clients={clients}
            onSaved={(id) => {
              setCreatingUnder(undefined);
              setSelectedId(id);
              router.refresh();
            }}
            onCancel={() => setCreatingUnder(undefined)}
          />
        ) : selected ? (
          details(selected)
        ) : (
          <p className="text-sm text-neutral-600">{t("pick")}</p>
        )}
      </section>
    </div>
  );
}
```

- [ ] **Step 4: Build and try it**

```bash
cd web && npm run build
```

Expected: the build succeeds and lists `/p/[key]/modules`. Try it on the running stack:
1. Open a project as an admin.
2. Add "HR", then under it the module "Attendance", then under Attendance a client-specific menu.
3. Sign in as a member scoped to another client. The menu must be missing.

- [ ] **Step 5: Commit**

```bash
git add web/app/p
git commit -m "feat(web): module tree editor"
```

### Task 12: End-to-end exit check

**Files:**
- Create: `web/e2e/helpers.ts`, `web/e2e/registry.spec.ts`
- Modify: `web/e2e/global-setup.ts` (a second admin), `web/e2e/signin.spec.ts` (shared helpers)

**Interfaces:**
- Consumes: the running stack (`make up`, rebuilt with Tasks 1–11) and the accessible names from Tasks 8–11.
- Produces:
  - `E2E_TREE_ADMIN_EMAIL` and `E2E_TREE_ADMIN_LINK` next to the existing `E2E_ADMIN_*`, so the two test files run in parallel with their own admins.
  - `make e2e` proves both exit checks.

- [ ] **Step 1: Share the sign-in helpers and create a second admin**

`web/e2e/helpers.ts`:

```ts
import { expect, type Page } from "@playwright/test";

// The tests run in the default UI language, Indonesian: after sign-in the UI
// follows the user's profile language, and new users start with `id`.

export async function setPassword(page: Page, link: string, password: string) {
  await page.goto(link);
  await page.getByLabel("Kata sandi baru").fill(password);
  await page.getByLabel("Ulangi kata sandi").fill(password);
  await page.getByRole("button", { name: "Simpan kata sandi" }).click();
  await expect(page.getByRole("status")).toHaveText("Kata sandi tersimpan. Silakan masuk.");
}

export async function signIn(page: Page, email: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Kata sandi").fill(password);
  await page.getByRole("button", { name: "Masuk" }).click();
  await expect(page).toHaveURL(/\/$/);
}
```

In `web/e2e/signin.spec.ts`:
- Delete the local `setPassword` and `signIn` functions and the comment above the password constants.
- Change the first import to `import { expect, test } from "@playwright/test";`.
- Add `import { setPassword, signIn } from "./helpers";`.

The test body stays as it is. Its "Pengguna" link now lives in the top bar.

Replace `web/e2e/global-setup.ts`:

```ts
import { execFileSync } from "node:child_process";

const compose = ["compose", "-f", "../deploy/compose.yaml", "--env-file", "../deploy/.env"];

// createAdmin makes an admin through the CLI, as an installer would, and returns their setup link.
function createAdmin(name: string) {
  const email = `${name.toLowerCase().replaceAll(" ", "-")}-${Date.now()}@example.com`;
  const out = execFileSync(
    "docker",
    [...compose, "exec", "-T", "app", "/app", "admin", "create-admin", "--email", email, "--name", name],
    { encoding: "utf8" },
  );
  const link = out.match(/https?:\/\/\S+\/setup\/\S+/)?.[0];
  if (!link) throw new Error(`no setup link in: ${out}`);
  return { email, link };
}

// Waits for the stack, then creates one fresh admin per test file.
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
  const signin = createAdmin("E2E Admin");
  process.env.E2E_ADMIN_EMAIL = signin.email;
  process.env.E2E_ADMIN_LINK = signin.link;
  const tree = createAdmin("Tree Admin");
  process.env.E2E_TREE_ADMIN_EMAIL = tree.email;
  process.env.E2E_TREE_ADMIN_LINK = tree.link;
}
```

- [ ] **Step 2: Write the exit-check test**

`web/e2e/registry.spec.ts`:

```ts
import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// FSD §21 Iteration 1 exit check: an admin builds the HRIS tree and scopes members by client.
test("an admin builds the HRIS tree and a member scoped to one client sees only its menus", async ({ page, browser }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase(); // keys and names stay unique across runs on one stack
  const key = `H${run.slice(-6)}`;
  const clientA = `Klien A ${run}`;
  const clientB = `Klien B ${run}`;
  const budiEmail = `budi-tree-${run.toLowerCase()}@example.com`;
  const adminPassword = "e2e-tree-admin-passphrase-3";
  const budiPassword = "e2e-budi-tree-passphrase-4";

  await setPassword(page, process.env.E2E_TREE_ADMIN_LINK!, adminPassword);
  await signIn(page, process.env.E2E_TREE_ADMIN_EMAIL!, adminPassword);

  // A user who will be scoped to Client B.
  await page.getByRole("link", { name: "Pengguna" }).click();
  await page.getByLabel("Nama").fill("Budi Tree");
  await page.getByLabel("Email").fill(budiEmail);
  await page.getByRole("button", { name: "Buat pengguna" }).click();
  const budiLink = await page.getByTestId("setup-link").textContent();

  // Two clients.
  await page.getByRole("link", { name: "Klien", exact: true }).click();
  for (const name of [clientA, clientB]) {
    await page.getByLabel("Nama").fill(name);
    await page.getByRole("button", { name: "Buat klien" }).click();
    await expect(page.getByRole("cell", { name, exact: true })).toBeVisible();
  }

  // The project; creating it opens its settings.
  await page.getByRole("link", { name: "Muasal" }).click();
  await page.getByRole("link", { name: "Proyek baru" }).click();
  await page.getByLabel("Kunci").fill(key);
  await page.getByLabel("Nama").fill(`HRIS ${run}`);
  await page.getByRole("button", { name: "Buat proyek" }).click();
  await expect(page).toHaveURL(new RegExp(`/p/${key}/settings$`));

  // Link both clients, then scope Budi to Client B.
  const links = page.getByRole("form", { name: "Klien proyek" });
  await links.getByRole("checkbox", { name: clientA }).check();
  await links.getByRole("checkbox", { name: clientB }).check();
  await links.getByRole("button", { name: "Simpan klien" }).click();
  await expect(links.getByRole("status")).toHaveText("Tersimpan");

  await page.getByRole("form", { name: "Tambah anggota" }).getByLabel("Email").fill(budiEmail);
  await page.getByRole("button", { name: "Tambah anggota" }).click();
  const budiRow = page.getByRole("row", { name: budiEmail });
  await budiRow.getByLabel("Cakupan klien").selectOption("some");
  await budiRow.getByRole("checkbox", { name: clientB }).check();
  await page.getByRole("button", { name: "Simpan anggota" }).click();
  await expect(page.getByRole("region", { name: "Anggota" }).getByRole("status")).toHaveText("Tersimpan");

  // The tree: HR › Attendance › Overtime Approval (Client A only) and Leave Request (shared).
  await page.getByRole("link", { name: "Modul" }).click();
  const addNode = async (name: string, options: { type?: "module"; clients?: string[] } = {}) => {
    const form = page.getByRole("form", { name: "Modul atau menu baru" });
    await form.getByLabel("Nama").fill(name);
    if (options.type) await form.getByLabel("Jenis").selectOption(options.type);
    if (options.clients) {
      await form.getByRole("radio", { name: "Khusus klien" }).check();
      for (const c of options.clients) await form.getByRole("checkbox", { name: c }).check();
    }
    await form.getByRole("button", { name: "Simpan" }).click();
    await expect(page.getByRole("button", { name, exact: true })).toBeVisible();
  };
  await page.getByRole("button", { name: "Tambah modul" }).click();
  await addNode("HR");
  await page.getByRole("button", { name: "Tambah anak" }).click();
  await addNode("Attendance", { type: "module" });
  await page.getByRole("button", { name: "Tambah anak" }).click();
  await addNode("Overtime Approval", { clients: [clientA] });
  await page.getByRole("button", { name: "Attendance", exact: true }).click();
  await page.getByRole("button", { name: "Tambah anak" }).click();
  await addNode("Leave Request");
  await expect(page.getByText(clientA, { exact: true })).toBeVisible(); // the badge on Overtime Approval

  // Budi, scoped to Client B, sees the shared menu but not Client A's.
  const budi = await (await browser.newContext()).newPage();
  await setPassword(budi, budiLink!, budiPassword);
  await signIn(budi, budiEmail, budiPassword);
  await budi.getByRole("link", { name: key }).click();
  await expect(budi.getByRole("button", { name: "Leave Request", exact: true })).toBeVisible();
  await expect(budi.getByRole("button", { name: "Attendance", exact: true })).toBeVisible();
  await expect(budi.getByText("Overtime Approval")).toHaveCount(0);
});
```

- [ ] **Step 3: Run it on a rebuilt stack**

```bash
make up
make e2e
```

On this Mac the stack answers on port 8080, so run `cd web && E2E_BASE_URL=http://localhost:8080 npx playwright test` instead of `make e2e`.

Expected: `2 passed`.

- [ ] **Step 4: Run every check CI runs**

```bash
cd server && go generate ./... && git diff --exit-code && go vet ./... && TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable' go test ./...
cd ../web && npm run gen:api && git diff --exit-code && npm run build
```

Expected: no diff, no vet findings, `ok` for every Go package, and a successful web build.

- [ ] **Step 5: Record the new rules in the FSD**

Use the Claude Docs connector, not a file edit:
- §5.2: add R-AC-11, "Project admins always see all clients; the database enforces it."
- §16: note the composite foreign keys of `membership_clients` and `node_clients` to `project_clients`, and `node_clients.project_id`.
- §21: mark Iteration 1 done, with this plan's deviations.

- [ ] **Step 6: Commit**

```bash
git add web/e2e
git commit -m "test(e2e): Iteration 1 exit check: tree built, member scoped by client"
```

## Exit check (FSD §21, Iteration 1)

- `make test` passes, including the permission suite (`TestPermissionSuiteReads`, `TestPermissionSuiteWrites`).
- On a stack rebuilt with `make up`, `make e2e` passes. In the browser:
  1. A system admin creates two clients and project HRIS.
  2. The admin links the clients and scopes a member to Client B.
  3. The admin builds HR › Attendance › Overtime Approval (Client A only) and Leave Request.
  4. The member sees Leave Request but not Overtime Approval.
- CI is green on `server`, `web` and `e2e`.
