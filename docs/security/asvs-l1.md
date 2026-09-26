# OWASP ASVS level 1 review

FSD §18 asks for an OWASP ASVS 4.0 level 1 checklist before the pilot. This review covers each chapter against the code on `main` as of 26 Sep 2026. It names the test or file that shows each control, and lists what is still open.

**Status:** ready for a pilot inside a customer network. Before the public launch, repeat the review with an outside tester.

## V2 Authentication

| Requirement | Status | Evidence |
| --- | --- | --- |
| Passwords of 12+ characters, no maximum below 64, all characters allowed (2.1.1–2.1.4) | Done | `auth.CheckPolicy`, `TestCheckPolicy`, `TestSetupRejectsWeakPasswords` |
| Checked against breached or common passwords (2.1.7) | Done | 100,000-entry SecLists list, embedded (`auth/common-passwords.txt`) |
| Password change needs the current password (2.1.6) | Done | `TestChangingPasswordNeedsTheCurrentOne` |
| Brute-force protection (2.2.1) | Done | 5 failures lock an account for 15 minutes, and 20 attempts per IP per minute are rate-limited: `TestFiveFailuresLockTheAccount`, `TestTwentyOneAttemptsFromOneIPAreRateLimited` |
| Passwords stored with a slow hash (2.4.1) | Done | argon2id, `TestHashAndCheckPassword` |
| Initial and reset secrets random, short-lived and single-use (2.3.1, 2.5.x) | Done | 256-bit setup links, 72 hours, stored hashed: `TestExpiredSetupLinkIsRefused`, `TestResetPasswordEndsSessionsAndIssuesANewLink` |
| No default accounts (2.5.4) | Done | The installer creates the first admin with a setup link; no built-in credentials |
| Unknown and known emails answer alike (2.2.x) | Done | `dummyHash` makes sign-in for an unknown email as slow as for a known one |

## V3 Session management

| Requirement | Status | Evidence |
| --- | --- | --- |
| 256-bit random tokens, stored hashed (3.2.2) | Done | `auth.NewToken`, `TestNewToken` |
| Cookie `HttpOnly`, `SameSite=Lax`, `Secure` over HTTPS (3.4.1–3.4.3) | Done | `TestSessionCookieFlags` |
| New token at sign-in (3.2.1) | Done | `TestLoginStartsASessionAndIsAudited` |
| Sign-out, disable and password reset end sessions (3.3.1, 3.3.3) | Done | `TestLogoutRevokesTheSession`, `TestDisablingAUserSignsThemOut`, `TestResetPasswordEndsSessionsAndIssuesANewLink` |
| Idle and absolute timeouts (3.3.2) | Done | 12 hours idle, 7 days absolute: `TestIdleSessionExpires` |

## V4 Access control

| Requirement | Status | Evidence |
| --- | --- | --- |
| Deny by default; checks on the server (4.1.1–4.1.3) | Done | The visibility predicate lives in SQL; there are no Go-side filters (§5.3) |
| Row-level access across projects and clients (4.2.1) | Done | The permission suite runs a seeded role and client matrix on every list, read, search and Ask path: `TestPermissionSuiteReads`, `TestPermissionSuiteWrites`, `TestPermissionSuiteAsk`. It is merge-blocking in CI. |
| Hidden and missing look the same | Done | 404 for both, in every endpoint test |
| CSRF (4.2.2) | Done | Writes need the app's `Origin` (`TestWritesNeedTheAppOrigin`), plus `SameSite=Lax` cookies |
| Admin functions need the admin flag (4.3.1) | Done | `requireAdmin`; `TestNonAdminsCannotManageUsers`, and the Ask log, audit, backups and status tests |

## V5 Validation, sanitisation and encoding

