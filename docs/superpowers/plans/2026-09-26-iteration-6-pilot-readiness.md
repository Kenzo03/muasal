# Muasal Iteration 6 — Pilot Readiness

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Muasal is ready for its first pilot team (FSD §21 Iteration 6: fixes, usability sessions, the first hardware guide). The exit is the MVP exit of PRD Phase 1: one team uses it daily for a month. That needs people, so this plan ends with the pilot kit ready and the exit left to the pilot.

**How this plan was written:** each task was built on branch `feat/iteration-6` and committed as named below; the commits hold the code. This plan lists the tests, the deliberate deviations and the results.

**Architecture:** no new services. Changes are:
- a faster Ask keyword search;
- PostgreSQL settings;
- two CI checks;
- four documents.

**Spec:** Claude Docs "FSD — Muasal" (rev 110): §11.3, §18, §18.1, §21.

## Deliberate Deviations from the FSD

- **Keyword search (§11.3).** The FSD ORs the question's words and ranks every match. Muasal now ranks the chunks holding all the words first. Only when fewer than 50 are found does it add chunks holding any word, and it ranks at most 5,000 candidates per search. A word found in thousands of chunks carries little information, and the vector search covers meaning when AI is on.
- **Usability sessions.** These need real users. `docs/pilot.md` holds the script, the six tasks tied to PRD stories and what to note. The fixes the sessions lead to become the pilot's first changes.
- **Vulnerability checks.** `govulncheck` needs `vuln.go.dev`, which the development container cannot reach, so it runs only in CI.

## Tasks

### Task 1: Load-test seed at realistic word frequencies

**Commit:** `test(deploy): seed 100,000 tickets in minutes: ready phrases instead of a lookup per word`
- The seed's 60-word vocabulary matched every chunk, so every search touched all 1,000,000 chunks.
- A 3,000-word Zipf vocabulary places HR words every 50th rank. Phrases are built once (5,000 of 25 words and 5,000 of 60), and each row picks one by a scalar subquery.
- Seeding 100,000 tickets, 200,000 comments and 1,000,000 chunks takes minutes, where a word-by-word join ran for over 19 minutes.

### Task 2: Licence scan (FSD §18)

**Commit:** `ci: licence scan of Go and web dependencies (FSD §18); NOTICE names the MPL and LGPL components`
- `server/cmd/licenses` lists each Go module compiled into the server with its licence, recognised from the licence file. It exits 1 on anything outside Apache-2.0, MIT, BSD-2-Clause, BSD-3-Clause, ISC and MPL-2.0. Test: `TestClassify`.
- `web/scripts/check-licenses.mjs` (`npm run check:licenses`) does the same for the production npm tree. Its one named exception is `@img/sharp-*`, the LGPL libvips binaries loaded by Next's image optimiser.
- NOTICE names River (MPL-2.0) and libvips (LGPL-3.0).

### Task 3: Hardware guide and security review

**Commit:** `docs: hardware guide and the ASVS level 1 review; CI checks dependencies for known vulnerabilities`
- `docs/hardware.md`:
  - tiers: AI off, minimum, recommended, large team;
  - PostgreSQL memory per server size, and disk planning at 100,000 tickets;
  - the measured load test (Task 5) and the network needs.
- `docs/security/asvs-l1.md`: OWASP ASVS 4.0 level 1, chapter by chapter, with the test or file behind each control. The one open item is an outside penetration test before the public launch.
- CI runs `govulncheck` (v1.8.0) on the server and `npm audit --omit=dev --audit-level=high` on the web app.
- `TestSecurityHeaders` also asserts `Cache-Control: no-store` on API responses.

### Task 4: Pilot kit

**Commit:** `docs: the pilot kit: setup checklist, usability sessions, what to watch and the exit`
- `docs/pilot.md`:
  - the setup checklist before day one;
  - three 45-minute usability sessions with six tasks tied to PRD stories 1–3, closing, moving menus and backups;
  - what to watch during the month (Ask log, system status, ticket quality);
  - the exit.

### Task 5: Load-test fixes

**Commits:**
- `perf(ask): keyword search ranks chunks with all the words first, at most 5,000 candidates; PostgreSQL JIT off`
- `test(deploy): load test askers take turns per question; results in the hardware guide`

**Diagnosis:**
- With all 55 users, the API p95 was 313 ms and the page p95 1.03 s. Browsing alone met both targets: 172 ms and 727 ms.
- Sampling active queries showed Ask's `KeywordSearch` as the top consumer. For two common words it ranked 136,000 chunks, reading about 126,000 heap pages: 3 s of database time per question.
- `ts_rank_cd` needs each row's `tsvector`, so a GIN index cannot rank. More `work_mem` removed the lossy bitmap but saved little.

**Fix:**
- `Retrieve` searches with the words AND-ed first, then OR-ed if fewer than 50 chunks come back, keeping the first 50 distinct chunks. `KeywordSearch` takes a `candidates` limit (5,000) inside a subquery, and ranks only those.
- `jit=off` in compose: JIT compiled for 176 ms on queries that run in milliseconds.
- `k6.js`:
  - the five askers take users 46–50 in turn per question (`exec.scenario.iterationInTest`), so two askers never share one user's 10-a-minute limit;
  - the Ask threshold is the median, as in FSD §18.
- Test: `TestKeywordSearchPutsAllTheWordsFirst`. With AI off, a ticket holding both words ranks above one that repeats just one of them. It fails on the old query.

**Result:** 3 minutes, 4 vCPU, every service and k6 on one host.

| Measure | Target | Before | After |
| --- | --- | --- | --- |
| API p95 | < 200 ms | 313 ms | 142 ms |
| Page p95 | < 1 s | 1.03 s | 680 ms |
| Ask median, keyword path | < 15 s | 2 s | 0.25 s |
| Failed requests | < 1% | 0% | 0% |

### Task 6: Exit check

- [x] Go tests, web type check and build, i18n check, unit tests, licence scans and Playwright pass locally.
- [x] The load test passes all four targets (Task 5).
- [ ] CI green, including the first `govulncheck` and `npm audit` runs.
- [ ] Pilot: one team uses Muasal daily for a month (`docs/pilot.md`, Exit). The results go into FSD §21's Progress paragraph.
