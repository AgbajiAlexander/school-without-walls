# SCHOOL WITHOUT WALLS — MVP PRODUCT REQUIREMENTS DOCUMENT

**Document ID:** SWW-PRD-001  
**Version:** 1.0 (implementation baseline)  
**Date:** 13 November 2026  
**Status:** Proposed baseline; product owner sign-off required before real-student pilot  
**Primary market:** Nigerian private schools, Early Years through SS3  
**Owner:** School Without Walls product team  
**Companion files:** `ARCHITECTURE.md`, `AGENTS.md`, `.agents/rules/security.md`, `IMPLEMENTATION_PLAN.md`

## 1. Executive summary

School Without Walls (SWW) is a configurable, multi-tenant school operating system and learning platform. A school can run core administration and deliver teacher-supervised digital education regardless of whether its buildings are accessible. The MVP targets a real, end-to-end school experience, not a static demonstration dashboard: onboard a school, register and associate users, configure academics, publish curriculum-aligned lessons, submit homework, mark attendance, record assessments, publish reports, and let parents monitor progress. Gemini adds restricted teacher and student assistance without becoming the authority for grades, academic decisions or child safeguarding.

**North-star outcome:** a student can continue an approved course of study from home, a teacher can maintain instructional continuity, and a parent can see authoritative progress; two schools can do so on the same platform without data leakage.

**Positioning:** a modular SaaS platform for Nigerian nursery, primary, junior secondary, and senior secondary schools, with a Nigerian-first curriculum and British/EYFS enrichment.

### 1.1 Assumptions needing confirmation before pilot

- Pilot school, owner, pricing and branding have not been contractually confirmed. Use fictional seed records until written approval.
- First MVP is web-first, responsive and text-first. Mobile native apps and offline submission sync are excluded.
- One campus per tenant initially; multi-campus is a future extension. A campus entity may be reserved in the schema, but no UI required.
- School year, term dates, grade calculations and subject lists vary by tenant and are configurable.
- Student accounts for minors are provisioned by a school with guardian involvement. No open public signup for minors.
- AI-generated materials always require an adult educator's review before publication to a class.
- Payment gateway, payroll, library circulation, transport, hostel, live native video and subscriptions are post-MVP.

## 2. Product objectives and measurable outcomes

| ID | Objective | MVP verification |
|---|---|---|
| O-01 | Provide complete school onboarding | An operator creates two independent tenants with branding and settings |
| O-02 | Maintain accurate school records | Students, guardians, teachers, classes, subjects and enrolments function end-to-end |
| O-03 | Support blended learning | Teacher publishes content; student consumes and submits work remotely |
| O-04 | Support evidence-based assessment | Teacher records validated marks; authorized school user publishes report; parent views child-only report |
| O-05 | Demonstrate curriculum alignment | Lesson links to a framework version and approved learning objective, plus optional enrichment mapping |
| O-06 | Safeguard data and minors | Cross-tenant access tests, permission tests, AI safety tests and audit logs pass |
| O-07 | Demonstrate real deployment | Public HTTPS frontend on Vercel, independent HTTPS Go API, managed DB, smoke tests |
| O-08 | Fit low-bandwidth users | Core lesson pages and text/PDF resources work on entry-level mobile browsers under throttled network tests |

**Pilot success targets (provisional, not guaranteed):** all eight scripted cross-role journeys pass; zero confirmed cross-tenant reads/writes; all critical permission tests pass; 95% of primary task smoke checks pass; >= 90% of sampled teacher-approved lessons have valid curriculum links; browser flows remain usable at 360px width and simulated slow mobile network. Set operational latency/SLO thresholds after staging measurements.

## 3. Scope and release boundary

### 3.1 P0 — mandatory for MVP launch

1. Tenant creation, school branding, academic settings and feature toggles.
2. Authentication, invitations, session revocation and role/relationship-based authorization.
3. Staff profiles, teacher-to-class/subject assignments.
4. Student records, guardians, relationships and academic enrolments.
5. Academic years, terms, levels, classes/sections, subjects and basic schedules (simple timetable display).
6. Versioned curriculum framework registration, objectives and mapping; curated seed lessons for Early Years, Primary, JSS and SS.
7. Teacher-managed learning resources, published lessons, progress markers, assignments and submissions.
8. Attendance per class/date with authorized edits.
9. Teacher-entered continuous assessment/exam scores, configurable grade bands, review and report publication.
10. Parent portal scoped to linked children; results, progress, attendance and announcements.
11. Teacher AI draft assistant and age-controlled student AI tutor, both via server-side gateway.
12. School notices and remote-learning mode (status, banner, learning/schedule links, continuity view).
13. Structured audit trail, access control, data export for authorized admins, monitoring and basic recovery procedures.
14. Responsive and accessible web experience, tests and hosted pilot.

### 3.2 P1 — next release

Fee invoices and receipts; online admissions; full digital library/catalog; scheduled external meeting integrations; advanced timetable conflicts; richer notifications by email/SMS; school subscription billing; validated offline cache of approved content; push notifications.

### 3.3 P2 — future

Hostel, transport, payroll, leave management, inventory, native video classroom, native mobile apps, offline submission sync, advanced AI learning insights and multi-campus reporting.

### 3.4 Explicit exclusions

- AI issuing final grades, recommendations for student exclusion, admissions decisions or disciplinary penalties.
- Fully autonomous student chat with unrestricted external internet browsing.
- Direct code execution, arbitrary web tools or shell access for learner-facing AI.
- Claims of British curriculum accreditation or awarding-body affiliation.
- Full copyrighted textbook ingestion without permission.
- Hosting production personally identifying student data in demo seeds or public test accounts.