| Requirement | Status | Evidence |
| --- | --- | --- |
| Input validated by type, length and format (5.1.x) | Done | OpenAPI-generated types, then per-field checks with 422 problem details |
| No raw HTML from users (5.2.x, 5.3.x) | Done | Markdown through `rehype-sanitize`, and images only from the app's own attachment URLs |
| SQL injection (5.3.4) | Done | All SQL through sqlc and pgx parameters |
| Prompt injection (Ask) | Mitigated | Evidence is fenced as data, output is schema-constrained, the model has no tools, and every citation is checked against the evidence (§11.5) |

## V7 Error handling and logging

| Requirement | Status | Evidence |
| --- | --- | --- |
| No stack traces or internals in responses (7.4.1) | Done | RFC 9457 problems with codes; `s.fail` logs the error and returns a generic 500 |
| Security events logged (7.1.x, 7.2.x) | Done | `audit_events` is append-only (UPDATE and DELETE revoked from the app role); sign-in is audited |
| No secrets or personal text in logs (7.1.1) | Done | Logs carry IDs; question text lives only in the Ask log (§18.2) |
| Log access for admins (7.3.x) | Done | Admin → Audit log with CSV export, and Admin → Ask log |

## V8 Data protection

| Requirement | Status | Evidence |
| --- | --- | --- |
| Sensitive data not cached by browsers (8.2.1) | Done | `Cache-Control: no-store` on every API response (`requestContext`), asserted in `TestSecurityHeaders` |
| Secrets at rest encrypted (8.3.x) | Done | AI API keys sealed with AES-GCM under `APP_SECRET_KEY`, never returned (R-AI-3) |
| Data leaves only by admin choice | Done | Local and off modes make no outbound calls; BYOK needs the audited acknowledgement (R-AI-1) |

## V9 Communications

| Requirement | Status | Evidence |
| --- | --- | --- |
| TLS for all traffic (9.1.1) | Done in deploy | Caddy with the customer's certificate or its internal CA; HSTS for a year |
| Internal services not exposed (9.2.x) | Done | Only Caddy publishes ports; app, web, db, backup and model sit on an internal network |

## V12 Files and resources

| Requirement | Status | Evidence |
| --- | --- | --- |
| Upload size limits (12.1.1) | Done | 25 MB per file: `TestAttachmentLimits` |
| Downloads cannot run as content (12.5.x) | Done | `nosniff`, `attachment` disposition except for images, and a closed CSP on API responses |
| Files reachable only through the ticket's access (12.4.x) | Done | `TestAttachmentAccessFollowsTheTicket` |
| Uploads named by hash, not by user input (12.3.x) | Done | Stored as `sha256` under the attachments volume |

## V13 API

| Requirement | Status | Evidence |
| --- | --- | --- |
| Same checks on every API endpoint (13.1.x) | Done | One middleware chain, with the auth and origin checks, for `/api/v1` |
| Content types enforced (13.2.5) | Done | JSON decoding refuses unknown fields; uploads are multipart only |
| Rate limits on expensive calls | Done | Sign-in per IP; Ask 10 a minute per user |

## V14 Configuration

| Requirement | Status | Evidence |
| --- | --- | --- |
| Security headers (14.4.x) | Done | Nonce CSP on pages, a closed policy on the API, `nosniff`, `Referrer-Policy`, `frame-ancestors 'none'`, HSTS: `TestSecurityHeaders` and `web/e2e/security.spec.ts` |
| Dependencies from trusted sources, licences checked (14.2.x) | Done | Pinned versions and lockfiles; the licence scan runs in CI |
| Dependency vulnerabilities (14.2.1) | Done | `govulncheck` and `npm audit --omit=dev --audit-level=high` run in CI |
| Secrets generated per install (14.1.x) | Done | `install.sh` writes new database passwords and `APP_SECRET_KEY` into `.env` (mode 600) |
| Server banners removed (14.3.x) | Done | Caddy's `Server` header dropped; Next's `X-Powered-By` off |

## Open items

1. An outside penetration test before the public launch.
