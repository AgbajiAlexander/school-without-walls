# School Without Walls — Antigravity Implementation Plan

Use task IDs below as bounded milestones. Each task ends with code, validation, tests, documentation and human review; do not skip tenant security gates.

| Task | Deliverable | Acceptance criteria | Dependencies |
|---|---|---|---|
| [x] SWW-001 | Repository scaffold, CI, API health/ready, local DB/migration workflow | Next.js boots; Go `health/ready` passes; no secrets committed; CI runs | None |
| [x] SWW-002 | Identity/session + invitations | Login, invitation acceptance, reset, logout, CSRF, revocation and abuse tests pass; PostgreSQL lifecycle integration passes; roles not client-controlled | 001 |
| [x] SWW-003 | Tenant provisioning + school settings | Two tenants independently configured; verified identity/membership checks; tenant isolation tests pass | 002 |
| [x] SWW-003H-A | Database, migration and startup security hardening | Explicit PostgreSQL configuration; API fails closed; migration ledger/status/up are transactional and repeat-safe; database-free and isolated-PostgreSQL tests pass | 003 |
| SWW-003H-B | Authentication validation and API security hardening | Readiness diagnostics are sanitized; invitation validation returns 422 without side effects; password-reset workflow and security regressions are verified | 003H-A |
| SWW-004 | Academic year, terms, levels, subjects and teacher assignments | Structural validation, assigned permissions and tenant constraints | 003 |
| SWW-005 | Students, guardians, staff and enrollment | Linked guardian sees only own child; unlink revokes; audit events | 004 |
| SWW-006 | Framework/version/objective registry and alignment workflow | Nigerian primary + British enrichment mapping; old versions retained, review gate | 004 |
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

### SWW-003H-B verification in this worktree

`go test -count=1 ./...` and `go vet ./...` passed locally. The suite exercised unit and handler coverage for sanitized readiness failures, invitation validation and side-effect prevention, generic password-reset responses, captured reset-email completion, token expiry/replay, session revocation, old/new password behavior, reset rate limiting, and secret-free reset logs. PostgreSQL integration tests were not run: `SWW_TEST_DATABASE_URL` was unset, and no existing database was contacted. The task remains unaccepted pending isolated PostgreSQL integration verification and technical review.

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
