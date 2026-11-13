# SCHOOL WITHOUT WALLS — MVP ARCHITECTURE

Status: Proposed ADR baseline (13 November 2026). The PRD is the source for behavioral requirements.

## Components

- **Web:** Next.js App Router, React, TypeScript, Tailwind, shadcn/ui. Browser uses same-origin BFF/proxy for API calls where suitable to simplify cookie/CORS security across Vercel frontend and separately hosted Go API. Use HTTPS, CSRF controls and no frontend secrets. Never authorize a request based solely on UI state.
- **API:** Go modular monolith, REST `/api/v1`; middleware resolves school + session membership, then enforces resource relation scopes in service layer. Modules: `auth`, `tenants`, `people`, `academics`, `curriculum`, `learning`, `attendance`, `assessment`, `communication`, `ai`, `files`, `audit`.
- **Data:** managed PostgreSQL, migrations, immutable report snapshots, tenant-aware FKs, optional tested RLS defense in depth. All server-authoritative data: role, tenant memberships, assessments, attendance, curriculum approval, grades, AI limits.
- **AI:** Go gateway -> Gemini using server secret. Structured draft response; curated curriculum context; per-role and age gating, hard quotas, moderation, redaction and logging.
- **Files:** private S3-compatible object storage; short-lived signed URLs, content-type validation and virus scan pipeline before file is released.
- **Ops:** Vercel frontend; Go backend on capable host; managed PostgreSQL; separate dev/staging/prod secrets and datasets; CI (Go + frontend + E2E), structured monitoring, backup and restore.

## Tenant request flow

1. Resolve requested tenant from trusted host/path mapping and verify it exists/is active.
2. Verify signed session and active membership for that tenant; reject conflicts between request tenant and session context.
3. Controller validates input and calls service with trusted `tenantContext` and authorized actor.
4. Service checks relationship/teacher assignment/guardian association and uses only tenant-filtered repository functions.
5. Repository executes tenant-scoped SQL and, where enabled, DB-level tenant policies using transaction-local context.
6. Audit privileged changes; return redacted response projection appropriate to role.

**Never** accept a client-supplied tenant ID as the tenant authority. All tenant-specific unique keys and FKs include tenant ID. Platform operators do not receive automatic classroom data access.

## Suggested repository shape

```text
apps/web/                   Next.js frontend
services/api/cmd/server/    Go server startup
services/api/internal/      feature modules, auth, middleware
services/api/migrations/    SQL versioned migrations
docs/                       PRD, architecture, implementation plan
.agents/rules/              Antigravity constraints
AGENTS.md                   repo-wide rules
```

## Example API response

```json
{
  "data": { "id": "<uuid>", "title": "Understanding Fractions", "status": "PUBLISHED" },
  "meta": { "request_id": "<uuid>" }
}
```

Error:

```json
{
  "error": {"code":"FORBIDDEN", "message":"Access denied", "details":{}, "request_id":"<uuid>"}
}
```

## Key ADRs to decide and record before coding

- ADR-001: Next.js frontend + Go modular monolith (no microservices).
- ADR-002: Postgres authoritative data; Go-generated UUID and CITEXT email; tenant composite keys.
- ADR-003: Tenant resolution using route/domain and server context, defense-in-depth SQL and authorization.
- ADR-004: Session cookie strategy for Vercel/Go cross-origin deployment; prefer same-origin proxy; CSRF safeguards.
- ADR-005: Curriculum version/reference model and licensed content provenance.
- ADR-006: AI gateway model, opt-in/gating, retention and red-team test contract.
- ADR-007: Report publication and immutable snapshot policy.
- ADR-008: Object storage and secure media lifecycle.

## Data boundaries

- Global entities: users, tenants, framework registry (published public metadata only), provider status.
- Tenant entities: memberships, students, guardians, all academic/learning/assessment/notice/AI records, media and audit events.
- Multiple tenant memberships per user are supported, but active tenant scope must be explicitly selected and checked on every request.

## Test pyramid

- Unit: grade computations, permission policies, curriculum matching, transitions, validation and AI prompt safety.
- Integration: real PostgreSQL for migration/foreign keys, tenant scoping, concurrent report publication, invitation replay, guardian unlink and transaction boundaries.
- End-to-end: Next.js and Go API in staging, student/teacher/guardian flows, mobile viewport and hostile cross-tenant ID tests.
- Security: dependency scanning, secret scanning, permission fuzz cases, prompt-injection test fixture, upload and CSRF tests.

**Deployment caveat:** A standard Vercel web deployment does not implicitly deploy the long-lived Go API described here. Configure the Go API as an independent service; connect over documented HTTPS routing with staging-tested cookie/CORS behavior.