## 4. School levels and terminology

Configure `education_stage` values `EARLY_YEARS`, `PRIMARY`, `JUNIOR_SECONDARY`, `SENIOR_SECONDARY`. The tenant defines grade labels and ordering (e.g., Nursery 1, Reception, Primary 1, Year 1, JSS 1, Year 7, SS 1). No mandatory one-to-one equivalence to UK key stages. Grade-to-stage mappings are tenant-maintained with an optional reference mapping table; the platform never assumes an automatic grade equivalence.

Early Years reports support observational milestones instead of only numerical exam scores. The MVP exposes a simple rubric-based observation record for Early Years and keeps numeric assessments optional. Teacher content for younger children emphasizes parent-mediated and low-screen activities. The EYFS framework applies in England; it is used here for carefully labelled enrichment and planning, not to imply UK legal applicability to Nigerian schools.

## 5. Personas and access model

**Global:** PLATFORM_ADMIN (explicit operator access; not a school academic identity).  
**Tenant roles:** SCHOOL_OWNER, SCHOOL_ADMIN, PRINCIPAL, TEACHER, STUDENT, PARENT, BURSAR.

A person may have multiple memberships and may be both staff and guardian, but must choose an active school context per request. Children may have more than one guardian. A guardian may have multiple linked children. Student access is restricted to the user's own student profile, verified through current tenant membership and relationship mappings. Teachers must have current class/subject assignment for class-specific mutations. Results must be published before parent/student viewing.

### 5.1 Permission matrix

Legend: M=manage/configure; W=write in assigned scope; R=read authorized scope; S=self or linked child; P=publish/approve; —=no access. Owner and school admin do not automatically override privacy permissions for the platform operator.

| Resource/action | Platform | Owner | Admin | Principal | Teacher | Student | Parent | Bursar |
|---|---|---|---|---|---|---|---|---|
| Tenant provisioning | M | — | — | — | — | — | — | — |
| Tenant configuration | — | M | M | R | — | — | — | — |
| Roles and invitations | — | M | M | R | — | — | — | — |
| Student & guardian records | — | R | M | R | R assigned | S | S | limited R |
| Academic structure | — | R | M | M | R | R | R linked | — |
| Teacher assignments | — | R | M | M | R self | — | — | — |
| Curriculum framework imports | — | R | M | P | R | — | — | — |
| Lessons | — | R | R | P | W assigned | R enrolled/published | R linked/published | — |
| Assignments/submissions | — | R | R | R | W assigned | W own | R linked | — |
| Attendance | — | R | R | R | W assigned | R own | R linked | — |
| Assessment scores | — | R | R | P | W assigned | R published own | R published linked | — |
| Official reports publishing | — | R | P | P | — | — | — | — |
| Notices / remote mode | — | M | M | M | R | R applicable | R applicable | R |
| AI teacher drafts | — | — | — | R if authorized | W assigned | — | — | — |
| AI student tutor | — | — | — | — | — | S if enabled | — | — |
| Fee module (P1) | — | R | R | — | — | — | S | M |
| Audit logs | platform-only events | R tenant | R tenant | limited R | — | — | — | — |

**Implementation rule:** enforce each permission server-side, after validating tenant membership AND row-level relationship. The matrix is a baseline: API-specific permission identifiers and exceptions must be documented in tests. No API trusts a `role`, `tenant_id`, `student_id` or `teacher_id` merely because the frontend sends it.

## 6. Core user journeys

### J-01 Platform operator provisions a school

1. Operator authenticated with an elevated platform account creates tenant slug, display name and default locale/timezone.
2. Backend validates slug uniqueness and creates a tenant, default settings and one invitation for initial school owner as one transaction.
3. School owner accepts time-limited single-use invitation, sets authentication credentials and configures branding, education stages, academic year and terms.
4. Operator lists tenant metadata only. No automatic access to student records.

**Failure/edge cases:** duplicate slug -> 409; expired invitation -> rejected; second redemption -> rejected; unsafe logo -> rejected; failed provisioning -> no partially active school.

### J-02 School administrator sets up academic structure

Create academic year -> terms -> levels -> class sections -> subjects -> teacher assignments -> students' enrolments. Attempt to overlap active terms or duplicate a class code in the same academic year is rejected. Historical years remain queryable read-only after closure.

### J-03 Admit student and link guardian

School administrator records limited student profile and enrollment, links one or more guardians with validated contact details, and initiates invitations for authorized accounts. A guardian sees only actively linked children within active school membership. Revoking a guardian link ends access immediately after server-side enforcement.

### J-04 Teacher delivers an asynchronous lesson

Teacher chooses assigned class and subject -> approved curriculum objective -> adds accessible learning text, optional file/video URL, duration and practice tasks -> saves draft -> submits for review if the tenant enables approval -> publishes -> enrolled student views. Lesson content cannot be published when its objective points to a draft or withdrawn curriculum version unless explicitly grandfathered under a documented rule.

### J-05 Student learns from home

Student opens dashboard, sees assigned published lessons and due assignments, reads lesson via mobile browser, completes practice, and submits text/file assignment before deadline (or receives a clear late-state policy). Student sees submission receipt and teacher's later feedback. Student cannot see another learner's submission or feedback.

### J-06 Teacher records attendance

Assigned teacher selects date and class -> marks Present/Absent/Late/Excused -> saves batch idempotently -> parent can read linked child's attendance summary. Edits retain old/new values, actor, reason, and timestamp.

### J-07 Assessment to report card

School admin configures term assessment components/weights and grade scale -> teacher records scores limited by maximums -> authorized principal/admin reviews -> publishes immutable report snapshot -> guardian/student sees snapshot. Any correction requires an authorized amendment, versioning and audit trail; drafts never leak.

