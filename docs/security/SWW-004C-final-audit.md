# SWW-004C — Subjects, Offerings and Teacher Assignments

Date: 2026-11-18
Branch: feature/SWW-004C-subjects-offerings
Status: Local verification passed; CI and merge pending.

## Scope

SWW-004C implements:
- Tenant-scoped Subject Catalog
- Subject Offerings linked to academic terms and classes
- Teacher Assignments linked to active teacher memberships
- Ten authenticated API endpoints
- Migration 000007_subjects_offerings.up.sql

## Authorization

Subject and offering mutations:
- SCHOOL_ADMIN and PRINCIPAL
- SCHOOL_OWNER has read-only access

Teacher Assignment mutations:
- SCHOOL_ADMIN and PRINCIPAL only

Teacher Assignment listing:
- SCHOOL_OWNER, SCHOOL_ADMIN, PRINCIPAL: all tenant assignments
- TEACHER: own assignments through active teacher membership
- STUDENT and PARENT: denied
- BURSAR: denied, with tenant-concealing response permitted
- Cross-tenant access: denied

An assignment does not independently grant teaching permissions.
Future lesson, attendance and assessment operations must independently
validate current membership, offering and assignment status.

## Database controls

Migration 000007 creates:
- subjects
- subject_offerings
- teacher_assignments

Constraints include same-tenant foreign keys, uniqueness,
lifecycle guards, protected historical records,
active teacher validation and active offering validation.

Mutations and audit events are transactional.

## Verification evidence

Verified locally on 2026-11-18:
- Subject Repository PostgreSQL integration tests: PASS
- Subject Offering PostgreSQL integration tests: PASS
- Teacher Assignment PostgreSQL integration tests: PASS
- Teacher Assignment listing role-scoping tests: PASS
- Subject Catalog HTTP security tests: PASS
- Subject Offering HTTP security tests: PASS
- Teacher Assignment HTTP security tests: PASS
- Migration runner tests: PASS
- go test ./... -count=1: PASS
- go vet ./...: PASS
- git diff --check: PASS

PostgreSQL integration tests use disposable schemas in
the allowlisted sww_structure_test database.

## Deployment and review notes

Migration 000007 has not been applied to sww_dev.
Deployment and migration execution are separate controlled steps.

GitHub CI results and pull request review remain pending.
Backups and unrelated untracked files must not be committed.