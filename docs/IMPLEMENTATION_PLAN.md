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
| [x] SWW-004A | Academic session and term lifecycle API/database | Three ordered valid terms; tenant isolation; one active session; atomic audit; PostgreSQL integration and concurrency checks pass | 003 |
| SWW-004B | Levels, classes and arms | School-configurable levels/classes with tenant-aware constraints and access tests | 004A |
| SWW-004C | Subjects, offerings and teacher assignments | Same-tenant active teacher assignments; tenant constraints and access tests | 004B |
| [x] SWW-004D | Responsive academic management workflow | Server-verified school context; accessible lifecycle states; real API E2E passes | 004A, 004B, 004C |
| [x] SWW-005 | Students, guardians, staff and enrollment | Linked guardian sees only own child; unlink revokes; audit events | 004D |
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

### SWW-004F-B21 Staff Invitation review corrections

- Staff issuance defaults to 10 invitations per actor per school and 25 total per school in a rolling 60-minute window. Set `STAFF_INVITATION_ACTOR_LIMIT` and `STAFF_INVITATION_TENANT_LIMIT` to integers from 1 through 1000, and `STAFF_INVITATION_WINDOW_MINUTES` from 1 through 1440. Use the same values on all API instances.
- PostgreSQL serializes quota decisions on the tenant row. Issuance explicitly uses Read Committed so waiting requests see the previous committed charge even when the pool defaults to another isolation level. A committed creation audit event charges one issuance in both scopes using database time sampled after the authorization locks, regardless of mail outcome. Invalid, denied, duplicate and rolled-back requests do not charge quota. Duplicate conflicts retain HTTP 409. An exhausted scope returns HTTP 429 and a rounded-up retry interval covering both scopes. Keep issuance audit events throughout the configured window.
- Issuance locks the active tenant, active issuer user, then active owner membership and holds those locks until commit. A suspension that wins the relevant lock and commits is observed and denies creation. If issuance wins and commits first, a later suspension does not retroactively cancel it. SMTP remains outside the transaction. Acceptance still locks invitation, tenant, user, membership. Issuance reads existing invitations without locking their rows. Confirmation updates only the invitation row.
- Dashboard and settings responses expose `can_invite_staff` from all active memberships. The browser uses this capability, and the write endpoint independently enforces owner authority. No migration is needed. The B14 ingress block, writer drain and mixed-version rollout requirements still apply.

### SWW-004F-B23 capability and quota boundary follow-up

- Academic management is granted in school responses when an active membership is School Owner, School Admin or Principal, matching the Go academic session and term management policy. Staff invitation capability still requires an active School Owner. The existing display role remains available to clients.
- The dashboard rechecks memberships when it regains focus or is restored from browser history. School settings refresh on entry, focus, browser history restoration, and Staff Invitation authorization denial. A discarded form cannot refresh a different school's settings.
- Quota regressions cover exact database-time window boundaries, committed issuance after failed delivery, duplicate requests, and independent PostgreSQL pools. No migration is needed. The B14 ingress block and writer drain still apply.

### SWW-004F-B25 denial handling and fixture isolation

- A read-only owner precheck rejects unauthorized invitation requests before they can contend for the school's issuance row lock. The subsequent tenant, user, and owner-membership locks still determine authority. Authorized requests still take the tenant lock and can contend for database capacity. The quota bounds issuance and mail volume.
- A denied invitation immediately disables submission. A forbidden response triggers a fresh, uncached capability and session check, scoped to the selected school. An affirmative result for the same school and a fresh CSRF token restore submission. A failed check leaves submission unavailable with a manual retry. An expired session refreshes to sign-in for the existing route. School switches abort pending checks.
- The loopback browser fixture uses an independent ephemeral authorization key. The browser test discards raw transport diagnostics before reporting a fixture failure.

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

### Academic acceptance and deployment access checkpoint (21 November 2026)

