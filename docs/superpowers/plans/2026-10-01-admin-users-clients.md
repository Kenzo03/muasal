# Admin Users and Clients Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild `/admin/users` and `/admin/clients` around search, status filters, a ⋯ row menu, one side panel for create and edit, styled confirmations, alias chips, and the projects that use each client.

**Architecture:**
- **Server:** one change that only adds a field. `ListClients` returns each client's project keys in a new optional `Client.projects`. No other endpoint changes.
- **Web:** the screens stay client components, fed by the existing server pages.
- **Logic:** search and alias rules are pure functions in `web/lib/admin.ts`, tested with `node --test`.
- **New shared UI:** `SidePanel`, `ConfirmDialog` and `FilterTabs` in `web/components/`.
- **Row menu:** reuses the existing `components/Menu.tsx`.

**Tech Stack:** Go 1.27 with sqlc and oapi-codegen; PostgreSQL 18; Next.js 16 with next-intl and Tailwind v4; Playwright 1.63.

**Spec:** `docs/superpowers/specs/2026-10-01-admin-users-clients-design.md`. The design canvas is "Muasal UI refresh", artboards Admin*.

## Global Constraints

- **Branch:** `feat/admin-users-clients`. Commit after each task, with authorship as configured (`user <user@mail.com>`). End each commit message with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Don't push.
- **Translations:** `web/messages/id.json` and `en.json` must match key for key, including placeholders (`npm run check:i18n`). Indonesian follows the app's existing words: "kata sandi" (not "sandi") and "tautan pengaturan".
- **Styling:** use the class lists in `web/lib/ui.ts` (`button.*`, `field.*`, `table.*`, `chip`) and the theme tokens (`bg-accent`, `text-muted`, `border-line` and so on). Raw hex values are allowed only for colours the canvas uses that have no token yet: the segmented ground `#F0EAE3`, the count chip `#EFE9E2`, and the avatar `#F6D9CC`/`#8A3417`, as `Sidebar.tsx` and `Chips.tsx` already do.
- **API:** `Client.projects` is optional and only `GET /clients` fills it. `/projects/{key}/clients` and the create and update responses must not change.
- **Aliases:** at most 20 per client (the API's `maxItems`). Duplicates are refused case-insensitively.
- **e2e:** keep `data-testid="setup-link"` on the setup-link banner.
- **This Windows host has no Go and no Docker.** Every command below runs in WSL Ubuntu, where Docker Engine runs the stack. The examples set two shell variables to save repetition:

```bash
# Run a command in WSL from Git Bash on Windows.
W='MSYS_NO_PATHCONV=1 wsl -d Ubuntu -u root -- bash -lc'
# Go, in a container sharing the repo and a module cache.
GO='docker run --rm --network host -v /mnt/d/Project/muasal:/src -v muasal-gomod:/go/pkg/mod -v muasal-gocache:/root/.cache/go-build -w /src/server -e TEST_DATABASE_URL=postgres://owner:owner@localhost:55432/postgres?sslmode=disable golang:1.27'
# Node and Playwright, in a container whose node_modules stays in a volume (Linux binaries) so it never mixes with Windows.
WEB='docker run --rm --network host -v /mnt/d/Project/muasal:/src -v muasal-web-nm:/src/web/node_modules -v /var/run/docker.sock:/var/run/docker.sock -v /usr/bin/docker:/usr/bin/docker -v /usr/libexec/docker/cli-plugins:/usr/libexec/docker/cli-plugins -w /src/web -e E2E_BASE_URL=http://localhost:8080 mcr.microsoft.com/playwright:v1.63.0-noble'
# The stack, with the route to Ollama.
UP='cd /mnt/d/Project/muasal && docker compose -f deploy/compose.yaml -f deploy/compose.host-ai.yaml --env-file deploy/.env up -d --build --wait'
```

Run a Go command as `eval "$W \"$GO go test ./internal/httpapi/ -run TestX\""`, or paste the expanded command. The first web command in a session needs `npm ci` (`$WEB npm ci`). The test database is started once with `$W 'cd /mnt/d/Project/muasal && make testdb'`.

---

## Tasks

### Task 1: Clients list names their projects

**Files:**
- Modify: `server/internal/db/queries/clients.sql` (the `ListClients` query)
- Modify: `api/openapi.yaml` (the `Client` schema, around line 2030)
- Modify: `server/internal/httpapi/clients.go:35-45` (`ListClients`)
- Regenerate: `server/internal/db/clients.sql.go`, `server/internal/httpapi/api.gen.go`, `web/lib/api-types.ts`
- Test: `server/internal/httpapi/clients_test.go`

**Interfaces:**
- Produces: `httpapi.Client.Projects *[]string` (JSON `projects`, omitted when nil). TypeScript: `Client.projects?: string[]`.
- Produces: `db.ListClientsRow{Client db.Client; Projects []string}` from `q.ListClients(ctx)`.

- [ ] **Step 1: Write the failing test.** Append to `server/internal/httpapi/clients_test.go`:

```go
// The admin list names the projects using each client, in key order; an
// unlinked client gets an empty list, never null.
func TestClientListNamesTheirProjects(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	e.seedProject("PAY", a)
	e.seedProject("HRIS", a)
	admin, _ := e.signedIn("admin@example.com", true)
	var list httpapi.ClientList
	if code := e.call(admin, http.MethodGet, "/clients", nil, &list); code != http.StatusOK || len(list.Items) != 2 {
		t.Fatalf("list: %d %+v", code, list)
	}
	byName := map[string]httpapi.Client{}
	for _, c := range list.Items {
		byName[c.Name] = c
	}
	if got := byName[a.Name].Projects; got == nil || !slices.Equal(*got, []string{"HRIS", "PAY"}) {
		t.Fatalf("Client A projects: %v", got)
	}
	if got := byName[b.Name].Projects; got == nil || len(*got) != 0 {
		t.Fatalf("Client B projects: %v", got)
	}
	var linked httpapi.ClientList
	if code := e.call(admin, http.MethodGet, "/projects/HRIS/clients", nil, &linked); code != http.StatusOK || linked.Items[0].Projects != nil {
		t.Fatalf("project clients must not carry projects: %d %+v", code, linked)
	}
}
```

- [ ] **Step 2: Confirm it fails to compile.** `Client` has no `Projects` field yet.

Run: `$W "$GO go test ./internal/httpapi/ -run TestClientListNamesTheirProjects"`
Expected: FAIL with `byName[a.Name].Projects undefined`.

- [ ] **Step 3: Change the query.** In `server/internal/db/queries/clients.sql`, replace the `ListClients` query:

```sql
-- name: ListClients :many
-- The admin list (GET /clients): each client with the keys of the projects
-- linked to it, in key order; none gives an empty array.
SELECT sqlc.embed(c),
       coalesce(array_agg(p.key ORDER BY p.key) FILTER (WHERE p.key IS NOT NULL), '{}')::text[] AS projects
FROM clients c
LEFT JOIN project_clients pc ON pc.client_id = c.id
LEFT JOIN projects p ON p.id = pc.project_id
GROUP BY c.id
ORDER BY lower(c.name), c.id;
```

- [ ] **Step 4: Add the field to the API.** In `api/openapi.yaml`, under `components.schemas.Client.properties`, after `archived`:

```yaml
        projects:
          type: array
          items: { type: string }
          description: Keys of the projects linked to this client. Only GET /clients fills it.
```

- [ ] **Step 5: Regenerate.**

Run: `$W "$GO go generate ./..."`, then `$W "$WEB npm ci"` and `$W "$WEB npm run gen:api"`.
Expected: `db.ListClientsRow` appears in `clients.sql.go`, and `Projects *[]string` appears in `api.gen.go`'s `Client`. `web/lib/api-types.ts` gains `projects?: string[]`.

- [ ] **Step 6: Use the rows in the handler.** In `server/internal/httpapi/clients.go`, `ListClients`:

```go
	rows, err := s.q.ListClients(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]Client, len(rows))
	for i, row := range rows {
		items[i] = toAPIClient(row.Client)
		items[i].Projects = &row.Projects
	}
	writeJSON(w, http.StatusOK, ClientList{Items: items})
```

`toAPIClients` keeps serving the other lists. Leave it as it is.

- [ ] **Step 7: Run the client tests and vet.**

Run: `$W "$GO sh -c 'go vet ./... && go test ./internal/httpapi/ -run Client'"`
Expected: PASS, including `TestClientListNamesTheirProjects`.

- [ ] **Step 8: Commit.**

```bash
git add api/openapi.yaml server/internal/db/queries/clients.sql server/internal/db/clients.sql.go server/internal/httpapi/api.gen.go server/internal/httpapi/clients.go server/internal/httpapi/clients_test.go web/lib/api-types.ts
git commit -m "feat(clients): the admin list names each client's projects"
```

---

### Task 2: Search and alias rules

**Files:**
- Create: `web/lib/admin.ts`
- Test: `web/lib/admin.test.ts`

**Interfaces:**
- Produces:
  - `type UserStatus = "active" | "invited" | "disabled"`
  - `userStatus(u: { disabled: boolean; has_password: boolean }): UserStatus`
  - `matches(query: string, ...fields: (string | null | undefined)[]): boolean`
  - `addAlias(list: string[], raw: string, max?: number): string[]`
  - `const maxAliases = 20`

- [ ] **Step 1: Write the failing tests.** Create `web/lib/admin.test.ts`:

```ts
import assert from "node:assert/strict";
import test from "node:test";
import { addAlias, matches, maxAliases, userStatus } from "./admin.ts";

test("a user is invited until they set a password, and disabled wins", () => {
  assert.equal(userStatus({ disabled: false, has_password: false }), "invited");
  assert.equal(userStatus({ disabled: false, has_password: true }), "active");
  assert.equal(userStatus({ disabled: true, has_password: false }), "disabled");
});

test("search ignores case, accents and surrounding space, and any field may match", () => {
  assert.ok(matches("", "Budi"));
  assert.ok(matches("  budi ", "Budi Santoso", "budi@example.com"));
  assert.ok(matches("jose", "José"));
  assert.ok(matches("sj group", "Sinar Jaya", "SJ", "SJ Group"));
  assert.ok(!matches("arunika", "Sinar Jaya", null, undefined));
});

test("an alias is trimmed, loses a trailing comma, and duplicates or blanks change nothing", () => {
  assert.deepEqual(addAlias([], "  SJ Group, "), ["SJ Group"]);
  assert.deepEqual(addAlias(["SJ Group"], "sj group"), ["SJ Group"]);
  assert.deepEqual(addAlias(["SJ Group"], " , "), ["SJ Group"]);
});

test("aliases stop at the API's limit", () => {
  const full = Array.from({ length: maxAliases }, (_, i) => `A${i}`);
  assert.equal(addAlias(full, "one more"), full);
});
```

- [ ] **Step 2: Confirm the tests fail.**

Run: `$W "$WEB node --test lib/admin.test.ts"`
Expected: FAIL with `Cannot find module` for `./admin.ts`.

- [ ] **Step 3: Implement the functions.** Create `web/lib/admin.ts`:

```ts
// Rules shared by the admin Users and Clients screens: a user's status, search
// over a few fields, and adding a client alias.

export type UserStatus = "active" | "invited" | "disabled";

// MSL-20: a user without a password yet is invited, not active.
export function userStatus(u: { disabled: boolean; has_password: boolean }): UserStatus {
  return u.disabled ? "disabled" : u.has_password ? "active" : "invited";
}

const fold = (s: string) => s.normalize("NFKD").replace(/\p{M}/gu, "").toLocaleLowerCase("id").trim();

// True when the query is empty or any field holds it.
export function matches(query: string, ...fields: (string | null | undefined)[]): boolean {
  const q = fold(query);
  return q === "" || fields.some((f) => f != null && fold(f).includes(q));
}

// The API takes at most 20 aliases per client (ClientUpdate.aliases.maxItems).
export const maxAliases = 20;

// Adds one alias typed in the chip input. It returns the same array when there
// is nothing to add, so callers can skip a state update.
export function addAlias(list: string[], raw: string, max = maxAliases): string[] {
  const alias = raw.replace(/,+\s*$/, "").trim();
  if (alias === "" || list.length >= max || list.some((a) => fold(a) === fold(alias))) return list;
  return [...list, alias];
}
```

- [ ] **Step 4: Run the tests.**

Run: `$W "$WEB node --test lib/admin.test.ts"`
Expected: PASS, 4 tests.

- [ ] **Step 5: Commit.**

```bash
git add web/lib/admin.ts web/lib/admin.test.ts
git commit -m "feat(admin): search and alias rules for the admin screens"
```

---

### Task 3: Shared panel, confirmation, filter tabs and ⋯ icon

**Files:**
- Create: `web/components/SidePanel.tsx`, `web/components/ConfirmDialog.tsx`, `web/components/FilterTabs.tsx`
- Modify: `web/components/Icon.tsx` (add `more` to `paths`)

**Interfaces:**
- Produces:
  - `SidePanel({ title, labelledBy, closeLabel, onClose, footer, children })`. `title` is a ReactNode that holds an element with `id={labelledBy}`. `footer` is a ReactNode of buttons; a submit button there uses `form="panel-form"`.
  - `ConfirmDialog({ title, body, action, cancelLabel, onConfirm, onCancel })`. `onConfirm` is `() => Promise<string | undefined>` and returns error text to show, or `undefined` once it succeeds.
  - `FilterTabs<K extends string>({ label, value, options, onChange })`, with `options: { key: K; label: string; count: number }[]`.
  - `Icon name="more"`.

- [ ] **Step 1: Add the ⋯ icon.** In `web/components/Icon.tsx`, inside `paths`, next to `menu`:

```tsx
  more: <path d="M3.4 8h.8M7.6 8h.8M11.8 8h.8" strokeWidth={2.4} />,
```

- [ ] **Step 2: Create `web/components/SidePanel.tsx`.**

```tsx
"use client";

import { useEffect, useRef } from "react";
import Icon from "./Icon";

type Props = {
  title: React.ReactNode;
  labelledBy: string;
  closeLabel: string;
  onClose: () => void;
  footer: React.ReactNode;
  children: React.ReactNode;
};

// A form that slides in from the right over a list (the admin screens' create
// and edit). It is a modal <dialog>, like CloseDialog: Escape closes it, focus
// stays inside, and on close focus goes back to whatever opened it.
export default function SidePanel({ title, labelledBy, closeLabel, onClose, footer, children }: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    if (!ref.current?.open) ref.current?.showModal();
    return () => opener?.focus();
  }, []);
  return (
    <dialog
      ref={ref}
      aria-labelledby={labelledBy}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
      className="fixed inset-y-0 right-0 left-auto m-0 h-dvh max-h-none w-full max-w-[460px] border-0 border-l border-line bg-white p-0 text-ink shadow-[-12px_0_40px_rgba(43,36,32,0.10)] backdrop:bg-ink/15"
    >
      <div className="flex h-full flex-col">
        <div className="flex items-center gap-3 border-b border-line-soft px-5 py-4">
          <div className="min-w-0 flex-1">{title}</div>
          <button type="button" onClick={onClose} aria-label={closeLabel} className="inline-flex size-9 items-center justify-center rounded-[9px] text-muted hover:bg-paper hover:text-ink">
            <Icon name="x" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-5">{children}</div>
        <div className="flex justify-end gap-2 border-t border-line-soft bg-paper px-5 py-3.5">{footer}</div>
      </div>
    </dialog>
  );
}
```

- [ ] **Step 3: Create `web/components/ConfirmDialog.tsx`.**

```tsx
"use client";

import { useEffect, useRef, useState } from "react";
import Icon from "./Icon";
import { button, field } from "@/lib/ui";

type Props = {
  title: string;
  body: string;
  action: string;
  cancelLabel: string;
  onConfirm: () => Promise<string | undefined>; // error text, or undefined when done
  onCancel: () => void;
};

// Asks before an action that can't be taken back quietly (reset password,
// disable, archive). The action stays open on failure and shows why.
export default function ConfirmDialog({ title, body, action, cancelLabel, onConfirm, onCancel }: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    if (!ref.current?.open) ref.current?.showModal();
    return () => opener?.focus();
  }, []);
  async function confirm() {
    setBusy(true);
    const problem = await onConfirm();
    setBusy(false);
    if (problem) setError(problem);
  }
  return (
    <dialog
      ref={ref}
      role="alertdialog"
      aria-labelledby="confirm-title"
      aria-describedby="confirm-body"
      onCancel={(e) => {
        e.preventDefault();
        onCancel();
      }}
      className="m-auto w-[min(420px,calc(100vw-2rem))] rounded-2xl bg-white p-0 text-ink shadow-[0_24px_64px_rgba(43,36,32,0.22)] backdrop:bg-ink/35"
    >
      <div className="flex flex-col gap-2.5 px-6 pb-1 pt-5">
        <span className="inline-flex size-10 items-center justify-center rounded-xl bg-danger-soft text-danger">
          <Icon name="warning" className="size-[18px]" />
        </span>
        <h2 id="confirm-title" className="text-[17px] font-extrabold">{title}</h2>
        <p id="confirm-body" className="text-sm leading-relaxed text-ink-soft">{body}</p>
        {error && <p role="alert" className={field.error}>{error}</p>}
      </div>
      <div className="flex justify-end gap-2 px-6 pb-5 pt-4">
        <button type="button" className={button.secondary} onClick={onCancel}>{cancelLabel}</button>
        <button type="button" className={button.danger} onClick={confirm} disabled={busy}>{action}</button>
      </div>
    </dialog>
  );
}
```

- [ ] **Step 4: Create `web/components/FilterTabs.tsx`.**

```tsx
import { cx } from "@/lib/ui";

type Option<K> = { key: K; label: string; count: number };

// Segmented status filters over a list, each with its count (the admin
// screens). Pressed buttons, not tabs: they filter one table, they don't switch panes.
export default function FilterTabs<K extends string>({ label, value, options, onChange }: { label: string; value: K; options: Option<K>[]; onChange: (key: K) => void }) {
  return (
    <div role="group" aria-label={label} className="flex gap-0.5 rounded-[11px] bg-[#F0EAE3] p-[3px]">
      {options.map((o) => (
        <button
          key={o.key}
          type="button"
          aria-pressed={o.key === value}
          onClick={() => onChange(o.key)}
          className={cx(
            "inline-flex h-[30px] items-center gap-1.5 rounded-lg px-3 text-[13px]",
            o.key === value ? "bg-white font-bold text-ink shadow-[0_1px_2px_rgba(43,36,32,0.1)]" : "font-semibold text-ink-soft hover:text-ink",
          )}
        >
          {o.label}
          <span className="text-xs font-bold text-muted">{o.count}</span>
        </button>
      ))}
    </div>
  );
}
```

`#F0EAE3` is the segmented control's ground from the canvas's Daftar/Papan toggle. If `globals.css` gains a token for it later, swap it then.

- [ ] **Step 5: Build to type-check.**

Run: `$W "$WEB npm run build"`
Expected: the build succeeds. The components aren't used yet, so type errors are the only possible failure.

- [ ] **Step 6: Commit.**

```bash
git add web/components/SidePanel.tsx web/components/ConfirmDialog.tsx web/components/FilterTabs.tsx web/components/Icon.tsx
git commit -m "feat(web): side panel, confirm dialog and filter tabs"
```

---

### Task 4: Users screen

**Files:**
- Rewrite: `web/app/admin/users/UsersAdmin.tsx`
- Modify: `web/app/admin/users/page.tsx` (the count next to the title)
- Modify: `web/messages/en.json` and `id.json` (`users.*`)
- Modify: `web/e2e/signin.spec.ts:16-18` and `web/e2e/registry.spec.ts:19-22`

**Interfaces:**
- Consumes: `userStatus` and `matches` (Task 2); `SidePanel`, `ConfirmDialog`, `FilterTabs` and `Icon "more"` (Task 3); `Menu` from `components/Menu.tsx`.
- Produces: the panel title "Pengguna baru" (dialog name), the fields labelled "Nama" and "Email", and the submit button "Buat pengguna". The e2e tests rely on these.

- [ ] **Step 1: Update the e2e tests first.** They fail until the screen exists.

In `web/e2e/signin.spec.ts`, replace lines 16–18:

```ts
  await page.getByRole("button", { name: "Pengguna baru" }).click();
  const panel = page.getByRole("dialog", { name: "Pengguna baru" });
  await panel.getByLabel("Nama", { exact: true }).fill("Budi");
  await panel.getByLabel("Email", { exact: true }).fill(email);
  await panel.getByRole("button", { name: "Buat pengguna" }).click();
```

In `web/e2e/registry.spec.ts`, replace lines 20–22 the same way, with `"Budi Tree"` and `budiEmail`.

- [ ] **Step 2: Update the messages.** In `web/messages/en.json`, set `users` to the following. Keep the existing keys that are still used and remove `back`, which is unused:

```json
"users": {
  "title": "Users", "name": "Name", "email": "Email", "admin": "Admin", "status": "Status",
  "lastLogin": "Last sign-in", "lastLoginAt": "Last sign-in {when}", "never": "Never",
  "active": "Active", "disabled": "Disabled", "invited": "Invited", "all": "All",
  "newUser": "New user", "create": "Create user", "save": "Save", "cancel": "Cancel", "close": "Close panel",
  "search": "Name or email", "statusFilter": "Status", "noMatch": "No users match.",
  "you": "(you)", "actionsFor": "Actions for {name}", "edit": "Edit",
  "systemAdmin": "System admin", "systemAdminHelp": "Manages users, clients, AI, backups and the audit log.",
  "emailFixed": "Email can't be changed. Create a new user if the address changes.",
  "newHelp": "Once the user is created, you get a one-time setup link to send them.",
  "access": "Access",
  "resetPassword": "Reset password", "resetHelp": "The old password stops working and every session ends.",
  "newLink": "New setup link", "newLinkHelp": "The old setup link stops working.",
  "disable": "Disable", "disableHelp": "They can't sign in. Their tickets and history stay.",
  "enable": "Enable", "enableHelp": "They can sign in again.",
  "confirmReset": "Reset {name}'s password?",
  "confirmResetBody": "The old password stops working at once and every session ends. You get a new setup link to send them.",
  "confirmLink": "Make a new setup link for {name}?", "confirmLinkBody": "The old setup link stops working.",
  "confirmDisable": "Disable {name}?",
  "confirmDisableBody": "They're signed out and can't sign in until enabled again. Their tickets and history stay.",
  "linkFor": "Setup link for {name} (valid 72 hours):", "copy": "Copy", "copied": "Copied", "dismiss": "Dismiss",
  "adminsOnly": "Only admins can manage users."
}
```

In `web/messages/id.json`:

```json
"users": {
  "title": "Pengguna", "name": "Nama", "email": "Email", "admin": "Admin", "status": "Status",
  "lastLogin": "Terakhir masuk", "lastLoginAt": "Terakhir masuk {when}", "never": "Belum pernah",
  "active": "Aktif", "disabled": "Nonaktif", "invited": "Diundang", "all": "Semua",
  "newUser": "Pengguna baru", "create": "Buat pengguna", "save": "Simpan", "cancel": "Batal", "close": "Tutup panel",
  "search": "Nama atau email", "statusFilter": "Status", "noMatch": "Tidak ada pengguna yang cocok.",
  "you": "(Anda)", "actionsFor": "Tindakan untuk {name}", "edit": "Ubah",
  "systemAdmin": "Admin sistem", "systemAdminHelp": "Mengelola pengguna, klien, AI, cadangan, dan log audit.",
  "emailFixed": "Email tidak bisa diubah. Buat pengguna baru bila alamatnya berganti.",
  "newHelp": "Setelah pengguna dibuat, Anda mendapat tautan pengaturan sekali pakai untuk dikirim kepadanya.",
  "access": "Akses",
  "resetPassword": "Reset kata sandi", "resetHelp": "Kata sandi lama berhenti berlaku dan semua sesinya berakhir.",
  "newLink": "Tautan pengaturan baru", "newLinkHelp": "Tautan pengaturan lama berhenti berlaku.",
  "disable": "Nonaktifkan", "disableHelp": "Ia tidak bisa masuk. Tiket dan riwayatnya tetap ada.",
  "enable": "Aktifkan", "enableHelp": "Ia bisa masuk lagi.",
  "confirmReset": "Reset kata sandi {name}?",
  "confirmResetBody": "Kata sandi lamanya langsung tidak berlaku dan semua sesinya berakhir. Anda mendapat tautan pengaturan baru untuk dikirim kepadanya.",
  "confirmLink": "Buat tautan pengaturan baru untuk {name}?", "confirmLinkBody": "Tautan pengaturan lama berhenti berlaku.",
  "confirmDisable": "Nonaktifkan {name}?",
  "confirmDisableBody": "Ia langsung keluar dan tidak bisa masuk sampai diaktifkan lagi. Tiket dan riwayatnya tetap ada.",
  "linkFor": "Tautan pengaturan untuk {name} (berlaku 72 jam):", "copy": "Salin", "copied": "Tersalin", "dismiss": "Tutup",
  "adminsOnly": "Hanya admin yang dapat mengelola pengguna."
}
```

Keep the files' existing formatting: one key per line, two-space indent.

- [ ] **Step 3: Show the count next to the title.** In `web/app/admin/users/page.tsx`, inside `<PageBar>` after the `h1`:

```tsx
        {me.is_admin && <span className="inline-flex h-6 items-center rounded-full bg-[#EFE9E2] px-2.5 text-[12.5px] font-extrabold text-ink-soft">{data?.items.length ?? 0}</span>}
```

- [ ] **Step 4: Rewrite `web/app/admin/users/UsersAdmin.tsx`.**

```tsx
"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTimeZone, useTranslations } from "next-intl";
import { Avatar } from "@/components/Chips";
import ConfirmDialog from "@/components/ConfirmDialog";
import FilterTabs from "@/components/FilterTabs";
import Icon from "@/components/Icon";
import Menu from "@/components/Menu";
import SidePanel from "@/components/SidePanel";
import { matches, userStatus, type UserStatus } from "@/lib/admin";
import { api } from "@/lib/api";
import { dateTime } from "@/lib/format";
import { useProblemText, type Problem, type User } from "@/lib/problem";
import { button, chip, cx, field, table } from "@/lib/ui";

type Filter = "all" | UserStatus;
type Confirm = { kind: "reset" | "link" | "disable"; user: User };

const statusTone: Record<UserStatus, string> = {
  active: "bg-ok-soft text-ok",
  invited: "bg-warn-soft text-warn",
  disabled: "bg-well text-muted",
};
const menuItem = "flex w-full items-center gap-2.5 px-3.5 py-2 text-left text-sm text-ink hover:bg-paper";

export default function UsersAdmin({ users, meId }: { users: User[]; meId: number }) {
  const t = useTranslations("users");
  const problemText = useProblemText();
  const locale = useLocale();
  const timeZone = useTimeZone();
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [panel, setPanel] = useState<User | "new" | null>(null);
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const [error, setError] = useState("");
  // MSL-20: every link made here stays listed until dismissed, newest first.
  const [links, setLinks] = useState<{ name: string; url: string }[]>([]);
  const [copied, setCopied] = useState("");
  const addLink = (name: string, url: string) => setLinks((ls) => [{ name, url }, ...ls.filter((l) => l.name !== name)]);

  const counts = useMemo(() => {
    const c = { all: users.length, active: 0, invited: 0, disabled: 0 };
    for (const u of users) c[userStatus(u)]++;
    return c;
  }, [users]);
  const shown = users.filter((u) => (filter === "all" || userStatus(u) === filter) && matches(query, u.name, u.email));
  const lastLogin = (u: User) => (u.last_login_at ? dateTime(u.last_login_at, locale, timeZone) : t("never"));

  async function setDisabled(u: User, disabled: boolean): Promise<string | undefined> {
    const { error } = await api.PATCH("/admin/users/{id}", { params: { path: { id: u.id } }, body: { disabled } });
    if (error) return problemText(error);
    router.refresh();
  }
  async function newLink(u: User): Promise<string | undefined> {
    const { data, error } = await api.POST("/admin/users/{id}/setup-link", { params: { path: { id: u.id } } });
    if (error) return problemText(error);
    addLink(u.name, data.url);
    router.refresh();
  }
  async function runConfirm(c: Confirm) {
    const problem = c.kind === "disable" ? await setDisabled(c.user, true) : await newLink(c.user);
    if (!problem) {
      setConfirm(null);
      setPanel(null);
    }
    return problem;
  }
  // Enable needs no confirmation; a failure shows in the page's alert line.
  async function enable(u: User) {
    const problem = await setDisabled(u, false);
    setError(problem ?? "");
  }
  const ask = (u: User, kind: Confirm["kind"]) => setConfirm({ user: u, kind });
  const askLink = (u: User) => ask(u, u.has_password ? "reset" : "link");

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex h-9 w-full items-center gap-2 rounded-[10px] border border-line bg-white px-3 text-muted sm:w-64">
          <Icon name="search" className="size-[15px]" />
          <input type="search" aria-label={t("search")} placeholder={t("search")} value={query} onChange={(e) => setQuery(e.target.value)} className="min-w-0 flex-1 bg-transparent text-[13.5px] text-ink outline-none" />
        </label>
        <FilterTabs
          label={t("statusFilter")}
          value={filter}
          onChange={setFilter}
          options={(["all", "active", "invited", "disabled"] as const).map((k) => ({ key: k, label: t(k), count: counts[k] }))}
        />
        <button type="button" className={cx(button.primary, "ml-auto")} onClick={() => setPanel("new")}>
          <Icon name="plus" />
          {t("newUser")}
        </button>
      </div>
      {error && <p role="alert" className={field.error}>{error}</p>}
      {links.map((link) => (
        <p key={link.url} role="status" className="flex flex-wrap items-center gap-x-2 gap-y-1 rounded-xl border border-warn-line bg-warn-soft px-3.5 py-2.5 text-[13.5px] text-warn">
          {t("linkFor", { name: link.name })}
          <code data-testid="setup-link" className="min-w-0 flex-1 break-all font-mono text-ink">{link.url}</code>
          <button type="button" className={button.quiet} onClick={() => navigator.clipboard?.writeText(link.url).then(() => setCopied(link.url), () => {})}>
            {copied === link.url ? t("copied") : t("copy")}
          </button>
          <button type="button" aria-label={t("dismiss")} className={button.quiet} onClick={() => setLinks((ls) => ls.filter((l) => l.url !== link.url))}>
            <Icon name="x" className="size-3.5" />
          </button>
        </p>
      ))}
      {/* md:overflow-visible: the row menu would be clipped by the wrapper's scroll box; phones keep the sideways scroll. */}
      <div className={cx(table.wrap, "md:overflow-visible")}>
        <table className={table.table}>
          <thead className={table.head}>
            <tr>
              <th className={table.th}>{t("name")}</th>
              <th className={table.th}>{t("email")}</th>
              <th className={table.th}>{t("status")}</th>
              <th className={table.th}>{t("lastLogin")}</th>
              <th className={cx(table.th, "w-14")}><span className="sr-only">{t("actionsFor", { name: "" })}</span></th>
            </tr>
          </thead>
          <tbody>
            {shown.map((u, i) => {
              const status = userStatus(u);
              const me = u.id === meId;
              return (
                <tr key={u.id} className={cx(table.row, panel !== "new" && panel?.id === u.id && "bg-accent-soft/40")}>
                  <td className={table.td}>
                    <button type="button" onClick={() => setPanel(u)} className="flex max-w-full items-center gap-2.5 text-left font-bold text-ink hover:text-accent-strong">
                      <Avatar name={u.name} className="size-7 bg-[#F6D9CC] text-[#8A3417] text-[11px]" />
                      <span className="truncate">{u.name}</span>
                      {u.is_admin && <span className={cx(chip, "bg-accent-soft text-accent-strong")}>{t("admin")}</span>}
                      {me && <span className="font-medium text-muted">{t("you")}</span>}
                    </button>
                  </td>
                  <td className={cx(table.td, "text-ink-soft")}>{u.email}</td>
                  <td className={table.td}><span className={cx(chip, statusTone[status])}>{t(status)}</span></td>
                  <td className={cx(table.td, "whitespace-nowrap text-muted")}>{lastLogin(u)}</td>
                  <td className={cx(table.td, "py-1.5 text-right")}>
                    {!me && (
                      <Menu
                        label={t("actionsFor", { name: u.name })}
                        align="right"
                        side={i >= shown.length - 2 && shown.length > 3 ? "up" : "down"}
                        summary={<Icon name="more" />}
                        summaryClassName="inline-flex size-8 items-center justify-center rounded-[9px] text-ink-soft hover:bg-paper"
                        panelClassName="min-w-56"
                      >
                        <button type="button" className={menuItem} onClick={() => setPanel(u)}><Icon name="edit" className="size-[15px] text-muted" />{t("edit")}</button>
                        <button type="button" className={menuItem} onClick={() => askLink(u)}><Icon name="lock" className="size-[15px] text-muted" />{u.has_password ? t("resetPassword") : t("newLink")}</button>
                        <hr className="my-1 border-line-soft" />
                        {u.disabled ? (
                          <button type="button" className={menuItem} onClick={() => enable(u)}><Icon name="check" className="size-[15px] text-muted" />{t("enable")}</button>
                        ) : (
                          <button type="button" className={cx(menuItem, "text-danger")} onClick={() => ask(u, "disable")}><Icon name="xCircle" className="size-[15px]" />{t("disable")}</button>
                        )}
                      </Menu>
                    )}
                  </td>
                </tr>
              );
            })}
            {shown.length === 0 && (
              <tr><td colSpan={5} className="px-3.5 py-8 text-center text-muted">{t("noMatch")}</td></tr>
            )}
          </tbody>
        </table>
      </div>
      {panel && (
        <UserPanel
          key={panel === "new" ? "new" : panel.id}
          user={panel === "new" ? null : panel}
          isMe={panel !== "new" && panel.id === meId}
          lastLogin={panel === "new" ? "" : lastLogin(panel)}
          onClose={() => setPanel(null)}
          onCreated={(name, url) => {
            addLink(name, url);
            setPanel(null);
            router.refresh();
          }}
          onSaved={() => {
            setPanel(null);
            router.refresh();
          }}
          onLink={askLink}
          onDisable={(u) => ask(u, "disable")}
          onEnable={enable}
        />
      )}
      {confirm && (
        <ConfirmDialog
          title={t(confirm.kind === "reset" ? "confirmReset" : confirm.kind === "link" ? "confirmLink" : "confirmDisable", { name: confirm.user.name })}
          body={t(confirm.kind === "reset" ? "confirmResetBody" : confirm.kind === "link" ? "confirmLinkBody" : "confirmDisableBody")}
          action={t(confirm.kind === "reset" ? "resetPassword" : confirm.kind === "link" ? "newLink" : "disable")}
          cancelLabel={t("cancel")}
          onConfirm={() => runConfirm(confirm)}
          onCancel={() => setConfirm(null)}
        />
      )}
    </div>
  );
}

type PanelProps = {
  user: User | null; // null: a new user
  isMe: boolean;
  lastLogin: string;
  onClose: () => void;
  onCreated: (name: string, url: string) => void;
  onSaved: () => void;
  onLink: (u: User) => void;
  onDisable: (u: User) => void;
  onEnable: (u: User) => void;
};

function UserPanel({ user, isMe, lastLogin, onClose, onCreated, onSaved, onLink, onDisable, onEnable }: PanelProps) {
  const t = useTranslations("users");
  const problemText = useProblemText();
  const [name, setName] = useState(user?.name ?? "");
  const [email, setEmail] = useState("");
  const [admin, setAdmin] = useState(user?.is_admin ?? false);
  const [problem, setProblem] = useState<Problem>();
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    if (!user) {
      const { data, error } = await api.POST("/admin/users", { body: { name, email, is_admin: admin } });
      setBusy(false);
      if (error) return setProblem(error);
      return onCreated(data.user.name, data.setup_link.url);
    }
    // Only the fields that changed, so a save never undoes someone else's edit.
    const body: { name?: string; is_admin?: boolean } = {};
    if (name !== user.name) body.name = name;
    if (admin !== user.is_admin) body.is_admin = admin;
    if (Object.keys(body).length === 0) return onClose();
    const { error } = await api.PATCH("/admin/users/{id}", { params: { path: { id: user.id } }, body });
    setBusy(false);
    if (error) return setProblem(error);
    onSaved();
  }

  const status = user ? userStatus(user) : null;
  return (
    <SidePanel
      labelledBy="user-panel-title"
      closeLabel={t("close")}
      onClose={onClose}
      title={
        user ? (
          <div className="flex items-center gap-3">
            <Avatar name={user.name} className="size-10 bg-[#F6D9CC] text-[#8A3417] text-sm" />
            <div className="flex min-w-0 flex-col gap-0.5">
              <h2 id="user-panel-title" className="truncate text-lg font-extrabold tracking-[-0.01em]">{user.name}</h2>
              <span className="flex items-center gap-2 text-[12.5px] text-muted">
                {status && <span className={cx(chip, statusTone[status])}>{t(status)}</span>}
                {t("lastLoginAt", { when: lastLogin })}
              </span>
            </div>
          </div>
        ) : (
          <h2 id="user-panel-title" className="text-lg font-extrabold tracking-[-0.01em]">{t("newUser")}</h2>
        )
      }
      footer={
        <>
          <button type="button" className={button.secondary} onClick={onClose}>{t("cancel")}</button>
          <button type="submit" form="panel-form" className={button.primary} disabled={busy}>{user ? t("save") : t("create")}</button>
        </>
      }
    >
      <form id="panel-form" onSubmit={submit} className="flex flex-col gap-4">
        <label className={field.label}>
          {t("name")}
          <input value={name} onChange={(e) => setName(e.target.value)} required maxLength={200} className={field.input} />
        </label>
        <label className={field.label}>
          {t("email")}
          {user ? (
            <input value={user.email} readOnly aria-describedby="email-help" className={cx(field.input, "bg-well text-ink-soft")} />
          ) : (
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required aria-describedby="email-help" className={field.input} />
          )}
          <span id="email-help" className={cx(field.hint, "font-normal")}>{user ? t("emailFixed") : t("newHelp")}</span>
        </label>
        <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-line bg-paper px-3.5 py-3">
          <input type="checkbox" checked={admin} disabled={isMe} onChange={(e) => setAdmin(e.target.checked)} className="mt-0.5 size-4 accent-accent" />
          <span className="flex flex-col gap-0.5">
            <span className="text-sm font-bold">{t("systemAdmin")}</span>
            <span className="text-[12.5px] text-muted">{t("systemAdminHelp")}</span>
          </span>
        </label>
        {problem && <p role="alert" className={field.error}>{problemText(problem)}</p>}
        {user && !isMe && (
          <section aria-labelledby="access-title" className="mt-2 flex flex-col gap-3 border-t border-line-soft pt-4">
            <h3 id="access-title" className="text-[13px] font-bold text-muted">{t("access")}</h3>
            <div className="flex items-center gap-3">
              <span className="flex flex-1 flex-col gap-0.5">
                <span className="text-[13.5px] font-bold">{user.has_password ? t("resetPassword") : t("newLink")}</span>
                <span className="text-[12.5px] text-muted">{user.has_password ? t("resetHelp") : t("newLinkHelp")}</span>
              </span>
              <button type="button" className={button.secondary} onClick={() => onLink(user)}>{user.has_password ? t("resetPassword") : t("newLink")}</button>
            </div>
            <div className="flex items-center gap-3">
              <span className="flex flex-1 flex-col gap-0.5">
                <span className="text-[13.5px] font-bold">{user.disabled ? t("enable") : t("disable")}</span>
                <span className="text-[12.5px] text-muted">{user.disabled ? t("enableHelp") : t("disableHelp")}</span>
              </span>
              {user.disabled ? (
                <button type="button" className={button.secondary} onClick={() => onEnable(user)}>{t("enable")}</button>
              ) : (
                <button type="button" className={button.danger} onClick={() => onDisable(user)}>{t("disable")}</button>
              )}
            </div>
          </section>
        )}
      </form>
    </SidePanel>
  );
}
```

`useProblemText` already maps API problem codes to `errors.*` text (see `lib/problem.ts:34`). The old `show()` helper goes away.

- [ ] **Step 5: Run the web checks.**

Run: `$W "$WEB sh -c 'npm run check:i18n && npm test && npm run build'"`
Expected: `i18n: en, id match`, every unit test passes, and the build succeeds.

- [ ] **Step 6: Rebuild the stack and run the two updated e2e tests.**

Run: `$W "$UP"`, then `$W "$WEB npx playwright test e2e/signin.spec.ts e2e/registry.spec.ts"`
Expected: both pass. The Clients screen is unchanged so far, so the registry spec's client steps still use the old form.

- [ ] **Step 7: Commit.**

```bash
git add web/app/admin/users web/messages/en.json web/messages/id.json web/e2e/signin.spec.ts web/e2e/registry.spec.ts
git commit -m "feat(users): search, status filters, row menu and a side panel to edit"
```

---

### Task 5: Clients screen

**Files:**
- Rewrite: `web/app/admin/clients/ClientsAdmin.tsx`
- Create: `web/app/admin/clients/AliasInput.tsx`
- Modify: `web/app/admin/clients/page.tsx` (the count)
- Modify: `web/messages/en.json` and `id.json` (`clients.*`)
- Modify: `web/e2e/registry.spec.ts:24-33`

**Interfaces:**
- Consumes: `Client.projects?: string[]` (Task 1); `matches`, `addAlias` and `maxAliases` (Task 2); `SidePanel`, `ConfirmDialog`, `FilterTabs` and `Icon "more"` (Task 3); `ClientChip` from `components/Chips.tsx`.
- Produces: `AliasInput({ id, value, onChange, labels })`, plus the panel title "Klien baru" and the submit button "Buat klien". The e2e tests rely on both.

- [ ] **Step 1: Update the e2e test first.** In `web/e2e/registry.spec.ts`, replace lines 24–33 (the comment and the loop that creates two clients):

```ts
  // Two clients, each from the New client panel.
  await page.getByRole("link", { name: "Klien", exact: true }).click();
  for (const name of [clientA, clientB]) {
    await page.getByRole("button", { name: "Klien baru" }).click();
    const panel = page.getByRole("dialog", { name: "Klien baru" });
    await panel.getByLabel("Nama", { exact: true }).fill(name);
    await panel.getByRole("button", { name: "Buat klien" }).click();
    await expect(panel).toBeHidden();
    await expect(page.getByRole("button", { name, exact: true })).toBeVisible();
  }
```

- [ ] **Step 2: Update the messages.** In `en.json`, set `clients` to the following. This removes `aliasesTitle` and the old comma hint:

```json
"clients": {
  "title": "Clients", "name": "Name", "code": "Code", "aliases": "Aliases", "projects": "Projects", "status": "Status",
  "active": "Active", "archived": "Archived", "all": "All", "statusFilter": "Status",
  "newClient": "New client", "create": "Create client", "edit": "Edit", "editTitle": "Edit {name}",
  "save": "Save", "cancel": "Cancel", "close": "Close panel",
  "search": "Name, code or alias", "noMatch": "No clients match.", "actionsFor": "Actions for {name}",
  "usedIn": "Used in {projects}", "notUsed": "Not linked to a project yet",
  "aliasPlaceholder": "Type an alias, press Enter",
  "aliasHelp": "Other names people use for this client. Search and imports recognise them.",
  "removeAlias": "Remove alias {alias}", "aliasCount": "{count} / {max}",
  "archive": "Archive", "archiveHelp": "It can't be picked for new tickets. Old tickets keep it.",
  "restore": "Restore", "restoreHelp": "It can be picked for new tickets again.",
  "confirmArchive": "Archive {name}?",
  "confirmArchiveBody": "It can't be picked for new tickets. Old tickets keep it, and you can restore it any time.",
  "adminsOnly": "Only system admins can manage clients."
}
```

In `id.json`:

```json
"clients": {
  "title": "Klien", "name": "Nama", "code": "Kode", "aliases": "Alias", "projects": "Proyek", "status": "Status",
  "active": "Aktif", "archived": "Diarsipkan", "all": "Semua", "statusFilter": "Status",
  "newClient": "Klien baru", "create": "Buat klien", "edit": "Ubah", "editTitle": "Ubah {name}",
  "save": "Simpan", "cancel": "Batal", "close": "Tutup panel",
  "search": "Nama, kode, atau alias", "noMatch": "Tidak ada klien yang cocok.", "actionsFor": "Tindakan untuk {name}",
  "usedIn": "Dipakai di {projects}", "notUsed": "Belum terhubung ke proyek",
  "aliasPlaceholder": "Ketik alias, tekan Enter",
  "aliasHelp": "Nama lain yang dipakai orang untuk klien ini. Pencarian dan impor ikut mengenalinya.",
  "removeAlias": "Hapus alias {alias}", "aliasCount": "{count} / {max}",
  "archive": "Arsipkan", "archiveHelp": "Tidak bisa dipilih untuk tiket baru. Tiket lama tetap menyebutnya.",
  "restore": "Pulihkan", "restoreHelp": "Bisa dipilih lagi untuk tiket baru.",
  "confirmArchive": "Arsipkan {name}?",
  "confirmArchiveBody": "Klien ini tidak bisa dipilih untuk tiket baru. Tiket lamanya tetap menyebutnya, dan Anda bisa memulihkannya kapan saja.",
  "adminsOnly": "Hanya admin sistem yang dapat mengelola klien."
}
```

Before saving, confirm nothing else reads the removed keys: `grep -rn 'aliasesTitle' web/app web/components` should print only `modules/NodeForm.tsx`, which uses the `modules` namespace.

- [ ] **Step 3: Create `web/app/admin/clients/AliasInput.tsx`.**

```tsx
"use client";

import { useState } from "react";
import Icon from "@/components/Icon";
import { addAlias, maxAliases } from "@/lib/admin";

type Props = {
  id: string;
  value: string[];
  onChange: (aliases: string[]) => void;
  labels: { placeholder: string; remove: (alias: string) => string };
};

// Aliases as chips: Enter or a comma adds what was typed, × removes one,
// Backspace in the empty box removes the last. What's typed but not yet added
// is added on blur, so a save doesn't lose it.
export default function AliasInput({ id, value, onChange, labels }: Props) {
  const [draft, setDraft] = useState("");
  const commit = () => {
    const next = addAlias(value, draft);
    if (next !== value) onChange(next);
    setDraft("");
  };
  return (
    <div className="flex min-h-[42px] flex-wrap items-center gap-1.5 rounded-[10px] border border-field bg-white px-2 py-1.5 focus-within:outline-2 focus-within:outline-accent">
      {value.map((alias) => (
        <span key={alias} className="inline-flex h-7 items-center gap-0.5 rounded-lg bg-well pl-2.5 pr-1 text-[13px] font-semibold text-ink">
          {alias}
          <button type="button" aria-label={labels.remove(alias)} onClick={() => onChange(value.filter((a) => a !== alias))} className="inline-flex size-[22px] items-center justify-center rounded-md text-muted hover:bg-line hover:text-ink">
            <Icon name="x" className="size-3" />
          </button>
        </span>
      ))}
      <input
        id={id}
        value={draft}
        disabled={value.length >= maxAliases}
        placeholder={labels.placeholder}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === ",") {
            e.preventDefault();
            commit();
          } else if (e.key === "Backspace" && draft === "" && value.length > 0) {
            onChange(value.slice(0, -1));
          }
        }}
        className="h-7 min-w-36 flex-1 bg-transparent text-[13.5px] text-ink outline-none placeholder:text-muted"
      />
    </div>
  );
}
```

- [ ] **Step 4: Show the count.** In `web/app/admin/clients/page.tsx`, after the `h1` in `<PageBar>`, add the same count chip as in Task 4 Step 3, using `data?.items.length`.

- [ ] **Step 5: Rewrite `web/app/admin/clients/ClientsAdmin.tsx`.**

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { ClientChip } from "@/components/Chips";
import ConfirmDialog from "@/components/ConfirmDialog";
import FilterTabs from "@/components/FilterTabs";
import Icon from "@/components/Icon";
import Menu from "@/components/Menu";
import SidePanel from "@/components/SidePanel";
import { matches, maxAliases } from "@/lib/admin";
import { api } from "@/lib/api";
import { useProblemText, type Client, type Problem } from "@/lib/problem";
import { button, chip, cx, field, table } from "@/lib/ui";
import AliasInput from "./AliasInput";

type Filter = "active" | "archived" | "all";
const menuItem = "flex w-full items-center gap-2.5 px-3.5 py-2 text-left text-sm text-ink hover:bg-paper";
const inFilter = (f: Filter, c: Client) => f === "all" || (f === "archived") === c.archived;

export default function ClientsAdmin({ clients }: { clients: Client[] }) {
  const t = useTranslations("clients");
  const problemText = useProblemText();
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("active");
  const [panel, setPanel] = useState<Client | "new" | null>(null);
  const [archiving, setArchiving] = useState<Client | null>(null);
  const [error, setError] = useState("");

  const shown = clients.filter((c) => inFilter(filter, c) && matches(query, c.name, c.code, ...c.aliases));

  async function setArchived(c: Client, archived: boolean): Promise<string | undefined> {
    const { error } = await api.PATCH("/clients/{id}", { params: { path: { id: c.id } }, body: { archived } });
    if (error) return problemText(error);
    router.refresh();
  }
  // Restore needs no confirmation; a failure shows in the page's alert line.
  async function restore(c: Client) {
    setError((await setArchived(c, false)) ?? "");
    setPanel(null);
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex h-9 w-full items-center gap-2 rounded-[10px] border border-line bg-white px-3 text-muted sm:w-64">
          <Icon name="search" className="size-[15px]" />
          <input type="search" aria-label={t("search")} placeholder={t("search")} value={query} onChange={(e) => setQuery(e.target.value)} className="min-w-0 flex-1 bg-transparent text-[13.5px] text-ink outline-none" />
        </label>
        <FilterTabs
          label={t("statusFilter")}
          value={filter}
          onChange={setFilter}
          options={(["active", "archived", "all"] as const).map((k) => ({ key: k, label: t(k), count: clients.filter((c) => inFilter(k, c)).length }))}
        />
        <button type="button" className={cx(button.primary, "ml-auto")} onClick={() => setPanel("new")}>
          <Icon name="plus" />
          {t("newClient")}
        </button>
      </div>
      {error && <p role="alert" className={field.error}>{error}</p>}
      {/* md:overflow-visible: the row menu would be clipped by the wrapper's scroll box; phones keep the sideways scroll. */}
      <div className={cx(table.wrap, "md:overflow-visible")}>
        <table className={table.table}>
          <thead className={table.head}>
            <tr>
              <th className={table.th}>{t("name")}</th>
              <th className={table.th}>{t("code")}</th>
              <th className={table.th}>{t("aliases")}</th>
              <th className={table.th}>{t("projects")}</th>
              <th className={table.th}>{t("status")}</th>
              <th className={cx(table.th, "w-14")}><span className="sr-only">{t("actionsFor", { name: "" })}</span></th>
            </tr>
          </thead>
          <tbody>
            {shown.map((c, i) => (
              <tr key={c.id} className={cx(table.row, panel !== "new" && panel?.id === c.id && "bg-accent-soft/40")}>
                <td className={table.td}>
                  <button type="button" onClick={() => setPanel(c)} className="rounded-md hover:ring-2 hover:ring-line">
                    <ClientChip client={c} coreLabel="" />
                  </button>
                </td>
                <td className={cx(table.td, "text-[12.5px] font-extrabold text-muted")}>{c.code ?? ""}</td>
                <td className={table.td}>
                  <span className="flex flex-wrap gap-1">
                    {c.aliases.map((a) => <span key={a} className={cx(chip, "bg-well text-ink-soft")}>{a}</span>)}
                  </span>
                </td>
                <td className={cx(table.td, "text-[12.5px] font-bold text-ink-soft")}>{c.projects?.length ? c.projects.join(" · ") : "—"}</td>
                <td className={table.td}>
                  <span className={cx(chip, c.archived ? "bg-well text-muted" : "bg-ok-soft text-ok")}>{c.archived ? t("archived") : t("active")}</span>
                </td>
                <td className={cx(table.td, "py-1.5 text-right")}>
                  <Menu
                    label={t("actionsFor", { name: c.name })}
                    align="right"
                    side={i >= shown.length - 2 && shown.length > 3 ? "up" : "down"}
                    summary={<Icon name="more" />}
                    summaryClassName="inline-flex size-8 items-center justify-center rounded-[9px] text-ink-soft hover:bg-paper"
                    panelClassName="min-w-52"
                  >
                    <button type="button" className={menuItem} onClick={() => setPanel(c)}><Icon name="edit" className="size-[15px] text-muted" />{t("edit")}</button>
                    <hr className="my-1 border-line-soft" />
                    {c.archived ? (
                      <button type="button" className={menuItem} onClick={() => restore(c)}><Icon name="archive" className="size-[15px] text-muted" />{t("restore")}</button>
                    ) : (
                      <button type="button" className={cx(menuItem, "text-danger")} onClick={() => setArchiving(c)}><Icon name="archive" className="size-[15px]" />{t("archive")}</button>
                    )}
                  </Menu>
                </td>
              </tr>
            ))}
            {shown.length === 0 && (
              <tr><td colSpan={6} className="px-3.5 py-8 text-center text-muted">{t("noMatch")}</td></tr>
            )}
          </tbody>
        </table>
      </div>
      {panel && (
        <ClientPanel
          key={panel === "new" ? "new" : panel.id}
          client={panel === "new" ? null : panel}
          onClose={() => setPanel(null)}
          onSaved={() => {
            setPanel(null);
            router.refresh();
          }}
          onArchive={setArchiving}
          onRestore={restore}
        />
      )}
      {archiving && (
        <ConfirmDialog
          title={t("confirmArchive", { name: archiving.name })}
          body={t("confirmArchiveBody")}
          action={t("archive")}
          cancelLabel={t("cancel")}
          onConfirm={async () => {
            const problem = await setArchived(archiving, true);
            if (!problem) {
              setArchiving(null);
              setPanel(null);
            }
            return problem;
          }}
          onCancel={() => setArchiving(null)}
        />
      )}
    </div>
  );
}

type PanelProps = {
  client: Client | null; // null: a new client
  onClose: () => void;
  onSaved: () => void;
  onArchive: (c: Client) => void;
  onRestore: (c: Client) => void;
};

function ClientPanel({ client, onClose, onSaved, onArchive, onRestore }: PanelProps) {
  const t = useTranslations("clients");
  const problemText = useProblemText();
  const [name, setName] = useState(client?.name ?? "");
  const [code, setCode] = useState(client?.code ?? "");
  const [aliases, setAliases] = useState<string[]>(client?.aliases ?? []);
  const [problem, setProblem] = useState<Problem>();
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    // On update an empty code clears it; on create it is left out.
    const { error } = client
      ? await api.PATCH("/clients/{id}", { params: { path: { id: client.id } }, body: { name, code, aliases } })
      : await api.POST("/clients", { body: { name, code: code.trim() || undefined, aliases } });
    setBusy(false);
    if (error) return setProblem(error);
    onSaved();
  }

  return (
    <SidePanel
      labelledBy="client-panel-title"
      closeLabel={t("close")}
      onClose={onClose}
      title={
        <div className="flex min-w-0 flex-col gap-0.5">
          <h2 id="client-panel-title" className="truncate text-lg font-extrabold tracking-[-0.01em]">{client ? t("editTitle", { name: client.name }) : t("newClient")}</h2>
          {client && <span className="text-[12.5px] text-muted">{client.projects?.length ? t("usedIn", { projects: client.projects.join(", ") }) : t("notUsed")}</span>}
        </div>
      }
      footer={
        <>
          <button type="button" className={button.secondary} onClick={onClose}>{t("cancel")}</button>
          <button type="submit" form="panel-form" className={button.primary} disabled={busy}>{client ? t("save") : t("create")}</button>
        </>
      }
    >
      <form id="panel-form" onSubmit={submit} className="flex flex-col gap-4">
        <div className="grid grid-cols-[minmax(0,1fr)_7.5rem] gap-3">
          <label className={field.label}>
            {t("name")}
            <input value={name} onChange={(e) => setName(e.target.value)} required maxLength={200} className={field.input} />
          </label>
          <label className={field.label}>
            {t("code")}
            <input value={code} onChange={(e) => setCode(e.target.value)} maxLength={20} className={cx(field.input, "font-extrabold tracking-[0.02em]")} />
          </label>
        </div>
        <div className="flex flex-col gap-1.5">
          <label htmlFor="client-aliases" className="flex items-baseline text-[13px] font-semibold">
            {t("aliases")}
            <span className="ml-auto text-xs text-muted">{t("aliasCount", { count: aliases.length, max: maxAliases })}</span>
          </label>
          <AliasInput
            id="client-aliases"
            value={aliases}
            onChange={setAliases}
            labels={{ placeholder: t("aliasPlaceholder"), remove: (alias) => t("removeAlias", { alias }) }}
          />
          <span className={field.hint}>{t("aliasHelp")}</span>
        </div>
        {problem && <p role="alert" className={field.error}>{problemText(problem)}</p>}
        {client && (
          <div className="mt-2 flex items-center gap-3 border-t border-line-soft pt-4">
            <span className="flex flex-1 flex-col gap-0.5">
              <span className="text-[13.5px] font-bold">{client.archived ? t("restore") : t("archive")}</span>
              <span className="text-[12.5px] text-muted">{client.archived ? t("restoreHelp") : t("archiveHelp")}</span>
            </span>
            {client.archived ? (
              <button type="button" className={button.secondary} onClick={() => onRestore(client)}>{t("restore")}</button>
            ) : (
              <button type="button" className={button.danger} onClick={() => onArchive(client)}>{t("archive")}</button>
            )}
          </div>
        )}
      </form>
    </SidePanel>
  );
}
```

`ClientChip` takes a `Ref`, and `Client` has `id` and `name`, so it type-checks. If the build disagrees, pass `{ id: c.id, name: c.name }`.

- [ ] **Step 6: Run the web checks.**

Run: `$W "$WEB sh -c 'npm run check:i18n && npm test && npm run build'"`
Expected: all three pass.

- [ ] **Step 7: Rebuild and run the registry e2e.**

Run: `$W "$UP"`, then `$W "$WEB npx playwright test e2e/registry.spec.ts e2e/signin.spec.ts"`
Expected: both pass.

- [ ] **Step 8: Commit.**

```bash
git add web/app/admin/clients web/messages/en.json web/messages/id.json web/e2e/registry.spec.ts
git commit -m "feat(clients): search, row menu, a side panel with alias chips, and each client's projects"
```

---

### Task 6: Admin e2e, the full suite and a visual check

**Files:**
- Create: `web/e2e/admin.spec.ts`
- Modify: `web/e2e/global-setup.ts` (one more admin, `E2E_ADMIN2_*`)

**Interfaces:**
- Consumes: every label named in Tasks 4 and 5.

- [ ] **Step 1: Give the new spec its own admin.** In `web/e2e/global-setup.ts`, after the existing `createAdmin` calls:

```ts
  const admin2 = createAdmin("Admin Screens");
  process.env.E2E_ADMIN2_EMAIL = admin2.email;
  process.env.E2E_ADMIN2_LINK = admin2.link;
