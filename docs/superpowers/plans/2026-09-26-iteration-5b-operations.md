# Muasal Iteration 5b — Hardening, Backups, Offline Installer and the Exit Check

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Muasal can be installed from one file on a server without internet, kept safe and healthy, backed up and restored. This is the second half of FSD §21 Iteration 5. Its exit check: the 100,000-ticket load test and an offline install on a clean VM both pass.

**How this plan was written:** each task was built test-first on branch `feat/iteration-5b` and committed as named below; the commits hold the code. This plan lists the tests, interfaces, deliberate deviations and the exit-check results.

**Architecture:** Same stack and patterns as Iterations 0–5a.
- **Headers (§18.2):**
  - Every page gets a per-request nonce Content-Security-Policy from `web/proxy.ts`, which Next.js applies to its own scripts.
  - The API sends `default-src 'none'; frame-ancestors 'none'`, `nosniff` and `Referrer-Policy: same-origin`.
  - Caddy adds HSTS and drops its `Server` header.
- **Backups (§15.6, §19.4):**
  - A `backup` service (the pgvector image, so `pg_dump` matches the server) runs `deploy/backup.sh`: daily at 01:00, or within 30 seconds of "Run backup now", with 14 days of dumps.
  - The app, a non-root user, mounts the same volume. It lists the dumps and leaves `requests/run-now`, in a folder `backup.sh` makes writable.
  - `deploy/restore.sh` puts a dump back.
- **System status (§18.3):**
  - One collector feeds Admin → System status (with warnings) and `GET /metrics` (Prometheus text).
  - It reads the database size, River jobs by state, Ask log counts, a 3-second model probe, and statfs of the two volumes.
  - `/metrics` is on the app's root; Caddy does not route it.
- **Installer (§19.3):**
  - `deploy/make-bundle.sh <version>` saves every image into `images.tar` and adds the deploy files, with `build:` lines stripped. `--models DIR` adds an Ollama models folder.
  - `install.sh` loads the images, writes `.env` secrets and the TLS snippet, and copies the models. It then starts with `--pull never`, creates the first admin and runs `app admin ai-local --tier` for local AI.
  - `upgrade.sh` backs up first, then swaps images and deploy files.
- **Load test (§18, §21.1):**
  - `deploy/loadtest/seed.sql` builds project LOAD in SQL: 100,000 tickets, 200,000 comments and 1,000,000 chunks. It also mints 50 sessions, because sign-in allows only 20 attempts per IP per minute.
  - `k6.js` runs 50 browsing users and 5 asking users for 3 minutes; `run.sh` ties them together.

**Spec:** Claude Docs "FSD — Muasal" (rev 110): §15.6, §18, §19, §21.

## Deliberate Deviations from the FSD

- The "Run backup now" marker is `/backups/requests/run-now` rather than `/backups/.run-now`, because the app runs as a non-root user and needs a folder it may write.
- Load-test chunks carry no vectors, so Ask runs on the keyword path under load. Model latency is measured separately by `app eval` on each tier (§11.8, §18.1), where the model server, not the app, is the bottleneck.
- The installer's `ai-local` presets cover the Ollama tiers (dev, minimum, recommended). The large vLLM tier is set up by hand in Admin → AI.
- The offline check here runs on the development container: the stack is installed from the bundle with the Muasal images removed first, `--pull never`, and the internal network verified to have no route out. The FSD's clean-VM install with a packet capture is repeated on real hardware before the pilot.
- The online installer (`--online`) needs the images published to a registry, which comes with the release pipeline (Launch).

## Tasks

### Task 1: Security headers

**Commit:** `feat: security headers: nonce-based CSP on pages, a closed policy on the API, HSTS`
- `web/proxy.ts` (CSP, nosniff, Referrer-Policy on every page, sign-in redirect only off login and setup); `securityHeaders` in `httpapi`; HSTS in `deploy/Caddyfile`.
- Tests: `TestSecurityHeaders`; `web/e2e/security.spec.ts` checks the headers and that signing in, Home, the Ask panel and the audit log run with no CSP violation.

### Task 2: Backups and system status

**Commit:** `feat: backups (service, page, restore.sh) and system status with /metrics`
- `GET /admin/backups` → `BackupList`, `POST /admin/backups/run` (202; 503 `backup_unavailable` without the service), `GET /admin/system/status` → `SystemStatus`, `GET /metrics`.
- `config.BackupsDir` (`BACKUPS_DIR`, default `/backups`); compose `backup` service and the app's `backups` volume.
- Pages `/admin/backups` (last backup, list, Run backup now, restore steps) and `/admin/system` (figures, meters, warnings).
- Tests: `TestBackupsPage`, `TestBackupsWithoutTheService`, `TestSystemStatus` (a downed model server warns), `TestMetrics`; `web/e2e/ops.spec.ts` runs a real backup through the button.
- Drill (manual, done on 26 Sep 2026): a backup requested through the API, the data changed, `./restore.sh db-…dump` brought it back and the app came back healthy.

### Task 3: Offline bundle, installer and upgrade

**Commit:** `feat(deploy): offline bundle, install.sh and upgrade.sh, with tier presets through app admin ai-local`
- `ai.Settings.LocalForTier(url, tier)`; `app admin ai-local --url --tier`.
- `deploy/make-bundle.sh`, `install.sh`, `upgrade.sh`, `caddy.d/` for the TLS snippet, `certs/`; `docs/operations.md` is the bundle's README.
- Test: `TestLocalForTier`.

### Task 4: Load test

**Commit:** `test(deploy): the 100,000-ticket load test`
- `deploy/loadtest/{seed.sql,k6.js,run.sh}`.

### Task 5: Exit check

- [ ] **Offline install:** `deploy/make-bundle.sh 0.5.0-rc1`; remove `muasal-*` and `caddy` images; unpack; `./install.sh --yes --url http://localhost --ai off …`.
- [ ] **Smoke test:** setup link, sign-in, project, ticket, ticket page 200, CSP present; the backup container cannot resolve outside names; `muasal_app` is `internal`.
- [ ] **Upgrade:** `./upgrade.sh muasal-0.5.0-rc2.tar` took a backup, loaded the new images, updated `VERSION` and restarted on them.
- [ ] **Load test:** `deploy/loadtest/run.sh 100000` against the stack; results below.

**Results (26 Sep 2026, development container: 4 vCPU, 15 GB, all services and k6 on one host):** see the PR description.
