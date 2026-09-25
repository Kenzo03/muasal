# Muasal Iteration 4b — Web Items Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** the web items of FSD §21 Iteration 4, split from the AI work on 25 Sep 2026:
- a ticket can be created from anywhere in a modal, opened with `c` (§8.1);
- descriptions and comments render Markdown, and a pasted screenshot becomes an attachment shown inline;
- modules and menus move by drag and drop in the module tree (§7.3). The board moves to the same library, dnd-kit;
- the ticket form lists the menus the user used most recently first (§8.1).

Plan 4a (`docs/superpowers/plans/2026-09-25-iteration-4a-ask-engine.md`) holds the AI backend and the Admin → AI page. The two plans share no code and can merge in either order.

**How this plan was written:** every task was built and tested in this order before the plan was written. Each code block below is the code that passed: the Go tests ran against PostgreSQL 18, and the rebuilt compose stack ran every end-to-end test. New files appear in full. Changes to existing files appear as unified diffs; apply each with `git apply` from the repository root, or by hand. Generated files (`api.gen.go`, `internal/db/*.sql.go`, `web/lib/api-types.ts`) and `package-lock.json` are never shown: run `make generate` or the `npm install` line where a step says so.

**Architecture:** Same stack and patterns as Iterations 0–3.
- **Modal:** Next.js intercepting and parallel routes. The root layout gains a `@modal` slot. `@modal/(.)p/[key]/tickets/new` catches in-app navigation to the create page and renders the existing `TicketForm` in a native `<dialog>`. The full page at `/p/[key]/tickets/new` stays for direct loads, reloads and shared links. Both use one loader, `newTicketData`.
- **Markdown:** `react-markdown` with `remark-gfm`, and `rehype-sanitize` on its default schema, so raw HTML never renders. Images render only from Muasal's own attachment URLs, which carry the ticket's access check; any other image is a link, so a ticket cannot make a reader's browser call out.
- **Paste:** the existing attachment upload. The pasted image's Markdown goes in at the cursor through `setRangeText`, and an `input` event keeps React's controlled value in step.
- **Drag and drop:** `@dnd-kit/core` 6.3 for the tree and the board. It gives pointer, touch and keyboard sensors, and screen-reader announcements. The HTML5 drag events on the board go away.
- **Recent menus:** one query over the caller's own tickets under `ListNodes`' visibility rules, served as `GET /projects/{key}/nodes/recent`.

**Tech Stack:** adds `react-markdown` 10.1.0, `remark-gfm` 4.0.1, `rehype-sanitize` 6.0.0 and `@dnd-kit/core` 6.3.1 to the web app. No server dependencies.

**Spec:** Claude Docs "FSD — Muasal" (rev 108): §7.3 module tree, §8.1 create ticket, §8.4 board, §8.5 ticket page, §21 delivery plan.

## Global Constraints

- **Carried over:** everything in the Iteration 0–3 Global Constraints still holds. Notably, "hidden and missing look the same" (404) and "the visibility predicate lives in SQL".
- **No outside requests from content:** Markdown never renders raw HTML, and an image renders only from `/api/v1/attachments/{id}`.
- **Keyboard parity:** everything a drag does can be done from the keyboard. Tree rows take Space and the arrows, and keep the parent picker and the up and down buttons. Board cards keep their status menu.
- **The `c` shortcut** never fires while typing (inputs, textareas, selects, contenteditable), with a modifier, or while a dialog is open.
- **UI strings:** every string ships in Indonesian (default) and English.

## Deliberate Deviations from the FSD

- Iteration 4 is split into 4a (AI) and 4b (this plan), each with its own branch and PR (decided 25 Sep 2026).
- "Recently used menus" means the menus on the caller's own latest tickets in the project, at most 8. Nothing new is stored.
- The create form has no ticket to attach to yet, so pasting an image there explains that images can be pasted once the ticket exists. The description in the edit form and every comment box take pasted images.
- The board's keyboard path is the status menu on each card, not dnd-kit's keyboard sensor. It reaches every column in one step, where arrow keys would walk column by column.

## Before You Start

- `main` is at 507b5db (Iteration 3 merged); work on branch `feat/iteration-4b`.
- Run `make testdb` for the Go tests and `make up` (rebuilt) for the end-to-end tests. The test commands below assume `TEST_DATABASE_URL='postgres://owner:owner@localhost:55432/postgres?sslmode=disable'` is exported.
- Whichever of 4a and 4b merges second meets small conflicts in `web/messages/*.json`, `web/e2e/global-setup.ts` and `web/package*.json`. Keep both sides.

## File Structure

```text
api/openapi.yaml                                GET /projects/{key}/nodes/recent
server/internal/
├── db/queries/nodes.sql                        ListRecentNodes
└── httpapi/nodes.go, recent_nodes_test.go      the handler and its test
web/
├── app/layout.tsx                              the @modal slot
├── app/@modal/                                 default, catch-all, (.)p/[key]/tickets/new (the dialog)
├── app/TopBar.tsx                              the c shortcut
├── app/p/[key]/tickets/new/data.ts             loader shared by the page and the modal
├── app/p/[key]/modules/ModuleTree.tsx          dnd-kit tree
├── app/p/[key]/board/Board.tsx                 dnd-kit board
├── app/t/[ticketKey]/{TicketView,Activity}.tsx Markdown, paste into comments
├── components/{Markdown,NodePicker,TicketForm,Menu,Icon}.tsx
├── lib/paste.ts                                pasted images → attachments → Markdown
└── e2e/{helpers,web.spec,tickets.spec,decisions.spec,global-setup}.ts
```

---

### Task 1: Markdown and pasted images

**Files:**
- Create: `web/components/Markdown.tsx`, `web/lib/paste.ts`
- Modify: `web/app/t/[ticketKey]/Activity.tsx`, `web/app/t/[ticketKey]/TicketView.tsx`, `web/components/TicketForm.tsx`, `web/messages/en.json`, `web/messages/id.json`

**Interfaces:**
- Consumes: `POST /tickets/{key}/attachments` and `GET /attachments/{id}` (Iteration 2).
- Produces:
  - `<Markdown>{text}</Markdown>` (`web/components/Markdown.tsx`): GitHub-flavoured Markdown through `rehype-sanitize`, no raw HTML. An image renders only when its source is `/api/v1/attachments/{id}`; any other image becomes a link. Outside links open in a new tab with `rel="noopener noreferrer"`.
  - `pasteImages(ticketKey, onError, onUploaded?)` (`web/lib/paste.ts`): an `onPaste` handler for a textarea. Each pasted image uploads as `pasted-<time>.<ext>`, then `![name](/api/v1/attachments/{id})` goes in at the cursor, and an `input` event keeps React's state in step. Text pastes are left alone.
  - The ticket description and comments render as Markdown. The comment boxes and the description in the edit form take pasted images. The create form has no ticket yet, so a pasted image there shows `ticketForm.pasteAfterCreate`.
  - Messages `ticketForm.descriptionHint`, `descriptionHintNew`, `pasteAfterCreate` and `activity.markdownHint`.

- [ ] **Step 1: Implement**

Add the dependencies: `cd web && npm install --save-exact react-markdown@10.1.0 remark-gfm@4.0.1 rehype-sanitize@6.0.0`

`web/app/t/[ticketKey]/Activity.tsx`:

```diff
diff --git a/web/app/t/[ticketKey]/Activity.tsx b/web/app/t/[ticketKey]/Activity.tsx
--- a/web/app/t/[ticketKey]/Activity.tsx
+++ b/web/app/t/[ticketKey]/Activity.tsx
@@ -5,9 +5,11 @@ import { useRouter } from "next/navigation";
 import { useLocale, useTranslations } from "next-intl";
 import { Avatar } from "@/components/Chips";
 import Icon from "@/components/Icon";
+import Markdown from "@/components/Markdown";
 import { api } from "@/lib/api";
 import { describeChange, shown, type Change } from "@/lib/activity";
 import { utc } from "@/lib/format";
+import { pasteImages } from "@/lib/paste";
 import { useProblemText, type ActivityItem } from "@/lib/problem";
 import { button, chip, cx, field, panel } from "@/lib/ui";
 
@@ -27,6 +29,7 @@ export default function Activity({ ticketKey, items, meId, canComment }: {
   const [filter, setFilter] = useState<Filter>("all");
   const [editing, setEditing] = useState<number | null>(null);
   const [error, setError] = useState("");
+  const paste = pasteImages(ticketKey, (p) => setError(problemText(p)), () => router.refresh());
   const visible = items.filter((it) => filter === "all" || (filter === "comments") === (it.kind === "comment"));
   const field_ = (k: string) => (t.has(`fields.${k}`) ? t(`fields.${k}`) : k);
 
@@ -142,14 +145,14 @@ export default function Activity({ ticketKey, items, meId, canComment }: {
                   <p className="text-sm italic text-muted">{t("deleted")}{it.body ? `: ${it.body}` : ""}</p>
                 ) : editing === it.comment_id ? (
                   <form onSubmit={(e) => saveEdit(e, it.comment_id!)} className="flex flex-col gap-2">
-                    <textarea name="body" defaultValue={it.body} required maxLength={20000} rows={3} aria-label={t("edit")} className={field.textarea} />
+                    <textarea name="body" defaultValue={it.body} required maxLength={20000} rows={3} aria-label={t("edit")} onPaste={paste} className={field.textarea} />
                     <div className="flex gap-2">
                       <button className={button.primary}>{t("save")}</button>
                       <button type="button" onClick={() => setEditing(null)} className={button.secondary}>{t("cancel")}</button>
                     </div>
                   </form>
                 ) : (
-                  <p className="whitespace-pre-wrap text-sm leading-relaxed">{it.body}</p>
+                  <Markdown text={it.body ?? ""} />
                 )}
                 {!it.deleted && canComment && it.actor?.id === meId && editing !== it.comment_id && (
                   <div className="flex gap-3">
@@ -173,7 +176,8 @@ export default function Activity({ ticketKey, items, meId, canComment }: {
       </ol>
       {canComment && (
         <form onSubmit={send} className="flex flex-col gap-2 border-t border-line-soft pt-3.5">
-          <textarea name="body" required maxLength={20000} rows={3} aria-label={t("placeholder")} placeholder={t("placeholder")} className={field.textarea} />
+          <textarea name="body" required maxLength={20000} rows={3} aria-label={t("placeholder")} placeholder={t("placeholder")} onPaste={paste} className={field.textarea} />
+          <p className={field.hint}>{t("markdownHint")}</p>
           <div className="flex flex-wrap items-center gap-3">
             <label className="flex items-center gap-2 text-[13px]">
               <input type="checkbox" name="internal" defaultChecked className="size-4 accent-accent" />
```

`web/app/t/[ticketKey]/TicketView.tsx`:

```diff
diff --git a/web/app/t/[ticketKey]/TicketView.tsx b/web/app/t/[ticketKey]/TicketView.tsx
--- a/web/app/t/[ticketKey]/TicketView.tsx
+++ b/web/app/t/[ticketKey]/TicketView.tsx
@@ -7,6 +7,7 @@ import { useLocale, useTranslations } from "next-intl";
 import { Avatar, ClientChip, PriorityChip, StatusDot, TypeIcon } from "@/components/Chips";
 import CloseDialog from "@/components/CloseDialog";
 import Icon from "@/components/Icon";
+import Markdown from "@/components/Markdown";
 import PageBar from "@/components/PageBar";
 import TicketForm from "@/components/TicketForm";
 import { api } from "@/lib/api";
@@ -141,7 +142,7 @@ export default function TicketView({ ticket, statuses, clients, nodes, assignees
               <section aria-label={t("details")} className={panel}>
                 <div className={block}>
                   <h2 className={sectionTitle}>{t("description")}</h2>
-                  <p className="whitespace-pre-wrap text-sm leading-relaxed">{ticket.description || <span className="text-muted">{t("none")}</span>}</p>
+                  {ticket.description ? <Markdown text={ticket.description} /> : <p className="text-sm text-muted">{t("none")}</p>}
                 </div>
                 <div className={block}>
                   <h2 className={sectionTitle}>{t("reason")}</h2>
```

