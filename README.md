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
- PostgreSQL 16 service with an explicitly named disposable `sww_ci_test` database; required integration tests fail rather than skip if the test database configuration is missing
- Frontend TypeScript typechecking (`tsc --noEmit`) and production build (`next build`)