### J-08 Early Years observational reporting

Teacher records a rubric observation tied to a developmental area and approved objective, optionally uploads evidence with appropriate access controls, and contributes to a descriptive term report. No forced numeric grading for EYFS-style observations.

### J-09 Remote learning mode

School admin enables `REMOTE` mode with reason, effective dates, help text and schedule URL -> school dashboard surfaces an announcement, lessons/assignments take priority, teacher can monitor participation -> admin later restores `ON_SITE` or `HYBRID`. Attendance semantics (physical vs online participation) remain explicit; remote mode does not automatically mark students present.

### J-10 AI teacher assistant

Assigned teacher selects class, topic, objective and output type -> server sends only necessary curriculum text and age band to AI gateway -> AI returns structured *draft*, provenance and cautions -> teacher reviews/edits -> publishes through normal content workflow. AI draft never self-publishes.

### J-11 AI student tutor

School enables student AI; learner opens tutor from a published lesson -> gateway checks tenant, relationship, student age band, plan limits and lesson approval -> tutor offers guided explanations and practice, refuses dangerous or inappropriate content and directs welfare concerns to trusted adults -> no access to other students or private staff information. AI session includes visible disclaimer that it can make mistakes.

### J-12 Tenant isolation

Operator creates School A and School B; a member of School A manipulates route identifiers and API payloads to reference School B's objects; all requests return a non-disclosing 404 or 403 per contract and never mutate B's data. Test parent-child cross-tenant links and guessed file keys as well.

## 7. Functional requirements (testable)

Format: **FR-ID / priority / behavior / acceptance**. All P0 items are launch gates.

### 7.1 Tenancy and identity

- **FR-TEN-001 P0:** Create tenant with unique normalized slug, name, status, locale, timezone (`Africa/Lagos` default), branding. **Accept:** duplicate slug is 409; default settings created atomically.
- **FR-TEN-002 P0:** Tenant-resolved routes use validated school slug and authenticated membership. **Accept:** unknown/disabled tenant rejects authenticated operations; conflicting route/header/session tenant IDs reject.
- **FR-TEN-003 P0:** Tenant feature flags toggle visible features and backend authorization. **Accept:** disabled AI endpoint returns 403 irrespective of client.
- **FR-ID-001 P0:** Email/password login with strong password hashing (Argon2id preferred), email verification/invitation, password reset and session logout. **Accept:** reset tokens one-use, short-lived; login rate-limited; invalid token not accepted.
- **FR-ID-002 P0:** Role-based, relation-aware authorization. **Accept:** cross-tenant and cross-child endpoints tested.
- **FR-ID-003 P0:** Audit privileged events and permission changes. **Accept:** actor, tenant, target, action, timestamp and request ID recorded without secrets.
- **FR-ID-004 P0:** First-run invitations for staff/guardian/student credentials; no unrestricted student signup. **Accept:** invitation cannot be re-used or transferred to another identity.

### 7.2 Academics and people

- **FR-ACA-001 P0:** CRUD academic years, terms, levels, sections and subjects with active/inactive state and tenant-local unique constraints. **Accept:** duplicates/invalid dates receive structured 422 errors.
- **FR-ACA-002 P0:** Teacher assignments to subject + class/section + academic period. **Accept:** teacher writes only in current assignment.
- **FR-ACA-003 P0:** Enrollment into class per academic year with optional transfer history. **Accept:** one primary active section per student per term; history remains.
- **FR-PEO-001 P0:** Create/list/update student profile, guardians and links. **Accept:** guardian unlink prevents subsequent reads; private fields excluded from teacher listing where unnecessary.
- **FR-PEO-002 P0:** Staff records and owner/admin invitations. **Accept:** staff records cannot grant tenant permissions without explicit role membership.

### 7.3 Curriculum engine

- **FR-CUR-001 P0:** Frameworks are identified by `jurisdiction`, `publisher`, `title`, `edition/version`, `effective_from`, `effective_to`, `source_url`, `license_note`, `status`. **Accept:** two versions coexist with distinct identifiers.
- **FR-CUR-002 P0:** Objective tree: stage -> grade band -> subject -> strand -> objective. **Accept:** objectives have stable per-version codes and language suitable for indexing.
- **FR-CUR-003 P0:** Lesson links to one primary Nigerian objective and zero or more enrichment objectives; Early Years may use an approved developmental framework as primary based on tenant configuration. **Accept:** lesson validation enforces applicable stage and published status.
- **FR-CUR-004 P0:** Alignment mappings include relation `EQUIVALENT`, `COMPLEMENTARY`, `EXTENDS`, `DIFFERENT`, confidence source `HUMAN_REVIEWED` or `DRAFT`, reviewer and review date. **Accept:** only reviewed mappings appear as verified alignment.
- **FR-CUR-005 P0:** Tenant may choose framework edition per academic session; updates do not retroactively overwrite historical lesson/report references. **Accept:** historical snapshot points to original framework version.
- **FR-CUR-006 P0:** Seed a *small, licensed, reviewed* sample for each stage. **Accept:** at least one complete teacher lesson + student assignment + objective mapping per stage; seeds are fictional.
- **FR-CUR-007 P0:** Show source metadata and distinguish required Nigerian outcomes from optional British enrichment. **Accept:** no 'UK accredited' label by default.
- **FR-CUR-008 P0:** Educational content import is admin-only, validates provenance and permissions. **Accept:** unlicensed third-party textbook upload blocked by process policy and rights confirmation.

### 7.4 Learning management

