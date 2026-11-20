# School Without Walls — Antigravity Implementation Plan

Use task IDs below as bounded milestones. Each task ends with code, validation, tests, documentation and human review; do not skip tenant security gates.

| Task | Deliverable | Acceptance criteria | Dependencies |
|---|---|---|---|
| [x] SWW-001 | Repository scaffold, CI, API health/ready, local DB/migration workflow | Next.js boots; Go `health/ready` passes; no secrets committed; CI runs | None |
| [x] SWW-002 | Identity/session + invitations | Login, invitation acceptance, reset, logout, CSRF, revocation and abuse tests pass; PostgreSQL lifecycle integration passes; roles not client-controlled | 001 |
| [x] SWW-003 | Tenant provisioning + school settings | Two tenants independently configured; verified identity/membership checks; tenant isolation tests pass | 002 |
| [x] SWW-003H-A | Database, migration and startup security hardening | Explicit PostgreSQL configuration; API fails closed; migration ledger/status/up are transactional and repeat-safe; database-free and isolated-PostgreSQL tests pass | 003 |
| [x] SWW-003H-B | Authentication validation and API security hardening | Readiness diagnostics are sanitized; invitation validation returns 422 without side effects; password-reset workflow and security regressions are verified | 003H-A |
| [x] SWW-003H-C1 | Frontend security headers and Content Security Policy | Nonce-based production CSP and secure headers pass focused tests; local development remains usable | 003H-B |
| [x] SWW-003H-C2 | Real frontend API readiness and component tests | Same-origin bounded readiness endpoint and accessible status states pass route/component tests | 003H-C1 |
| [x] SWW-003H-C3 | Real-browser authentication and CSP verification | Production frontend, Go API and isolated PostgreSQL pass HTTPS cookie, auth, CSRF and browser CSP tests | 003H-C2 |
| [x] SWW-003H-C4 | Frontend and browser CI integration | Existing Go/PostgreSQL checks remain intact; random masked CI DB credentials and frontend/browser jobs pass | 003H-C2, 003H-C3 |
| [ ] SWW-004A | Academic session and term lifecycle API/database | Three ordered valid terms; tenant isolation; one active session; atomic audit; PostgreSQL integration and concurrency checks pass | 003 |
| SWW-004B | Levels, classes and arms | School-configurable levels/classes with tenant-aware constraints and access tests | 004A |
| SWW-004C | Subjects, offerings and teacher assignments | Same-tenant active teacher assignments; tenant constraints and access tests | 004B |
| SWW-004D | Responsive academic management workflow | Server-verified school context; accessible lifecycle states; real API E2E passes | 004A, 004B, 004C |
| SWW-005 | Students, guardians, staff and enrollment | Linked guardian sees only own child; unlink revokes; audit events | 004D |
| SWW-006 | Framework/version/objective registry and alignment workflow | Nigerian primary + British enrichment mapping; old versions retained, review gate | 004C |
| SWW-007 | Teacher lessons + student reading | Published lesson available to enrolled learner only; responsive text-first UI | 005,006 |
| SWW-008 | Assignment/submission/feedback/progress | Student submission, teacher feedback, score constraints, access tests | 007 |
| SWW-009 | Attendance + school remote mode + notices | Attendance update logged; remote mode banner without false presence | 005,007 |
| SWW-010 | Assessment scheme, scores, report approval/publication | Weights sum 100; no draft report leakage; version snapshots | 005,008 |
| SWW-011 | Early Years observations + parental dashboard | Qualitative reports; child-specific view; no forced numeric mark | 005,010 |
| SWW-012 | Go AI gateway + teacher drafts + student tutor | No exposed secrets, age/tenant gating, human review, safety tests | 006,007 |
| SWW-013 | E2E hardening, docs, deployment | CI/E2E/security/mobile checks; Vercel + Go HTTPS; backup restore tested | All |

### SWW-004F-B9 session scope and acceptance

On 20 November 2026 the project owner authorized SWW-004F-B9 on `feature/SWW-004F-subjects-offerings-assignments` ahead of the existing milestone order. Prior acceptance history and checklist entries still apply. This slice verifies the current endpoint policy: an active School Owner may invite School Administrators and Principals within their own active tenant.

