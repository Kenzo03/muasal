# Muasal Pilot P1d — AI Decision Drafts and Change Summaries

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- "Draft with AI" fills the close dialog's decision record from the ticket's thread (FSD §9.3, AC-DC-6, AC-DC-7).
- A PM builds an editable, cited change summary for a client and prints it (§12.1, story 4). Internal comments never reach a client-facing summary (AC-TK-10).

**How this plan was written:** each task was built on branch `feat/iteration-p1d` and committed as named below; the commits hold the code.

**Spec:** Claude Docs "FSD — Muasal" (rev 116): §9.3, §12.1, §16, §17.2.

## Architecture

**Schema:** migration `00013_drafts_summaries.sql` adds:
- `decision_records.ai_drafted`;
- `summaries`: the scope as `params`, the ticked items with their appendix fields as `items`, the editable `markdown`, and the model badge.

**`internal/draft`:**
- **`generate`** runs one schema-constrained chat call under the Ask concurrency gate. It waits up to 90 seconds for a slot, then applies the AI settings' timeout. Failures map to `ai_off`, `ai_unavailable`, `ai_busy`, `ai_timeout` or `ai_invalid`.
- **`DraftDecision`** sends the thread and gets `{what_changed, why, alternatives_rejected}` in the UI language:
  - The thread is the title, type, client, menus, reason, description, the latest 30 comments and linked commit messages.
  - The prompt forbids inventing a reason, and an empty `why` stays empty.
- **`Summarize`** handles the ticked items:
  - Up to 40 items go in one call. More are split into one call per menu group of up to 40, then one call for the overview.
  - Each call returns an overview and bullets `{text, why, cites}`, with `cites` limited to that call's keys.
  - A bullet citing nothing valid is dropped. A bullet's date and menu come from its earliest cited item.
- **`Markdown`** lays out:
  - a title such as "Payroll changes for Client B, 1 Jan 2026 – 23 Sep 2026", localized;
  - the overview;
  - one section per menu, with bullets giving the date, the change, why and the keys;
  - an appendix table built from the items (key, title, date, requested by).

  Client-facing copies print keys without links.

**API:**
- **`POST /tickets/{key}/decision-draft`** is for members. It saves nothing, answers 409 `ai_off` when AI is off, and 503 on a model failure.
- **Closing:** `DecisionInput.ai_drafted` is stored, and `DecisionRecord.ai_drafted` is read back.
- **`POST /summaries/preview`** lists every visible closed ticket (Done, plus Cancelled when asked) and decision note in the scope, by menu, oldest first. It also returns the model badge and whether it is a cloud provider.
  - The scope is a node with its sub-nodes, one client or all, a closed-date range, a language and an audience.
  - A one-client scope also includes core work and notes for all clients.
- **`POST /summaries`** is generated from the ticked keys only. What the model reads depends on the audience:
  - **Client-facing:** the decision record and Client-safe comments.
  - **Internal:** also the reason, the description and Internal comments.
- **`GET /summaries?project=`**, **`GET /summaries/{id}`** and **`PATCH /summaries/{id}`** are for the creator and project admins; anyone else gets 404.

**Web:**
- **The close dialog** gains "Draft with AI":
  - It replaces the prefills and fills empty fields, but never text the user typed.
  - When the thread never says why, Why stays empty with an amber highlight and a hint.
  - A status line names the model.
- **The project tab "Summaries"** holds:
  - the list;
  - the builder (scope, then a preview grouped by menu with checkboxes, then Generate, naming the model and warning when it is a cloud provider);
  - the editor at `/summaries/[id]` (title, markdown beside a live preview, Save, Copy as markdown, Print view).
- **`/summaries/[id]/print`** prints on A4 with a header and `@page` page numbers. The top bar is hidden when printing, and the browser saves the page as PDF.

## Deliberate Deviations from the FSD

- **Where "Draft with AI" appears:** only in the close dialog, not also on open tickets' pages. The dialog is where the record is confirmed.
- **Generation:** summaries are generated inside the request, with no SSE progress and no background job. Each call is bounded by the AI timeout, and the builder shows "Generating…". A background job with a progress bar can come if pilots summarize more than 40 items often.
- **Assignee names** never appear in any summary, internal ones included. The appendix names the requester only.
- **"Open as change summary" from an Ask answer** is not built yet.
- **Weak-reason metrics** for AI-drafted records come with P1e's metrics report.

## Tasks

### Task 1: Server

**Commit:** `feat: AI-drafted decision records and change summaries (FSD §9.3, §12.1)`
- **Tests:**
  - `TestDraftDecisionFromTheThread`:
    - AI off answers 409.
    - The prompt holds the thread and the menus.
    - An empty Why stays empty, and nothing is saved.
    - The close keeps `ai_drafted`.
    - A stopped model answers 503.
  - `TestChangeSummary`:
    - The preview holds closed items only, respects the member's client scope and adds Cancelled when asked.
    - Internal comments and unticked items never reach the model.
    - A bullet without cites is dropped.
    - The appendix comes from the database, and client keys have no links.
    - Another member gets 404; a project admin edits; the lists are right.
  - `draft` unit tests for batching and titles.

### Task 2: Web

**Commit:** `feat(web): Draft with AI in the close dialog, and the change summary builder, editor and print view`
- **E2e:** `web/e2e/summaries.spec.ts`:
  - The draft endpoint is stubbed at the browser.
  - The fields fill, Why stays empty and highlighted, and typed text is kept.
  - Nothing is saved until Close ticket, and the record keeps `ai_drafted`.
  - The builder lists the closed ticket under its menu.

### Task 3: Checks

- [x] Go tests, web type check, i18n check, unit tests and build.
- [ ] Playwright in CI. Docker Hub rate limits blocked a local image build.
- [ ] CI green; merge.