- SWW-004A, academic calendar scope. An independent read-only source review traced coverage of ordered terms, cross-tenant denial, audit rollback and concurrent activation. On the local Subject Management baseline 57d6953, all seven academics PostgreSQL suites passed with SWW_REQUIRE_POSTGRES_TESTS=1 against disposable loopback PostgreSQL 18. Academics Go vet passed. The scoped frontend run passed 124 tests across 17 files, and TypeScript passed. The disposable server was stopped after verification.
- SWW-004B and SWW-004C have passing repository evidence for structure, subjects, offerings and teacher assignment constraints on this branch. SWW-004D still requires a real authenticated browser journey through the academic dashboard, including a write, persistence, role denial and cross-school denial. The current Playwright entry point covers authentication and CSP. Keep SWW-005 behind that acceptance gate.
- SWW-DEPLOY-017. On this workstation Vercel API DNS returned IPv4 only. The HTTPS request timed out before TCP connected, and the CLI returned fetch failed. No shell or WinHTTP proxy was configured. Provider-side Git triggers are still unverified, so the deployment commit remains local. Supabase organization access was verified. Frankfurt is supported by the installed CLI, but organization billing and Free project eligibility still need owner confirmation.
- SWW-DEPLOY-018 and SWW-DEPLOY-019 remain pending. The Docker CLI cannot reach the Linux daemon, and no alternate local builder was found. No hosted resource was provisioned. PR #9 and the deployment worktree were untouched.

### SWW-004D local browser acceptance (21 November 2026)

- The independent reviewer checked the browser assertions against the Go authorization path. The owner creates a draft through the Next.js form, receives HTTP 201 from Go, and sees the stored draft after reload. A separately authenticated teacher can read the draft but cannot create a session, and the UI hides the write action. Go rejects a missing CSRF header with 403, a teacher write with a distinct 403, and cross-school access with 404. The browser also renders the cross-school denial. Tests use fictional roles and the existing HTTPS browser proxy.
- The Windows runner creates a new disposable PostgreSQL cluster and the exact guarded database `sww_e2e_test`, migrates it, and seeds two fictional schools. It runs the Edge acceptance spec and stops the cluster. No change to `sww_dev`, production schema, or application code. The Go test server retains its existing all-interface bind on port 8080. Run the fixture on a trusted development machine.
- Reproduce after installing the frontend dependencies with `powershell.exe -NoProfile -File .\scripts\test-academics-browser.ps1`. Git for Windows OpenSSL, local PostgreSQL and Microsoft Edge are required. The runner accepts `-PostgresBin` and `-OpenSSL` for alternative installed paths.
- Cloud provisioning and paid integrations are paused by owner direction. Keep deployment commit 403ee85 and PR #9 isolated. SWW-005 starts from FR-PEO-001, FR-PEO-002, FR-ACA-003 and FR-ID-004. Keep people records separate from permission grants, use tenant-aware relationships, require one active section per student/term, and test immediate guardian unlink and cross-school denial. Coordinate migration numbering with PR #9, which already owns 000008.

### SWW-005 — student, guardian, and enrollment schema slice

- Local branch `feature/SWW-005-people-enrollment` starts at accepted academics commit `3681ed7`. FR-PEO-001 and FR-ACA-003 storage is implemented in migration `000009_people_enrollment`. The existing Staff Invitation branch retains ownership of `000008`.
- Students and guardians can be recorded before receiving login access. Any linked identity must have the matching tenant role. Composite foreign keys constrain guardian relationships, academic year, class and arm. A partial unique index permits one active enrollment per student and term, while ended rows remain available for history.
- Mandatory PostgreSQL migration tests passed on a disposable loopback database, including duplicate admission numbers, wrong-role identity links, cross-school guardian links, incompatible enrollment scope, active enrollment uniqueness and retained history. Full Go regression, vet and build passed. No migration ran against `sww_dev`.
- SWW-005 remains open. The next application slice must enforce role and relationship checks in Go, lock and audit guardian link changes, implement atomic enrollment transfers, and provide the frontend journeys. No student or guardian data route is enabled by this migration. Verify the combined migration sequence with the Staff Invitation line before integration.

