# Muasal Pilot P1f — Project Documents and AI Tree Drafts

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- A project admin uploads an FSD and gets a proposed module tree to review and apply, instead of typing the tree by hand (FSD §7.7, AC-MR-8, AC-MR-10).
- Ask cites the document by section, such as HRIS-DOC1/7.4 (R-MR-13, AC-MR-9).

**How this plan was written:** each task was built on branch `feat/iteration-p1f` and committed as named below; the commits hold the code.

**Spec:** Claude Docs "FSD — Muasal" (rev 117): §7.7, §7.8, §13.1, §16, §17.2.

## Architecture

**Schema:** migration `00014_documents.sql` adds:
- `projects.doc_seq`;
- `documents`: key `HRIS-DOC1`, title, client, the original file stored like attachments, the Markdown, and `superseded_by`;
- `document_sections`: number, title, level and body;
- `document_section_nodes`;
- `tree_drafts`: status, proposal, `used_ai`, and progress as parts done of total;
- chunks may belong to a section (`section_id`, source type `document`).

**Conversion in the browser** (`web/lib/convert.ts`):
- **Markdown** is read as is.
- **DOCX** goes through mammoth to HTML, then headings, paragraphs, list items and table rows become Markdown.
- **PDF** goes through pdf.js in a module worker. A line becomes a heading when it is short and either set larger than the body text or numbered like "7.4 Node page"; the number's depth sets its level.
- The CSP gains `worker-src 'self' blob:`.

**`internal/docs`:**
- **`Split`** cuts Markdown at its headings, outside fenced code:
  - a heading such as "7.4 Node page" keeps its number, and others get a running `s<n>`;
  - numbers stay unique;
  - text before the first heading becomes section `0`.
- **`Parts`** groups sections into parts that fit the AI context budget. A section is split only when it alone is too long.
- **`FromHeadings`** handles AI off (R-MR-11). The two shallowest heading levels below a lone title become modules and menus; with the usual `#` title, those are `##` and `###`.
- **`Merge`** makes no model call:
  - paths are normalized for case and spacing, and merged;
  - missing parents become modules;
  - nodes the tree has are marked `exists` and left unticked;
  - siblings with trigram similarity of 0.8 or more are flagged as possible duplicates, with pg_trgm's formula.

**`internal/draft`:**
- **`ExtractTree`** makes one schema-constrained call per part. Each call returns `{path, type, aliases, description, section}`, with `section` limited to that part's numbers.
- **`DraftTree`** is a River job, 45-minute timeout, with 3 attempts on model errors. It records progress per part, saves the proposal as ready, and notifies the starter (`job_done`).
- **With AI off**, the heading proposal is built inside the request and is ready at once.

**Ask:**
- `ask.Ref` gains a kind (ticket, note, section).
- Keyword and vector search return `section_id`, and a section's evidence block names its document, number, menus and "superseded by".
- Items are of kind `document`, and citation chips open `/documents/HRIS-DOC1#s-7.4`.
- `IndexSection` jobs rebuild a section's chunks on upload, on supersede and after apply. Re-index-all includes sections.

**API:**
- `GET, POST /projects/{key}/documents`: upload is multipart, for project admins, 20 MB, PDF, DOCX or Markdown, with an optional client and `supersedes`.
- `GET /documents/{key}` and `GET /documents/{key}/file`, for anyone who may see the document (R-MR-15).
- `POST /documents/{key}/tree-drafts`, answering 202.
- `GET, PUT /tree-drafts/{id}`, and `POST /tree-drafts/{id}/apply` and `/discard`, for project admins.
- **Applying** creates the ticked nodes the tree lacks in one transaction:
  - parents come first, with `source = ai_draft` and the extracted description;
  - it links every proposed node, new or existing, to its source sections;
  - a ticked node under an unticked new parent, or two nodes on one path, is refused with 422.

**Web:**
- **The Documents tab** holds the list and the upload form, which converts the file, suggests a title, and offers a client and "Replaces".
- **The document page** has sections with anchors, the nodes each produced, an outline, the download, "Draft module tree" and the drafts.
- **The tree draft page:**
  - a progress bar polls every 2 seconds while the draft runs;
  - once ready, the admin reviews an indented tree with checkboxes, name and type, "already in the tree", possible-duplicate warnings, links to source sections, aliases and descriptions;
  - unticking a node unticks what is under it, and ticking one ticks its parents;
  - an admin can add a node under any other; Save, Apply and Discard finish the review.

## Deliberate Deviations from the FSD

- **Review actions:** the review renames, retypes, unticks and adds nodes, but has no drag or merge. The applied tree has both, in the module tree (P1b's merge).
- **Headings without AI:** the headings draft uses the two shallowest levels under a lone title rather than fixed `##` and `###`, so documents whose chapters are `#` still work.
- **Superseded documents:** Ask sees their sections with a "superseded by" line and cites them as history. Drafting from the newer document proposes everything, but nodes the tree has show as existing and stay unticked.
- **Client scope:** created nodes are not client-specific, even from a one-client document; admins mark client menus in the tree.
- **Scanned PDFs** have no text layer and cannot be read; the upload says so.
- **The Phase 0 test** (three real FSDs, 70% of menus kept) waits on the pilot teams' documents.

## Tasks

### Task 1: Server

**Commit:** `feat: project documents and AI tree drafts (FSD §7.7)`
- **Tests:**
  - `TestDocumentTreeFromHeadings`, for AC-MR-10 and R-MR-15:
    - upload rights, file types, and client scope;
    - download;
    - the heading proposal, with existing nodes marked;
    - the tree unchanged until apply;
    - rename, the parent-unticked refusal, apply, `ai_draft`, section links, and applying twice.
  - `TestDocumentTreeFromTheModelAndAskCitesIt`, for AC-MR-8 and AC-MR-9:
    - the job with progress and the source section of every node;
    - the starter notified;
    - apply;
    - Ask citing `HRIS-DOC1/7.4`.
  - `docs` unit tests for splitting, headings, merging, duplicates and similarity.

### Task 2: Web

**Commit:** `feat(web): documents, PDF and DOCX conversion, and the tree draft review`
- `lib/convert.test.ts` covers the PDF heading heuristic.
- **E2e:** `web/e2e/documents.spec.ts` uploads a real PDF and checks that:
  - the browser converts it and the sections show;
  - the heading draft is reviewed with one menu unticked;
  - applying creates two nodes.
- **Licence scan:** Zlib is allowed (pako). duck, mammoth's dependency, is named as BSD-2-Clause from its LICENSE file.

### Task 3: Checks

- [x] Go tests, web type check, i18n check, unit tests, licence scan and build.
- [x] Playwright: 15 of 15 pass on a fresh stack.
- [ ] CI green; merge.
