# School Without Walls — Antigravity Implementation Plan

Use task IDs below as bounded milestones. Each task ends with code, validation, tests, documentation and human review; do not skip tenant security gates.

| Task | Deliverable | Acceptance criteria | Dependencies |
|---|---|---|---|
| SWW-001 | Repository scaffold, CI, API health/ready, local DB/migration workflow | Next.js boots; Go `health/ready` passes; no secrets committed; CI runs | None |
| SWW-002 | Identity/session + invitations | Login/invite/reset/logout and negative tests pass; roles not client-controlled | 001 |
| SWW-003 | Tenant provisioning + school settings | Two tenants independently configured; verified identity/membership checks; tenant isolation tests pass | 002 |
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