- [x] Verify PostgreSQL-backed HTTP authorization, conflicts, token hashing, audit, delivery recovery, staff token redemption and owner activation on isolated `sww_structure_test`. The project owner reported that Terminal 3 passed both complete staff suites with PostgreSQL required, the expiration lock-wait case five consecutive times, and the full Go regression, vet and build.
- The staff create transaction locks the tenant and performs a non-locking lookup of existing invitations. Acceptance locks the existing invitation, tenant, user and existing memberships in that order. Staff creation inserts a new invitation row, so the reverse tenant/invitation order does not itself establish a cycle between those two paths. Owner delivery rollback locks the invitation before updating its tenant.
- At PostgreSQL's default Read Committed isolation, the pending-token predicate and row lock arbitrate concurrent token redemption. The tenant lock serializes acceptance with tenant state updates, and existing membership locks serialize updates to rows acceptance observes. The membership uniqueness constraint protects the same tenant, user and role even when an insert races.
- A read of existing memberships cannot lock an absent row. Future membership writers should acquire the tenant and user locks in a consistent order, especially for suspended memberships of a different role. Existing-owner authorization at issuance also reads membership status without a row lock. Cross-tenant first-time acceptance for the same email can contend on the global email constraint and fail transiently. These interleavings need coordinated database tests before claiming race safety.
- The expiry regression confirmed that an invitation could be accepted after its deadline while waiting for the tenant lock. Acceptance now uses `clock_timestamp()` for both expiry checks, with the final check after the invitation, tenant, user and membership lock waits and before granting access.
- Before B13, delivery happened after a redeemable token was committed. Failed revocation or concurrent redemption could leave access granted. B13 added the database delivery gate.

### SWW-004F-B10 staff onboarding verification

- [x] Automated backend verification. The project owner confirmed the expanded staff invitation HTTP workflow passed with PostgreSQL required in Terminal 3 against isolated `sww_structure_test`. It covers captured mail, redemption, a fresh login, dashboard membership and school API permissions, cross-school and owner-only denials, replay, expiry, and a pending second invitation after membership suspension.
- The invitation page uses role-neutral school copy and replaces the invitation URL after successful redemption. Verification, acceptance, and authenticated mail capture responses use `Cache-Control: no-store`. See B13 for the remaining delivery and lock-order risks.
- Local checks passed for Go regression, vet and build, 163 frontend tests, four security-header tests, frontend lint and typecheck, and the Next.js production build. The expanded PostgreSQL workflow compiled and skipped because the Codex terminal lacked `SWW_TEST_DATABASE_URL`. The workflow exercises the real API and dashboard data endpoints. The invitation page has separate component coverage.

- B12 completed the local browser-level E2E journey. B13 records the delivery-risk review and remaining operational risks.

### SWW-004F-B11 School Owner staff invitation interface

- [x] Controlled local browser demonstration of the owner-only Invite Staff action using fictional addresses and local mail capture. See B12 evidence below.
- The dashboard links to the selected school's invitation form. School settings verify the active membership through the API before showing the form, and the invitation endpoint rechecks authorization on submission. The form reports success only after a matching HTTP 201 receipt.
- Frontend verification passed with 175 tests, including 12 invitation interface cases, four security-header tests, TypeScript checking, lint, and the Next.js production build.

### SWW-004F-B12 controlled browser demonstration

- [x] Playwright ran one complete Microsoft Edge journey with both staff roles against a production Next.js build, the real Go router, disposable PostgreSQL 18, and in-memory mail capture. Two fictional schools and all accounts were created inside the isolated fixture. The web, API, and PostgreSQL used loopback ports 13000, 18080, and 15432 respectively.
- Edge verified one captured message per recipient, successful activation and fresh sign-in, the expected dashboard role, permitted academic session creation, cross-school and owner-only denials, and replay rejection. The owner form showed validation, duplicate invitation, authorization, session expiry, network, pending submission, and success states. An intercepted in-flight request was held while switching schools to verify that form state cleared.
- The journey checked that the acceptance response did not echo the password, invitation token or session cookie, and that the invite URL was replaced after redemption. It also checked browser console output, browser storage and JavaScript cookie visibility for the tested invitation secrets. The fixture used local HTTP and did not exercise deployed HTTPS or SMTP behavior.
- The browser command is `node node_modules/@playwright/test/cli.js test --config playwright.staff.config.ts` from `apps/web`. The configuration expects a production build in a separate `SWW_B12_WEB_DIR` built with `API_INTERNAL_URL=http://127.0.0.1:18080`, the matching disposable cluster on loopback port 15432, and an installed Chromium browser or `SWW_B12_BROWSER_CHANNEL=msedge`. The Go fixture creates a fresh schema and ephemeral operator for each invocation.
- Component tests (175), TypeScript, lint, isolated production build, and Go tests, vet and build passed. The ordinary Go regression ran without the existing integration database URL. The browser fixture itself used real PostgreSQL.

