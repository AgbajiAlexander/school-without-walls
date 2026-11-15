# SCHOOL WITHOUT WALLS — MVP ARCHITECTURE

Status: Proposed ADR baseline (13 November 2026). The PRD is the source for behavioral requirements.

## Components

- **Web:** Next.js App Router, React, TypeScript, Tailwind, shadcn/ui. Browser uses same-origin BFF/proxy for API calls where suitable to simplify cookie/CORS security across Vercel frontend and separately hosted Go API. Use HTTPS, CSRF controls and no frontend secrets. Never authorize a request based solely on UI state.
- **API:** Go modular monolith, REST `/api/v1`; middleware resolves school + session membership, then enforces resource relation scopes in service layer. Modules: `auth`, `tenants`, `people`, `academics`, `curriculum`, `learning`, `attendance`, `assessment`, `communication`, `ai`, `files`, `audit`.
- **Data:** managed PostgreSQL, migrations, immutable report snapshots, tenant-aware FKs, optional tested RLS defense in depth. All server-authoritative data: role, tenant memberships, assessments, attendance, curriculum approval, grades, AI limits.
- **AI:** Go gateway -> Gemini using server secret. Structured draft response; curated curriculum context; per-role and age gating, hard quotas, moderation, redaction and logging.
- **Files:** private S3-compatible object storage; short-lived signed URLs, content-type validation and virus scan pipeline before file is released.
- **Ops:** Vercel frontend; Go backend on capable host; managed PostgreSQL; separate dev/staging/prod secrets and datasets; CI (Go + frontend + E2E), structured monitoring, backup and restore.

## Global authentication boundary

- `users` is a global identity table; authentication does not assign school roles. `platform_admins` is an explicit global operator grant, not a school membership. Tenant memberships and school-specific RBAC are introduced in SWW-003.
- Migrations `000002_auth_identity`, `000003_auth_hardening` and `000004_tenants` hold identities, sessions, invitations, password-reset tokens, audit events, platform-admin grants, shared rate-limit state, tenants, settings and memberships. Invitation, reset and session tokens are stored as hashes; lifecycle operations that consume tokens, revoke sessions and write audit events use transactions.
- `/api/v1/auth` exposes `POST /login`, `POST /forgot-password`, `POST /reset-password`, `GET /me`, `POST /logout`, `GET /sessions` and `DELETE /sessions/{id}`. Invitation endpoints are `GET /api/v1/invitations/{token}/verify` and `POST /api/v1/invitations/accept`. The browser never supplies an authoritative role.
- Browser sessions use HttpOnly `SameSite=Lax` cookies; production and staging use `Secure` `__Host-` cookies. Unsafe authenticated requests also require a CSRF header matching the session-bound CSRF cookie/hash. Mutating requests are checked against configured origins and rejected when Fetch Metadata identifies a cross-site request. Forwarded client IPs are trusted only from configured proxy CIDRs.
- Authentication rate limits use PostgreSQL so all API instances share counters. Reset email delivery requires configured SMTP with STARTTLS; without SMTP, the reset-request flow is unavailable rather than returning an undelivered success.
- The bootstrap command creates only the first global platform administrator and runs only when `SERVER_ENV=development` and `SWW_ENABLE_DEV_BOOTSTRAP=true`; it requires explicit email, name and password inputs. Never use this path in staging or production.

## Tenant provisioning and school settings (SWW-003)

- `GET/POST /api/v1/platform/tenants` require a verified session and a database-backed `platform_admins` grant. The create request accepts school identity, locale, time zone and the initial owner's email; it cannot select a role, status, tenant ID or membership.
- Provisioning writes the tenant, hashed one-time `SCHOOL_OWNER` invitation and audit event in one PostgreSQL transaction. SMTP must be configured outside development/test. Development and test may use the authenticated, in-memory `/api/v1/platform/mail-capture` endpoint; it is not registered in staging or production.
- Owner invitation acceptance uses the role and tenant stored with the invitation token. New accounts are activated; an existing account must prove its current password, which is not changed. In that same transaction it creates the active membership, activates the pending tenant, creates the user session and records the tenant-scoped audit event.
- `GET /api/v1/tenants/{slug}/settings` requires an active membership. `PATCH` additionally requires the server-stored `SCHOOL_OWNER` or `SCHOOL_ADMIN` membership and CSRF validation. The slug only selects a candidate record; it never grants tenant authority. Platform admins do not implicitly gain school membership.
- Tenant settings include locale, time zone, limited branding and configured education stages. Feature flags, roles and membership identifiers are not editable through the school settings API. Nigerian curriculum stages are the defaults; British enrichment is explicitly optional.

## Academic sessions and terms (SWW-004A)

- Migration `000005_academics` adds only `academic_sessions` and `academic_terms`. Both are tenant-owned PostgreSQL records. Composite `(tenant_id, id)` keys and `(tenant_id, session_id)` term foreign keys prevent cross-school references. Session identifiers are unique per tenant, and a partial unique index allows at most one `ACTIVE` session per school.
- A session transitions from `DRAFT` to `ACTIVE` or `ARCHIVED`; an active session may be explicitly archived. `ARCHIVED` is terminal and both it and its terms are read-only. The API has no destructive delete operation and archiving never replaces another session.
- Each session must have exactly three terms, sequence 1–3, before activation. Terms must have valid ranges within their session and cannot overlap. PostgreSQL triggers lock the parent session while terms or session dates change; a deferred constraint trigger checks the final term set at commit, allowing atomic full-set term updates while rejecting concurrent overlap. Session activation checks the term count in the same transaction. The unique active-session index arbitrates concurrent activation attempts.
- The REST contract uses the standard response envelope. `POST /api/v1/tenants/{slug}/academic-sessions` accepts `{name, starts_on, ends_on}` and returns `201` with a `DRAFT` session; `GET` on the collection lists sessions. `GET/PATCH /api/v1/tenants/{slug}/academic-sessions/{sessionID}` retrieves or partially updates the name/date fields. `GET/PUT /api/v1/tenants/{slug}/academic-sessions/{sessionID}/terms` reads or atomically configures an array of exactly three `{sequence, name, starts_on, ends_on}` terms. `POST` on `.../{sessionID}/activate` and `.../{sessionID}/archive` performs explicit lifecycle transitions. Invalid input returns `422`, lifecycle or uniqueness conflicts return `409`, missing or inaccessible tenant resources return a non-disclosing `404`, and authenticated role failures return `403`.
- Reads are available to active tenant members in the academic-read roles; management is limited to active `SCHOOL_OWNER`, `SCHOOL_ADMIN`, and `PRINCIPAL` memberships. A platform-admin grant alone provides no school academic access.
- The backend derives the actor from the verified session and resolves the tenant from the route slug plus an active membership. Unsafe operations require the existing CSRF middleware. Every successful mutation and its redacted `audit_events` row are committed atomically. No client-supplied tenant ID, role, or user ID establishes authority.

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