- **FR-LMS-001 P0:** Teacher creates lesson draft, optional reviewer approves, publishes/archives; attaches safe file or external video URL. **Accept:** no student visibility before publish.
- **FR-LMS-002 P0:** Student feed filters content by current enrollment, publish time, active module and tenant. **Accept:** not-enrolled student cannot fetch lesson by ID.
- **FR-LMS-003 P0:** Assignments have due date, instructions, max score, submission type and status. **Accept:** invalid file types/size rejected; repeat submit follows configured resubmission policy.
- **FR-LMS-004 P0:** Teacher comments, scores within range and returns work; student receives authoritative update. **Accept:** score modifications audited.
- **FR-LMS-005 P0:** Track lesson progress as not started/in progress/completed with timestamps. **Accept:** client cannot mark another student's record complete.
- **FR-LMS-006 P0:** Media uses object storage with signed upload/download URLs and short expiry. **Accept:** private file cannot be fetched through a copied permanent public URL.

### 7.5 Attendance, results, parental access

- **FR-ATT-001 P0:** Assigned teacher takes dated attendance with explicit mode `ON_SITE`/`REMOTE` and status. **Accept:** unique per student/class/date/session; repeated identical save is idempotent.
- **FR-ATT-002 P0:** Change attendance with logged reason. **Accept:** guardian sees corrected current state, audit retains prior state.
- **FR-RES-001 P0:** Per-tenant assessment scheme stores components and validated weight total (100%). **Accept:** scheme cannot publish if components missing/weights invalid.
- **FR-RES-002 P0:** Scores range 0..component maximum; school-defined grade bands non-overlapping. **Accept:** out-of-range values 422.
- **FR-RES-003 P0:** Results follow `DRAFT -> UNDER_REVIEW -> PUBLISHED -> AMENDED`; reports are versioned snapshots. **Accept:** child/parent sees only latest authorized published snapshot; old versions audited.
- **FR-RES-004 P0:** Early Years teacher observation records support descriptive rubric and evidence, not forced GPA. **Accept:** qualitative observation renders in Early Years report.
- **FR-PAR-001 P0:** Guardian views child-specific lessons/progress, attendance and released reports. **Accept:** unlinked child ID denied at query AND object-level authorization.
- **FR-COM-001 P0:** School publishes notices targeted by role, stage or class; remote mode banner prominent. **Accept:** notices cannot leak across tenants.

### 7.6 AI and remote continuity

- **FR-AI-001 P0:** Only Go AI gateway may call Gemini. **Accept:** browser build contains no provider secret; logged payloads redacted.
- **FR-AI-002 P0:** Teacher assistant returns JSON draft with objectives, outline, teacher checks and unsafe-content flags; no auto publish. **Accept:** content is not published without explicit authorized action.
- **FR-AI-003 P0:** Student tutor uses only lesson objectives, curated content and age band; conversation limited by turn, token and spend quotas. **Accept:** disabled, unapproved or unassigned content unavailable.
- **FR-AI-004 P0:** Prompt injection and harmful-content handling. **Accept:** adversarial inputs cannot override server system rules, obtain secrets or reveal third-party records; flagged safeguarding requests follow escalation UX.
- **FR-AI-005 P0:** Parent/student and school controls for AI enablement and retention; record minimal metadata for audit. **Accept:** AI disabled for a child = endpoint blocked; no unrestricted retention of conversational content.
- **FR-REM-001 P0:** Mode `ON_SITE`, `HYBRID` or `REMOTE`, with scheduled start/end and reason. **Accept:** school-level setting reflected to active members and report of participation available.
- **FR-REM-002 P0:** Text-first lesson accessible on mobile; downloadable resource available if authorized. **Accept:** learning journey functional without synchronous video.

## 8. Domain model and data definitions

Use PostgreSQL with Go-generated UUIDv7/UUIDv4 (chosen in ADR) and timestamptz UTC storage; translate into tenant timezone on display. All tenant-owned rows carry mandatory `tenant_id UUID NOT NULL` and foreign keys that preserve ownership. Use composite unique constraints including tenant ID. Primary entities:

| Entity | Essential columns | Constraints and notes |
|---|---|---|
| tenants | id, slug, name, status, timezone, branding_json, created_at | slug unique globally; no personal student data in operator views |
| users | id, email (CITEXT), password_hash, status, verified_at | global identity; no role field as sole authority |
| memberships | id, tenant_id, user_id, role, status | unique active tenant/user/role; audited changes |
| invitations | id, tenant_id, invited_email, target_role, token_hash, expires_at, used_at | store hash, never plaintext token |
| staff_profiles | id, tenant_id, user_id, display_name, staff_code | unique staff code per school |
| students | id, tenant_id, user_id nullable, admission_no, stage, birth_date nullable, status | minimal data, unique admission number within tenant |
| guardians | id, tenant_id, user_id, display_name, contact | not globally shared without verified tenant membership |
| student_guardians | tenant_id, student_id, guardian_id, relationship, active | composite FKs prevent crossing schools |
| academic_years | id, tenant_id, name, starts_on, ends_on, status | date bounds and overlap validation |
| terms | id, tenant_id, year_id, sequence, starts_on, ends_on | constrained within year |
| grade_levels | id, tenant_id, stage, name, sort_order | tenant-configured labels |
| class_sections | id, tenant_id, grade_level_id, year_id, code, label | unique per year/grade/code |
| subject_offerings | id, tenant_id, section_id, subject_id, term_id | unique per class/subject/term |
| teacher_assignments | id, tenant_id, teacher_id, offering_id, active | ensure membership and school alignment |
| enrollments | id, tenant_id, student_id, section_id, year_id, term_id, status | prevent overlapping active enrollments |
| frameworks | id, publisher, jurisdiction, version, status, source_url, license_note | global curated registry, immutable version |
| objectives | id, framework_id, stage, band, subject, strand, code, text | code unique per version |
| objective_alignments | id, primary_objective_id, related_objective_id, relation, review_status, reviewer_id | separate metadata from curriculum record |
| tenant_frameworks | tenant_id, year_id, framework_id, role | curriculum edition per school/year |
| lessons | id, tenant_id, offering_id, primary_objective_id, title, content, status, published_at, author_id | immutable audit references; version or revision log |
| lesson_objectives | tenant_id, lesson_id, objective_id, alignment_type | enrichment references |
| learning_progress | tenant_id, lesson_id, student_id, state, updated_at | unique lesson/student |
| assignments | id, tenant_id, lesson_id, due_at, max_score, status | linked to teacher-owned lesson |
| submissions | id, tenant_id, assignment_id, student_id, content, attachment_id, state, submitted_at | version/resubmit policy |
| feedback | id, tenant_id, submission_id, teacher_id, score, comment | audit score edits |
| attendance | id, tenant_id, student_id, section_id, on_date, mode, status, recorded_by | unique per student/section/date; period rules |
| assessment_schemes | id, tenant_id, term_id, stage, version, status | sum weights = 100 at publication |
| assessment_components | id, tenant_id, scheme_id, name, max_score, weight | validated grade math |
| student_scores | id, tenant_id, student_id, offering_id, component_id, score, recorded_by | constraints + contextual authorization |
| report_snapshots | id, tenant_id, student_id, term_id, version, status, payload_json, published_at | authoritative published immutable payload |
| observations | id, tenant_id, student_id, objective_id, rubric, note, evidence_id | Early Years descriptive report |
| notices | id, tenant_id, audience_json, title, body, status, published_at | target filtering |
| tenant_modes | id, tenant_id, mode, reason, starts_at, ends_at | history plus effective mode |
| media_assets | id, tenant_id, owner_id, object_key, kind, mime, bytes, visibility | never expose raw bucket keys |
| ai_requests | id, tenant_id, actor_id, kind, lesson_id, model, token_count, status, flags, created_at | redact/limit conversational data |
| audit_events | id, tenant_id nullable, actor_id, action, subject_type, subject_id, request_id, metadata_redacted, at | append-only operational log |

### 8.1 Database integrity requirements

- Composite tenant-aware foreign keys for cross-tenant relations (e.g. `(tenant_id, student_id)` references `(tenant_id, id)`); unique tenant-id pairs on referenced tables.
- Shared global curriculum entities cannot carry student personal information; tenant curriculum choices do.
- Use DB transactions for tenant provisioning, enrollments, grading publication, guardian link removal and role changes.
- Add PostgreSQL row-level security (RLS) as defense in depth where viable; Go authorization remains required and RLS must be tested with pooled connections.
- No permanent hard deletes of published reports or audit logs; use lifecycle statuses and defined retention/deletion policy.
- Never assume user-supplied `tenant_id` is authoritative; determine from server-authenticated tenant context.

## 9. State machines and rules

- **Tenant:** `PENDING_SETUP -> ACTIVE -> SUSPENDED -> ARCHIVED`; suspended tenants may have authorized export/recovery access only.
- **Lesson:** `DRAFT -> IN_REVIEW -> APPROVED -> PUBLISHED -> ARCHIVED`. In MVP school can disable reviewer step (`DRAFT -> PUBLISHED` by authorized teacher), but all publications logged.
- **Assignment:** `DRAFT -> OPEN -> CLOSED`; submission: `NOT_SUBMITTED -> SUBMITTED -> RETURNED` and optional `RESUBMITTED` with revisions.
- **Report:** `DRAFT -> UNDER_REVIEW -> PUBLISHED -> AMENDED` with an immutable new version per amendment.
- **Academic year:** `PLANNED -> ACTIVE -> CLOSED`; reopening is controlled, audited admin workflow.
- **AI content:** `GENERATED_DRAFT -> HUMAN_REVIEWED -> PUBLISHED`; never implicitly advance.

**Grade formula:** for score component `c`, normalized contribution = `(score_c / max_score_c) * weight_c`; total = sum contributions with decimals, rounded only for display/report by tenant policy. Missing scores are `NULL`, not zero; publish blocked unless an authorized explicit missing-score policy applies. Grade bands must be exhaustive and non-overlapping for numerical reports.

## 10. APIs and contracts

Base prefix `/api/v1`; JSON with consistent envelope `{data, meta}` or `{error:{code,message,details,request_id}}`. Route tenant context resolved from validated subdomain/path and authenticated membership. Internal IDs are UUIDs; clients cannot bypass authorization via identifiers. Pagination uses cursor or bounded `page_size<=100`. Timestamps RFC3339 UTC. Mutations use idempotency keys when repeat submissions risk duplicates; ETags/version checks for concurrent editing.