### SWW-005 — student summary and guardian revocation

- `GET /api/v1/tenants/{slug}/students/{studentID}` returns only the student ID, display name, and education stage. It requires a session and active tenant and user state. School managers can read the tenant summary. A student can read their own active record, and a parent can read an active record only while their active guardian relationship exists. Other requests receive a generic not-found response.
- `DELETE /api/v1/tenants/{slug}/students/{studentID}/guardians/{guardianID}` is a CSRF-protected soft unlink for active owners, school admins, and principals. It locks tenant, actor, and authority membership in that order. Unlink and its redacted audit event commit together. Each read rechecks the link, so a read started after commit is denied for a parent with no remaining authorized relationship.
- Student and guardian create/list/update, staff records, enrollment transfers, and frontend workflows remain outstanding. No migration was run against `sww_dev`.

### SWW-005 — atomic enrollment transfer

- `POST /api/v1/tenants/{slug}/students/{studentID}/enrollments/{enrollmentID}/transfers` accepts `class_id` and optional `class_arm_id`. The path identifies the expected current placement. A stale or unchanged placement returns 409. Missing or inaccessible records return 404.
- Go checks the active school and manager membership, locks the active student and academic session, and verifies the destination belongs to the same session. The existing placement ends at the same instant as the replacement begins in the same term. The audit event and both changes commit together. The original row remains available for enrollment history.
- The transfer endpoint does not create initial enrollments. Student and guardian management, staff records, enrollment creation and history views, and frontend journeys remain outstanding.

### SWW-005 — initial enrollment and history

- `POST /api/v1/tenants/{slug}/students/{studentID}/enrollments` takes `term_id`, `class_id`, and an optional `class_arm_id`. An active school manager with a valid session and CSRF token may place an active student in an active class within the term's active session. Creation and its redacted audit event commit together. Duplicate active placement returns 409.
- `GET /api/v1/tenants/{slug}/students/{studentID}/enrollments` is manager-only and sends `Cache-Control: no-store`. It returns active and ended placements with timestamps, up to 50 per page. Supply the returned `next_cursor` as `before` to retrieve older placements for that same student.
- Student and guardian management, staff records, enrollment frontend workflows, and browser acceptance remain outstanding.

### SWW-005 — student registration and profile management

- Active school owners, school administrators, and principals use `GET /api/v1/tenants/{slug}/students` for a tenant-scoped list of up to 50 records and `next_cursor` as the next request's `after` parameter. Registration uses `POST` on the same collection and replacement of editable fields uses `PUT /students/{studentID}` within the tenant path. Writes require CSRF and audit the record ID without profile values. A duplicate admission number returns 409.
- Registration captures admission number, display name, and education stage. This slice does not collect optional birth dates or grant account access. The student and guardian summary remains relationship-scoped and excludes admission numbers. Guardian and staff management and frontend acceptance remain outstanding.

### SWW-005 — guardian profiles and relationships

- Active school owners, school administrators, and principals can create and list guardian profiles via `POST` and `GET /api/v1/tenants/{slug}/guardians` and edit them via `PUT /guardians/{guardianID}` under the tenant path. Lists are uncached and paginated. Optional `student_id` restricts the list to active links for that student. Contact details are manager-only.
- `PUT /api/v1/tenants/{slug}/students/{studentID}/guardians/{guardianID}` saves the relationship label and activates the link. The existing `DELETE` soft-unlinks. Writes require CSRF and audit changes in the same transaction. Saving an identical active link does not write another audit event. Profile and link writes do not create identities or memberships.
- Staff records, student and guardian account onboarding, and frontend acceptance remain outstanding.

### SWW-005 — staff profiles