`web/components/Markdown.tsx` (new):

```tsx
import ReactMarkdown, { type Components } from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import remarkGfm from "remark-gfm";
import { cx } from "@/lib/ui";

// Descriptions and comments are markdown (FSD §8.1, §8.7). Raw HTML is never
// rendered, and rehype-sanitize strips anything unsafe from the rest. Images
// show only when they are this app's attachments, which Go serves after a
// visibility check; any other image stays a plain link, so a ticket cannot
// load outside content (§18.1: nothing leaves the server).
const attachment = /^\/api\/v1\/attachments\/\d+$/;

const components: Components = {
  h1: ({ children }) => <h3 className="mt-3 text-base font-semibold first:mt-0">{children}</h3>,
  h2: ({ children }) => <h3 className="mt-3 text-[15px] font-semibold first:mt-0">{children}</h3>,
  h3: ({ children }) => <h4 className="mt-2 text-sm font-semibold first:mt-0">{children}</h4>,
  p: ({ children }) => <p className="my-1.5 first:mt-0 last:mb-0">{children}</p>,
  ul: ({ children }) => <ul className="my-1.5 list-disc pl-5">{children}</ul>,
  ol: ({ children }) => <ol className="my-1.5 list-decimal pl-5">{children}</ol>,
  blockquote: ({ children }) => <blockquote className="my-1.5 border-l-2 border-line pl-3 text-muted">{children}</blockquote>,
  code: ({ children, className }) => <code className={cx("rounded bg-paper px-1 font-mono text-[12.5px]", className)}>{children}</code>,
  pre: ({ children }) => <pre className="my-2 overflow-x-auto rounded border border-line-soft bg-paper p-2.5 [&_code]:bg-transparent [&_code]:p-0">{children}</pre>,
  table: ({ children }) => (
    <div className="my-2 overflow-x-auto">
      <table className="border-collapse text-[13px] [&_td]:border [&_td]:border-line [&_td]:px-2 [&_td]:py-1 [&_th]:border [&_th]:border-line [&_th]:bg-paper [&_th]:px-2 [&_th]:py-1 [&_th]:text-left">{children}</table>
    </div>
  ),
  a: ({ href, children }) => {
    const external = href !== undefined && /^https?:\/\//.test(href);
    return (
      <a href={href} {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}>
        {children}
      </a>
    );
  },
  img: ({ src, alt }) =>
    typeof src === "string" && attachment.test(src) ? (
      <a href={src} target="_blank" rel="noopener" className="my-1.5 inline-block">
        <img src={src} alt={alt ?? ""} loading="lazy" className="max-h-96 max-w-full rounded border border-line" />
      </a>
    ) : (
      <a href={typeof src === "string" ? src : undefined} target="_blank" rel="noopener noreferrer">{alt || src?.toString()}</a>
    ),
};

export default function Markdown({ text, className }: { text: string; className?: string }) {
  return (
    <div className={cx("break-words text-sm leading-relaxed", className)}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]} components={components}>
        {text}
      </ReactMarkdown>
    </div>
  );
}
```

`web/components/TicketForm.tsx`:

```diff
diff --git a/web/components/TicketForm.tsx b/web/components/TicketForm.tsx
--- a/web/components/TicketForm.tsx
+++ b/web/components/TicketForm.tsx
@@ -11,6 +11,7 @@ import {
   type Client, type Contact, type Node, type Priority, type Ref, type Ticket, type TicketType,
 } from "@/lib/problem";
 import { button, cx, field } from "@/lib/ui";
+import { pasteImages } from "@/lib/paste";
 import { isWeak } from "@/lib/weak";
 
 type Props = {
@@ -284,7 +285,27 @@ const [reason, setReason] = useState(ticket?.reason ?? "");
           </div>
         </Row>
         <Row label={t("description")} htmlFor="tf-description">
-          <textarea id="tf-description" name="description" defaultValue={ticket?.description} maxLength={50000} rows={5} className={field.textarea} />
+          <textarea
+            id="tf-description"
+            name="description"
+            defaultValue={ticket?.description}
+            maxLength={50000}
+            rows={5}
+            aria-describedby="description-hint"
+            onPaste={
+              ticket
+                ? pasteImages(ticket.key, (p) => setError(problemText(p)), () => router.refresh())
+                : (e) => {
+                    // A new ticket has nowhere to keep a file yet.
+                    if (Array.from(e.clipboardData.files).some((f) => f.type.startsWith("image/"))) {
+                      e.preventDefault();
+                      setNotice(t("pasteAfterCreate"));
+                    }
+                  }
+            }
+            className={field.textarea}
+          />
+          <p id="description-hint" className={field.hint}>{t(ticket ? "descriptionHint" : "descriptionHintNew")}</p>
         </Row>
         <details className="group" open={Boolean(ticket?.assignee || ticket?.due_date)}>
           <summary className="flex cursor-pointer items-center gap-1.5 text-sm font-semibold">
```

`web/lib/paste.ts` (new):

```ts
import type { Problem } from "./problem";

// Pasting an image into a description or comment
// uploads it as one of the ticket's attachments and inserts a markdown image
// at the cursor (FSD §8.1, §8.7). Text pastes are left alone.
export function pasteImages(ticketKey: string, onError: (p?: Problem) => void, onUploaded: () => void) {
  return async (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const images = Array.from(e.clipboardData.files).filter((f) => f.type.startsWith("image/"));
    if (images.length === 0) return;
    e.preventDefault();
    const el = e.currentTarget;
    for (const [i, image] of images.entries()) {
      const ext = image.type.split("/")[1]?.replace("jpeg", "jpg") || "png";
      const name = image.name && image.name !== "image.png" ? image.name : `pasted-${Date.now()}${i ? `-${i}` : ""}.${ext}`;
      const form = new FormData();
      form.append("file", new File([image], name, { type: image.type }));
      // openapi-fetch sends JSON, so the multipart upload uses fetch directly (same origin, same cookie).
      const res = await fetch(`/api/v1/tickets/${encodeURIComponent(ticketKey)}/attachments`, { method: "POST", body: form });
      if (!res.ok) return onError(await res.json().catch(() => undefined));
      const { id } = (await res.json()) as { id: number };
      const md = `![${name}](/api/v1/attachments/${id})\n`;
      el.setRangeText(md, el.selectionStart, el.selectionEnd, "end");
      el.dispatchEvent(new Event("input", { bubbles: true }));
    }
    onUploaded();
  };
}
```

`web/messages/en.json`:

```diff
diff --git a/web/messages/en.json b/web/messages/en.json
--- a/web/messages/en.json
+++ b/web/messages/en.json
@@ -408,7 +408,10 @@
     "created": "{key} created.",
     "save": "Save",
     "cancel": "Cancel",
-    "reload": "Reload"
+    "reload": "Reload",
+    "descriptionHint": "Markdown works. Paste an image to attach it to the ticket.",
+    "descriptionHintNew": "Markdown works. Images can be pasted once the ticket exists.",
+    "pasteAfterCreate": "Create the ticket first; then paste the image into its description or a comment."
   },
   "ticket": {
     "edit": "Edit",
@@ -526,7 +529,8 @@
       "outcome": "Outcome",
       "state": "State",
       "confirmed_by": "Confirmed by"
-    }
+    },
+    "markdownHint": "Markdown works: **bold**, lists, `code`, links. Paste an image to attach it."
   },
   "attachments": {
     "title": "Attachments",
```

`web/messages/id.json`:

```diff
diff --git a/web/messages/id.json b/web/messages/id.json
--- a/web/messages/id.json
+++ b/web/messages/id.json
@@ -408,7 +408,10 @@
     "created": "{key} dibuat.",
     "save": "Simpan",
     "cancel": "Batal",
-    "reload": "Muat ulang"
+    "reload": "Muat ulang",
+    "descriptionHint": "Mendukung Markdown. Tempel gambar untuk melampirkannya ke tiket.",
+    "descriptionHintNew": "Mendukung Markdown. Gambar bisa ditempel setelah tiket dibuat.",
+    "pasteAfterCreate": "Buat tiketnya dulu, lalu tempel gambar di deskripsi atau komentar."
   },
   "ticket": {
     "edit": "Ubah",
@@ -526,7 +529,8 @@
       "outcome": "Hasil",
       "state": "Keadaan",
       "confirmed_by": "Dikonfirmasi oleh"
-    }
+    },
+    "markdownHint": "Mendukung Markdown: **tebal**, daftar, `kode`, tautan. Tempel gambar untuk melampirkannya."
   },
   "attachments": {
     "title": "Lampiran",
```

- [ ] **Step 2: Run the tests**

Run: `cd web && npx tsc --noEmit && npm run build`

Expected: no type errors and a clean build.