| Method and route | Responsible role | Notes |
|---|---|---|
| POST `/platform/tenants` | Platform admin | provision tenant/invitation |
| GET/PATCH `/schools/current` | Owner/admin | branding and settings |
| POST `/auth/login`; `/auth/logout`; `/auth/password-reset` | User | rate-limit and secure cookie session |
| POST `/invitations`; POST `/invitations/accept` | Owner/admin / invitee | signed one-use invite |
| GET/POST `/students` | Admin | `tenant_id` internal; scoped list |
| GET/PATCH `/students/{id}` | Authorized users | field projections differ by role |
| POST `/students/{id}/guardians` | Admin | validates same-school relation |
| GET/POST `/staff`, `/teacher-assignments` | Admin/principal | scope and assignment |
| GET/POST `/academic-years`, `/terms`, `/class-sections`, `/subjects` | Admin/principal | academic validation |
| GET `/curriculum/frameworks`; GET `/curriculum/objectives` | Authorized member | published curated frameworks |
| POST `/curriculum/alignments` | Curriculum manager/admin | review workflow |
| GET/POST `/lessons` | Teacher/authorized | create/edit/publish actions separate |
| POST `/lessons/{id}/publish` | Teacher/approver | validate mapping/state |
| GET `/student/lessons`; PATCH `/student/lessons/{id}/progress` | Student | membership and enrollment |
| POST `/assignments/{id}/submissions` | Student | idempotent and deadline policy |
| POST `/submissions/{id}/feedback` | Assigned teacher | score validation |
| PUT `/attendance/batches` | Assigned teacher | scoped batch and reasoned edits |
| POST `/assessments/schemes`; POST `/assessments/scores` | Admin/assigned teacher | validations |
| POST `/reports/{id}/publish`; GET `/reports/{id}` | Principal/admin; authorized reader | versioned reports |
| GET `/parent/children`; GET `/parent/children/{id}/summary` | Parent | active guardian link only |
| GET/POST `/notices`; PATCH `/school/mode` | Admin/principal | audience and remote mode |
| POST `/ai/teacher/draft`; POST `/ai/student/tutor` | Teacher/student | quotas, moderation, logging |
| POST `/media/upload-intent`; GET `/media/{id}/download` | Authorized | short-lived signed object URL |
| GET `/health`; GET `/ready` | Operations | readiness checks DB, not secret values |

### 10.1 API error codes

`UNAUTHENTICATED (401)`, `FORBIDDEN (403)`, `NOT_FOUND (404, non-disclosing where appropriate)`, `CONFLICT (409)`, `INVALID_INPUT (422)`, `RATE_LIMITED (429)`, `AI_UNAVAILABLE (503)`, `INTERNAL_ERROR (500)`.

## 11. AI design, safety, controls and evaluation

**Model access:** server-only Gemini through a dedicated Go `ai` module. No browser SDK initialized with API key. Secret injected through hosting secret store. The gateway performs identity checks, academic scope checks, content selection, guardrails, budget enforcement, request/response validation, and redacted logging. Do not transmit names, contact details, addresses, medical information, or raw student records unless an explicitly reviewed educational use requires them and lawful handling is documented; default pseudonymized or no PII.

**Grounding:** student tutor retrieves only approved tenant lesson content and relevant objectives; no global unrestricted web retrieval or access to user-uploaded content in MVP. Treat lessons, file text and user prompts as untrusted data; never treat them as higher-priority system instructions. Future RAG requires separate retrieval permissions and source filtering.

**Age bands:** `EARLY_YEARS` no direct unsupervised chatbot; adult/teacher mediated only. `PRIMARY` constrained, short question-and-hint interactions with optional school/guardian gating. `JUNIOR_SECONDARY` and `SENIOR_SECONDARY` allow richer curriculum-focused tutoring subject to policy. Tenant can disable the feature; guardian opt-out must be enforceable where applicable. Verify local legal basis and guardian notices before pilot.

**Safeguarding:** respond supportively to distress, bullying, abuse and self-harm concerns, avoid secrecy or pretending to be a human counselor, encourage contacting a trusted adult and appropriate emergency resources. Define an administrator-reviewed safeguarding escalation procedure before live pilot; do not indiscriminately store sensitive transcript text or send the user's words to unrelated teachers. If severe safety concerns arise, show a safe response rather than promise monitoring that does not exist.

**Prohibitions:** no automatic publishing of teaching material; no final scoring of high-stakes work; no legal/medical diagnosis; no unrestricted personal conversations or manipulation; no contact between adult outsiders and children; no leak of answer keys not explicitly permitted; no financial action; no student profiling without human-reviewed purpose.

**Controls:** per-tenant/age-band toggle; request rate, token and monthly spend limits; model timeout and bounded retries; circuit breaker; safe fallback to standard lesson content; transparent AI notice; content filtering, structured outputs and review queue for flagged drafts; minimal retention; deletion workflows; opt-out controls; approved providers/data-region review.

**Evaluation before release:** assemble at least 50 representative prompts covering curriculum adherence, hallucinations, age appropriateness, prompt injection, disallowed requests, PII extraction attempts, teacher draft quality and bias. Review false positives/negatives with educators. Block launch if any test permits cross-tenant data exposure, provider secret exposure or autonomous publication.

## 12. UX / IA / pages

Global: marketing landing (optional simple MVP), school selection, login/reset/invite acceptance; accessible 404/403; school-branded shell. Tenant pages:

| Role | Minimum routes |
|---|---|
| Platform admin | `/platform/schools`, `/platform/schools/new`, `/platform/health` |
| School admin | `/dashboard`, `/settings/school`, `/settings/users`, `/academics`, `/students`, `/staff`, `/curriculum`, `/notices`, `/remote-mode` |
| Teacher | `/teacher/dashboard`, `/teacher/classes`, `/teacher/lessons`, `/teacher/assignments`, `/teacher/attendance`, `/teacher/assessments`, `/teacher/ai-drafts` |
| Student | `/learn`, `/learn/lessons/[id]`, `/learn/assignments`, `/learn/grades`, `/learn/ai-tutor` |
| Parent | `/parent/dashboard`, `/parent/children/[id]`, `/parent/reports`, `/parent/notices` |
| Principal | Academic oversight, curriculum review, result approval, school notices |