- `GET /api/v1/tenants/{slug}/staff` lists staff profiles for active school owners, school administrators, and principals. It returns up to 50 records with an optional `next_cursor` for the `after` query parameter. The response is uncached.
- `POST` on that collection records `membership_id`, `display_name`, and `staff_code` for an existing active staff membership in the same school. `PUT /staff/{staffID}` under the tenant path changes display name and staff code only. Writes require CSRF and commit with redacted audit events.
- Migration `000009_people_enrollment` adds staff profiles with tenant-aware membership references and one profile per school and user. Staff code is case-insensitively unique per school. Staff profile changes do not grant, update, or reactivate membership. Invitations remain a separate workflow.
- Student and guardian account onboarding, staff membership selection in the UI, people management screens, and browser acceptance remain outstanding.

### SWW-005 — staff membership selection and dashboard

- `GET /api/v1/tenants/{slug}/staff/candidates` lists active staff memberships without a profile, one candidate per user. It is manager-only, tenant-scoped, uncached and paginated using `after`. The response includes the existing account email so managers can distinguish staff with the same display name, but exposes no credentials or separate contact details.
- `/schools/{slug}/staff` loads staff profiles and candidates through authenticated Go endpoints before showing the form. It creates and edits staff profiles with CSRF protection and no membership changes. The existing disposable Edge runner now exercises the staff workflow as well as academics.
- Student and guardian onboarding and the remaining people and enrollment dashboard journeys are still open.

The local staff and academics browser runner uses PostgreSQL 15435, Next.js 13003, HTTPS proxy 13443, and Go API 18083. The hosted authentication/CSP workflow retains its existing default ports.

The staff and academic Edge journeys passed against production Next.js, the real Go API and disposable PostgreSQL. The owner selected a staff account, created and edited its profile, and saw the saved staff code after reload. The teacher and a school outside the owner's membership received the denial UI. The candidate disappeared after profile creation. The full Go regression with mandatory PostgreSQL, Go vet and build, 165 frontend tests, TypeScript, lint and production build passed.

### SWW-005 — student dashboard

- My Schools links to `/schools/{slug}/students`. The page loads the current session and the manager-only student list from Go before rendering any records or edit controls. The list uses the existing API cursor for navigation.
- Registration and editing use same-origin requests and the current session CSRF token. The form defaults to Primary, with the other approved stages available. Admission number conflicts and access failures produce safe messages. Student registration does not create an account or collect a birth date.
- Guardian and enrollment dashboard controls and student and guardian account onboarding remain pending.

The three Edge journeys passed against production Next.js, the Go API, and fresh disposable PostgreSQL. The owner registered and edited a student through My Schools and verified the saved education stage after reload. Teacher and cross-school views disclosed neither student data nor edit controls. All 168 frontend tests, TypeScript, lint, production build and diff checks passed.

### SWW-005 — guardian dashboard

- The Students page links to the manager-only guardian directory and to each student's relationship view. Managers can register and edit guardians, save a relationship, or confirm unlinking. Directory and relationship lists use independent cursors. The page loads the relevant Go responses before rendering profiles or controls.
- Confirmed unlinking removes the active relationship while retaining the guardian profile. Link changes use same-origin CSRF-protected calls. Guardian account invitation and identity binding remain separate work, along with the enrollment dashboard.

All four Edge journeys passed against production Next.js, the real Go API and disposable PostgreSQL. The guardian journey verified registration, editing, a link after reload, unlinking without deleting the profile, and cross-school denial without exposure of contact details. The 168 frontend tests, TypeScript, lint and production build passed.

### SWW-005 — enrollment dashboard

- Each student row links to a manager-only enrollment page. It lists active and ended placements with academic names and timestamps. The first enrollment form uses terms and classes from the active session. Class arms are loaded for the selected class. Transfers operate on the displayed active placement and retain the same term. History is cursor-paginated.
- The page uses the existing Go authorization, CSRF, transaction, and audit controls. Initial placements and transfers refresh history after success. A stale transfer or duplicate enrollment shows a conflict and refreshes history. Student and guardian account onboarding remains open.

