# Muasal Pilot P1c-2 — Git Webhooks

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Commits and merge requests that name a ticket key show on that ticket and become evidence for Ask (FSD §14.1), from GitHub, GitLab and Gitea webhooks.

**How this plan was written:** each task was built on branch `feat/iteration-p1c2` and committed as named below; the commits hold the code.

**Spec:** Claude Docs "FSD — Muasal" (rev 115): §14.1, §16, §17.2.

## Architecture

**Schema:** migration `00012_git.sql` adds:
- `git_repos`, per project, with the webhook secret sealed by `APP_SECRET_KEY`;
- `commits` and `merge_requests`, unique per repository;
- `ticket_commits` and `ticket_merge_requests`;
- `webhook_deliveries`, the raw body waiting for its job;
- the chunk source type `code`.

**`internal/gitlink`:**
- **`Verify`** checks GitHub's `X-Hub-Signature-256`, Gitea's `X-Gitea-Signature` (both HMAC-SHA256) and GitLab's `X-Gitlab-Token`, in constant time.
- **`Event`** and **`Parse`** read push and pull/merge request payloads.
- **`Keys`** finds ticket keys with §14.1's pattern `[A-Z][A-Z0-9]{1,9}-[0-9]{1,7}`, in commit messages, merge request titles, bodies and branch names.
- **`Worker`** runs `ProcessDelivery` in River: it upserts the commits or the merge request, links the tickets whose keys exist in the repository's project, re-indexes them and deletes the delivery.

**Webhook:** `POST /webhooks/git/{repo_id}` (outside `/api/v1`, no session):
- 404 for an unknown repository, 413 over 5 MB;
- 401 for a bad signature, audited as `webhook_rejected`;
- otherwise the delivery is stored and queued, and the answer is 202 at once.

Caddy passes `/webhooks/*` to the app.

**Ask:** each linked ticket gains a `code` chunk listing its merge requests and commits. Its readers include the commit authors whose email matches a member. The evidence block carries the same text.

**Web:**
- Project settings → Repositories: add (provider, name, address), then copy the webhook URL and the secret, shown once, with how-to text for each provider. "New secret" replaces the secret, and repositories can be removed.
- Ticket page → Code: merge requests with Open, Merged or Closed badges, then commits with a short SHA, author and date.

## Deliberate Deviations from the FSD

- **Other events:** tags, issues and comments are acknowledged with 202 and dropped.
- **Unknown keys** are not stored. A commit naming a ticket that does not exist yet is not linked later.
- **Removing a repository** keeps the links already made on tickets.

## Tasks

### Task 1: Server

**Commit:** `feat: Git webhooks link commits and merge requests to tickets (FSD §14.1)`
- **API:**
  - `GET, POST /projects/{key}/repos` and `PATCH, DELETE /repos/{id}`, for project admins (409 `secret_key_missing` without `APP_SECRET_KEY`);
  - `Ticket.code`.
- **Tests:**
  - `TestGitHubPushLinksCommits` covers a signed push linking two tickets, a bad signature being refused and audited, and the code chunk.
  - `TestGitLabMergeRequestLinks` covers the token check and the merged state.
  - `gitlink` unit tests cover the signatures and keys.

### Task 2: Web

**Commit:** `feat(web): repositories in project settings and a Code section on tickets`

### Task 3: Checks

- [x] Go tests, web type check, i18n check and unit tests.
- [ ] CI green; merge.
