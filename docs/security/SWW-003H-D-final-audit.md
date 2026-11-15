# SWW-003H-D — Final Multi-Tenant Security Audit

**Audit date:** 15 November 2026  
**Scope:** SWW-001 through SWW-003 and existing hardening  
**Repository state:** `audit/SWW-003H-D`, based on `2170a2889503be372b77d8734c75b8ff57838860`  
**Audit mode:** Source review and local verification; no production data, deployment, commit, or push

## 1. Executive summary

The implemented identity, session, invitation, tenant-provisioning, school-settings, and frontend security boundaries were reviewed. Server-side checks, PostgreSQL-backed authorization, CSRF and same-origin protections, token hashing, and CI isolation are present for the implemented routes.

One Medium-severity password-reset issue was confirmed and remediated in this audit: completing a reset previously consumed only the submitted reset token, leaving other unused tokens for the same account valid until expiry. The transaction now consumes all unused reset tokens for that account. Unit and PostgreSQL integration regression tests cover the behavior. The PostgreSQL-backed integration test could not be executed locally because `SWW_TEST_DATABASE_URL` is not configured.

**Release recommendation: NO-GO for SWW-004 pending a successful PostgreSQL integration and CI run against this remediation.** The previous successful CI run is evidence for the pre-remediation baseline, not this patch. After the patch passes required CI, the repository is ready to begin SWW-004 subject to the tenant-scoping acceptance criteria in this report.

## 2. Scope and architecture reviewed

The application uses a Next.js App Router frontend, Go modular-monolith REST API, and PostgreSQL as the authoritative database. The audit did not introduce services, dependencies, migrations, or academic features.

### Security-sensitive inventory

- **Authentication and identity:** `services/api/internal/modules/auth/{handler.go,service.go,repository.go,middleware.go,password.go,mailer.go}`; `services/api/migrations/000002_auth_identity.up.sql`; `services/api/migrations/000003_auth_hardening.up.sql`.
- **Tenant authorization and settings:** `services/api/internal/modules/tenants/{handler.go,service.go,repository.go,models.go}`; `services/api/migrations/000004_tenants.up.sql`.
- **API wiring/configuration:** `services/api/internal/platform/http/router.go`, `services/api/internal/config/config.go`, `services/api/internal/platform/middleware/logging.go`, and `services/api/cmd/server/main.go`.
- **First administrator bootstrap:** `services/api/cmd/bootstrap/main.go`.
- **Database and migrations:** `services/api/internal/platform/db/db.go`, `services/api/migrations/runner.go`, and migration integration tests.
- **Frontend security and protected pages:** `apps/web/src/middleware.ts`, `apps/web/src/lib/security-headers.mjs`, `apps/web/next.config.ts`, `apps/web/src/app/dashboard/page.tsx`, `apps/web/src/app/platform/tenants/page.tsx`, and `apps/web/src/app/schools/[slug]/settings/page.tsx`.
- **Automated regression coverage:** Go auth/tenant repository, service, handler, middleware and migration tests; `apps/web/e2e/auth-security.spec.ts`; frontend unit and security-header tests; `.github/workflows/ci.yml`.

### Tenant-specific API surface reviewed

- `GET /api/v1/platform/tenants` and `POST /api/v1/platform/tenants`
- `GET /api/v1/platform/mail-capture` (registered only in development/test)
- `GET /api/v1/tenants/{slug}/settings` and `PATCH /api/v1/tenants/{slug}/settings`
- Authentication/session routes, invitation verification and acceptance, and the API's readiness endpoints

The tenant slug selects a record but does not grant authority. The platform tenant routes require a database-backed platform-admin grant; settings access is based on active membership and writes additionally require `SCHOOL_OWNER` or `SCHOOL_ADMIN`.

## 3. Threat model and assessment

