package academics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresSubjectOfferingsConstraints tests SWW-004C migration rules in an
// isolated schema. It must never run against a development or production DB.
func TestPostgresSubjectOfferingsConstraints(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("SWW_TEST_DATABASE_URL"))
	if databaseURL == "" {
		if os.Getenv("SWW_REQUIRE_POSTGRES_TESTS") == "1" {
			t.Fatal("SWW_TEST_DATABASE_URL is required")
		}
		t.Skip("set SWW_TEST_DATABASE_URL to an isolated PostgreSQL test database")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("invalid PostgreSQL test URL:", err)
	}
	if cfg.ConnConfig.Database != "sww_structure_test" && cfg.ConnConfig.Database != "sww_ci_test" {
		t.Fatalf("refusing SWW-004C test on database %q", cfg.ConnConfig.Database)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal("connect to test database:", err)
	}
	defer admin.Close()

	schema := "sww_offerings_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal("create disposable schema:", err)
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanupCtx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("drop disposable schema: %v", err)
		}
	}()
	// Shadow public.schema_migrations before any migration is loaded.
	if _, err := admin.Exec(ctx, `CREATE TABLE `+schema+`.schema_migrations (
        version VARCHAR(64) PRIMARY KEY,
        applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
    )`); err != nil {
		t.Fatal("create local migration ledger:", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("connect to disposable schema:", err)
	}
	defer pool.Close()

	for _, name := range []string{
		"000001_initial_schema.up.sql", "000002_auth_identity.up.sql",
		"000003_auth_hardening.up.sql", "000004_tenants.up.sql",
		"000005_academics.up.sql", "000006_academic_structure.up.sql",
		"000007_subjects_offerings.up.sql",
	} {
		applyAcademicMigrationForTest(t, ctx, pool, filepath.Join("..", "..", "..", "migrations", name))
	}

	must := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("seed/valid SQL failed: %v", err)
		}
	}
	reject := func(name, code, query string, args ...any) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			_, err := pool.Exec(ctx, query, args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) {
				t.Fatalf("expected PostgreSQL error %s, got %v", code, err)
			}
			if pgErr.Code != code {
				t.Fatalf("expected SQLSTATE %s, got %s (%s)", code, pgErr.Code, pgErr.Message)
			}
		})
	}

	tenantA, tenantB := uuid.New(), uuid.New()
	userTeacher, userAdmin := uuid.New(), uuid.New()
	teacherMembership, adminMembership := uuid.New(), uuid.New()
	sessionA, sessionA2, sessionB := uuid.New(), uuid.New(), uuid.New()
	termA, termA2, termB := uuid.New(), uuid.New(), uuid.New()
	levelA, levelA2, levelB := uuid.New(), uuid.New(), uuid.New()
	classA, classA2, classB := uuid.New(), uuid.New(), uuid.New()
	armA, armA2, armB := uuid.New(), uuid.New(), uuid.New()
	subjectA, subjectB := uuid.New(), uuid.New()
	offeringA, assignmentA := uuid.New(), uuid.New()

	for _, user := range []struct {
		id    uuid.UUID
		email string
	}{
		{userTeacher, "sww004c-teacher@school.invalid"},
		{userAdmin, "sww004c-admin@school.invalid"},
	} {
		must(`INSERT INTO users (id,email,password_hash,full_name,status)
            VALUES ($1,$2,'fictional-hash','Fictional Test User','ACTIVE')`, user.id, user.email)
	}
	must(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'sww004c-a','School A','ACTIVE')`, tenantA)
	must(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'sww004c-b','School B','ACTIVE')`, tenantB)
	must(`INSERT INTO memberships (id,tenant_id,user_id,role,status)
        VALUES ($1,$2,$3,'TEACHER','ACTIVE')`, teacherMembership, tenantA, userTeacher)
	must(`INSERT INTO memberships (id,tenant_id,user_id,role,status)
        VALUES ($1,$2,$3,'SCHOOL_ADMIN','ACTIVE')`, adminMembership, tenantA, userAdmin)

	for _, session := range []struct {
		id, tenant uuid.UUID
		name       string
	}{
		{sessionA, tenantA, "2026/2027"}, {sessionA2, tenantA, "2027/2028"}, {sessionB, tenantB, "2026/2027"},
	} {
		must(`INSERT INTO academic_sessions (id,tenant_id,name,starts_on,ends_on,status)
            VALUES ($1,$2,$3,'2026-09-01','2027-07-31','DRAFT')`, session.id, session.tenant, session.name)
	}
	for _, term := range []struct{ id, tenant, session uuid.UUID }{
		{termA, tenantA, sessionA}, {termA2, tenantA, sessionA2}, {termB, tenantB, sessionB},
	} {
		must(`INSERT INTO academic_terms (id,tenant_id,session_id,sequence,name,starts_on,ends_on)
            VALUES ($1,$2,$3,1,'First Term','2026-09-01','2026-12-15')`, term.id, term.tenant, term.session)
	}
	for _, level := range []struct {
		id, tenant uuid.UUID
		code       string
	}{
		{levelA, tenantA, "P3"}, {levelA2, tenantA, "P4"}, {levelB, tenantB, "P3"},
	} {
		must(`INSERT INTO grade_levels (id,tenant_id,stage,code,name,sort_order)
            VALUES ($1,$2,'PRIMARY',$3,$3,3)`, level.id, level.tenant, level.code)
	}
	for _, class := range []struct {
		id, tenant, session, level uuid.UUID
		code                       string
	}{
		{classA, tenantA, sessionA, levelA, "P3"},
		{classA2, tenantA, sessionA, levelA2, "P4"},
		{classB, tenantB, sessionB, levelB, "P3"},
	} {
		must(`INSERT INTO classes (id,tenant_id,session_id,grade_level_id,code,name)
            VALUES ($1,$2,$3,$4,$5,$5)`, class.id, class.tenant, class.session, class.level, class.code)
	}
	for _, arm := range []struct{ id, tenant, class uuid.UUID }{
		{armA, tenantA, classA}, {armA2, tenantA, classA2}, {armB, tenantB, classB},
	} {
		must(`INSERT INTO class_arms (id,tenant_id,class_id,code,name)
            VALUES ($1,$2,$3,'A','Arm A')`, arm.id, arm.tenant, arm.class)
	}
	must(`INSERT INTO subjects (id,tenant_id,code,name) VALUES ($1,$2,'MATH','Mathematics')`, subjectA, tenantA)
	must(`INSERT INTO subjects (id,tenant_id,code,name) VALUES ($1,$2,'MATH','Mathematics')`, subjectB, tenantB)

	insertOffering := `INSERT INTO subject_offerings (id,tenant_id,subject_id,term_id,class_id,class_arm_id)
        VALUES ($1,$2,$3,$4,$5,$6)`
	must(insertOffering, offeringA, tenantA, subjectA, termA, classA, nil)
	reject("duplicate class-wide offering with NULL arm", "23505", insertOffering,
		uuid.New(), tenantA, subjectA, termA, classA, nil)
	reject("cross-tenant subject", "23514", insertOffering,
		uuid.New(), tenantA, subjectB, termA, classA, nil)
	reject("cross-tenant class", "23514", insertOffering,
		uuid.New(), tenantA, subjectA, termA, classB, nil)
	reject("cross-tenant term", "23514", insertOffering,
		uuid.New(), tenantA, subjectA, termB, classA, nil)
	reject("term from a different session", "23514", insertOffering,
		uuid.New(), tenantA, subjectA, termA2, classA, nil)
	reject("arm from a different class", "23514", insertOffering,
		uuid.New(), tenantA, subjectA, termA, classA, armA2)
	reject("arm from a different tenant", "23514", insertOffering,
		uuid.New(), tenantA, subjectA, termA, classA, armB)

	insertAssignment := `INSERT INTO teacher_assignments
        (id,tenant_id,offering_id,teacher_membership_id) VALUES ($1,$2,$3,$4)`
	must(insertAssignment, assignmentA, tenantA, offeringA, teacherMembership)
	reject("duplicate active teacher assignment", "23505", insertAssignment,
		uuid.New(), tenantA, offeringA, teacherMembership)
	reject("admin is not a teacher", "23514", insertAssignment,
		uuid.New(), tenantA, offeringA, adminMembership)
	reject("cross-tenant teacher assignment", "23514", insertAssignment,
		uuid.New(), tenantB, offeringA, teacherMembership)

	must(`UPDATE memberships SET status='SUSPENDED' WHERE id=$1`, teacherMembership)
	reject("suspended teacher cannot receive active assignment", "23514", insertAssignment,
		uuid.New(), tenantA, offeringA, teacherMembership)
	must(`UPDATE teacher_assignments SET status='INACTIVE' WHERE id=$1`, assignmentA)
	reject("inactive assignment cannot reactivate", "23514",
		`UPDATE teacher_assignments SET status='ACTIVE' WHERE id=$1`, assignmentA)
	reject("assignment hard delete forbidden", "23514",
		`DELETE FROM teacher_assignments WHERE id=$1`, assignmentA)

	must(`UPDATE subjects SET status='ARCHIVED' WHERE id=$1`, subjectA)
	reject("archived subject cannot be changed", "23514",
		`UPDATE subjects SET name='Changed' WHERE id=$1`, subjectA)
	reject("archived subject cannot be offered", "23514", insertOffering,
		uuid.New(), tenantA, subjectA, termA, classA2, nil)
	reject("subject hard delete forbidden", "23514",
		`DELETE FROM subjects WHERE id=$1`, subjectA)
	must(`UPDATE subject_offerings SET status='ARCHIVED' WHERE id=$1`, offeringA)
	reject("archived offering immutable", "23514",
		`UPDATE subject_offerings SET status='ACTIVE' WHERE id=$1`, offeringA)
	reject("offering hard delete forbidden", "23514",
		`DELETE FROM subject_offerings WHERE id=$1`, offeringA)

	t.Log("SWW-004C tenant, session, arm, membership and lifecycle constraints passed")
	_ = armA // arm A is a valid fixture for future focused scoped-offering tests.
}
