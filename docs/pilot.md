# Pilot kit

Iteration 6 prepares the first pilot (FSD §21). Its exit is the MVP exit of PRD Phase 1: one team uses Muasal daily for a month. This kit lists what to set up, how to run the usability sessions, what to watch, and when the pilot counts as passed.

## Before day one

- [ ] **Server:** sized with [hardware.md](hardware.md) and installed with [operations.md](operations.md).
- [ ] **HTTPS:** with the customer's certificate, or Caddy's internal CA trusted on the team's machines.
- [ ] **AI mode:** chosen with the team's IT: local (a GPU for the recommended tier), bring-your-own-key with the acknowledgement signed off, or off.
- [ ] **Exit check for the chosen model:** run `app eval --seed` on the pilot server (FSD §11.8). Record precision, recall, abstention and the median latency.
- [ ] **Load test:** run `deploy/loadtest/run.sh 100000` on a scratch install of the same hardware, then remove the scratch install.
- [ ] **Backups:** run a first backup from Admin → Backups, then a restore drill into a scratch stack. Show IT where `/backups` is and agree who copies it off the server.
- [ ] **Accounts:** the team's users are created, with project roles and client scopes; each person has opened their setup link.
- [ ] **Registry:** the module tree is built for the pilot project (§7): modules, menus, client-specific menus, aliases.
- [ ] **History:** 20–50 recent requests are entered as tickets, with reasons and closing decisions, so Ask has history to answer from.
- [ ] **Feedback:** a channel exists, such as a chat group or a weekly call, and one named person on each side.

## Usability sessions

Run three 45-minute sessions in the first two weeks, one person each (a PM, a developer, a support lead). Sit beside them, give each task as written, and do not help unless they are stuck for two minutes.

Note for each task:
- whether it was done;
- the time it took;
- where they hesitated;
- what they said, in their words.

| # | Task (PRD story) | Done when |
| --- | --- | --- |
| 1 | "A client just called about overtime approval. Log their request." (story 3) | A ticket exists with client, requester, menu and reason |
| 2 | "Find out why overtime approval skips the supervisor for Client A." (story 1) | They open the menu's page, or ask, and name the ticket behind it |
| 3 | "Ask Muasal the same question in your own words." (story 2) | They read an answer and open a cited ticket, or see "Not enough information" and know what to try next |
| 4 | "The request is done. Close it." | The close dialog is filled and the decision record is confirmed |
| 5 | "Move a menu to the module where it belongs." | It is moved by dragging or with the parent picker |
| 6 | (Admin) "Check that last night's backup ran." | They find the time and size in Admin → Backups |

Afterwards, ask:
- What did you expect that did not happen?
- What would stop you using this every day?
- On a scale of 1 to 5, how much do you trust Ask's answers, and why?

## What to watch during the month

- **Admin → Ask log:** the not-enough-information rate, answers slower than 30 seconds, and the questions nobody could answer. These point to missing history or menu aliases.
- **Admin → System status:** disk use, failed jobs, the model server.
- **Tickets:**
  - the share linked to a menu (target 90%) and the share with a filled reason (target 85%);
  - weak reasons, such as "client request", to coach.
- **Daily use:** someone on the team creates, updates or asks at least once each working day.

## Exit

The pilot passes when, over one month:
- the team uses Muasal on at least 18 of about 22 working days;
- the success metrics in FSD §15.5 are at or near their targets, measured from the database;
- the team wants to keep it.

Record the results, and the fixes the sessions led to, in FSD §21's Progress paragraph.