### SWW-004F-B13 delivery hardening and release review

#### Verified acceptance

- [x] New staff invitations start in `AWAITING_DELIVERY`. Verification and redemption already require `PENDING`. After the mailer reports success, a conditional update makes the invitation redeemable. Delivery-failure revocation can transition only an awaiting invitation and writes its audit event in the same transaction. Owner provisioning and existing invitation states keep their previous behavior.
- PostgreSQL tests simulated a lost SMTP confirmation and a failed revocation transaction. The affected token remained unverifiable and unredeemable. Additional cases verified redemption versus revocation, competing confirmation and revocation, and rollback of the losing transaction after a deliberately inverted membership writer triggered a PostgreSQL deadlock.
- All three focused PostgreSQL staff suites and the full Go regression passed with PostgreSQL required on disposable `sww_structure_test` at `127.0.0.1:15432`. Go vet and build passed. An earlier focused run on the fresh database exposed interference from shared extensions installed in temporary schemas. Preinstalling `citext` and `pgcrypto` in `public` resolved it, and backend CI now performs the same setup before testing.
- Release review excludes generated browser artifacts, TypeScript build metadata, backups and unrelated local files. The CI workflow YAML parsed locally. Hosted CI has not run for this working tree.

#### Remaining risks

- An SMTP server can accept a message even if its reply or the later QUIT reply is lost. The recipient may receive an unusable link, and the application cannot recall the email or guarantee inbox delivery. If database cleanup fails, the awaiting invitation remains unusable and may block reissue until expiration. A lost response after a committed confirmation can leave a usable invitation even though the API did not return success.
- Membership writers must lock the tenant, then the user, then membership rows when they need those locks. PostgreSQL resolves a deadlock by aborting a transaction, which can require a retry. The exercised inversion does not establish safety for every future writer.
- Apply migration `000008_staff_invitation_delivery` before enabling the new staff creation code and drain older staff invitation writers. Existing pending staff invitations are retained, so the new delivery gate applies to newly issued invitations. B12 used loopback HTTP and local capture. Deployed HTTPS and SMTP behavior remain outside that verification.

### SWW-004F-B14 release audit and rollout checkpoint

- Migration `000008_staff_invitation_delivery` adds one enum value without changing existing rows, the default, or invitation constraints. The migration runner serializes migrations with a transaction advisory lock and commits the enum addition with its ledger row before application code uses the new value. Existing `PENDING` invitations and School Owner provisioning retain their behavior.
- Disable staff invitation creation at ingress before updating any application instance, including routes that can reach an older API directly. Wait for in-flight staff sends to finish and assess any pending staff invitations issued by the older code. Apply migration 000008 and verify its ledger entry. Replace or drain every older writer, verify the hardened endpoint in staging, then reopen creation. The enum addition alone cannot prevent an older writer from inserting `PENDING`.
- If rollout fails, keep staff creation disabled and retain the additive enum migration. Existing readers continue to require `PENDING`, so awaiting rows remain ineligible even under older readers. If recovery is needed for a stranded link, leave it to expire or use an authorized, tenant-scoped and audited revocation followed by reissue through the hardened endpoint. Evidence of email delivery is required for activation. Do not convert stranded rows to `PENDING` or remove the enum value in an emergency down migration.
- The backend CI job uses disposable PostgreSQL 16 at loopback port 5432 with database `sww_structure_test`, mandatory integration tests, masked per-run credentials, and shared extensions installed before parallel schemas. Port 15432 is the separately guarded local test fixture and is outside hosted CI. The hosted browser job uses its own disposable database for HTTPS auth and CSP. The B12 staff invitation browser journey remains local.
- Staff invitation formatting normalizes editable school-name whitespace before putting the name in the mail subject. A regression test covers a school name containing CRLF.
- The proposed feature scope contains 31 source, test, migration, documentation and CI paths. Hosted CI, deployed route isolation, invitation-path log redaction at the edge, and SMTP behavior still require release verification. B14 stops before staging or committing.

