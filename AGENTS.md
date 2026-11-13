# School Without Walls — Agent Instructions (Google Antigravity)

Read `docs/PRD.md`, `docs/ARCHITECTURE.md`, `docs/IMPLEMENTATION_PLAN.md` and `.agents/rules/security.md` BEFORE making changes. These are controlling instructions; when they conflict, stop and report the conflict rather than silently guessing.

## Mission
Implement the **MVP P0** in small, testable vertical slices. Deliver end-to-end school workflows, not mock interfaces. Keep the product Nigerian-first with versioned curriculum and labeled optional British enrichment. No real student data in seeds or test runs.

## Fixed architecture
- Next.js App Router + React + TypeScript + Tailwind/shadcn/ui.
- Go backend modular monolith exposing REST `/api/v1`.
- PostgreSQL authoritative state, Go-generated UUID, CITEXT email, migrations.
- Gemini accessed ONLY through server-side Go AI gateway; no API key in browser.
- Frontend Vercel; Go API separate host; database managed. Use secure HTTPS and cookie/CSRF plan.
- Do not introduce microservices, Firebase as authoritative database, client-owned roles/grades, or third-party paid services without written approval.

## Non-negotiable invariants
1. Every tenant-owned record has `tenant_id`, scoped queries, tenant-aware FK integrity and access tests.
2. Server validates role AND resource relationship on every read/write; client-provided role/tenant is untrusted.
3. AI cannot publish, grade officially, authorize actions or read unrelated PII; human reviews drafts.
4. Student AI is age-limited, lesson-grounded, rate/usage-capped and disableable.
5. Report cards are only visible when published; published results versioned and audited.
6. Never use production/student PII in tests, demos, errors, logs or AI prompts.
7. Never run untrusted learner code inside the API process.
8. Curriculum references are versioned, reviewed and rights-safe.
9. Do not copy repository implementation from AGPL-licensed CloudSchool without explicit licensing review. Feature concepts alone may be referenced.
10. No secrets, credentials, `.env` contents or provider responses containing PII in commits.

## Working protocol
1. Pick the next unchecked task in `docs/IMPLEMENTATION_PLAN.md` only.
2. Announce task ID, related PRD IDs, affected files, API/migration changes and planned tests.
3. Implement the smallest usable full-stack vertical slice, with typed frontend contracts and Go validation.
4. Add positive/negative, unauthorized and cross-tenant tests for every data feature.
5. Run formatting/lint/typecheck, `go test ./...`, build and relevant E2E. Report exact results, including blocked checks.
6. Update docs/migrations, mark task done only when acceptance tests pass, and propose a meaningful Git commit message.
7. Avoid broad refactors or changing architecture without explicit approval. Stop and ask before destructive migrations or handling real student data.

## Definition of done
- User-visible workflow functional; no UI-only mocks marked complete.
- API authorization/data validation correct; migrations and tests reviewed.
- Mobile, error, empty and loading states present.
- Documentation and acceptance checklist updated.
- Code secure, formatted and reproducible; no secrets.

## First instruction to agent
Read the three docs and rules, summarize the architecture and P0 tasks, then implement **SWW-001** only. Do not start the next task automatically; provide tests and a commit recommendation for review.