```

- [ ] **Step 2: Write `web/e2e/admin.spec.ts`.**

```ts
import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// The admin Users and Clients screens: edit in the side panel, confirm before
// a reset, search, archive from the row menu, and a client's projects.
test("an admin edits a user, confirms a reset, searches, and archives a client", async ({ page }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36);
  const password = "e2e-admin-screens-passphrase-7";
  await setPassword(page, process.env.E2E_ADMIN2_LINK!, password);
  await signIn(page, process.env.E2E_ADMIN2_EMAIL!, password);

  // A user to work on.
  await page.getByRole("link", { name: "Pengguna" }).click();
  await page.getByRole("button", { name: "Pengguna baru" }).click();
  let panel = page.getByRole("dialog", { name: "Pengguna baru" });
  await panel.getByLabel("Nama", { exact: true }).fill(`Sari ${run}`);
  await panel.getByLabel("Email", { exact: true }).fill(`sari-${run}@example.com`);
  await panel.getByRole("button", { name: "Buat pengguna" }).click();
  await expect(page.getByTestId("setup-link").first()).toContainText("/setup/");

  // Search narrows the table to her.
  await page.getByLabel("Nama atau email").fill(`sari-${run}`);
  await expect(page.locator("tbody tr")).toHaveCount(1);

  // Rename her and make her an admin from the panel.
  await page.getByRole("button", { name: `Sari ${run}` }).click();
  panel = page.getByRole("dialog", { name: `Sari ${run}` });
  await panel.getByLabel("Nama", { exact: true }).fill(`Sari Dewi ${run}`);
  await panel.getByLabel("Admin sistem").check();
  await panel.getByRole("button", { name: "Simpan" }).click();
  await expect(panel).toBeHidden();
  await page.getByLabel("Nama atau email").fill(`Sari Dewi ${run}`);
  const row = page.locator("tbody tr").filter({ hasText: `Sari Dewi ${run}` });
  await expect(row.getByText("Admin", { exact: true })).toBeVisible();

  // A new setup link asks first; cancelling changes nothing.
  await row.getByLabel(`Tindakan untuk Sari Dewi ${run}`).click();
  await page.getByRole("button", { name: "Tautan pengaturan baru" }).click();
  const confirm = page.getByRole("alertdialog");
  await expect(confirm).toContainText(`Sari Dewi ${run}`);
  await confirm.getByRole("button", { name: "Batal" }).click();
  await expect(confirm).toBeHidden();

  // A client linked to a project shows the project; archiving asks, then moves it to Archived.
  const client = `Klien ${run}`;
  await page.getByRole("link", { name: "Klien", exact: true }).click();
  await page.getByRole("button", { name: "Klien baru" }).click();
  panel = page.getByRole("dialog", { name: "Klien baru" });
  await panel.getByLabel("Nama", { exact: true }).fill(client);
  await panel.getByLabel("Alias").fill("KR Group");
  await panel.getByLabel("Alias").press("Enter");
  await expect(panel.getByRole("button", { name: "Hapus alias KR Group" })).toBeVisible();
  await panel.getByRole("button", { name: "Buat klien" }).click();
  await expect(panel).toBeHidden();
  await page.getByLabel("Nama, kode, atau alias").fill("kr group");
  const crow = page.locator("tbody tr").filter({ hasText: client });
  await expect(crow).toContainText("—");
  await crow.getByLabel(`Tindakan untuk ${client}`).click();
  await page.getByRole("button", { name: "Arsipkan" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Arsipkan" }).click();
  await expect(crow).toBeHidden();
  await page.getByRole("button", { name: /^Diarsipkan/ }).click();
  await expect(page.locator("tbody tr").filter({ hasText: client })).toBeVisible();
});
```

The registry spec already links clients to a project. Its run checks the Proyek column through the API test in Task 1, and this spec checks the "—" for an unlinked client.

- [ ] **Step 3: Run the new spec, then the whole suite.**

Run: `$W "$WEB npx playwright test e2e/admin.spec.ts"`, then `$W "$WEB npx playwright test"`
Expected: `admin.spec.ts` passes. The whole suite passes. A setup link can be used only once, so don't add `--repeat-each`. If an unrelated spec was already failing on `main`, check it there and note it in the commit message; don't fix it here.

- [ ] **Step 4: Run every Go test.**

Run: `$W "$GO go test ./..."`
Expected: PASS.

- [ ] **Step 5: Visual check.** Open http://localhost:8080/admin/users and http://localhost:8080/admin/clients, signed in as the e2e admin from Step 2. Take screenshots of:
  - the list with the ⋯ menu open;
  - the edit panel;
  - the reset confirmation;
  - the client panel with alias chips.

  Compare them with the canvas artboards. Fix any gap in spacing or colour within this task.

- [ ] **Step 6: Commit.**

```bash
git add web/e2e/admin.spec.ts web/e2e/global-setup.ts
git commit -m "test(e2e): the admin screens: edit, confirm, search, archive"
```

---

## Spec coverage

| Spec item | Task |
| --- | --- |
| `Client.projects` on `GET /clients` only | 1 |
| Search and status filters | 2, 4, 5 |
| ⋯ row menu (reusing `Menu.tsx`) | 3, 4, 5 |
| One side panel for create and edit | 3, 4, 5 |
| Edit a user's name and admin flag; read-only email; your own admin box locked | 4 |
| Confirm reset, new link, disable and archive; enable and restore run directly | 3, 4, 5 |
| Alias chips, at most 20, duplicates refused | 2, 5 |
| Projects column and "Used in" | 1, 5 |
| Translations match; unused keys removed | 4, 5 |
| e2e updated and added; setup-link testid kept | 4, 5, 6 |
| Risks: menu clipped by the table (opens upward on the last rows), focus goes back on close | 3, 4, 5 |