### Previously recorded acceptance results

The repository previously recorded successful SWW-002 and SWW-003 acceptance checks, including Go tests, PostgreSQL integration checks, migration validation and frontend checks. Their original dates were future-dated, and the underlying historical run evidence is not available in this repository; these results are therefore retained only as unverified historical claims and were not independently confirmed during SWW-003H-A.

### SWW-003H-A acceptance verification

GitHub Actions run [43616938656](https://github.com/AgbajiAlexander/school-without-walls/actions/runs/43616938656) succeeded on 14 November 2026 for commit `5b66043`. The PostgreSQL 16 `sww_ci_test` service ran the migration up/repeat, ledger, rollback and concurrency checks plus the authentication and tenant integration tests. `go test -count=1 ./...`, `go vet ./...`, `go test -race ./...`, dependency verification, frontend typecheck and production build all passed. `SWW_REQUIRE_POSTGRES_TESTS=1` made missing database configuration fail rather than skip required integration tests.

### SWW-003H-B acceptance verification

Implementation commit [`947049d`](https://github.com/AgbajiAlexander/school-without-walls/commit/947049d30846d2f82d19d68526338b045860ac36) passed GitHub Actions run [43622592103](https://github.com/AgbajiAlexander/school-without-walls/actions/runs/43622592103). Review of that run's initialization logs found the static disposable PostgreSQL password was printed, so the CI-only credential was changed to the automatically masked, run-scoped GitHub token in commit [`d492611`](https://github.com/AgbajiAlexander/school-without-walls/commit/d4926112025f79f8994d13e5728872024f972d33). That hardening commit passed GitHub Actions run [43622885687](https://github.com/AgbajiAlexander/school-without-walls/actions/runs/43622885687), including verification that the service password was masked and no unmasked database URL appeared in captured job logs.

Local validation passed: `gofmt` on modified Go files, `go test -count=1 ./...`, `go vet ./...`, and `git diff --check`. In both successful CI runs, PostgreSQL 16 backed the required test suite with `SWW_REQUIRE_POSTGRES_TESTS=1`. The auth, tenant, and migration packages passed; the integration test harness fails rather than skips when the required database URL is absent. Migration tests cover concurrent execution, repeat/up ledger state and failure rollback. The race-enabled Go test step, dependency verification, frontend typecheck and production build also passed. The run-scoped database was disposable CI infrastructure; no developer, staging or production database was used.

### SWW-003H-C1 verification

`node --test src/lib/security-headers.test.mjs` passed all 4 tests; `npm run typecheck`, `npm run build`, and `git diff --check` passed. The Next.js 15.5.29 production build reports App Router pages as dynamically server-rendered. A production-server HTTP smoke check returned 200 for the landing, login, forgot-password, reset-password, and invitation pages; each response had a unique nonce, and the rendered script nonce matched its response CSP. The same check confirmed `/api/health` is excluded from CSP middleware and no HSTS was sent over local HTTP. A Next.js development-server smoke check returned 200 for login without production CSP/HSTS and confirmed unauthenticated `/dashboard` still redirects to `/login` (307). The final C3 Playwright run also verified per-request nonce matching, hydration, client navigation, authentication and no relevant CSP violations.

### SWW-003H-C2/C3/C4 implementation verification

The bounded readiness helper/route and React login/readiness tests are implemented. Locally, `npm ci` succeeded; `npm run test:ci` passed 14 tests; `npm run test:security` passed 4 tests; and `npm run typecheck`, `npm run lint`, `npm run build`, `go test -count=1 ./...`, `go vet ./...`, and `git diff --check` passed. `npx playwright test --list` found all 3 browser tests, and the CI workflow YAML parsed successfully. PostgreSQL-backed Go tests and Playwright were not run locally because no isolated local test database was configured; local `go test` therefore skipped DB-backed cases. `go test -race ./...` could not run on Windows because CGO is disabled and no GCC compiler is installed.

The final GitHub Actions verification passed at commit [`6301833aba7ea4275a4253ad46fc956bb14ef276`](https://github.com/AgbajiAlexander/school-without-walls/commit/6301833aba7ea4275a4253ad46fc956bb14ef276), run [43631888387](https://github.com/AgbajiAlexander/school-without-walls/actions/runs/43631888387). All three jobs succeeded: PostgreSQL 16-backed Go tests, vet and race tests; frontend unit/component (14), CSP/security-header (4), lint, typecheck and production build; and all 3 Playwright tests against the production frontend, Go API and disposable PostgreSQL database. The browser checks passed for unauthenticated redirect, invalid credentials, real login and verified identity, Secure/HttpOnly/SameSite=Lax cookie behavior, JavaScript cookie inaccessibility, CSRF rejection, logout revocation, protected-route rejection, CSP nonces, hydration, client navigation and absence of relevant CSP violations. Required database tests ran; none were silently skipped.

The first hosted attempts exposed an unsupported `ubuntu-latest` image for Playwright 1.55.1 and ambiguous password/error locators. The browser job was pinned to `ubuntu-24.04`, and the locators were narrowed without weakening assertions; the final run above passed. CI creates a fresh cryptographically random PostgreSQL password per database job and masks both the password and complete connection URI before use; GitHub Actions logs show only `***` for the database URL variables. `GITHUB_TOKEN` is not used as a database credential. The browser job uses a separately generated fictional account and disposable PostgreSQL 16; no developer, staging or production database is used.

The initial dependency audit flagged PostCSS source-map/CSS-stringification advisories and a minimatch pattern-amplification advisory. Targeted overrides use PostCSS 8.5.33 under Next.js 15 and minimatch 11.0.1 for the affected ESLint parser chain; Next.js was not major-upgraded. Clean `npm ci`, lint, build and the final `npm audit` all passed, with 0 reported vulnerabilities.

### SWW-004A verification (15 November 2026)

`gofmt` passed for the new academics package and router changes. `go test -count=1 ./...` and `go vet ./...` passed. The focused academic unit/service tests passed, including validation, permissions, cross-tenant denial, lifecycle, term overlap and one-active-session behavior. The PostgreSQL integration test was discovered but skipped because `SWW_TEST_DATABASE_URL` is not configured on this Windows workstation; no migration was applied to the developer database. The test is mandatory in CI because the workflow sets `SWW_REQUIRE_POSTGRES_TESTS=1`. Focused race testing is blocked locally because CGO is disabled (`go test -race` reports that CGO is required). The SWW-004A checklist remains unchecked until the isolated PostgreSQL integration and concurrency tests run successfully.

## Sprint rhythm (not calendar commitments)

For each task: read PRD IDs -> write API contract and tests -> migration -> Go handler/service/repository -> frontend -> permission and abuse tests -> review -> commit. The product owner accepts the outcome before the agent starts the next task.

## Suggested initial commands (after scaffold)

```powershell
# run frontend in its folder
cd apps/web
npm install
npm run dev

# separate PowerShell session for API
cd services/api
go test ./...
go run ./cmd/server
```

Commands may be adjusted to actual generated directory and package scripts; do not fabricate a passing test result. Database credentials only in local ignored environment files.

## First Antigravity task prompt

> You are working in the School Without Walls repository. First read `AGENTS.md`, `.agents/rules/security.md`, `docs/PRD.md`, `docs/ARCHITECTURE.md` and this plan. Do not implement anything outside **SWW-001**. Create a Next.js + TypeScript frontend scaffold and Go modular-monolith backend scaffold with PostgreSQL configuration and migrations, `/health` and `/ready`, CI checks, environment examples, Windows PowerShell setup instructions, and basic unit/integration tests. Keep all secrets outside Git, use fictional seed data only, and provide exact test results and a proposed commit message. Do not advance to SWW-002 until reviewed.
