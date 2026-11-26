# School Without Walls (SWW)

An AI-powered, multi-tenant digital school operating system and learning continuity SaaS platform designed for Nigerian schools (Early Years through SS3) with NERDC curriculum baseline and optional British enrichment.

---

## Architecture Overview

- **Web Frontend (`apps/web`):** Next.js 15+ App Router, React 19, TypeScript, Tailwind CSS. Responsive down to 360px viewports with same-origin BFF proxy.
- **Backend API (`services/api`):** Go modular monolith with REST `/api/v1`, standard JSON envelopes `{ data, meta }` and `{ error }`, and graceful lifecycle management.
- **Database:** PostgreSQL authoritative state, UUID keys, `citext` for emails, composite tenant foreign keys, and reversible SQL migrations.
- **AI Gateway:** Server-side Go gateway communicating with Google Gemini. No browser keys, strict lesson grounding, and age-banded guardrails.

---

## Operational Probes & Health Checks

| Endpoint | Method | Scope | Description |
|---|---|---|---|
| `/health` | `GET` | Root / Ops | Liveness probe returning service uptime and status |
| `/ready` | `GET` | Root / Ops | Readiness probe pinging PostgreSQL with timeout |
| `/api/v1/health` | `GET` | API v1 | Liveness probe in API namespace |
| `/api/v1/ready` | `GET` | API v1 | Readiness probe in API namespace |
| `/api/health` | `GET` | Web App | Next.js internal health route |

---

## Local Development (Windows PowerShell)

### 1. Prerequisites
- Go 1.24+ (`go version`)
- Node.js 20+ (`node -v`)
- PostgreSQL 16+ (local or Docker container)

### 2. Backend Setup
The Go API reads configuration from the current process environment; it does not
automatically load `.env` files. Set a local, non-production PostgreSQL URI
before starting the API or running migrations. Do not paste production
credentials into a development shell:
```powershell
$env:DATABASE_URL = "<local PostgreSQL connection URI>"
```

`.env.example` is a reference template, not an automatically loaded file. Keep
any actual `.env` file local and ignored by Git.

```powershell
cd services/api
go test -v ./...
go run ./cmd/server
```

Or, from the repository root, run the provided PowerShell script after setting
`DATABASE_URL`:
```powershell
.\scripts\dev-api.ps1
```

### 3. Frontend Setup
```powershell
cd apps/web
Copy-Item .env.example .env.local
cmd.exe /c npm install
cmd.exe /c npm run dev
```

Or run using the provided PowerShell script:
```powershell
.\scripts\dev-web.ps1
```

### 4. Database Migrations
From the repository root, after setting `DATABASE_URL` to the intended
non-production database:
```powershell
.\scripts\migrate.ps1 status
.\scripts\migrate.ps1 up
```

The migration runner applies only versions absent from `schema_migrations`.
It does not provide an automated `down` action; review and execute any rollback
plan separately and only with explicit database-owner approval.

### Authentication development notes

- Password-reset email is sent only when the API has SMTP settings configured; the SMTP server must support STARTTLS. Use local-only values and never commit `.env` files.
- The initial platform administrator bootstrap is development-only. It requires `SERVER_ENV=development`, `SWW_ENABLE_DEV_BOOTSTRAP=true`, and explicit `BOOTSTRAP_ADMIN_EMAIL`, `BOOTSTRAP_ADMIN_NAME` and `BOOTSTRAP_ADMIN_PASSWORD` inputs. Do not enable it outside local development.
- PostgreSQL integration tests require `SWW_TEST_DATABASE_URL` to point to a dedicated test database whose name contains a separate `test` segment and does not contain a `dev` or `prod` segment. Tests validate the database name before connecting and create/remove isolated schemas. Never point this variable at a developer, staging or production database.

---

## CI / Automated Checks

GitHub Actions (`.github/workflows/ci.yml`) runs on every push and pull request to `main`:
- Go module verification, uncached tests (`go test -count=1 ./...`), vet and race-detector tests
- Isolated PostgreSQL 16 containers with explicitly named disposable test databases; each database-using job generates and masks its own random password, and required backend integration tests fail rather than skip when configuration is missing
- Frontend unit/component tests, CSP helper tests, TypeScript typechecking and production build
- Production-built Playwright authentication and CSP tests against the real Go API and a separate disposable PostgreSQL database, served through a temporary local HTTPS proxy with fictional bootstrap credentials

## Frontend Security Headers

The Next.js app applies `Referrer-Policy: no-referrer`,
`X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, and a restrictive
`Permissions-Policy`. Production document responses use a per-request nonce
Content Security Policy. The nonce is passed on the request to Next.js so its
App Router scripts can be rendered with the matching nonce; the same policy is
returned on the response. The policy does not allow `unsafe-inline` or
`unsafe-eval` scripts and currently permits only same-origin application
connections and assets (plus `data:` images).

Nonce-based CSP requires request-time rendering. The root layout reads request
headers so Next.js can apply each request's nonce to its App Router scripts;
consequently, pages that might otherwise be statically rendered or cached are
rendered dynamically.
The middleware excludes API routes and static/image assets. Development does not
apply the production CSP, and HSTS is sent only for production HTTPS document
requests; local HTTP remains usable. The SWW-003H-C3 Playwright suite checks
nonce matching, hydration and client navigation in a real browser; its GitHub
Actions execution is still required before those checks can be considered
verified.

## Frontend Readiness and Tests

The landing page's API indicator requests the same-origin `/api/readiness`
route. That server-side route checks the Go API `/ready` endpoint with a
two-second timeout and no-store caching; public responses expose only ready,
unavailable or timeout state. The independent `/api/health` route remains a
web liveness probe. Readiness is informational and is not an authorization
check.

From `apps/web`, run the deterministic UI and security helper tests with:

```powershell
cmd.exe /c npm run test:ci
cmd.exe /c npm run test:security
```

The real authentication E2E suite is `npm run test:e2e`. It requires an
explicitly disposable PostgreSQL test database, applied migrations, a fictional
test identity and temporary HTTPS certificate. GitHub Actions provisions all
of these independently for the browser job; it does not use or connect to a
developer, staging or production database. Docker is used only by the hosted
CI runner and is not a local development requirement.
complete 