The end-to-end test in Task 6 covers rendering and paste.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat(web): Markdown in descriptions and comments, and pasted images"
```

### Task 2: Recently used menus first

**Files:**
- Create: `server/internal/httpapi/recent_nodes_test.go`
- Modify: `api/openapi.yaml`, `server/internal/db/queries/nodes.sql`, `server/internal/httpapi/nodes.go`, `web/components/NodePicker.tsx`, `web/components/TicketForm.tsx`, `web/messages/en.json`, `web/messages/id.json`
- Regenerate: `server/internal/db/*.sql.go`, `server/internal/httpapi/api.gen.go`, `web/lib/api-types.ts`

**Interfaces:**
- Consumes: `ListNodes`' visibility rules (Iteration 1) and the reporter on `tickets` (Iteration 2).
- Produces:
  - Query `ListRecentNodes{ProjectID, AllClients, ClientIds, ReporterID}`: the live nodes the caller can see (`ListNodes`' rules), taken from the tickets they reported, ordered by the latest such ticket, at most 8.
  - `GET /projects/{key}/nodes/recent` (`listRecentNodes`) → `RecentNodes{node_ids}`; a project the caller cannot open answers 404.
  - `NodePicker` takes `recent?: number[]`: those nodes come first, in that order, each with a "Terakhir" chip; the rest keep the tree order.
  - `TicketForm` loads the list when it opens. When the type is Bug and menus are chosen, it links to the first menu's Behaviors tab (§8.1: "is this a bug or agreed behavior?").
  - Messages `ticketForm.recent`, `ticketForm.bugBehaviors`.

- [ ] **Step 1: Write the failing test**

`server/internal/httpapi/recent_nodes_test.go` (new):

```go
package httpapi_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// FSD §8.1: the menu picker puts the caller's recently used menus first: those
// of their own latest tickets, live and visible to them, most recent first.
func TestRecentNodesAreTheCallersOwnLatest(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	other, ou := e.signedIn("ani@example.com", false)
	e.seedMember(ou, w.p, "member")
	leave := e.seedNode(w.p, &w.hr, "menu", "Leave Request")
	archived := e.seedNode(w.p, &w.hr, "menu", "Old Menu")
	create := func(c *http.Client, title string, nodes ...int64) {
		t.Helper()
		if code := e.call(c, http.MethodPost, "/projects/HRIS/tickets", map[string]any{
			"type": "change_request", "title": title, "node_ids": nodes, "requester_user_id": w.pmUser.ID,
		}, nil); code != http.StatusCreated {
			t.Fatalf("create %q: %d", title, code)
		}
	}
	create(w.pm, "Overtime export", w.ot.ID)
	create(w.pm, "Leave carry-over", leave.ID, archived.ID)
	create(w.pm, "Overtime cap", w.ot.ID) // Overtime Approval is used most recently again
	create(other, "Someone else's ticket", w.hr.ID)
	e.seedTicket(w.p, w.pmUser, "On a menu the PM cannot see", nil, w.secret)
	if _, err := e.d.Pool.Exec(context.Background(), "UPDATE nodes SET archived_at = now() WHERE id = $1", archived.ID); err != nil {
		t.Fatal(err)
	}
	var got httpapi.RecentNodes
	if code := e.call(w.pm, http.MethodGet, "/projects/HRIS/nodes/recent", nil, &got); code != http.StatusOK ||
		!slices.Equal(got.NodeIds, []int64{w.ot.ID, leave.ID}) {
		t.Fatalf("recent: %d %v, want [%d %d]", code, got.NodeIds, w.ot.ID, leave.ID)
	}
	if code := e.call(other, http.MethodGet, "/projects/HRIS/nodes/recent", nil, &got); code != http.StatusOK || !slices.Equal(got.NodeIds, []int64{w.hr.ID}) {
		t.Fatalf("another member's recent: %d %v", code, got.NodeIds)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd server && go test ./internal/httpapi/ -run RecentNodes`
Expected: a compile error: `httpapi.RecentNodes` is undefined.

- [ ] **Step 3: Implement**

`api/openapi.yaml`:

```diff
diff --git a/api/openapi.yaml b/api/openapi.yaml
--- a/api/openapi.yaml
+++ b/api/openapi.yaml
@@ -412,6 +412,22 @@ paths:
             application/json:
               schema: { $ref: "#/components/schemas/Node" }
         default: { $ref: "#/components/responses/Problem" }
+  /projects/{key}/nodes/recent:
+    parameters:
+      - { name: key, in: path, required: true, schema: { type: string } }
+    get:
+      operationId: listRecentNodes
+      tags: [nodes]
+      description: >-
+        The menus and modules of the caller's own latest tickets in the project, most recent first, at most 8, for
+        "recently used first" in the menu picker (FSD §8.1). Only live nodes the caller sees.
+      responses:
+        "200":
+          description: Node ids, most recent first.
+          content:
+            application/json:
+              schema: { $ref: "#/components/schemas/RecentNodes" }
+        default: { $ref: "#/components/responses/Problem" }
   /nodes/{id}:
     parameters:
       - { name: id, in: path, required: true, schema: { type: integer, format: int64 } }
@@ -1484,3 +1500,10 @@ components:
         items:
           type: array
           items: { $ref: "#/components/schemas/RecentTicket" }
+    RecentNodes:
+      type: object
+      required: [node_ids]
+      properties:
+        node_ids:
+          type: array
+          items: { type: integer, format: int64 }
```

`server/internal/db/queries/nodes.sql`:

```diff
diff --git a/server/internal/db/queries/nodes.sql b/server/internal/db/queries/nodes.sql
--- a/server/internal/db/queries/nodes.sql
+++ b/server/internal/db/queries/nodes.sql
@@ -114,3 +114,31 @@ WHERE id = ANY (sqlc.arg('ids')::bigint[]);
 
 -- name: DeleteNode :exec
 DELETE FROM nodes WHERE id = $1;
+
+-- name: ListRecentNodes :many
+-- The live menus and modules of the user's own latest tickets in the project,
+-- most recent first, for "recently used first" in the menu picker (FSD §8.1).
+-- Same visibility as ListNodes: a client-specific node, or anything under one,
+-- needs one of its clients in scope (R-AC-5).
+WITH RECURSIVE visible AS (
+  SELECT n.id FROM nodes n
+  WHERE n.project_id = sqlc.arg('project_id') AND n.parent_id IS NULL AND n.archived_at IS NULL
+    AND (NOT n.client_specific OR sqlc.arg('all_clients')::boolean
+         OR EXISTS (SELECT 1 FROM node_clients nc
+                    WHERE nc.node_id = n.id AND nc.client_id = ANY (sqlc.arg('client_ids')::bigint[])))
+  UNION
+  SELECT n.id FROM nodes n
+  JOIN visible v ON n.parent_id = v.id
+  WHERE n.archived_at IS NULL
+    AND (NOT n.client_specific OR sqlc.arg('all_clients')::boolean
+         OR EXISTS (SELECT 1 FROM node_clients nc
+                    WHERE nc.node_id = n.id AND nc.client_id = ANY (sqlc.arg('client_ids')::bigint[])))
+)
+SELECT tn.node_id
+FROM ticket_nodes tn
+JOIN tickets t ON t.id = tn.ticket_id
+JOIN visible v ON v.id = tn.node_id
+WHERE t.project_id = sqlc.arg('project_id') AND t.reporter_id = sqlc.arg('reporter_id')
+GROUP BY tn.node_id
+ORDER BY max(t.created_at) DESC, tn.node_id DESC
+LIMIT 8;
```

Then regenerate: `make generate`

`server/internal/httpapi/nodes.go`:

```diff
diff --git a/server/internal/httpapi/nodes.go b/server/internal/httpapi/nodes.go
--- a/server/internal/httpapi/nodes.go
+++ b/server/internal/httpapi/nodes.go
@@ -403,3 +403,20 @@ func nodeAudit(n db.Node, clients []db.ListNodeClientsRow) map[string]any {
 		"archived": n.ArchivedAt != nil,
 	}
 }
+
+// ListRecentNodes lists the nodes of the caller's own latest tickets, so the
+// menu picker can put them first (FSD §8.1).
+func (s *Server) ListRecentNodes(w http.ResponseWriter, r *http.Request, key string) {
+	pc, ok := s.projectFor(w, r, key, access.Viewer)
+	if !ok {
+		return
+	}
+	ids, err := s.q.ListRecentNodes(r.Context(), db.ListRecentNodesParams{
+		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs), ReporterID: pc.user.ID,
+	})
+	if err != nil {
+		s.fail(w, r, err)
+		return
+	}
+	writeJSON(w, http.StatusOK, RecentNodes{NodeIds: orEmpty(ids)})
+}
```

`web/components/NodePicker.tsx`:

```diff
diff --git a/web/components/NodePicker.tsx b/web/components/NodePicker.tsx
--- a/web/components/NodePicker.tsx
+++ b/web/components/NodePicker.tsx
@@ -12,16 +12,23 @@ type Props = {
   selected: Set<number>;
   onToggle: (id: number, on: boolean) => void;
   legend: string; // read by screen readers; the visible label sits beside the picker
+  recent?: number[]; // the user's recently used nodes, most recent first
 };
 
 // The menu picker of the ticket form and the close dialog (FSD §8.3): a filter
 // by path, alias or code, then a checkbox per menu or module, named by its path.
-export default function NodePicker({ nodes, selected, onToggle, legend }: Props) {
+// Recently used menus come first, marked "Recent" (§8.1).
+export default function NodePicker({ nodes, selected, onToggle, legend, recent = [] }: Props) {
   const t = useTranslations("ticketForm");
   const [filter, setFilter] = useState("");
   const pathOf = useMemo(() => nodePaths(nodes), [nodes]);
   const q = filter.trim().toLowerCase();
-  const choices = nodes.filter((n) => !q || [pathOf(n.id), n.code ?? "", ...n.aliases].some((s) => s.toLowerCase().includes(q)));
+  const rank = (id: number) => (recent.includes(id) ? recent.indexOf(id) : recent.length);
+  const choices = nodes
+    .filter((n) => !q || [pathOf(n.id), n.code ?? "", ...n.aliases].some((s) => s.toLowerCase().includes(q)))
+    .map((n, i) => ({ n, i }))
+    .sort((a, b) => rank(a.n.id) - rank(b.n.id) || a.i - b.i) // stable: the tree order stays within each group
+    .map(({ n }) => n);
   return (
     <fieldset className="flex flex-col gap-2">
       <legend className="sr-only">{legend}</legend>
@@ -40,6 +47,7 @@ export default function NodePicker({ nodes, selected, onToggle, legend }: Props)
           <label key={n.id} className={cx("flex items-center gap-2 border-b border-line-soft px-2.5 py-1.5 last:border-0", selected.has(n.id) && "bg-accent-soft")}>
             <input type="checkbox" checked={selected.has(n.id)} onChange={(e) => onToggle(n.id, e.target.checked)} className="size-4 accent-accent" />
             {pathOf(n.id)}
+            {recent.includes(n.id) && <span className="rounded-[3px] bg-paper px-1.5 text-[11px] font-semibold text-muted">{t("recent")}</span>}
             {n.code && <span className="ml-auto font-mono text-[11px] text-muted">{n.code}</span>}
           </label>
         ))}
```

`web/components/TicketForm.tsx`:

```diff
diff --git a/web/components/TicketForm.tsx b/web/components/TicketForm.tsx
--- a/web/components/TicketForm.tsx
+++ b/web/components/TicketForm.tsx
@@ -60,8 +60,14 @@ export default function TicketForm({ projectKey, clients, nodes, assignees, tick
   const [adding, setAdding] = useState(false);
   const [newName, setNewName] = useState("");
   const [newTitle, setNewTitle] = useState("");
-const [nodeIds, setNodeIds] = useState<Set<number>>(() => new Set(ticket ? ticket.nodes.map((n) => n.id) : nodeId ? [nodeId] : []));
-const [reason, setReason] = useState(ticket?.reason ?? "");
+  const [nodeIds, setNodeIds] = useState<Set<number>>(() => new Set(ticket ? ticket.nodes.map((n) => n.id) : nodeId ? [nodeId] : []));
+  const [recent, setRecent] = useState<number[]>([]);
+  useEffect(() => {
+    // Recently used menus first (§8.1); the picker works without them.
+    api.GET("/projects/{key}/nodes/recent", { params: { path: { key: projectKey } } }).then(({ data }) => setRecent(data?.node_ids ?? []));
+  }, [projectKey]);
+  const [reason, setReason] = useState(ticket?.reason ?? "");
+  const [type, setType] = useState<TicketType>(ticket?.type ?? "change_request");
   const [error, setError] = useState("");
   const [stale, setStale] = useState(false);
   const [notice, setNotice] = useState("");
@@ -243,7 +249,7 @@ const [reason, setReason] = useState(ticket?.reason ?? "");
           <input id="tf-title" name="title" defaultValue={ticket?.title} required minLength={5} maxLength={200} className={field.input} />
         </Row>
         <Row label={t("menus")} id="tf-menus">
-          <NodePicker nodes={nodes} selected={nodeIds} onToggle={toggleNode} legend={t("menus")} />
+          <NodePicker nodes={nodes} selected={nodeIds} onToggle={toggleNode} legend={t("menus")} recent={recent} />
           <p className={field.hint}>{t("menusHint")}</p>
           {warnings.map((n) => (
             <p key={n.id} className="flex items-center gap-1.5 text-xs text-warn">
@@ -268,7 +274,7 @@ const [reason, setReason] = useState(ticket?.reason ?? "");
           </p>
         </Row>
         <Row label={t("type")} id="tf-type">
-          <div role="radiogroup" aria-labelledby="tf-type" className="flex flex-wrap">
+          <div role="radiogroup" aria-labelledby="tf-type" className="flex flex-wrap" onChange={(e) => setType((e.target as HTMLInputElement).value as TicketType)}>
             {types.map((ty, i) => (
               <label
                 key={ty}
@@ -283,6 +289,20 @@ const [reason, setReason] = useState(ticket?.reason ?? "");
               </label>
             ))}
           </div>
+          {type === "bug" && nodeIds.size > 0 && (
+            // A "bug" may be the agreed behavior for this client: the menu's Behaviors tab says (§7.4, story 5).
+            <p className={field.hint}>
+              {t("bugBehaviors")}{" "}
+              {nodes
+                .filter((n) => nodeIds.has(n.id))
+                .map((n, i) => (
+                  <span key={n.id}>
+                    {i > 0 && ", "}
+                    <a href={`/p/${projectKey}/modules/${n.id}?tab=behaviors`} target="_blank" rel="noopener">{n.name}</a>
+                  </span>
+                ))}
+            </p>
+          )}
         </Row>
         <Row label={t("description")} htmlFor="tf-description">
           <textarea
```

`web/messages/en.json`:

```diff
diff --git a/web/messages/en.json b/web/messages/en.json
--- a/web/messages/en.json
+++ b/web/messages/en.json
@@ -411,7 +411,9 @@
     "reload": "Reload",
     "descriptionHint": "Markdown works. Paste an image to attach it to the ticket.",
     "descriptionHintNew": "Markdown works. Images can be pasted once the ticket exists.",
-    "pasteAfterCreate": "Create the ticket first; then paste the image into its description or a comment."
+    "pasteAfterCreate": "Create the ticket first; then paste the image into its description or a comment.",
+    "recent": "Recent",
+    "bugBehaviors": "Is it a bug, or how the menu was agreed to work? See the behaviors of"
   },
   "ticket": {
     "edit": "Edit",
```

`web/messages/id.json`:

```diff
diff --git a/web/messages/id.json b/web/messages/id.json
--- a/web/messages/id.json
+++ b/web/messages/id.json
@@ -411,7 +411,9 @@
     "reload": "Muat ulang",
     "descriptionHint": "Mendukung Markdown. Tempel gambar untuk melampirkannya ke tiket.",
     "descriptionHintNew": "Mendukung Markdown. Gambar bisa ditempel setelah tiket dibuat.",
-    "pasteAfterCreate": "Buat tiketnya dulu, lalu tempel gambar di deskripsi atau komentar."
+    "pasteAfterCreate": "Buat tiketnya dulu, lalu tempel gambar di deskripsi atau komentar.",
+    "recent": "Terakhir",
+    "bugBehaviors": "Bug, atau memang perilaku yang sudah disepakati? Lihat perilaku"
   },
   "ticket": {
     "edit": "Ubah",
```

- [ ] **Step 4: Run the tests**

Run: `cd server && gofmt -l . && go vet ./...`, then `cd server && go test ./internal/httpapi/ -run RecentNodes`

Expected: no gofmt output, no vet findings, and `ok` (or every test passed).

Then `cd web && npm run build`: a clean build.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: recently used menus first in the ticket form"
```

### Task 3: Create modal and the `c` shortcut

**Files:**
- Create: `web/app/@modal/(.)p/[key]/tickets/new/NewTicketModal.tsx`, `web/app/@modal/(.)p/[key]/tickets/new/page.tsx`, `web/app/@modal/[...catchAll]/page.tsx`, `web/app/@modal/default.tsx`, `web/app/p/[key]/tickets/new/data.ts`
- Modify: `web/app/TopBar.tsx`, `web/app/layout.tsx`, `web/app/p/[key]/tickets/new/page.tsx`, `web/components/Menu.tsx`, `web/messages/en.json`, `web/messages/id.json`

**Interfaces:**
- Consumes: `TicketForm` and its `onCancel` prop (Iteration 2).
- Produces:
  - A `@modal` parallel route in the root layout. `app/@modal/(.)p/[key]/tickets/new` intercepts client-side navigation to the create page and shows the form in a native `<dialog>` over the current page. A direct load or a reload still shows the full page. `@modal/default.tsx` and `@modal/[...catchAll]/page.tsx` render nothing, so the dialog closes on any other navigation.
  - `newTicketData(key)` (`app/p/[key]/tickets/new/data.ts`): the loader shared by the page and the modal.
  - Closing (×, Batal, Escape, a click on the backdrop) calls `router.back()`. Creating goes to the new ticket.
  - `c` anywhere but a text field, a select or an open dialog, and without modifiers: inside a project it opens that project's create modal. Elsewhere it opens it straight away when the user can create in exactly one project, or else opens the New ticket menu and focuses its first item. The button has `aria-keyshortcuts="c"`.
  - `Menu` takes an `id`.
  - Messages `ticketForm.newIn`, `ticketForm.close`, `nav.newTicketShortcut`.

- [ ] **Step 1: Implement**

`web/app/@modal/(.)p/[key]/tickets/new/NewTicketModal.tsx` (new):

```tsx
"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import Icon from "@/components/Icon";
import TicketForm from "@/components/TicketForm";
import type { Client, Node, Ref } from "@/lib/problem";

type Props = {
  title: string;
  closeLabel: string;
  projectKey: string;
  clients: Client[];
  nodes: Node[];
  assignees: Ref[];
  statusId?: number;
  nodeId?: number;
};

// A native <dialog> around the create form. Closing it (×, Cancel, Escape or
// a click on the backdrop) goes back, so the page under it stays as it was.
export default function NewTicketModal({ title, closeLabel, ...form }: Props) {
  const router = useRouter();
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (!ref.current?.open) ref.current?.showModal();
  }, []);
  return (
    <dialog
      ref={ref}
      aria-labelledby="new-ticket-title"
      onCancel={(e) => {
        e.preventDefault();
        router.back();
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) router.back(); // the backdrop
      }}
      className="m-auto max-h-[92vh] w-[min(820px,96vw)] overflow-y-auto rounded border border-line bg-white p-0 text-ink shadow-xl backdrop:bg-ink/40"
    >
      <div className="sticky top-0 z-10 flex items-center gap-2 border-b border-line bg-white px-5 py-3">
        <h2 id="new-ticket-title" className="text-base font-semibold">{title}</h2>
        <button type="button" onClick={() => router.back()} aria-label={closeLabel} className="ml-auto rounded p-1 text-muted hover:bg-paper hover:text-ink">
          <Icon name="x" className="size-4" />
        </button>
      </div>
      <TicketForm {...form} onCancel={() => router.back()} />
    </dialog>
  );
}
```

`web/app/@modal/(.)p/[key]/tickets/new/page.tsx` (new):

```tsx
import { getTranslations } from "next-intl/server";
import { newTicketData } from "@/app/p/[key]/tickets/new/data";
import NewTicketModal from "./NewTicketModal";

// The create form as a modal over the current page (FSD §8.3): a link or the
// `c` shortcut to /p/{key}/tickets/new lands here on client navigation; a
// reload or a shared link opens the full page instead.
export default async function NewTicketModalPage({
  params,
  searchParams,
}: {
  params: Promise<{ key: string }>;
  searchParams: Promise<{ status_id?: string; node_id?: string }>;
}) {
  const { key } = await params;
  const { status_id, node_id } = await searchParams;
  const data = await newTicketData(key);
  if (!data || data.project.role === "viewer") return null;
  const t = await getTranslations("ticketForm");
  return (
    <NewTicketModal
      title={t("newIn", { project: data.project.key })}
      closeLabel={t("close")}
      projectKey={key}
      clients={data.clients}
      nodes={data.nodes}
      assignees={data.assignees}
      statusId={status_id ? Number(status_id) : undefined}
      nodeId={node_id ? Number(node_id) : undefined}
    />
  );
}
```

`web/app/@modal/[...catchAll]/page.tsx` (new):

```tsx
// Any other page closes the modal: without this, a soft navigation away from
// the create modal (after "Create") would keep it on screen.
export default function NoModal() {
  return null;
}
```

`web/app/@modal/default.tsx` (new):

```tsx
// No modal unless a route below opens one.
export default function NoModal() {
  return null;
}
```

`web/app/TopBar.tsx`:

```diff
diff --git a/web/app/TopBar.tsx b/web/app/TopBar.tsx
--- a/web/app/TopBar.tsx
+++ b/web/app/TopBar.tsx
@@ -2,6 +2,7 @@
 
 import Link from "next/link";
 import Form from "next/form";
+import { useEffect } from "react";
 import { usePathname, useRouter } from "next/navigation";
 import { useTranslations } from "next-intl";
 import { Avatar } from "@/components/Chips";
@@ -29,6 +30,27 @@ export default function TopBar({ me, projects }: { me: User; projects: Project[]
   const project = projects.find((p) => p.key === currentKey(path));
   // Off a project page, New ticket asks which project (FSD §6.1).
   const creatable = projects.filter((p) => p.role !== "viewer");
+  const newTicketKey = project ? (project.role !== "viewer" ? project.key : undefined) : creatable.length === 1 ? creatable[0].key : undefined;
+
+  // The `c` shortcut opens New ticket from anywhere (§6.1, §8.3), unless the
+  // user is typing or a dialog is open. With several projects to choose from,
+  // it opens the project menu instead.
+  useEffect(() => {
+    const onKey = (e: KeyboardEvent) => {
+      if (e.key !== "c" || e.ctrlKey || e.metaKey || e.altKey || e.repeat) return;
+      const el = e.target as HTMLElement;
+      if (el.closest("input, textarea, select, [contenteditable=true], dialog[open]") || document.querySelector("dialog[open]")) return;
+      e.preventDefault();
+      if (newTicketKey) return router.push(`/p/${newTicketKey}/tickets/new`);
+      const menu = document.getElementById("new-ticket-menu") as HTMLDetailsElement | null;
+      if (menu) {
+        menu.open = true;
+        menu.querySelector<HTMLAnchorElement>("a")?.focus();
+      }
+    };
+    document.addEventListener("keydown", onKey);
+    return () => document.removeEventListener("keydown", onKey);
+  }, [newTicketKey, router]);
 
   async function setLocale(locale: "id" | "en") {
     if (locale === me.locale) return;
@@ -116,19 +138,20 @@ export default function TopBar({ me, projects }: { me: User; projects: Project[]
           </Form>
           {project ? (
             project.role !== "viewer" && (
-              <Link href={`/p/${project.key}/tickets/new`} aria-label={t("newTicket")} className={button.primary}>
+              <Link href={`/p/${project.key}/tickets/new`} aria-label={t("newTicket")} aria-keyshortcuts="c" title={t("newTicketShortcut")} className={button.primary}>
                 <Icon name="plus" />
                 <span className="hidden sm:inline">{t("newTicket")}</span>
               </Link>
             )
           ) : creatable.length === 1 ? (
-            <Link href={`/p/${creatable[0].key}/tickets/new`} aria-label={t("newTicket")} className={button.primary}>
+            <Link href={`/p/${creatable[0].key}/tickets/new`} aria-label={t("newTicket")} aria-keyshortcuts="c" title={t("newTicketShortcut")} className={button.primary}>
               <Icon name="plus" />
               <span className="hidden sm:inline">{t("newTicket")}</span>
             </Link>
           ) : (
             creatable.length > 1 && (
               <Menu
+                id="new-ticket-menu"
                 align="right"
                 label={t("newTicket")}
                 summaryClassName={button.primary}
```

`web/app/layout.tsx`:

```diff
diff --git a/web/app/layout.tsx b/web/app/layout.tsx
--- a/web/app/layout.tsx
+++ b/web/app/layout.tsx
@@ -12,7 +12,9 @@ const mono = IBM_Plex_Mono({ subsets: ["latin"], weight: ["500", "600"], variabl
 
 export const metadata: Metadata = { title: "Muasal" };
 
-export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
+// modal is the parallel route of the create-ticket modal (@modal), which opens
+// over the current page (FSD §8.3).
+export default async function RootLayout({ children, modal }: Readonly<{ children: React.ReactNode; modal: React.ReactNode }>) {
   const locale = await getLocale();
   return (
     <html lang={locale} className={`${sans.variable} ${mono.variable}`}>
@@ -20,6 +22,7 @@ export default async function RootLayout({ children }: Readonly<{ children: Reac
         <NextIntlClientProvider>
           <Header />
           {children}
+          {modal}
         </NextIntlClientProvider>
       </body>
     </html>
```

`web/app/p/[key]/tickets/new/data.ts` (new):

```ts
import { getProject, serverApi } from "@/lib/server-api";

// What the create form needs, for the full page and the modal alike.
export async function newTicketData(key: string) {
  const project = await getProject(key);
  if (!project) return null;
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, nodes, assignees] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/nodes", path),
    api.GET("/projects/{key}/assignees", path),
  ]);
  return {
    project,
    clients: clients.data?.items ?? [],
    nodes: nodes.data?.items ?? [],
    assignees: assignees.data?.items ?? [],
  };
}
```

`web/app/p/[key]/tickets/new/page.tsx`:

```diff
diff --git a/web/app/p/[key]/tickets/new/page.tsx b/web/app/p/[key]/tickets/new/page.tsx
--- a/web/app/p/[key]/tickets/new/page.tsx
+++ b/web/app/p/[key]/tickets/new/page.tsx
@@ -4,8 +4,8 @@ import { getTranslations } from "next-intl/server";
 import Icon from "@/components/Icon";
 import PageBar from "@/components/PageBar";
 import TicketForm from "@/components/TicketForm";
-import { getProject, serverApi } from "@/lib/server-api";
 import { panel } from "@/lib/ui";
+import { newTicketData } from "./data";
 
 export default async function NewTicketPage({
   params,
@@ -16,17 +16,11 @@ export default async function NewTicketPage({
 }) {
   const { key } = await params;
   const { status_id, node_id } = await searchParams;
-  const project = await getProject(key);
-  if (!project) notFound();
+  const data = await newTicketData(key);
+  if (!data) notFound();
+  const { project, clients, nodes, assignees } = data;
   const t = await getTranslations("ticketForm");
   const tp = await getTranslations("project");
-  const api = await serverApi();
-  const path = { params: { path: { key } } };
-  const [clients, nodes, assignees] = await Promise.all([
-    api.GET("/projects/{key}/clients", path),
-    api.GET("/projects/{key}/nodes", path),
-    api.GET("/projects/{key}/assignees", path),
-  ]);
   return (
     <>
       <PageBar>
@@ -45,9 +39,9 @@ export default async function NewTicketPage({
           <div className={`${panel} mx-auto max-w-[820px]`}>
             <TicketForm
               projectKey={key}
-              clients={clients.data?.items ?? []}
-              nodes={nodes.data?.items ?? []}
-              assignees={assignees.data?.items ?? []}
+              clients={clients}
+              nodes={nodes}
+              assignees={assignees}
               statusId={status_id ? Number(status_id) : undefined}
               nodeId={node_id ? Number(node_id) : undefined}
             />
```

`web/components/Menu.tsx`:

```diff
diff --git a/web/components/Menu.tsx b/web/components/Menu.tsx
--- a/web/components/Menu.tsx
+++ b/web/components/Menu.tsx
@@ -8,12 +8,13 @@ type Props = {
   summary: React.ReactNode;
   summaryClassName?: string;
   align?: "left" | "right";
+  id?: string;
   children: React.ReactNode;
 };
 
 // A dropdown on native <details>, so the summary is a real button for keyboards
 // and screen readers. A click outside, Escape or following a link closes it.
-export default function Menu({ label, summary, summaryClassName, align = "left", children }: Props) {
+export default function Menu({ label, summary, summaryClassName, align = "left", id, children }: Props) {
   const ref = useRef<HTMLDetailsElement>(null);
   useEffect(() => {
     const close = () => {
@@ -36,7 +37,7 @@ export default function Menu({ label, summary, summaryClassName, align = "left",
     };
   }, []);
   return (
-    <details ref={ref} className="relative">
+    <details ref={ref} id={id} className="relative">
       <summary aria-label={label} className={cx("cursor-pointer list-none", summaryClassName)}>
         {summary}
       </summary>
```

`web/messages/en.json`:

```diff
diff --git a/web/messages/en.json b/web/messages/en.json
--- a/web/messages/en.json
+++ b/web/messages/en.json
@@ -32,7 +32,8 @@
     "search": "Search tickets or menus",
     "searchPlaceholder": "Search, or type HRIS-231",
     "language": "Language",
-    "account": "Account of {name}"
+    "account": "Account of {name}",
+    "newTicketShortcut": "New ticket (C)"
   },
   "search": {
     "heading": "Search",
@@ -413,7 +414,9 @@
     "descriptionHintNew": "Markdown works. Images can be pasted once the ticket exists.",
     "pasteAfterCreate": "Create the ticket first; then paste the image into its description or a comment.",
     "recent": "Recent",
-    "bugBehaviors": "Is it a bug, or how the menu was agreed to work? See the behaviors of"
+    "bugBehaviors": "Is it a bug, or how the menu was agreed to work? See the behaviors of",
+    "newIn": "New ticket in {project}",
+    "close": "Close"
   },
   "ticket": {
     "edit": "Edit",
```

`web/messages/id.json`:

```diff
diff --git a/web/messages/id.json b/web/messages/id.json
--- a/web/messages/id.json
+++ b/web/messages/id.json
@@ -32,7 +32,8 @@
     "search": "Cari tiket atau menu",
     "searchPlaceholder": "Cari tiket, menu, atau HRIS-231",
     "language": "Bahasa",
-    "account": "Akun {name}"
+    "account": "Akun {name}",
+    "newTicketShortcut": "Tiket baru (C)"
   },
   "search": {
     "heading": "Pencarian",
@@ -413,7 +414,9 @@
     "descriptionHintNew": "Mendukung Markdown. Gambar bisa ditempel setelah tiket dibuat.",
     "pasteAfterCreate": "Buat tiketnya dulu, lalu tempel gambar di deskripsi atau komentar.",
     "recent": "Terakhir",
-    "bugBehaviors": "Bug, atau memang perilaku yang sudah disepakati? Lihat perilaku"
+    "bugBehaviors": "Bug, atau memang perilaku yang sudah disepakati? Lihat perilaku",
+    "newIn": "Tiket baru di {project}",
+    "close": "Tutup"
   },
   "ticket": {
     "edit": "Ubah",
```

- [ ] **Step 2: Run the tests**

Run: `cd web && npx tsc --noEmit && npm run build`

Expected: no type errors and a clean build.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat(web): create a ticket in a modal, opened with c"
```

### Task 4: Module tree drag and drop

**Files:**
- Modify: `web/app/p/[key]/modules/ModuleTree.tsx`, `web/components/Icon.tsx`, `web/messages/en.json`, `web/messages/id.json`

**Interfaces:**
- Consumes: `PATCH /nodes/{id}` with `move{parent_id, position}` (Iteration 1).
- Produces:
  - Each live row has a grip handle for project admins. dnd-kit's `PointerSensor` needs a 6 px move before it starts; the `KeyboardSensor` works with Space, the arrows and Space again.
  - The drop point is the pointer, or the handle's centre with the keyboard (`pointOf`). A custom collision check (`underPoint`) uses the same point, so the target row and the drop position always agree. The top third of a row means before it, the bottom third after it, the middle inside it as its last child. A marker line or outline shows the drop while dragging.
  - A drop onto the node itself or its own subtree does nothing. Before and after compute `position` among the target's live siblings, not counting the dragged node.
  - Screen-reader announcements and instructions in both languages (`modules.dnd.*`), the handle label `modules.dragHandle`, and a `grip` icon.
  - The parent picker and the up and down buttons stay as the non-drag way to move nodes.

- [ ] **Step 1: Implement**

Add the dependencies: `cd web && npm install --save-exact @dnd-kit/core@6.3.1`

`web/app/p/[key]/modules/ModuleTree.tsx`:

```diff
diff --git a/web/app/p/[key]/modules/ModuleTree.tsx b/web/app/p/[key]/modules/ModuleTree.tsx
--- a/web/app/p/[key]/modules/ModuleTree.tsx
+++ b/web/app/p/[key]/modules/ModuleTree.tsx
@@ -1,6 +1,22 @@
 "use client";
 
 import { useMemo, useState } from "react";
+import {
+  DndContext,
+  DragOverlay,
+  KeyboardSensor,
+  PointerSensor,
+  pointerWithin,
+  useDraggable,
+  useDroppable,
+  useSensor,
+  useSensors,
+  type Active,
+  type Announcements,
+  type CollisionDetection,
+  type DragEndEvent,
+  type DragMoveEvent,
+} from "@dnd-kit/core";
 import Link from "next/link";
 import { useRouter } from "next/navigation";
 import { useTranslations } from "next-intl";
@@ -37,8 +53,88 @@ function subtree(id: number, children: Children): Set<number> {
 
 type Props = { projectKey: string; nodes: Node[]; clients: Client[]; canEdit: boolean; showArchived: boolean };
 
-// The module tree screen (FSD §7.3): the tree on the left, the selected node on the right.
-// ponytail: moves use a parent picker and up/down buttons; add tree drag-and-drop if admins ask for it.
+// Where a dragged node would land: before or after a row, or inside it as its last child.
+type Drop = { id: number; where: "before" | "inside" | "after" };
+
+// pointOf is where a drag points: the pointer for a mouse or touch, and the
+// handle's centre for the keyboard. dnd-kit's own collision rect is the
+// overlay's, which is larger than the handle and offset by the activation move.
+function pointOf(e: { active: Active; activatorEvent: Event; delta: { x: number; y: number } }) {
+  if ("clientX" in e.activatorEvent) {
+    const p = e.activatorEvent as PointerEvent;
+    return { x: p.clientX + e.delta.x, y: p.clientY + e.delta.y };
+  }
+  const start = e.active.rect.current.initial;
+  return start && { x: start.left + start.width / 2 + e.delta.x, y: start.top + start.height / 2 + e.delta.y };
+}
+
+// dropOf reads the drop from where the drag points over the target row: its
+// top third means before, its bottom third after, the middle inside.
+function dropOf(e: DragMoveEvent | DragEndEvent): Drop | null {
+  const over = e.over;
+  const at = pointOf(e);
+  if (!over || !at || over.id === e.active.id) return null;
+  const y = (at.y - over.rect.top) / over.rect.height;
+  return { id: Number(over.id), where: y < 1 / 3 ? "before" : y > 2 / 3 ? "after" : "inside" };
+}
+
+// underPoint finds the row under that same point.
+const underPoint: CollisionDetection = (args) => {
+  if (args.pointerCoordinates) return pointerWithin(args);
+  const start = args.active.rect.current.initial;
+  if (!start) return [];
+  const at = { x: args.collisionRect.left + start.width / 2, y: args.collisionRect.top + start.height / 2 };
+  return pointerWithin({ ...args, pointerCoordinates: at });
+};
+
+// A tree row: the drag handle for project admins, and a drop target for every
+// live node (FSD §7.3).
+function TreeRow({ n, canEdit, drop, handleLabel, className, style, children }: {
+  n: Node;
+  canEdit: boolean;
+  drop: Drop | null;
+  handleLabel: string;
+  className: string;
+  style: React.CSSProperties;
+  children: React.ReactNode;
+}) {
+  const drag = useDraggable({ id: n.id, disabled: !canEdit || n.archived });
+  const target = useDroppable({ id: n.id, disabled: n.archived });
+  const here = drop?.id === n.id ? drop.where : null;
+  return (
+    <div
+      ref={target.setNodeRef}
+      className={cx(
+        "relative",
+        className,
+        drag.isDragging && "opacity-40",
+        here === "inside" && "outline-2 -outline-offset-2 outline-accent",
+      )}
+      style={style}
+    >
+      {here === "before" && <span aria-hidden className="absolute inset-x-0 top-0 h-0.5 bg-accent" />}
+      {canEdit && !n.archived && (
+        <button
+          ref={drag.setNodeRef}
+          type="button"
+          aria-label={handleLabel}
+          {...drag.attributes}
+          {...drag.listeners}
+          className="inline-flex size-5 shrink-0 cursor-grab touch-none items-center justify-center rounded text-muted hover:bg-well active:cursor-grabbing"
+        >
+          <Icon name="grip" className="size-3.5" />
+        </button>
+      )}
+      {children}
+      {here === "after" && <span aria-hidden className="absolute inset-x-0 bottom-0 h-0.5 bg-accent" />}
+    </div>
+  );
+}
+
+// The module tree screen (FSD §7.3): the tree on the left, the selected node on
+// the right. Project admins move nodes by dragging a row's handle, with the
+// pointer or the keyboard (Space, arrows, Space), or with the parent picker
+// and the up and down buttons.
 export default function ModuleTree({ projectKey, nodes, clients, canEdit, showArchived }: Props) {
   const t = useTranslations("modules");
   const problemText = useProblemText();
@@ -48,6 +144,9 @@ export default function ModuleTree({ projectKey, nodes, clients, canEdit, showAr
   const [filter, setFilter] = useState("");
   const [collapsed, setCollapsed] = useState<Set<number>>(() => new Set());
   const [error, setError] = useState("");
+  const [drop, setDrop] = useState<Drop | null>(null);
+  const [dragging, setDragging] = useState<number | null>(null);
+  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }), useSensor(KeyboardSensor));
 
   const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
   const children = useMemo(() => {
@@ -100,6 +199,35 @@ export default function ModuleTree({ projectKey, nodes, clients, canEdit, showAr
     router.refresh();
   }
 
+  // A drop becomes one move: inside a node puts it last among its children;
+  // before or after a node puts it at that place among the node's siblings,
+  // counted without the moved node, as the server counts (§7.1).
+  function onDragEnd(e: DragEndEvent) {
+    const d = dropOf(e);
+    setDrop(null);
+    const moved = byId.get(Number(e.active.id));
+    const target = d && byId.get(d.id);
+    if (!d || !moved || !target || subtree(moved.id, children).has(target.id)) return;
+    if (d.where === "inside") {
+      if (target.id === moved.parent_id) return;
+      return run(api.PATCH("/nodes/{id}", { params: { path: { id: moved.id } }, body: { move: { parent_id: target.id } } }));
+    }
+    const siblings = (children.get(target.parent_id) ?? []).filter((x) => !x.archived && x.id !== moved.id);
+    const position = siblings.findIndex((x) => x.id === target.id) + (d.where === "after" ? 1 : 0);
+    run(api.PATCH("/nodes/{id}", { params: { path: { id: moved.id } }, body: { move: { parent_id: target.parent_id, position } } }));
+  }
+
+  const announcements: Announcements = {
+    onDragStart: ({ active }) => t("dnd.start", { name: byId.get(Number(active.id))?.name ?? "" }),
+    onDragOver: ({ active, over }) =>
+      over
+        ? t("dnd.over", { name: byId.get(Number(active.id))?.name ?? "", target: byId.get(Number(over.id))?.name ?? "" })
+        : t("dnd.none", { name: byId.get(Number(active.id))?.name ?? "" }),
+    onDragEnd: ({ active, over }) =>
+      over ? t("dnd.end", { name: byId.get(Number(active.id))?.name ?? "", target: byId.get(Number(over.id))?.name ?? "" }) : t("dnd.cancel"),
+    onDragCancel: () => t("dnd.cancel"),
+  };
+
   function badge(n: Node) {
     if (n.type === "module") {
       return hasSpecific.has(n.id) ? <span className={cx(chip, "bg-accent-soft text-accent-strong")}>{t("hasClientSpecific")}</span> : null;
@@ -120,7 +248,11 @@ export default function ModuleTree({ projectKey, nodes, clients, canEdit, showAr
           return (
             <li key={n.id}>
               {/* R-MR-1: indentation stops growing after six levels. */}