All five Edge journeys passed against production Next.js, Go, and disposable PostgreSQL. The enrollment journey selected an active term, class and arm, transferred to a second class, and verified one ended and one active placement after reload. A cross-school route showed no placement details. Mandatory PostgreSQL Go tests, Go vet and build, 168 frontend tests, TypeScript, lint, production build, and diff whitespace checks passed.

### SWW-005 local invitation integration checkpoint

The local onboarding integration branch combines the verified people and enrollment checkpoint with both Staff Invitation commits from PR #9. The source commits remain traceable in the cherry-pick messages.

The combined branch passed the migration and full Go regression with PostgreSQL required, Go vet and build, all 189 frontend tests, TypeScript, lint, and the Next.js production build. Five academics and people journeys passed in Edge. A separate Edge run passed both staff roles in the existing Staff Invitation journey using the real Go API, disposable loopback PostgreSQL, and local mail capture.

Student and guardian account invitation and profile binding remain open under FR-ID-004. This checkpoint verifies the existing staff flow on the combined branch.

### SWW-005 profile invitation redemption

Migration 000010 requires each STUDENT or PARENT invitation to reference exactly one profile of the corresponding kind in the same tenant. Redemption locks the profile after the tenant, user, and membership checks, then rechecks expiry with the database clock. Profile binding, membership grant, token consumption, session creation, and acceptance audit share one transaction. A guardian invitation must still match the stored contact address, and a student must still be active and unbound.

The focused PostgreSQL test exercises delivery gating, cross-tenant target rejection, changed guardian contact rollback, exact account binding, and replay for both roles. Its fixtures insert invitations directly. The manager issuance API, email handoff, and browser workflow are still open. Before enabling issuance, migrate the database and upgrade or drain all older API instances, because their redemption code does not bind the target profile. Check for any legacy unbound STUDENT or PARENT invitation rows before applying the migration. The constraint rejects those rows instead of guessing a target.

### SWW-005 profile invitation issuance API

School Owners, School Administrators, and Principals with active membership may issue account invitations in an active school through the authenticated, CSRF-protected student and guardian profile routes. The student route takes an email address. The guardian route uses the profile's stored contact address. The routes determine the role on the server and return only the invitation identifier and expiry after successful mail handoff and database confirmation.

Staff and profile issuance share the configured rolling actor and school quotas under the same tenant lock. Duplicate invitations do not consume quota. Delivery or confirmation failure retains the issuance charge, and uncertain handoff can leave an unusable invitation awaiting delivery. A pending invitation for the same profile blocks a replacement until the first expires. Keep the documented rollout and ingress restrictions, and complete migration 000010 and the API upgrade before exposing either route.

The PostgreSQL HTTP regression covers authorization, CSRF, cross-school profile denial, guardian address override rejection, invalid student email, conflict, quota and retry guidance, local mail capture, acceptance of both captured links, and replay rejection. Browser manager controls and browser-level onboarding remain open.

### SWW-005 local browser acceptance

Student and guardian account invitations are available from the respective profile lists. Edge verified both account journeys using the real Go API, disposable loopback PostgreSQL, and local mail capture. The test confirmed accepted account roles after fresh sign-in, access to the intended student record, denial of the manager directory, replay rejection, and immediate guardian access revocation after unlinking. The existing staff invitation browser journey also passed in the same run. The 189 frontend tests, TypeScript, lint, and the production build passed.

SWW-005 local acceptance is complete. The migration and API rollout order above still applies before hosted exposure.

### SWW-006 curriculum registry schema

Migration 000011 adds a global framework edition registry and objectives indexed by stable edition-specific codes. Frameworks record publisher, jurisdiction, effective dates, source, and license metadata. Nigerian primary frameworks, developmental frameworks, and optional enrichment are distinguished by kind. Objective rows store the stage, grade band, subject, and strand for browsing and indexing.

