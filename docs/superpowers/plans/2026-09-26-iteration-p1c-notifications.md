# Muasal Pilot P1c — Notifications and @Mentions

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tell people about work on their tickets while the app is open, in the bell and as browser notifications, without email (FSD §8.10). Members @mention each other in comments (§8.7).

**How this plan was written:** each task was built on branch `feat/iteration-p1c` and committed as named below; the commits hold the code.

**Spec:** Claude Docs "FSD — Muasal" (rev 114): §8.7, §8.10, §8.11 (AC-TK-9), §16, §17.2.

**Change to the P1 split:** Git webhooks (§14.1) move to their own slice, P1c-2.

## Architecture

**Schema:** migration `00011_notifications.sql` adds `users.notify_prefs` and `notifications`.

**Recording (`NotifyTicket`):** one statement inserts rows only for users who:
- are named;
- are not the actor;
- are active;
- have the event on;
- can see the ticket, under the visibility predicate.

The same statement calls `pg_notify('muasal_notifications', 'user:id')`, which fires on commit, in the same transaction as the event.

**Events:**
- **Assignment:** on create, and when an update changes the assignee.
- **Comments:** they go to the reporter, the assignee and earlier commenters. A comment's @handles notify those members as a mention instead of a comment.
- **Status changes:** to the reporter and the assignee.
- **A finished import:** to the admin who started it, as `job_done`.

**Push:**
- `Server.listen` holds one `LISTEN` connection and publishes to a per-user hub.
- `GET /notifications/stream` subscribes one tab, and checks visibility again before each push.
- The listener starts with the first stream, lives until `Server.Close`, and reconnects after a failure.

**Web:**
- **The bell** loads the latest 50, subscribes with `EventSource`, and raises an OS notification through the Notifications API when the tab is hidden and the user opted in.
- **The profile** holds the preferences; choosing browser notifications asks the browser's permission.
- **The comment box** suggests mentionable members after "@".

**Retention:** the daily purge drops notifications older than 90 days.

## Fix on the way

Event streams did not flush. `startSSE` asserted `http.Flusher` on the middleware's `statusWriter`, which only offers `Unwrap`. Every SSE response, Ask's claims included, was buffered until the handler returned. It now flushes through `http.ResponseController`, and writes an opening comment so headers go out at once.

## Deliberate Deviations from the FSD

- **Mention format:** `@handle`, where the handle is the part of the member's email before the @, rather than a name with spaces. It survives copy and paste, and needs no special markup.
- **Who can be mentioned:** members of the ticket's project who can see it. System admins who are not members are not suggested.
- **Tree drafts and re-index:** the `job_done` event exists, but only imports send it for now. Tree drafts come with P1f, and re-index-all has no single finish.

## Tasks

### Task 1: Server

**Commit:** `feat: notifications (FSD §8.10): …`
- **API:**
  - `GET /notifications`, with the latest 50 and the unread count;
  - `POST /notifications/read`, for one or all;
  - `GET /notifications/stream`;
  - `GET /tickets/{key}/mentionable`;
  - `User.notify_prefs`, and `MeUpdate.notify_prefs`.
- **Tests:**
  - `TestTicketEventsNotify`: assignment, mention versus comment, visibility, status, reading one then all;
  - `TestNotificationPreferences`;
  - `TestNotificationsStreamToOpenTabs`: a push within 5 seconds, the server side of AC-TK-9.

### Task 2: Web

**Commit:** `feat(web): the notification bell …`
- **E2e:** `web/e2e/notifications.spec.ts`. Rina's open tab counts an assignment within 5 seconds, opens it, and then receives an @mention picked from the suggestions.

### Task 3: Checks

- [x] Go tests, web type check, i18n check, unit tests and build.
- [x] Playwright: 13 of 13 pass on a fresh stack.
- [ ] CI green; merge.
