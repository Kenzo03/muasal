# User guide

Muasal has three roles in a project. **Viewers** read. **Members** file, comment on and close tickets. **Project admins** also manage the tree, members, statuses, repositories and documents. A member can be limited to some clients; they then see only those clients' tickets and menus, and core work.

## The module tree

**Modules** group **menus**, which are single screens or features, for example HR › Attendance › Overtime Approval. A menu can be specific to some clients.

Admins build the tree in three ways:
- by hand, on the project's **Modules** tab, dragging nodes to reorder or move them;
- from a CSV file (**Modules → Import**);
- from a specification on the **Documents** tab (see below).

Each menu has a page showing its **timeline**, newest first with each decision record, and the **behaviours in force** for each client.

## Tickets

Press **c** anywhere to file a ticket. It needs a title, a type, the client (or all clients for core work), who asked, and the menus it touches. A **reason**, saying why the client needs this, is what later answers "why".

- **Comments** are Internal by default. Mark one Client-safe only when it may appear in a client-facing summary. Type **@** to mention a member.
- **Links** say that a ticket reverses, extends or relates to another. A reversing ticket supersedes the older decision.
- **The board** moves tickets between statuses by drag and drop.

**Closing** a ticket, by moving it to Done or Cancelled, opens the close dialog. It asks what changed, why, and what alternatives were rejected, prefilled from the ticket. **Draft with AI** fills these fields from the ticket's comments and linked commits. It leaves Why empty when the thread never says why, so you ask the requester instead of guessing. Nothing is saved until **Close ticket**.

**Decision notes** record decisions made in meetings, calls or emails, outside any ticket. They appear on menu pages and in Ask, and are cited like `HRIS-DN7`.

## Ask

Open Ask from the top bar, a menu page or a ticket, and ask in Indonesian or English.

- **Chips** above the question narrow the scope: project, menu, client, dates and person. Muasal also detects chips in the question.
- **Every claim** ends with citation chips. Hover to see the source; click to open it. Sources can be tickets, decision notes (`HRIS-DN7`) and document sections (`HRIS-DOC1/7.4`).
- **When nothing in scope answers the question,** Ask says "Not enough information" and shows the closest items instead of guessing.
- **Thumbs up or down** tell admins which answers to improve.

## Change summaries

On the project's **Summaries** tab:
1. Pick a module, a client, a closed-date range, a language and the audience.
2. Preview the closed tickets and notes by menu, and untick anything that should stay out. Unticked items are never sent to the model.
3. Generate. The summary is editable Markdown with cited bullets and an appendix of the items.
4. Print it (the browser saves A4 PDF), or copy it as Markdown.

Client-facing summaries use decision records and Client-safe comments only, and never Internal comments.

## Documents and tree drafts

Project admins upload a PDF, DOCX or Markdown specification on the **Documents** tab. The browser reads the file, and its headings become sections that Ask can cite.

**Draft module tree** proposes modules and menus from the document:
- **With AI on,** the model reads it in the background, and the bell tells you when the draft is ready.
- **With AI off,** the headings become the tree at once.

Review the draft: rename, retype, untick or add nodes. Every node shows the sections it came from, and nodes the tree already has are marked. **Apply** creates the ticked nodes and links them to their sections.

## Code

Project admins connect GitHub, GitLab or Gitea repositories in project settings. Commits and merge requests that mention a ticket key such as `HRIS-231` then show in the ticket's **Code** section, and Ask uses them as evidence.

## Notifications

The bell shows assignments, comments, mentions, status changes and finished imports or tree drafts, live while the app is open. Your profile turns each event on or off, and can turn on browser notifications, and, once your admin has set up email, an email of what you have not read after a couple of minutes.

## For scripts

Personal API tokens (**Settings → API tokens**) call the same REST API as the app, read-only or read-write. The API is described in `api/openapi.yaml`.
