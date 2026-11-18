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
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresTeacherAssignmentRepository runs only against an allowlisted test database.
// Each run creates and drops its own schema. Never run it against sww_dev.
func TestPostgresTeacherAssignmentRepository(t *testing.T) {
	url := strings.TrimSpace(os.Getenv("SWW_TEST_DATABASE_URL"))
	if url == "" {
		if os.Getenv("SWW_REQUIRE_POSTGRES_TESTS") == "1" {
			t.Fatal("SWW_TEST_DATABASE_URL required")
		}
		t.Skip("set SWW_TEST_DATABASE_URL to an isolated PostgreSQL test database")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Database != "sww_structure_test" && cfg.ConnConfig.Database != "sww_ci_test" {
		t.Fatalf("refusing teacher assignment repository test on %q", cfg.ConnConfig.Database)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "sww_teacher_assignment_repo_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("cleanup schema: %v", err)
		}
	}()
	if _, err := admin.Exec(ctx, `CREATE TABLE `+schema+`.schema_migrations (version VARCHAR(64) PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, name := range []string{
		"000001_initial_schema.up.sql", "000002_auth_identity.up.sql", "000003_auth_hardening.up.sql",
		"000004_tenants.up.sql", "000005_academics.up.sql", "000006_academic_structure.up.sql",
		"000007_subjects_offerings.up.sql",
	} {
		applyAcademicMigrationForTest(t, ctx, pool, filepath.Join("..", "..", "..", "migrations", name))
	}
	must := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	tenantA, tenantB := uuid.New(), uuid.New()
	adminID, principalID, ownerID, teacherID, outsiderID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for i, id := range []uuid.UUID{adminID, principalID, ownerID, teacherID, outsiderID} {
		must(`INSERT INTO users (id,email,password_hash,full_name,status) VALUES ($1,$2,'fictional-hash','Fictional Offering User','ACTIVE')`, id, "assignment-repo-"+string(rune('a'+i))+"@school.invalid")
	}
	must(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'assignment-repo-a','School A','ACTIVE')`, tenantA)
	must(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'assignment-repo-b','School B','ACTIVE')`, tenantB)
	for _, m := range []struct {
		tenant, user uuid.UUID
		role         string
	}{
		{tenantA, adminID, "SCHOOL_ADMIN"}, {tenantA, principalID, "PRINCIPAL"},
		{tenantA, ownerID, "SCHOOL_OWNER"}, {tenantA, teacherID, "TEACHER"}, {tenantB, outsiderID, "SCHOOL_ADMIN"},
	} {
		must(`INSERT INTO memberships (id,tenant_id,user_id,role,status) VALUES ($1,$2,$3,$4,'ACTIVE')`, uuid.New(), m.tenant, m.user, m.role)
	}

	sessionA, sessionA2, sessionB := uuid.New(), uuid.New(), uuid.New()
	for _, s := range []struct {
		id, tenant uuid.UUID
		name       string
	}{
		{sessionA, tenantA, "2026/2027"}, {sessionA2, tenantA, "2027/2028"}, {sessionB, tenantB, "2026/2027"},
	} {
		must(`INSERT INTO academic_sessions (id,tenant_id,name,starts_on,ends_on,status) VALUES ($1,$2,$3,'2026-09-01','2027-07-31','DRAFT')`, s.id, s.tenant, s.name)
	}
	termA, termA2, termB := uuid.New(), uuid.New(), uuid.New()
	for _, a := range []struct{ id, tenant, session uuid.UUID }{
		{termA, tenantA, sessionA}, {termA2, tenantA, sessionA2}, {termB, tenantB, sessionB},
	} {
		must(`INSERT INTO academic_terms (id,tenant_id,session_id,sequence,name,starts_on,ends_on) VALUES ($1,$2,$3,1,'First Term','2026-09-01','2026-12-15')`, a.id, a.tenant, a.session)
	}
	levelA, levelB := uuid.New(), uuid.New()
	must(`INSERT INTO grade_levels (id,tenant_id,stage,code,name,sort_order) VALUES ($1,$2,'PRIMARY','P3','Primary 3',3)`, levelA, tenantA)
	must(`INSERT INTO grade_levels (id,tenant_id,stage,code,name,sort_order) VALUES ($1,$2,'PRIMARY','P3','Primary 3',3)`, levelB, tenantB)
	classA, classB := uuid.New(), uuid.New()
	must(`INSERT INTO classes (id,tenant_id,session_id,grade_level_id,code,name) VALUES ($1,$2,$3,$4,'P3','Primary 3')`, classA, tenantA, sessionA, levelA)
	must(`INSERT INTO classes (id,tenant_id,session_id,grade_level_id,code,name) VALUES ($1,$2,$3,$4,'P3','Primary 3')`, classB, tenantB, sessionB, levelB)
	armA, armB := uuid.New(), uuid.New()
	must(`INSERT INTO class_arms (id,tenant_id,class_id,code,name) VALUES ($1,$2,$3,'A','Arm A')`, armA, tenantA, classA)
	must(`INSERT INTO class_arms (id,tenant_id,class_id,code,name) VALUES ($1,$2,$3,'A','Arm A')`, armB, tenantB, classB)
	subjectA, subjectB := uuid.New(), uuid.New()
	must(`INSERT INTO subjects (id,tenant_id,code,name) VALUES ($1,$2,'MATH','Mathematics')`, subjectA, tenantA)
	must(`INSERT INTO subjects (id,tenant_id,code,name) VALUES ($1,$2,'MATH','Mathematics')`, subjectB, tenantB)

	// Obtain server-created membership identifiers: clients never choose tenant IDs.
	var teacherMemberID, ownerMemberID uuid.UUID
	for _, entry := range []struct {
		user uuid.UUID
		role string
		out  *uuid.UUID
	}{
		{teacherID, "TEACHER", &teacherMemberID},
		{ownerID, "SCHOOL_OWNER", &ownerMemberID},
	} {
		if err := pool.QueryRow(ctx, `SELECT id FROM memberships WHERE tenant_id=$1 AND user_id=$2 AND role=$3`, tenantA, entry.user, entry.role).Scan(entry.out); err != nil {
			t.Fatal(err)
		}
	}
	// An isolated B-school teacher membership tests composite foreign keys.
	outsiderTeacherMemberID := uuid.New()
	must(`INSERT INTO memberships (id,tenant_id,user_id,role,status) VALUES ($1,$2,$3,'TEACHER','ACTIVE')`, outsiderTeacherMemberID, tenantB, outsiderID)

	offeringA, offeringB, offeringArmA := uuid.New(), uuid.New(), uuid.New()
	must(`INSERT INTO subject_offerings (id,tenant_id,subject_id,term_id,class_id) VALUES ($1,$2,$3,$4,$5)`, offeringA, tenantA, subjectA, termA, classA)
	must(`INSERT INTO subject_offerings (id,tenant_id,subject_id,term_id,class_id) VALUES ($1,$2,$3,$4,$5)`, offeringB, tenantB, subjectB, termB, classB)
	must(`INSERT INTO subject_offerings (id,tenant_id,subject_id,term_id,class_id,class_arm_id) VALUES ($1,$2,$3,$4,$5,$6)`, offeringArmA, tenantA, subjectA, termA, classA, armA)

	service := NewTeacherAssignmentService(NewTeacherAssignmentRepository(pool))
	request := CreateTeacherAssignmentRequest{OfferingID: offeringA, TeacherMembershipID: teacherMemberID}
	countAudit := func() int {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND subject_type='teacher_assignment'`, tenantA).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	created, err := service.CreateTeacherAssignment(ctx, adminID, "assignment-repo-a", request, "assign-create")
	if err != nil || created == nil || created.Status != "ACTIVE" || created.OfferingID != offeringA || created.TeacherMembershipID != teacherMemberID {
		t.Fatalf("create assignment: %v %+v", err, created)
	}
	if countAudit() != 1 {
		t.Fatalf("creation audit count %d, want 1", countAudit())
	}

	t.Run("owner and teacher cannot mutate", func(t *testing.T) {
		before := countAudit()
		for _, actor := range []uuid.UUID{ownerID, teacherID} {
			if _, err := service.CreateTeacherAssignment(ctx, actor, "assignment-repo-a", request, "denied-create"); !errors.Is(err, ErrForbidden) {
				t.Fatalf("create actor %s: want forbidden, got %v", actor, err)
			}
			if _, err := service.DeactivateTeacherAssignment(ctx, actor, "assignment-repo-a", created.ID, "denied-deactivate"); !errors.Is(err, ErrForbidden) {
				t.Fatalf("deactivate actor %s: want forbidden, got %v", actor, err)
			}
		}
		if countAudit() != before {
			t.Fatal("denied writes added audit events")
		}
	})

	t.Run("tenant isolated listings and mutations", func(t *testing.T) {
		for _, actor := range []uuid.UUID{ownerID, teacherID, principalID} {
			items, err := service.ListTeacherAssignments(ctx, actor, "assignment-repo-a")
			if err != nil || len(items) != 1 || items[0].ID != created.ID {
				t.Fatalf("listing actor %s: %v %+v", actor, err, items)
			}
		}
		if _, err := service.ListTeacherAssignments(ctx, outsiderID, "assignment-repo-a"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("outsider list: %v", err)
		}
		if _, err := service.DeactivateTeacherAssignment(ctx, outsiderID, "assignment-repo-a", created.ID, "outsider"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("outsider deactivation: %v", err)
		}
	})

	t.Run("assignment listing role scopes", func(t *testing.T) {
		teacherTwoID := uuid.New()
		teacherTwoMemberID := uuid.New()
		studentID := uuid.New()
		parentID := uuid.New()
		bursarID := uuid.New()

		users := []struct {
			id           uuid.UUID
			email        string
			role         string
			membershipID uuid.UUID
		}{
			{teacherTwoID, "assignment-teacher-two@school.invalid", "TEACHER", teacherTwoMemberID},
			{studentID, "assignment-student@school.invalid", "STUDENT", uuid.New()},
			{parentID, "assignment-parent@school.invalid", "PARENT", uuid.New()},
			{bursarID, "assignment-bursar@school.invalid", "BURSAR", uuid.New()},
		}

		for _, u := range users {
			must(`INSERT INTO users (id,email,password_hash,full_name,status)
            VALUES ($1,$2,'fictional-hash','Fictional Role Tester','ACTIVE')`,
				u.id, u.email)

			must(`INSERT INTO memberships (id,tenant_id,user_id,role,status)
            VALUES ($1,$2,$3,$4,'ACTIVE')`,
				u.membershipID, tenantA, u.id, u.role)
		}

		second, err := service.CreateTeacherAssignment(
			ctx, principalID, "assignment-repo-a",
			CreateTeacherAssignmentRequest{
				OfferingID:          offeringArmA,
				TeacherMembershipID: teacherTwoMemberID,
			},
			"second-teacher-assignment",
		)
		if err != nil {
			t.Fatalf("create second teacher assignment: %v", err)
		}

		for _, actor := range []uuid.UUID{ownerID, adminID, principalID} {
			items, err := service.ListTeacherAssignments(ctx, actor, "assignment-repo-a")
			if err != nil || len(items) != 2 {
				t.Fatalf("privileged actor %s: got %d assignments, err %v", actor, len(items), err)
			}
		}

		firstItems, err := service.ListTeacherAssignments(ctx, teacherID, "assignment-repo-a")
		if err != nil || len(firstItems) != 1 || firstItems[0].ID != created.ID {
			t.Fatalf("first teacher must see only own assignment: %v %+v", err, firstItems)
		}

		secondItems, err := service.ListTeacherAssignments(ctx, teacherTwoID, "assignment-repo-a")
		if err != nil || len(secondItems) != 1 || secondItems[0].ID != second.ID {
			t.Fatalf("second teacher must see only own assignment: %v %+v", err, secondItems)
		}

		for _, actor := range []uuid.UUID{studentID, parentID} {
			if _, err := service.ListTeacherAssignments(ctx, actor, "assignment-repo-a"); !errors.Is(err, ErrForbidden) {
				t.Fatalf("student/parent %s: want forbidden, got %v", actor, err)
			}
		}

		// The existing shared tenant helper conceals unsupported
		// BURSAR access with NotFound rather than exposing tenant details.
		if _, err := service.ListTeacherAssignments(ctx, bursarID, "assignment-repo-a"); !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrForbidden) {
			t.Fatalf("bursar must be denied, got %v", err)
		}

		must(`UPDATE memberships SET status='SUSPENDED' WHERE id=$1`, teacherTwoMemberID)

		if _, err := service.ListTeacherAssignments(ctx, teacherTwoID, "assignment-repo-a"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("suspended teacher must be denied: %v", err)
		}

		items, err := service.ListTeacherAssignments(ctx, principalID, "assignment-repo-a")
		if err != nil || len(items) != 2 {
			t.Fatalf("administrator must retain historical assignment visibility: %v %+v", err, items)
		}
	})
	t.Run("duplicate active assignment rolls back audit", func(t *testing.T) {
		before := countAudit()
		if _, err := service.CreateTeacherAssignment(ctx, principalID, "assignment-repo-a", request, "duplicate"); !errors.Is(err, ErrConflict) {
			t.Fatalf("duplicate: want conflict, got %v", err)
		}
		if countAudit() != before {
			t.Fatal("duplicate added audit event")
		}
	})

	t.Run("invalid membership and offering rejected", func(t *testing.T) {
		before := countAudit()
		for _, tc := range []struct {
			name    string
			request CreateTeacherAssignmentRequest
		}{
			{"non-teacher owner membership", CreateTeacherAssignmentRequest{OfferingID: offeringA, TeacherMembershipID: ownerMemberID}},
			{"cross-school teacher membership", CreateTeacherAssignmentRequest{OfferingID: offeringA, TeacherMembershipID: outsiderTeacherMemberID}},
			{"cross-school offering", CreateTeacherAssignmentRequest{OfferingID: offeringB, TeacherMembershipID: teacherMemberID}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if _, err := service.CreateTeacherAssignment(ctx, principalID, "assignment-repo-a", tc.request, "invalid"); !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("want invalid input, got %v", err)
				}
			})
		}
		if countAudit() != before {
			t.Fatal("invalid input added audit event")
		}
	})

	t.Run("archived offering cannot accept teacher", func(t *testing.T) {
		before := countAudit()
		// Archive the foreign school's offering, and verify it cannot be assigned by its admin.
		must(`UPDATE subject_offerings SET status='ARCHIVED' WHERE tenant_id=$1 AND id=$2`, tenantB, offeringB)
		if _, err := service.CreateTeacherAssignment(ctx, outsiderID, "assignment-repo-b", CreateTeacherAssignmentRequest{
			OfferingID: offeringB, TeacherMembershipID: outsiderTeacherMemberID,
		}, "archived-offering"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("want invalid input for archived offering, got %v", err)
		}
		if countAudit() != before {
			t.Fatal("archived offering attempt added audit")
		}
	})

	t.Run("principal deactivates after teacher suspension", func(t *testing.T) {
		must(`UPDATE memberships SET status='SUSPENDED' WHERE id=$1`, teacherMemberID)
		before := countAudit()
		if _, err := service.CreateTeacherAssignment(ctx, principalID, "assignment-repo-a", CreateTeacherAssignmentRequest{
			OfferingID: offeringArmA, TeacherMembershipID: teacherMemberID,
		}, "suspended-member"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("suspended teacher membership: want invalid input, got %v", err)
		}
		if countAudit() != before {
			t.Fatal("suspended attempt added audit")
		}
		got, err := service.DeactivateTeacherAssignment(ctx, principalID, "assignment-repo-a", created.ID, "deactivate-after-suspension")
		if err != nil || got == nil || got.Status != "INACTIVE" {
			t.Fatalf("deactivate: %v %+v", err, got)
		}
		if countAudit() != before+1 {
			t.Fatalf("deactivation audit count %d want %d", countAudit(), before+1)
		}
		if _, err := service.DeactivateTeacherAssignment(ctx, principalID, "assignment-repo-a", created.ID, "repeat"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("repeat deactivate: %v", err)
		}
		if countAudit() != before+1 {
			t.Fatal("repeat deactivation added audit")
		}
		var stored string
		if err := pool.QueryRow(ctx, `SELECT status FROM teacher_assignments WHERE tenant_id=$1 AND id=$2`, tenantA, created.ID).Scan(&stored); err != nil || stored != "INACTIVE" {
			t.Fatalf("persisted status: %q %v", stored, err)
		}
	})

	t.Run("inactive assignment cannot be reactivated", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE teacher_assignments SET status='ACTIVE' WHERE id=$1`, created.ID); err == nil {
			t.Fatal("reactivation unexpectedly succeeded")
		}
	})

	t.Run("suspended administrator cannot write", func(t *testing.T) {
		must(`UPDATE memberships SET status='SUSPENDED' WHERE tenant_id=$1 AND user_id=$2 AND role='SCHOOL_ADMIN'`, tenantA, adminID)
		before := countAudit()
		if _, err := service.DeactivateTeacherAssignment(ctx, adminID, "assignment-repo-a", created.ID, "suspended-admin"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("suspended administrator: %v", err)
		}
		if countAudit() != before {
			t.Fatal("suspended administrator added audit")
		}
	})
}