**Design requirements:** accessible keyboard navigation, labels/error associations, minimum WCAG 2.2 AA target for web MVP, responsive at 360px/768px/desktop, restrained data-heavy screens, visible loading/empty/error states, meaningful confirmation for destructive actions, Nigeria-friendly names, DD/MM/YYYY presentation where configured (API stores ISO), timezone-aware calendar, text-first and downloadable lessons. Use age-appropriate layouts: guardians/teachers act on behalf of Early Years pupils.

## 13. Non-functional and operational requirements

- **Security:** TLS, HttpOnly Secure SameSite cookies (choose deployment-compatible strategy), CSRF protection when cookies are used, password hashing, MFA for privileged users targeted before live production, access and refresh expiry, least privilege, strict CORS, CSP, secure headers, input validation, upload scanning/limits, no unsafe HTML rendering, rate limits and audit logs.
- **Privacy:** data inventory and minimization, clear school/vendor roles, lawful basis, guardian notices, documented retention, export/deletion process and verified processor/subprocessor contracts. Nigeria Data Protection Act 2023 and NDPC GAID are review inputs; obtain legal advice for real deployment.
- **Availability:** fail gracefully when AI is offline; student lesson access and teacher grading must not depend on Gemini.
- **Backups:** managed PostgreSQL daily backup baseline, PITR if service supports it, tested restore rehearsal before pilot, secure object storage backup policy.
- **Observability:** structured JSON logs with request_id, tenant context safely represented, traces/metrics and alerting; never log passwords, tokens or full student AI prompts by default.
- **Performance:** pagination and indexed tenant queries; measure P95 and optimize after deployment. Set pilot provisional targets: normal read API P95 < 500ms under defined load excluding AI; first lesson text display < 3s under staged mobile conditions. Targets are design goals, not present measurements.
- **Quality:** Go unit/integration tests, frontend component tests, Playwright E2E, migration checks, accessibility audit, no critical dependency vulnerabilities before pilot.
- **Accessibility/localization:** English first, locale support designed for later; content type supports UTF-8 and future multilingual assistance.
- **Disaster recovery:** RPO/RTO documented once provider and plan are chosen; rehearse restore and verify encrypted backups.

## 14. Data protection and school safeguarding governance

Nigeria's Data Protection Act 2023 includes special handling of children's personal data and contains circumstances relevant to educational service provision. Do **not** assume all child processing always requires guardian consent or that an education-related exception grants unrestricted processing. Assess the applicable lawful basis for each processing purpose (enrollment, academic records, AI tutoring, optional marketing, analytics). The NDPC's 2025 GAID may impose additional requirements; counsel should validate controls, appropriate notices, DPIA need, processor agreements, cross-border transfers, children's rights and retention schedule.

Data classification:

- **Public:** school branding, published generic page content.
- **Internal tenant:** academic structure, staff lesson drafts, tenant operations.
- **Confidential student:** identifying profiles, guardian contacts, attendance, grades, submissions, observations.
- **Highly restricted:** authentication secrets, safeguarding events, sensitive health/discipline data (not part of MVP unless strictly necessary).

Provide school administrators with appropriate export mechanisms, log all export requests, and limit operator support access to minimum necessary metadata. Define and test incident response / breach notification process before production. Seek permission/licensing for curriculum texts and educational media; reference links and citations do not constitute reuse rights.

## 15. Validation, testing and acceptance

**Mandatory release gates:**

- **AC-001 Multi-tenant:** School A and B created; 20 cross-tenant read/write ID manipulations deny without data exposure.
- **AC-002 Identity:** invalid/expired/replayed invitation and password reset tokens denied; session revoked on logout.
- **AC-003 Relationships:** revoked guardian cannot fetch child's old report URL, file URL or API summary.
- **AC-004 Teacher scopes:** teacher not assigned to subject cannot change grade or publish lesson for that class.
- **AC-005 Curriculum:** lesson with approved primary objective and reviewed British enrichment is publishable; outdated/unapproved mapping clearly flagged; version reference preserved.
- **AC-006 LMS:** assignment submission, feedback, revision and progress operate via mobile browser; unauthorized submission IDs denied.
- **AC-007 Attendance:** duplicate batch safe; edit audit exists; guardian sees correct state.
- **AC-008 Results:** 100%-weight validation; missing scores blocked for publish; only published, versioned report visible to family.
- **AC-009 Early Years:** qualitative milestones render without mandatory numeric grades.
- **AC-010 AI:** browser has no secret; student cannot request another child's data; generated teacher plan stays unpublished until teacher action; all critical red-team prompts pass.
- **AC-011 Remote mode:** ON_SITE/HYBRID/REMOTE state changes reflected, notices displayed, no inferred attendance.
- **AC-012 Operations:** GitHub CI green; Vercel page and hosted Go API HTTPS work; `/ready` confirms DB; migrations run with rollback strategy; backups verified.
- **AC-013 Accessibility:** primary journeys keyboard usable and automated checks show no critical blockers; manual screen-reader review on major forms.
- **AC-014 Production privacy:** sample tenant has no real pupil data; privacy notice and processor contract drafts ready before actual pilot.

### 15.1 E2E scenarios

`e2e/tenant-isolation.spec.ts`, `auth-invitation.spec.ts`, `academics-setup.spec.ts`, `teacher-publish-lesson.spec.ts`, `student-submit-assignment.spec.ts`, `parent-child-scope.spec.ts`, `teacher-attendance.spec.ts`, `reports-publish.spec.ts`, `early-years-observation.spec.ts`, `remote-mode.spec.ts`, `ai-safety.spec.ts`, `mobile-learning.spec.ts`.

## 16. Dependencies and delivery risks

