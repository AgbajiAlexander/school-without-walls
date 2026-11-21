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
| SWW-005 | Students, guardians, staff and enrollment | Linked guardian sees only own child; unlink revokes; audit events | 004D |
| SWW-006 | Framework/version/objective registry and alignment workflow | Nigerian primary + British enrichment mapping; old versions retained, review gate | 004C |
| SWW-007 | Teacher lessons + student reading | Published lesson available to enrolled learner only; responsive text-first UI | 005,006 |
| SWW-008 | Assignment/submission/feedback/progress | Student submission, teacher feedback, score constraints, access tests | 007 |
| SWW-009 | Attendance + school remote mode + notices | Attendance update logged; remote mode banner without false presence | 005,007 |
| SWW-010 | Assessment scheme, scores, report approval/publication | Weights sum 100; no draft report leakage; version snapshots | 005,008 |
| SWW-011 | Early Years observations + parental dashboard | Qualitative reports; child-specific view; no forced numeric mark | 005,010 |
| SWW-012 | Go AI gateway + teacher drafts + student tutor | No exposed secrets, age/tenant gating, human review, safety tests | 006,007 |
| SWW-013 | E2E hardening, docs, deployment | CI/E2E/security/mobile checks; Vercel + Go HTTPS; backup restore tested | All |

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