| Threat | Assessment for implemented SWW-001–003 surface |
|---|---|
| Broken object-level authorization / cross-tenant access | No confirmed bypass in the implemented tenant APIs. Settings queries and updates join membership by tenant and user; integration tests cover cross-tenant rejection. Future academic resources are not implemented. |
| Broken function-level authorization | Platform tenant list/provision and local mail capture check a stored platform-admin grant. Settings writes check owner/admin membership in the SQL update. |
| Privilege escalation | Roles come from stored invitation and membership data; request DTOs reject unknown fields. The browser does not confer roles. |
| CSRF / cross-site requests | Authenticated unsafe routes require a session-bound CSRF token. Global middleware rejects cross-site Fetch Metadata and unapproved non-empty origins. |
| Session fixation / replay | Server generates random session tokens and stores hashes. Logout revokes sessions; expiry and revocation are checked when authenticating. Secure environments use `__Host-` Secure, HttpOnly, SameSite=Lax cookies. |
| Invitation replay | Acceptance locks a pending, unexpired invitation and consumes it in the same transaction that creates/activates identity, membership and session. Raw tokens are not stored; invitation URL path logging is redacted. |
| Password-reset replay | Submitted-token replay is rejected. This audit additionally fixed invalidation of other outstanding reset tokens for the same user. |
| Sensitive data exposure | Passwords and raw tokens are not logged by the inspected request logger; invitation tokens are redacted from paths. Password hashes and token hashes are excluded from JSON DTOs. Readiness failures are sanitized. |
| Insecure configuration / database fallback | Startup requires an explicit PostgreSQL URL and successful database connection. Production router construction rejects mock repository overrides. Bootstrap requires development mode plus explicit opt-in and credentials. |
| Input validation / SQL injection | JSON request bodies are size-limited, reject unknown fields and extra values; repository statements use parameterized SQL. No SQL injection path was identified in reviewed routes. |
| Race conditions | Invitation and reset token use are transactional and row-locked. Migration application uses a PostgreSQL advisory lock. Reset-token invalidation is transactional with the password update and session revocation. |
| Incomplete audit trails | Sensitive auth and tenant operations write redacted audit events in the relevant transaction. The database does not enforce audit immutability; see Finding 2. |

## 4. Findings and severity

### Finding 1 — Other outstanding password-reset tokens remained usable

- **Severity:** Medium
- **Status:** Remediated locally; PostgreSQL CI verification pending
- **Affected files:** `services/api/internal/modules/auth/repository.go`, `repository_mock.go`, `repository_integration_test.go`, `service_test.go`
- **Root cause:** `CompletePasswordReset` marked only the presented reset-token row as used. A second valid link issued for the same account could still change the password during its validity window.
- **Impact:** Someone who retained or obtained another reset link could reset the account again after a legitimate password change.
- **Remediation:** Within the existing transaction, mark every unused reset token for the same user as used. The mock repository matches that behavior.
- **Regression tests:** The service test issues two reset tokens, completes one, and asserts the other is rejected. The PostgreSQL auth lifecycle integration test does the same against the real repository.
- **Evidence:** `CompletePasswordReset` in `services/api/internal/modules/auth/repository.go`; the added assertions in `repository_integration_test.go` and `service_test.go`.
- **Blocks SWW-004:** Yes, until the PostgreSQL-backed test and CI run pass on this patch.

### Finding 2 — Audit-event append-only behavior is not enforced by PostgreSQL

- **Severity:** Low
- **Status:** Open; defense-in-depth improvement
- **Affected component:** `audit_events` in `services/api/migrations/000002_auth_identity.up.sql`
- **Evidence:** The table has no database trigger or privilege rule preventing updates/deletes. Application code inserts audit records, but database-level immutability is not established by the schema.
- **Impact:** A database principal with broader write access could alter or remove operational audit evidence.
- **Recommended remediation:** Before official results or safeguarding decisions depend on audit evidence, use a restricted application DB role and/or an append-only database control, and test that ordinary application credentials cannot update/delete audit events.
- **Blocks SWW-004:** No for academic-year/term/class setup; should be addressed before publication of official results or audit-dependent safeguarding workflows.

### Migration rollback caution

Down migrations drop tables, including tenant and identity data. The migration runner executes `Up` migrations and does not automatically invoke down scripts; no down migration was run during this audit. Treat manual production downgrades as destructive and require an approved backup/restore procedure.

No other confirmed Critical, High, or Medium findings were identified in the reviewed implemented surface.

## 5. Remediations applied

The reset transaction now consumes all unused reset tokens belonging to the account that was successfully reset. This operation remains in the same transaction as the password hash update, session revocation, and audit event. Unit and PostgreSQL integration regression assertions cover the second-token rejection. No database schema change or migration was needed.

No other code was changed as part of this audit. A pre-existing one-line `README.md` newline/line-ending difference was present before implementation and has been preserved.

## 6. Tests executed and actual outcomes

### Local checks