| Risk | Impact | Mitigation |
|---|---|---|
| Copyright/reuse of curriculum material | Legal/content risk | Curate licensed/permissioned summaries, metadata and original exercises; provenance logs |
| Conflicting curriculum changes | Wrong instruction | Version sources, human reviewer, periodic update SOP |
| AI hallucination or unsafe interaction | Child safety/trust | Approved lesson grounding, age controls, educator oversight, testing and kill switch |
| Cross-school data leak | Critical privacy failure | Composite FKs, backend policies, RLS where viable, automated hostile ID tests |
| Frontend/server deployment mismatch | Failed integration | Early staging deploy, HTTPS, CORS/cookie plan, API contract tests |
| Unreliable network | Interrupted learning | Text-first content, small assets, retry/clear status; offline sync later |
| Over-scoped MVP | Delays/non-functional product | Explicit P0/P1/P2 and end-to-end vertical slices |
| School-specific assessment variations | Product rigidity | Configurable scheme/grade bands and versioned reports |
| Student identity/safeguarding gaps | Compliance | Guardian relationship verification, school-led provisioning, documented escalation |

## 17. Release plan (vertical slices)

1. **Slice A: Foundation:** repository, CI, Next.js shell, Go API, PostgreSQL migrations, health/ready, auth baseline.
2. **Slice B: Tenants and authorization:** provisioning, school setup, roles, guardian relationships; cross-tenant tests.
3. **Slice C: Academic administration:** years, terms, stages/classes, student/staff, assignments.
4. **Slice D: Curriculum and lessons:** framework versions, objective alignment, seeded examples, teacher publishing, student reading.
5. **Slice E: Assignments and attendance:** submissions, feedback, attendance and student progress.
6. **Slice F: Assessment and parents:** grade schemes, approval, versioned report cards, parent portal, Early Years observations.
7. **Slice G: AI and remote mode:** Gemini gateway, safety controls, remote mode, notifications.
8. **Slice H: Hardening and deployment:** E2E, accessibility, security, load checks, backups, Vercel/Go staging and pilot signoff.

Each slice must include a migration where relevant, API validation/auth, UI, tests, documentation and commit. Do not begin dependent features until prior API contracts are validated.

## 18. Pilot operating procedure

- Load fictional seeds only. Create two schools with different branding, four student stages and independent admin teams.
- Run all journeys with demo educator, learner, guardian and principal accounts; use dedicated sandbox secrets.
- Record tester feedback: completion %, confusion, broken flows, unexpected authorization behavior, content quality and load time.
- Complete safeguarding/privacy checklists, source license review, school approval and backup restore rehearsal before admitting actual students.
- Set support channel, incident owner and school onboarding/offboarding procedure.

## 19. Definition of done (per work item)

1. Requirement ID mapped to code/PR and acceptance test.
2. Database migration/reversible strategy reviewed (if applicable).
3. Input and role/tenant/relationship validation present in Go handlers/services.
4. Appropriate loading/error/empty/success UX present.
5. Unit and integration tests cover success, validation, unauthorized and cross-tenant access.
6. No secrets/client AI calls or unchecked unsafe output.
7. Logging/audit included for privileged or academic state changes.
8. README/docs/API contracts updated, `go test ./...`, frontend typecheck/lint/test/build and relevant E2E pass.
9. Working functionality manually verified on mobile-sized viewport and staging.
10. Product owner approves visible workflow and educational correctness.

## 20. Open product decisions (defaults for development)

| Decision | MVP default | Approval needed before live school data? |
|---|---|---|
| School legal operator/controller and vendor processor relationship | School operates student records; platform acts under defined contract | Yes |
| First pilot brand/name | Fictional demo schools; Morfol-like workflow only if permission granted | Yes |
| School owner MFA | Implement or enable prior to live production | Yes |
| Parent consent and minor AI policy | No AI for early years students; primary gated/limited; opt-out | Yes |
| Student date of birth | Optional; minimize personal details and use school-assigned age band | Yes |
| AI provider retention/data processing arrangements | Provider usage blocked for real data until approved | Yes |
| Report calculations | Configurable score components, default demo scheme 40 CA + 60 exam | Yes |
| Hosting | Vercel frontend + Go host + managed PostgreSQL + private object storage | Infrastructure signoff |
| Curriculum rights and official content import | Original summaries + source references, reviewed enrichment | Rights review |
| Domains and subscriptions | Subdomain demo; payment subscriptions deferred | No |

## 21. External sources and provenance (verified 13 Nov 2026)

- Nigeria Educational Research and Development Council revised basic curriculum index: https://nerdc.gov.ng/content_manager/new_curriculum_home.html
- Nigerian Federal Ministry of Education September 2025 curriculum press release: https://nerdc.gov.ng/content_manager/pdf_files/Press%20Release%20On%20New%20FG%20Curriculum.pdf
- EYFS framework (updated 1 September 2026): https://www.gov.uk/government/publications/early-years-foundation-stage-framework--2
- England national curriculum overview: https://www.gov.uk/national-curriculum
- Nigerian Data Protection Act 2023: https://ndpc.gov.ng/wp-content/uploads/2024/03/Nigeria_Data_Protection_Act_2023.pdf
- NDPC General Application and Implementation Directive (GAID): https://ndpc.gov.ng/wp-content/uploads/2025/03/NDP-ACT-GAID-2025-MARCH-20TH.pdf
- Antigravity workspace rules: https://antigravity.google/docs/rules/
- Feature reference only, not source code to copy: https://github.com/hrshadhin/school-management-system (AGPL-3.0)

**Implementation note:** These references are informational and require ongoing review. All legal, safeguarding, data-processing and curriculum licensing decisions for a real school should be validated by qualified local advisers and school leadership.
