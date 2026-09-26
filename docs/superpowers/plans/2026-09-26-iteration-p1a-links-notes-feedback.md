# Muasal Pilot P1a — Ticket Links, Decision Notes, Ask Feedback and Follow-ups

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The first slice of the Pilot phase (FSD §21, "Pilot (P1)"). It covers the P1 items that make Ask's history more complete and more trusted:
- ticket links that supersede decisions (§8.8);
- decision notes (§9.4);
- thumbs up or down on answers (§10.7);
- follow-up questions (§11.9).

**How this plan was written:** each task was built test-first on branch `feat/iteration-p1a` and committed as named below; the commits hold the code. This plan lists the tests, interfaces and deliberate deviations.

**Splitting P1:** FSD §21 lists the Pilot phase as one row, so it is split into slices, each with its own pull request:
- **P1a** (this plan): links, decision notes, feedback, follow-ups.
- **P1b:** API tokens and idempotency, CSV export, ticket and tree imports (CSV, Jira), node merge.
- **P1c:** notifications (bell, SSE, browser), @mentions, Git webhooks and project repositories.
- **P1d:** AI-drafted decision records and change summaries with print.
- **P1e:** SSO through OpenID Connect, sign-in settings, the success metrics dashboard.
- **P1f:** project documents and AI tree drafts (onboarding).

**Spec:** Claude Docs "FSD — Muasal" (rev 111): §8.8, §9.4, §10.6, §10.7, §11.9, §13.1, §15.4, §17.2.

## Architecture

**Migration `00008_links_notes_feedback.sql`:**
- adds `ticket_links`, `decision_records.superseded_by`, `projects.note_seq`, `decision_notes` with its menus and tickets, and `ask_feedback`;
- lets a chunk belong to a ticket or to a note (`chunks.ticket_id` nullable, `note_id`, source type `note`, and a check that exactly one owner is set).

**Evidence refs:**
- Retrieval, packing, the Ask log and threads name each piece of evidence as an `ask.Ref`: a ticket or a note.
- Note chunks carry the note's project, client and menus, and its decision date. So the visibility predicate, scope filters and both search lists apply to notes unchanged.
- `ask.Found.TicketIDs()` keeps the ticket view for callers and tests.
- Citation keys accept `HRIS-DN7`.

**Supersede:**
- `RefreshSuperseded(ticket)` sets `superseded_by` to the newest ticket that reverses it, or clears it.
- It runs when a link is added or removed, and when a decision is confirmed, so a ticket closed after its reversal is superseded at once.
- Behaviors in force leave out superseded decisions.
- The decision chunk and the evidence block say "Superseded by …".

**Follow-ups:**
- The engine reads the thread's last two turns.
- `CarryOver` keeps the previous detected chips of every kind the new question does not name.
- Retrieval text is the new question plus the previous one.
- Keys cited in the previous answer return as named evidence, but only while the client chips are unchanged. "And for Client B?" therefore never cites Client A (AC-AK-9).
- The last turns go in as a fenced CONVERSATION block, at most 2,100 characters (600 tokens), taken out of the context budget.

## Deliberate Deviations from the FSD

- **Timeline pages:** notes arrive on the first timeline page only, all of them, in `TimelinePage.notes`. The page places them among closed tickets by date, and holds back notes older than the last loaded ticket until the rest loads.
- **Indonesian naming:** the Indonesian UI calls decision notes "Notulen keputusan", because "Catatan keputusan" already names the ticket's decision record.
- **Deleting links:** members of the linking ticket's project remove a link. The FSD does not say who may.
- **Linked tickets on a note:** they are entered as keys, in any project the author can see, and hidden from readers who cannot see them.

## Tasks

### Task 1: Ticket links and supersede

**Commit:** `feat: ticket links with supersede (R-TK-5..7); Ask evidence covers tickets and decision notes`
- **API:**
  - `POST /tickets/{key}/links` → `TicketLink`: 422 `self_link` or `not_found`, 409 `link_exists`.
  - `DELETE /links/{id}`.
  - `Ticket.links`, and `DecisionRecord.superseded_by`, which timelines show too.