| Check | Result |
|---|---|
| `gofmt` on modified Go files | Passed |
| `go test -count=1 ./...` (`services/api`) | Passed across all Go packages |
| `go vet ./...` (`services/api`) | Passed |
| Targeted PostgreSQL integration tests | Skipped: `SWW_TEST_DATABASE_URL` is not configured. Auth lifecycle, tenant isolation, and migration runner integration tests each reported the expected skip. No developer, staging, or production database was accessed. |
| `go test -race ./...` | Not run: this Windows environment has CGO disabled; Go reported that race testing requires CGO. |
| `npm run test:ci` (`apps/web`) | Passed: 14 tests across 4 files |
| `npm run test:security` (`apps/web`) | Passed: 4 tests, 0 skipped |
| `npm run lint` (`apps/web`) | Passed with no ESLint warnings/errors; Next.js printed that `next lint` is deprecated for Next.js 16. |
| `npm run typecheck` (`apps/web`) | Passed |
| `npm run build` (`apps/web`) | Passed; Next.js 15.5.29 production build completed and routes were dynamically server-rendered. |
| `npx playwright test --list` (`apps/web`) | Passed; lists 3 browser tests. This lists tests only; it does not execute them. |
| `npm audit --json` (`apps/web`) | Passed with 0 reported vulnerabilities (0 low, moderate, high, or critical). |
| `git diff --check` | Passed |

Playwright E2E was not executed locally: its configuration requires matching loopback `DATABASE_URL` and `SWW_TEST_DATABASE_URL` for the disposable `sww_e2e_test` database, plus HTTPS test certificates. No local database was provisioned for this audit.

### GitHub Actions evidence

The latest recorded baseline run is [CI run 43632143231](https://github.com/AgbajiAlexander/school-without-walls/actions/runs/43632143231), for commit `2170a2889503be372b77d8734c75b8ff57838860`. The GitHub Actions API reports all three jobs successful:

- Backend: PostgreSQL 16 setup, required Go tests, `go vet`, and race tests.
- Frontend: unit/component tests, CSP/security-header tests, lint, typecheck, and production build.
- Browser: production frontend and real Go API against disposable PostgreSQL, with the authentication/CSP Playwright command completing successfully.

This run predates the password-reset remediation in this audit. It is baseline evidence only; it does not verify the new reset-token invalidation or regression assertions. A successful CI run against the patch remains required before GO.

The CI workflow generates random per-job PostgreSQL passwords, masks passwords and database URLs before use, and does not reuse `GITHUB_TOKEN` as a database credential. The browser job provisions a fictional CI account and disposable PostgreSQL database.

## 7. Remaining risks and limitations

- No academic resources exist yet, so teacher-to-class, student-to-profile, guardian-to-child, published-result, and academic cross-tenant authorization are not verified as implemented.
- `memberships` and current tenant records are scoped in application SQL, but PostgreSQL row-level security is not configured. Future tenant-owned academic tables must have `tenant_id`, tenant-aware foreign keys, scoped repository operations, and cross-tenant tests.
- Audit-event immutability is not enforced at the database layer.
- PostgreSQL integration and browser E2E tests for this patch remain pending a CI run; race tests were unavailable locally.
- Existing down migrations are destructive and must not be used operationally without an approved backup/restore procedure.

## 8. CI evidence

Workflow: `.github/workflows/ci.yml`. PostgreSQL integration is mandatory in CI through `SWW_REQUIRE_POSTGRES_TESTS=1`; the integration tests fail if the required database URL is absent. The previously successful run is linked above, and a new successful run against this patch is required to close Finding 1.

## 9. Release readiness recommendation

**NO-GO at this point.** The fix is implemented and local Go unit tests pass, but its real PostgreSQL integration regression test was skipped locally, and the previous successful CI run predates the patch. Do not start SWW-004 until a CI run on the remediation commit passes the required PostgreSQL tests, Go checks, frontend checks, and browser suite.

## 10. Explicit GO / NO-GO recommendation for SWW-004

**NO-GO pending CI verification of the password-reset remediation.** Once that run passes, the current SWW-001–003 surface is a reasonable foundation for SWW-004, provided each new tenant-owned academic record includes a tenant identifier, tenant-aware constraints, server-side membership and resource-relationship checks, and positive, unauthorized, and cross-tenant read/write tests. This report does not claim that academic, guardian, student, curriculum, AI, assessment, or safeguarding functionality is already implemented.