Published metadata and objectives are retained, including after an edition is withdrawn. Editors create a new edition for revisions. The PostgreSQL regression verified coexisting editions, edition-scoped objective codes, Nigerian jurisdiction validation, and retention of a withdrawn edition's original objective. The fixture uses fictional text and does not confer curriculum rights or approval.

SWW-006 remains open. The published registry API, alignment review, tenant edition selection, and rights-reviewed content acceptance are pending.

### SWW-006 published curriculum catalog API

`GET /api/v1/curriculum/frameworks` lists published edition metadata and identifies Nigerian primary, developmental, and enrichment frameworks. `GET /api/v1/curriculum/objectives` requires a `framework_version_id` and accepts an optional `stage`. Both return up to 50 items in ID order with an optional `next` cursor passed back as `after`. Unknown, draft, and withdrawn editions return an empty objective list. The catalog is global and contains no tenant selections or learner information.

Reads require an authenticated user with an active membership in an active school. The PostgreSQL-backed router test verified anonymous denial, platform-operator denial without school membership, school and membership suspension, published-only results, provenance metadata, stage filtering, and cursor pagination. Registry browsing in the dashboard, alignment review, tenant session edition selection, and rights-reviewed content remain open.

### SWW-006 dashboard curriculum browser

My Schools links active members to the published curriculum catalog. The page displays edition provenance and source links, distinguishes Nigerian curriculum from optional enrichment, and supports education-stage filtering and cursor navigation. Session and membership checks remain in Go. The browser fixture uses fictional curriculum only.

All 189 frontend tests, TypeScript, lint, and the Next.js build passed. All six Edge journeys passed against the real Go API and disposable PostgreSQL. The curriculum journey verified anonymous redirect, visible provenance, separate enrichment labeling, draft exclusion, filtering, an empty result, and no horizontal overflow at 360 pixels.

Alignment review, tenant edition selection, and rights-reviewed content remain open.

### SWW-006 alignment storage and review gate

Migration 000012 implements the FR-CUR-004 storage and verification boundary. Nigerian objectives can link to enrichment objectives through one of the four declared relations. Mappings start as drafts. Review requires a reviewer identity and two published editions, stamps the review time in PostgreSQL, and preserves the approved mapping as history. The verified view omits drafts and hides a mapping when either edition is withdrawn while retaining its review record.

The PostgreSQL test covered invalid direction and relation, duplicate links, review before publication, missing reviewer, edits during review, visibility after review, immutability, and withdrawal. The database records reviewer identity. Reviewer authorization, audited review actions, verified catalog display, and tenant edition selection remain to be implemented in the application.

### SWW-006 alignment review API

Active platform administrators create a draft with POST /api/v1/curriculum/alignments and explicitly approve it with POST /api/v1/curriculum/alignments/{alignmentID}/review. Global catalog changes require the existing global grant. School roles do not authorize global publication. Both writes require an authenticated session and CSRF validation. The approval uses the authenticated actor, and each successful write records a global audit event in the same transaction.

Active school members can read verified links through GET /api/v1/curriculum/alignments with primary_objective_id and optional after cursor. The result includes the review date and omits the internal reviewer identifier. The PostgreSQL HTTP regression verified draft invisibility, forbidden school-role writes, CSRF, reviewer-field rejection, duplicate and premature approvals, audit counts, grant removal, and suppression after withdrawal.

The review UI and verified alignment display, tenant edition selection, and rights-reviewed content remain open.

### SWW-006 verified alignment display

Members can open verified enrichment for a published Nigerian objective. The browser shows the related objective, relationship, review date, edition, publisher, and source and license information. The read endpoint supplies these fields from the verified view and published registry. Draft and withdrawn mappings remain excluded, and reviewer identity stays internal.

The full Go suite with PostgreSQL required, Go vet and build, all 189 frontend tests, TypeScript, lint, and the production build passed. All six Edge journeys passed against the real API and disposable PostgreSQL. The curriculum journey confirmed the optional enrichment label, provenance link, draft exclusion, and return to the selected stage.

The platform review interface, tenant edition selection, and rights-reviewed content remain open.