- **Audit:** `link` and `unlink` go on both tickets, in each ticket's own wording.
- **Tests:**
  - `TestReversingSupersedesTheDecision` (AC-TK-8, R-TK-7);
  - `TestADecisionConfirmedAfterItsReversalIsSuperseded`;
  - `TestLinksNeedTwoVisibleDistinctTickets` (R-TK-6: hidden tickets never show as links).

### Task 2: Decision notes

**Commit:** `feat: decision notes (FSD §9.4): API, node timelines, search, index and Ask citations; in the permission suite`
- **API:**
  - `GET, POST /projects/{key}/notes`;
  - `GET, PATCH /notes/{noteKey}`, where `archived` archives or restores;
  - `SearchResults.notes`.
- **Permissions and audit:** members create; the author and project admins edit (`can_edit`). Edits are audited as entity `note`.
- **Indexing:**
  - The `IndexNote` job is queued in the same transaction as the change.
  - Re-index all, and renaming or moving a menu, cover notes too.
- **Tests:**
  - `TestMembersRecordDecisionNotes` (AC-DC-8);
  - `TestNotesFollowVisibilityAndAuthorship`;
  - `TestAskCitesDecisionNotes`;
  - the permission suite gains three notes: the project list, two single notes, and Ask evidence for every user.

### Task 3: Ask feedback

**Commit:** `feat: Ask feedback (FSD §10.7): thumbs up or down with reasons, in threads and the Ask log with a thumbs-down filter`
- **API:**
  - `POST /ask/queries/{id}/feedback` takes `rating` (up or down), `reasons` and `comment`. Only the asker may rate; another user's question answers 404.
  - `AskThreadQuery.feedback` and `AskLogEntry.feedback`.
  - `GET /admin/ask-log?down=true`.
- **Test:** `TestThumbsDownReachesTheAskLog` (AC-AK-8).

### Task 4: Follow-up questions

**Commit:** `feat: Ask follow-ups (FSD §11.9): …; 10 follow-up pairs in the golden set`
- **Code:** `ask/followup.go` (`CarryOver`, `plan`).
- **Prompt:** `User(question, evidence, conversation)`.
- **Golden set:** questions may name the question they `follows`, and `app eval` asks them in that thread. The embedded set gains f01–f10.
- **Tests:**
  - `TestFollowUpsSwapTheClient` (AC-AK-9: chips, evidence, citations, one model call per question, and the CONVERSATION block);
  - `TestGoldenSetV0IsWellFormed` (10 follow-ups, each after its question).

### Task 5: Web

**Commits:**
- `feat(web): ticket links with inverse wording, and superseded decisions on the ticket page`
- `feat(web): decision notes: list, form and page; notes and superseded decisions on node timelines`
- `feat(web): Ask feedback and note citations; the Ask log's thumbs-down filter; notes in search; link events in history`
- `test(web): e2e for links, decision notes and thumbs-down feedback; …`

**Pages:**
- **Ticket page:** a Links card, plus a "Superseded by" banner on the decision card.
- **Notes pages:** the project's Notes tab (`/p/[key]/notes`, `/p/[key]/notes/new`) and one note (`/notes/[noteKey]`), all using `NoteForm`.
- **Node timeline:** notes by date, and superseded decisions marked.

**Ask:**
- citation chips and source lists open notes at `/notes/…`;
- a thumbs up or down under each answer, where down asks for reasons and a comment;
- the Ask log gets a thumbs-down quick filter, a feedback column, and feedback on each entry.

**Also:**
- search lists notes, and a note key jumps straight to the note;
- ticket history words link events.

**E2e:** `web/e2e/pilot.spec.ts`:
1. reverse a decision from the ticket page;
2. record a note and see it on the timeline;
3. give a thumbs-down that the Ask log filter finds.

### Task 6: Checks

- [x] Go tests (with the permission suite), web type check, i18n check, unit tests and build.
- [x] Playwright: 10 of 10 pass on a fresh stack.
- [ ] CI green; merge.
- [ ] Run `app eval` with the follow-up pairs on the dev laptop, then record precision, recall and abstention for f01–f10.
