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
- Go 1.23+ (`go version`)
- Node.js 20+ (`node -v`)
- PostgreSQL 16+ (local or Docker container)

### 2. Backend Setup
```powershell
cd services/api
Copy-Item .env.example .env
go test -v ./...
go run ./cmd/server
```

Or run using the provided PowerShell script:
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
```powershell
.\scripts\migrate.ps1 up
# Or rollback:
.\scripts\migrate.ps1 down
```

---

## CI / Automated Checks

GitHub Actions (`.github/workflows/ci.yml`) runs on every push and pull request to `main`:
- Go vet, module verification, and race-detector test suite (`go test -v -race ./...`)
- Frontend TypeScript typechecking (`tsc --noEmit`) and production build (`next build`)