-              <div
+              <TreeRow
+                n={n}
+                canEdit={canEdit}
+                drop={drop}
+                handleLabel={t("dragHandle", { name: n.name })}
                 className={cx("flex min-h-8 items-center gap-1.5 pr-3", current && "bg-accent-soft")}
                 style={{ paddingLeft: `${0.75 + Math.min(depth, 6) * 1.25}rem` }}
               >
@@ -154,7 +286,7 @@ export default function ModuleTree({ projectKey, nodes, clients, canEdit, showAr
                 </button>
                 {badge(n)}
                 {n.archived && <span className={cx(chip, "bg-well text-muted")}>{t("archivedBadge")}</span>}
-              </div>
+              </TreeRow>
               {open && level(n.id, depth + 1)}
             </li>
           );
@@ -297,7 +429,35 @@ export default function ModuleTree({ projectKey, nodes, clients, canEdit, showAr
           )}
         </div>
         <div className="py-1.5 text-[13px]">
-          {nodes.length === 0 ? <p className="px-3 py-2 text-muted">{canEdit ? t("emptyAdmin") : t("empty")}</p> : level(null, 0)}
+          {nodes.length === 0 ? (
+            <p className="px-3 py-2 text-muted">{canEdit ? t("emptyAdmin") : t("empty")}</p>
+          ) : (
+            <DndContext
+              sensors={sensors}
+              collisionDetection={underPoint}
+              accessibility={{ announcements, screenReaderInstructions: { draggable: t("dnd.instructions") } }}
+              onDragStart={(e) => setDragging(Number(e.active.id))}
+              onDragMove={(e) => setDrop(dropOf(e))}
+              onDragEnd={(e) => {
+                setDragging(null);
+                onDragEnd(e);
+              }}
+              onDragCancel={() => {
+                setDragging(null);
+                setDrop(null);
+              }}
+            >
+              {level(null, 0)}
+              <DragOverlay dropAnimation={null}>
+                {dragging !== null && (
+                  <span className="inline-flex items-center gap-1.5 rounded border border-accent bg-white px-2 py-1 text-[13px] font-semibold shadow">
+                    <Icon name={byId.get(dragging)?.type === "module" ? "folder" : "screen"} className="size-4 text-muted" />
+                    {byId.get(dragging)?.name}
+                  </span>
+                )}
+              </DragOverlay>
+            </DndContext>
+          )}
         </div>
       </section>
       <section aria-label={t("details")} className={cx(panel, "p-4")}>
```

`web/components/Icon.tsx`:

```diff
diff --git a/web/components/Icon.tsx b/web/components/Icon.tsx
--- a/web/components/Icon.tsx
+++ b/web/components/Icon.tsx
@@ -53,6 +53,16 @@ const paths = {
       <path d="m5.5 8 1.8 1.8 3.2-3.6" />
     </>
   ),
+  grip: (
+    <>
+      <circle cx="6" cy="4" r=".9" />
+      <circle cx="10" cy="4" r=".9" />
+      <circle cx="6" cy="8" r=".9" />
+      <circle cx="10" cy="8" r=".9" />
+      <circle cx="6" cy="12" r=".9" />
+      <circle cx="10" cy="12" r=".9" />
+    </>
+  ),
   upload: <path d="M8 10.5V3M5 6l3-3 3 3M3 12.5h10" />,
   arrowRight: <path d="M3.5 8h9M9 4.5 12.5 8 9 11.5" />,
 } satisfies Record<string, React.ReactNode>;
