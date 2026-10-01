# Admin Users and Clients redesign

**Date:** 2026-10-01
**Branch:** `feat/admin-users-clients`
**Design canvas:** "Muasal UI refresh", artboards AdminUsers, AdminUserPanel, AdminUserNew, AdminUserConfirm, AdminClients and AdminClientPanel (https://claude.ai/artifact/3Kjw9TDkPzGjcpmoXBcq82).

## Problem

The admin screens `/admin/users` and `/admin/clients` work, but:

- An existing user can't be renamed or promoted, though `PATCH /admin/users/{id}` already accepts `name` and `is_admin`.
- Reset password, disable and archive run on one click. A reset ends that person's sessions.
- Neither list can be searched or filtered.
- Both pages open on a create form that is always there. Clients edit inline in the table row, with aliases typed as comma-separated text.
- The clients list doesn't say which projects use each client.

## Goals

1. Search and status filters on both lists.
2. A ⋯ menu per row in place of the row's text links.
3. One side panel per screen for creating and editing. Clicking a name opens it.
4. Users: edit the name and the admin flag. Email stays read-only.
5. Clients: aliases as removable chips, and the projects that use each client.
6. A styled confirmation before reset password, new setup link, disable and archive.

## Non-goals

- Bulk actions. The lists are short.
- A URL per open panel (`?user=3`). Add one when someone needs to link to a user.
- Server-side search or paging. `GET /admin/users` and `GET /clients` already return whole lists.
- Changing a user's email. The API doesn't allow it.

## Server and API

The clients list gains each client's project keys. The change only adds fields.

- **Query** `ListClientsWithProjects` in `server/internal/db/queries/clients.sql`. It reads every client, plus `array_agg(p.key ORDER BY p.key)` over `project_clients` joined to `projects`. The array is empty when no project links the client.
- **OpenAPI:** `Client` gets an optional `projects: string[]`, described as "Keys of the projects linked to this client. Only `GET /clients` fills it."
- **Handler:** `ListClients` uses the new query. `toAPIClient` stays as it is for every other response, so `/projects/{key}/clients` and the create and update responses don't change.
- **Generated code:** `make generate` regenerates the Go stubs, the sqlc code and `web/lib/api-types.ts`.
- **Test:** a new `TestClientListNamesTheirProjects` in `clients_test.go` links one client to two projects and checks two keys in key order. It checks that an unlinked client returns an empty list, not null.

## Web

The changes stay in `web/app/admin/users/`, `web/app/admin/clients/` and `web/components/`, following the patterns already there.

### Shared components

- **Row menu:** reuse `components/Menu.tsx`, the `<details>` dropdown with Escape and click-outside closing, with `align="right"`. The summary is a 32 px ⋯ icon button labelled "Actions for {name}". Items use the menu-item classes from `Sidebar.tsx`'s account menu. The risky item (Disable, Archive) sits below a divider in `text-danger`.
- **`components/SidePanel.tsx` (new):** a native `<dialog>`, opened with `showModal()` like `CloseDialog`. It is pinned to the right, 460 px wide and full height, with no rounded right corners and a light backdrop. It takes a title node, children and a footer. Escape and the × button call `onClose`. On phones it takes the full width.
- **`components/ConfirmDialog.tsx` (new):** a small native `<dialog>` with a title, body text, Cancel, and an action button styled `button.danger`. It receives `onConfirm` and `onCancel`, and it shows a problem message when the action fails.

### Users screen (`UsersAdmin.tsx`)

- **PageBar** keeps the title and adds the user count.
- **Toolbar:** a search box (name or email, case-insensitive) and segmented status filters: All, Active, Invited, Disabled, each with a count. The **New user** button sits on the right. Filtering happens in the browser.
- **Table columns:** Name (avatar, name, the Admin chip, and "(you)" on your own row), Email, Status, Last sign-in, and ⋯. Your own row has no ⋯ menu. The name is a button that opens the edit panel.
- **Menu:** Edit; Reset password, or New setup link for an invited user; then Disable or Enable below the divider.
- **Panel (new):** Name, Email and System admin. **Create user** calls `POST /admin/users`, closes the panel and shows the setup-link banner as today.
- **Panel (edit):** the header shows the avatar, name, status and last sign-in. The form has Name, a read-only Email with the note "Email can't be changed", and System admin. On your own row the admin box is disabled. An **Access** section holds Reset password (or New setup link) and Disable (or Enable), each with a line saying what it does. **Save** sends `PATCH /admin/users/{id}` with only the fields that changed.
- **Confirmations:** reset password ends the old password and every session, and makes a new link. A new setup link ends the old link. Disable signs the person out and blocks sign-in, while their tickets stay. Enable runs without asking.
- The setup-link banner (MSL-20) stays as it is: listed newest first until dismissed, with Copy.

### Clients screen (`ClientsAdmin.tsx`)

- **Toolbar:** a search box (name, code or any alias) and filters Active, Archived and All, with Active as the default. The **New client** button sits on the right.
- **Table columns:** Name (a client chip that opens the edit panel), Code, Alias chips, Projects (keys joined with " · ", or "—"), Status, and ⋯.
- **Menu:** Edit, then Archive or Restore below the divider.
- **Panel:** Name, Code, and an alias chip input. Enter or a comma adds an alias, × removes one, and Backspace in the empty box removes the last. It refuses duplicates and holds at most 20, the API's `maxItems`, with a counter. The edit panel adds an Archive or Restore section, and its header says "Used in {projects}".
- **Archive** asks first. Restore doesn't.
- `ClientChip` from `components/Chips.tsx` draws the name chip, as it does today.

### Text

New keys go under `users.*` and `clients.*` in `web/messages/en.json` and `id.json`. Any key the new screens no longer use is removed, such as `clients.aliases` "(comma-separated)". The Indonesian text follows the canvas.

### Errors

A problem from the API shows inside the open panel or confirmation, using `useProblemText`, and that panel or dialog stays open. A problem from an action started in the menu shows in the page's alert line, as today.

## Testing

- **Go:** the new `TestClientListNamesTheirProjects`, and the existing httpapi suite still passes.
- **Web checks** (as in CI): `npm run gen:api` leaves no diff, and `npm run check:i18n`, `npm test` and `npm run build` pass. The build also type-checks.
- **e2e:**
  - `signin.spec.ts` and `registry.spec.ts` click **Pengguna baru** and **Klien baru**, then fill the panel's fields.
  - The setup-link `data-testid` stays.
  - A new `admin.spec.ts`:
    1. rename a user and make them admin from the panel;
    2. reset password, checking that the confirmation shows and that cancelling changes nothing;
    3. search narrows the table;
    4. archive a client from the ⋯ menu after confirming, and see it under Archived;
    5. a linked client shows its project key.
- **Visual check:** rebuild the `web` and `app` containers and screenshot both screens, with a panel open, at http://localhost:8080.

## Risks

- **The `<details>` menu inside a table row:** the table wrapper uses `overflow-x-auto`, which can clip the open menu. The fix is to open the menu upward on the last two rows (`side="up"`), or to let the wrapper show overflow on wide screens.
- **Focus:** opening a panel with `showModal()` traps focus. Closing it must return focus to the button that opened it, so the code stores that button and focuses it again.