```

`web/messages/en.json`:

```diff
diff --git a/web/messages/en.json b/web/messages/en.json
--- a/web/messages/en.json
+++ b/web/messages/en.json
@@ -231,7 +231,16 @@
     "showArchived": "Show archived",
     "hideArchived": "Hide archived",
     "archivedBadge": "Archived",
-    "openPage": "Open page"
+    "openPage": "Open page",
+    "dragHandle": "Drag {name}",
+    "dnd": {
+      "instructions": "To move a menu or module, press Space, move it with the arrow keys, and press Space again to drop it above, below or inside another. Escape cancels.",
+      "start": "Picked up {name}.",
+      "over": "{name} is over {target}.",
+      "none": "{name} is not over a menu or module.",
+      "end": "Dropped {name} at {target}.",
+      "cancel": "Move cancelled."
+    }
   },
   "nodePage": {
     "path": "Path",
```

`web/messages/id.json`:

```diff
diff --git a/web/messages/id.json b/web/messages/id.json
--- a/web/messages/id.json
+++ b/web/messages/id.json
@@ -231,7 +231,16 @@
     "showArchived": "Tampilkan arsip",
     "hideArchived": "Sembunyikan arsip",
     "archivedBadge": "Diarsipkan",
-    "openPage": "Buka halaman"
+    "openPage": "Buka halaman",
+    "dragHandle": "Seret {name}",
+    "dnd": {
+      "instructions": "Untuk memindahkan menu atau modul, tekan Spasi, geser dengan tombol panah, lalu tekan Spasi lagi untuk meletakkannya di atas, di bawah, atau di dalam yang lain. Escape membatalkan.",
+      "start": "{name} diambil.",
+      "over": "{name} berada di atas {target}.",
+      "none": "{name} tidak berada di atas menu atau modul.",
+      "end": "{name} diletakkan di {target}.",
+      "cancel": "Pemindahan dibatalkan."
+    }
   },
   "nodePage": {
     "path": "Jejak",
```

- [ ] **Step 2: Run the tests**

Run: `cd web && npx tsc --noEmit && npm run build`

Expected: no type errors and a clean build.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat(web): drag and drop in the module tree"
```

### Task 5: Board drag with dnd-kit

**Files:**
- Modify: `web/app/p/[key]/board/Board.tsx`, `web/e2e/decisions.spec.ts`, `web/e2e/helpers.ts`, `web/e2e/tickets.spec.ts`

**Interfaces:**
- Consumes: the board's `move(ticketId, statusId)` (Iteration 2), unchanged: an open move is applied at once and rolled back on error, and a closing status opens the close dialog.
- Produces:
  - Columns are `useDroppable` (`status-{id}`, data `{statusId}`) and cards are `useDraggable` with pointer listeners only. Keyboards keep the status menu on each card. One drag library now serves the whole app.
  - A `DragOverlay` shows the card's key and title under the pointer.
  - `drag(page, source, target, to?)` in `web/e2e/helpers.ts`: a press, a small move to start the drag, then twelve steps to the target. Playwright's `dragTo` does not follow dnd-kit. The ticket and decision specs use it.

- [ ] **Step 1: Implement**

`web/app/p/[key]/board/Board.tsx`:

```diff
diff --git a/web/app/p/[key]/board/Board.tsx b/web/app/p/[key]/board/Board.tsx
--- a/web/app/p/[key]/board/Board.tsx
+++ b/web/app/p/[key]/board/Board.tsx
@@ -1,6 +1,7 @@
 "use client";
 
 import { useEffect, useState } from "react";
+import { DndContext, DragOverlay, PointerSensor, useDraggable, useDroppable, useSensor, useSensors } from "@dnd-kit/core";
 import Link from "next/link";
 import { useRouter } from "next/navigation";
 import { useLocale, useTranslations } from "next-intl";
@@ -24,7 +25,32 @@ type Props = {
 
 const closes = (s: Status) => s.category === "done" || s.category === "cancelled";
 
-// Cards move by native drag-and-drop or by their status menu, which keyboards
+// A status column that takes dropped cards.
+function Column({ status, canEdit, children }: { status: Status; canEdit: boolean; children: React.ReactNode }) {
+  const { setNodeRef, isOver } = useDroppable({ id: `status-${status.id}`, data: { statusId: status.id }, disabled: !canEdit });
+  return (
+    <section
+      ref={setNodeRef}
+      aria-label={status.name}
+      className={cx("flex w-[270px] shrink-0 flex-col gap-1.5 self-start rounded-md bg-well p-2", isOver && "outline-2 -outline-offset-2 outline-accent")}
+    >
+      {children}
+    </section>
+  );
+}
+
+// A card that the pointer drags. Keyboards use its status menu instead, so the
+// card itself stays a plain article with a link inside.
+function Card({ id, canEdit, className, children }: { id: number; canEdit: boolean; className: string; children: React.ReactNode }) {
+  const { setNodeRef, listeners, isDragging } = useDraggable({ id, disabled: !canEdit });
+  return (
+    <article ref={setNodeRef} {...listeners} className={cx(className, canEdit && "cursor-grab touch-none", isDragging && "opacity-40")}>
+      {children}
+    </article>
+  );
+}
+
+// Cards move by dragging (dnd-kit) or by their status menu, which keyboards
 // reach too. An open move shows at once and rolls back when the API refuses it;
 // a move into Done or Cancelled opens the close dialog, and the card stays
 // where it was until the close is confirmed (FSD §8.4, AC-TK-2).
@@ -38,6 +64,9 @@ export default function Board({ projectKey, statuses, tickets, nodes, canEdit, t
   const [items, setItems] = useState(tickets);
   const [dragging, setDragging] = useState<number | null>(null);
   const [error, setError] = useState("");
+  // A short move before a drag starts, so clicks on the title and the status menu still work.
+  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }));
+  const draggedCard = dragging === null ? undefined : items.find((x) => x.id === dragging);
   const [closing, setClosing] = useState<{ ticket: Ticket; status: Status } | null>(null);
   const showAll = query.closed === "all";
   const { closed: _closed, ...recent } = query;
@@ -69,102 +98,101 @@ export default function Board({ projectKey, statuses, tickets, nodes, canEdit, t
   return (
     <div className="flex flex-col gap-2">
       {error && <p role="alert" className={field.error}>{error}</p>}
-      <div className="flex gap-3 overflow-x-auto pb-2">
-        {statuses.map((s) => {
-          const cards = items.filter((x) => x.status_id === s.id);
-          const droppable = canEdit;
-          return (
-            <section
-              key={s.id}
-              aria-label={s.name}
-              className="flex w-[270px] shrink-0 flex-col gap-1.5 self-start rounded-md bg-well p-2"
-              onDragOver={droppable ? (e) => e.preventDefault() : undefined}
-              onDrop={
-                droppable
-                  ? (e) => {
-                      e.preventDefault();
-                      if (dragging !== null) move(dragging, s.id);
-                      setDragging(null);
-                    }
-                  : undefined
-              }
-            >
-              <h2
-                className="flex h-8 items-center gap-2 border-t-[3px] px-1 pt-1 text-xs font-semibold uppercase tracking-[0.04em]"
-                style={{ borderTopColor: s.color }}
-              >
-                <span className="truncate">{s.name}</span>
-                <span className="rounded-[3px] bg-white px-1.5 font-mono text-[11px] leading-5 text-muted">{cards.length}</span>
-                {droppable && (
-                  <Link
-                    href={`/p/${projectKey}/tickets/new?status_id=${s.id}`}
-                    aria-label={t("addHere", { status: s.name })}
-                    className="ml-auto inline-flex size-6 items-center justify-center rounded text-muted hover:bg-white hover:text-ink"
-                  >
-                    <Icon name="plus" />
-                  </Link>
-                )}
-              </h2>
-              {closes(s) && (
-                <p className="flex items-center gap-2 px-1 text-xs text-muted">
-                  {showAll ? t("allClosed") : t("recentClosed")}
-                  <Link href={`?${new URLSearchParams(showAll ? recent : { ...query, closed: "all" })}`} className="ml-auto">
-                    {showAll ? t("showRecent") : t("showAll")}
-                  </Link>
-                </p>
-              )}
-              {cards.map((c) => (
-                <article
-                  key={c.id}
-                  draggable={canEdit}
-                  onDragStart={() => setDragging(c.id)}
-                  onDragEnd={() => setDragging(null)}
-                  className={cx("flex flex-col gap-1.5 rounded border border-line bg-white px-2.5 py-2", canEdit && "cursor-grab")}
+      <DndContext
+        sensors={sensors}
+        onDragStart={(e) => setDragging(Number(e.active.id))}
+        onDragCancel={() => setDragging(null)}
+        onDragEnd={(e) => {
+          setDragging(null);
+          const statusId = e.over?.data.current?.statusId as number | undefined;
+          if (statusId !== undefined) move(Number(e.active.id), statusId);
+        }}
+      >
+        <div className="flex gap-3 overflow-x-auto pb-2">
+          {statuses.map((s) => {
+            const cards = items.filter((x) => x.status_id === s.id);
+            const droppable = canEdit;
+            return (
+              <Column key={s.id} status={s} canEdit={canEdit}>
+                <h2
+                  className="flex h-8 items-center gap-2 border-t-[3px] px-1 pt-1 text-xs font-semibold uppercase tracking-[0.04em]"
+                  style={{ borderTopColor: s.color }}
                 >
-                  <div className="flex items-center gap-1.5 text-xs">
-                    <TypeIcon type={c.type} label={tTypes(c.type)} />
-                    <span className="font-mono font-semibold">{c.key}</span>
-                    {(c.missing_reason || c.node_names.length === 0) && (
-                      <span role="img" title={t("missing")} aria-label={t("missing")} className="size-[7px] rounded-full bg-[#D97706]" />
-                    )}
-                    <span className="ml-auto">
-                      <PriorityChip priority={c.priority} label={tPri(c.priority)} />
-                    </span>
-                  </div>
-                  <Link href={`/t/${c.key}`} className="text-[13px] font-medium leading-snug text-ink no-underline hover:text-ink hover:underline">
-                    {c.title}
-                  </Link>
-                  {c.node_names.length > 0 && (
-                    <span className="truncate font-mono text-[11px] text-muted">
-                      {c.node_names[0]}
-                      {c.node_names.length > 1 ? ` +${c.node_names.length - 1}` : ""}
-                    </span>
-                  )}
-                  <div className="flex items-center gap-1.5 text-xs text-muted">
-                    <ClientChip client={c.client} coreLabel={t("noClient")} />
-                    {c.due_date && (
-                      <span className={c.due_date < today ? "font-medium text-danger" : ""}>{day(c.due_date, locale, c.due_date.slice(0, 4) !== today.slice(0, 4))}</span>
-                    )}
-                    {c.assignee && <Avatar name={c.assignee.name} className="ml-auto size-5 bg-well text-[9px] text-ink" />}
-                  </div>
-                  {canEdit && (
-                    <select
-                      aria-label={t("moveTo", { key: c.key })}
-                      value={c.status_id}
-                      onChange={(e) => move(c.id, Number(e.target.value))}
-                      className="mt-0.5 h-7 w-full rounded border border-line-soft bg-paper px-1 text-xs text-muted"
+                  <span className="truncate">{s.name}</span>
+                  <span className="rounded-[3px] bg-white px-1.5 font-mono text-[11px] leading-5 text-muted">{cards.length}</span>
+                  {droppable && (
+                    <Link
+                      href={`/p/${projectKey}/tickets/new?status_id=${s.id}`}
+                      aria-label={t("addHere", { status: s.name })}
+                      className="ml-auto inline-flex size-6 items-center justify-center rounded text-muted hover:bg-white hover:text-ink"
                     >
-                      {statuses.map((o) => (
-                        <option key={o.id} value={o.id}>{o.name}</option>
-                      ))}
-                    </select>
+                      <Icon name="plus" />
+                    </Link>
                   )}
-                </article>
-              ))}
-            </section>
-          );
-        })}
-      </div>
+                </h2>
+                {closes(s) && (
+                  <p className="flex items-center gap-2 px-1 text-xs text-muted">
+                    {showAll ? t("allClosed") : t("recentClosed")}
+                    <Link href={`?${new URLSearchParams(showAll ? recent : { ...query, closed: "all" })}`} className="ml-auto">
+                      {showAll ? t("showRecent") : t("showAll")}
+                    </Link>
+                  </p>
+                )}
+                {cards.map((c) => (
+                  <Card key={c.id} id={c.id} canEdit={canEdit} className="flex flex-col gap-1.5 rounded border border-line bg-white px-2.5 py-2">
+                    <div className="flex items-center gap-1.5 text-xs">
+                      <TypeIcon type={c.type} label={tTypes(c.type)} />
+                      <span className="font-mono font-semibold">{c.key}</span>
+                      {(c.missing_reason || c.node_names.length === 0) && (
+                        <span role="img" title={t("missing")} aria-label={t("missing")} className="size-[7px] rounded-full bg-[#D97706]" />
+                      )}
+                      <span className="ml-auto">
+                        <PriorityChip priority={c.priority} label={tPri(c.priority)} />
+                      </span>
+                    </div>
+                    <Link href={`/t/${c.key}`} className="text-[13px] font-medium leading-snug text-ink no-underline hover:text-ink hover:underline">
+                      {c.title}
+                    </Link>
+                    {c.node_names.length > 0 && (
+                      <span className="truncate font-mono text-[11px] text-muted">
+                        {c.node_names[0]}
+                        {c.node_names.length > 1 ? ` +${c.node_names.length - 1}` : ""}
+                      </span>
+                    )}
+                    <div className="flex items-center gap-1.5 text-xs text-muted">
+                      <ClientChip client={c.client} coreLabel={t("noClient")} />
+                      {c.due_date && (
+                        <span className={c.due_date < today ? "font-medium text-danger" : ""}>{day(c.due_date, locale, c.due_date.slice(0, 4) !== today.slice(0, 4))}</span>
+                      )}
+                      {c.assignee && <Avatar name={c.assignee.name} className="ml-auto size-5 bg-well text-[9px] text-ink" />}
+                    </div>
+                    {canEdit && (
+                      <select
+                        aria-label={t("moveTo", { key: c.key })}
+                        value={c.status_id}
+                        onChange={(e) => move(c.id, Number(e.target.value))}
+                        className="mt-0.5 h-7 w-full rounded border border-line-soft bg-paper px-1 text-xs text-muted"
+                      >
+                        {statuses.map((o) => (
+                          <option key={o.id} value={o.id}>{o.name}</option>
+                        ))}
+                      </select>
+                    )}
+                  </Card>
+                ))}
+              </Column>
+            );
+          })}
+        </div>
+        <DragOverlay dropAnimation={null}>
+          {draggedCard && (
+            <div className="flex w-[254px] flex-col gap-1 rounded border border-accent bg-white px-2.5 py-2 shadow-lg">
+              <span className="font-mono text-xs font-semibold">{draggedCard.key}</span>
+              <span className="text-[13px] font-medium leading-snug">{draggedCard.title}</span>
+            </div>
+          )}
+        </DragOverlay>
+      </DndContext>
       {closing && (
         <CloseDialog
           ticket={closing.ticket}
```

`web/e2e/decisions.spec.ts`:

```diff
diff --git a/web/e2e/decisions.spec.ts b/web/e2e/decisions.spec.ts
--- a/web/e2e/decisions.spec.ts
+++ b/web/e2e/decisions.spec.ts
@@ -1,5 +1,5 @@
 import { expect, test, type Page } from "@playwright/test";
-import { setPassword, signIn } from "./helpers";
+import { drag, setPassword, signIn } from "./helpers";
 
 // api returns a JSON caller that acts as the page's signed-in user.
 function api(page: Page) {
@@ -82,14 +82,14 @@ test("a developer sees a menu's history newest first", async ({ page, browser })
   const todo = pm.getByRole("region", { name: "To do" });
   const done = pm.getByRole("region", { name: "Done" });
   const weekend = pm.getByRole("article").filter({ hasText: `${key}-2` });
-  await weekend.dragTo(done);
+  await drag(pm, weekend, done);
   const dialog2 = pm.getByRole("dialog", { name: `Tutup ${key}-2 sebagai Done` });
   await dialog2.getByRole("button", { name: "Batal" }).click();
   await expect(dialog2).toHaveCount(0);
   await expect(todo).toContainText(`${key}-2`);
 
   // AC-DC-4: the ticket has no reason, so the dialog asks for one and flags a weak one.
-  await weekend.dragTo(done);
+  await drag(pm, weekend, done);
   const reason = dialog2.getByLabel("Alasan");
   await reason.fill("sesuai permintaan klien");
   await expect(dialog2.getByText(/Jelaskan mengapa klien membutuhkannya/)).toBeVisible();
```

`web/e2e/helpers.ts`:

```diff
diff --git a/web/e2e/helpers.ts b/web/e2e/helpers.ts
--- a/web/e2e/helpers.ts
+++ b/web/e2e/helpers.ts
@@ -1,4 +1,4 @@
-import { expect, type Page } from "@playwright/test";
+import { expect, type Locator, type Page } from "@playwright/test";
 
 // The tests run in the default UI language, Indonesian: after sign-in the UI
 // follows the user's profile language, and new users start with `id`.
@@ -18,3 +18,17 @@ export async function signIn(page: Page, email: string, password: string) {
   await page.getByRole("button", { name: "Masuk" }).click();
   await expect(page).toHaveURL(/\/$/);
 }
+
+// Drags with several pointer moves, as dnd-kit waits for a short move before a
+// drag starts. The grab point sits near the top-left corner, clear of links and
+// menus inside the source; `to` picks the drop point as a share of the target's box.
+export async function drag(page: Page, source: Locator, target: Locator, to = { x: 0.5, y: 0.5 }) {
+  const from = await source.boundingBox();
+  const box = await target.boundingBox();
+  if (!from || !box) throw new Error("drag: source or target is not visible");
+  await page.mouse.move(from.x + 8, from.y + 8);
+  await page.mouse.down();
+  await page.mouse.move(from.x + 20, from.y + 20, { steps: 4 });
+  await page.mouse.move(box.x + box.width * to.x, box.y + box.height * to.y, { steps: 12 });
+  await page.mouse.up();
+}
```

`web/e2e/tickets.spec.ts`:

```diff
diff --git a/web/e2e/tickets.spec.ts b/web/e2e/tickets.spec.ts
--- a/web/e2e/tickets.spec.ts
+++ b/web/e2e/tickets.spec.ts
@@ -1,5 +1,5 @@
 import { expect, test } from "@playwright/test";
-import { setPassword, signIn } from "./helpers";
+import { drag, setPassword, signIn } from "./helpers";
 
 // FSD §21 Iteration 2 exit check: a PM logs a client request in one form (story 3),
 // then works it: an Internal comment (AC-TK-10) and a move on the board (AC-TK-1).
@@ -64,7 +64,7 @@ test("a PM logs a client request in one form and moves it on the board", async (
   // AC-TK-1: dragging the card from To do to In progress records the move.
   await rina.goto(`/p/${key}/board`);
   const inProgress = rina.getByRole("region", { name: "In progress" });
-  await rina.getByRole("article").filter({ hasText: `${key}-1` }).dragTo(inProgress);
+  await drag(rina, rina.getByRole("article").filter({ hasText: `${key}-1` }), inProgress);
   await expect(inProgress.getByRole("article")).toContainText(`${key}-1`);
   await rina.goto(`/t/${key}-1`);
   await expect(rina.getByText(/Rina PM mengubah Status dari To do menjadi In progress/)).toBeVisible();
```

- [ ] **Step 2: Run the tests**

Run: `make up && cd web && E2E_BASE_URL=http://localhost:8080 npx playwright test`

Expected: 4 passed (signin, registry, tickets, decisions).

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat(web): move board cards with dnd-kit"
```

### Task 6: End-to-end test for Iteration 4b

**Files:**
- Create: `web/e2e/web.spec.ts`
- Modify: `web/e2e/global-setup.ts`

**Interfaces:**
- Consumes: Tasks 1–5.
- Produces: `web/e2e/web.spec.ts`, with its own admin in `global-setup.ts`. It covers:
  - `c` on the board opens the modal over the board, and Escape returns to it;
  - the menu used last comes first, marked Terakhir;
  - Markdown renders, and an outside image stays a link;
  - a pasted PNG uploads and shows in the comment;
  - dragging a menu into a module, and before a module.

- [ ] **Step 1: Write the tests**

This test covers every task above, so it comes last and passes at once.

`web/e2e/global-setup.ts`:

```diff
diff --git a/web/e2e/global-setup.ts b/web/e2e/global-setup.ts
--- a/web/e2e/global-setup.ts
+++ b/web/e2e/global-setup.ts
@@ -34,6 +34,9 @@ export default async function globalSetup() {
   const tree = createAdmin("Tree Admin");
   process.env.E2E_TREE_ADMIN_EMAIL = tree.email;
   process.env.E2E_TREE_ADMIN_LINK = tree.link;
+  const web = createAdmin("Web Admin");
+  process.env.E2E_WEB_ADMIN_EMAIL = web.email;
+  process.env.E2E_WEB_ADMIN_LINK = web.link;
   const tickets = createAdmin("Ticket Admin");
   process.env.E2E_TICKET_ADMIN_EMAIL = tickets.email;
   process.env.E2E_TICKET_ADMIN_LINK = tickets.link;
```

`web/e2e/web.spec.ts` (new):

```ts
import { expect, test } from "@playwright/test";
import { drag, setPassword, signIn } from "./helpers";

// A 1×1 PNG, pasted as the clipboard's only file.
const PNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==";

// FSD §21 Iteration 4b: the create modal behind `c` (§8.1), recently used menus
// first, Markdown with pasted images, and moving menus by dragging (§7.3).
test("a PM files a ticket from the keyboard, pastes a screenshot and reorganises menus", async ({ page }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase();
  const key = `W${run.slice(-6)}`;
  const password = "e2e-web-admin-passphrase-7";

  await setPassword(page, process.env.E2E_WEB_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_WEB_ADMIN_EMAIL!, password);
  const origin = new URL(page.url()).origin;
  const call = async (method: string, path: string, data?: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.json();
  };
  await call("POST", "/projects", { key, name: `Web ${run}` });
  const hr = await call("POST", `/projects/${key}/nodes`, { type: "module", name: "HR" });
  await call("POST", `/projects/${key}/nodes`, { type: "menu", name: "Attendance", parent_id: hr.id });
  const leave = await call("POST", `/projects/${key}/nodes`, { type: "menu", name: "Leave Request", parent_id: hr.id });
  const payroll = await call("POST", `/projects/${key}/nodes`, { type: "module", name: "Payroll" });
  await call("POST", `/projects/${key}/tickets`, { type: "change_request", title: "Earlier work on leave", node_ids: [leave.id], reason: "Setup." });

  // `c` on the board opens the create form over the board; Escape goes back to it.
  await page.goto(`/p/${key}/board`);
  await page.keyboard.press("c");
  const modal = page.getByRole("dialog", { name: `Tiket baru di ${key}` });
  await expect(modal).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/p/${key}/tickets/new$`));
  await page.keyboard.press("Escape");
  await expect(modal).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`/p/${key}/board$`));

  // The menu used last comes first, marked Terakhir.
  await page.keyboard.press("c");
  const form = modal.getByRole("form", { name: "Tiket baru" });
  const menus = form.getByRole("checkbox");
  await expect(menus.first()).toHaveAccessibleName(/HR › Leave Request\s*Terakhir/);
  await expect(form.getByRole("checkbox", { name: /Attendance/ })).not.toHaveAccessibleName(/Terakhir/);

  // Markdown renders; an image from outside the server stays a link.
  await form.getByLabel("Judul").fill("Leave balance shows the wrong year");
  await form.getByLabel("Deskripsi").fill("Saldo cuti **salah** setelah tutup tahun.\n\n- langkah satu\n\n![luar](https://example.com/x.png)");
  await form.getByRole("checkbox", { name: /HR › Leave Request/ }).check();
  await form.getByLabel("Alasan").fill("Employees cannot plan leave with a wrong balance.");
  await form.getByRole("button", { name: "Buat", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/t/${key}-2$`));
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.locator("strong", { hasText: "salah" })).toBeVisible();
  await expect(page.getByRole("listitem").filter({ hasText: "langkah satu" })).toBeVisible();
  await expect(page.getByRole("link", { name: "luar" })).toHaveAttribute("href", "https://example.com/x.png");
  await expect(page.locator('img[src^="https://"]')).toHaveCount(0);

  // A pasted screenshot uploads as an attachment and lands in the comment as Markdown.
  const comment = page.getByLabel("Tulis komentar");
  await comment.focus();
  await comment.evaluate((el, b64) => {
    const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
    const data = new DataTransfer();
    data.items.add(new File([bytes], "image.png", { type: "image/png" }));
    el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
  }, PNG);
  await expect(comment).toHaveValue(/!\[pasted-\d+\.png\]\(\/api\/v1\/attachments\/\d+\)/);
  await page.getByRole("button", { name: "Kirim" }).click();
  const shot = page.locator('img[src^="/api/v1/attachments/"]');
  await expect(shot).toHaveCount(1);
  await expect.poll(() => shot.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1); // served, and a real image

  // Dragging Leave Request onto Payroll's middle moves it inside Payroll.
  await page.goto(`/p/${key}/modules`);
  const handle = page.getByRole("button", { name: "Seret Leave Request" });
  const payrollRow = page.getByRole("button", { name: "Seret Payroll" }).locator("..");
  await drag(page, handle, payrollRow);
  await expect.poll(async () => (await call("GET", `/projects/${key}/nodes`)).items.find((n: { id: number }) => n.id === leave.id)?.parent_id).toBe(payroll.id);

  // Dropping Attendance on HR's top third puts it before HR, at the top level.
  const tree = page.getByRole("region", { name: "Pohon modul" });
  await drag(page, page.getByRole("button", { name: "Seret Attendance" }), page.getByRole("button", { name: "Seret HR" }).locator(".."), { x: 0.5, y: 0.15 });
  await expect(tree.getByRole("list").first().locator(":scope > li").first()).toContainText("Attendance");
});
```

- [ ] **Step 2: Run the tests**

Run: `make up && cd web && E2E_BASE_URL=http://localhost:8080 npx playwright test`
Expected: 5 passed.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "test(web): Iteration 4b end-to-end test"
```


### Task 7: Final checks

**Files:** none new. FSD §21 changes through the Claude Docs connector.

- [ ] **Step 1: Run every check**

```bash
cd server && gofmt -l . && go vet ./... && go test ./...
cd ../web && npm run gen:api && git diff --exit-code lib/api-types.ts && npx tsc --noEmit && npm run build
cd .. && make up && cd web && E2E_BASE_URL=http://localhost:8080 npx playwright test
```

Expected: no gofmt output, no vet findings, `ok` for every Go package, no drift in `lib/api-types.ts`, no type errors, a clean build, and 5 passed end-to-end tests (signin, registry, tickets, decisions, web). After 4a merges, admin-ai makes 6.

- [ ] **Step 2: Check by hand what the tests cannot**

On the rebuilt stack, in Chromium and in Safari:
- paste a real screenshot (macOS: ⌘⇧⌃4, then ⌘V) into a comment. It uploads, and shows once the comment is sent;
- with VoiceOver on, move a menu in the tree with Space, the arrows and Space. Each step is announced in the UI language;
- press `c` on Home with access to several projects. The New ticket menu opens with its first project focused.

- [ ] **Step 3: Update the FSD**

Through the Claude Docs connector (never as a file), in §21, the Progress paragraph records that Iteration 4b is built and points to this plan for the deliberate deviations. Keep the rest of the paragraph.
