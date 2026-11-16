package academics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SWW-004B runs entirely in a newly created schema in a disposable database.
// It never truncates, deletes, or modifies records in the public schema.
func TestPostgresAcademicStructureTenantIsolationAndConstraints(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("SWW_TEST_DATABASE_URL"))
	if databaseURL == "" {
		if os.Getenv("SWW_REQUIRE_POSTGRES_TESTS") == "1" {
			t.Fatal("SWW_TEST_DATABASE_URL is required")
		}
		t.Skip("set SWW_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal("connect to isolated PostgreSQL database:", err)
	}
	defer adminPool.Close()

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("parse PostgreSQL test URL:", err)
	}
	schema := "sww_structure_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal("create disposable structure schema:", err)
	}
	// Defer cleanup before later fatal calls, and close the schema pool first.
	defer func() {
		if _, err := adminPool.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("clean up disposable structure schema: %v", err)
		}
	}()
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal("open schema-scoped pool:", err)
	}
	defer pool.Close()

	for _, name := range []string{
		"000001_initial_schema.up.sql",
		"000002_auth_identity.up.sql",
		"000003_auth_hardening.up.sql",
		"000004_tenants.up.sql",
		"000005_academics.up.sql",
		"000006_academic_structure.up.sql",
	} {
		applyAcademicMigrationForTest(t, ctx, pool, filepath.Join("..", "..", "..", "migrations", name))
	}

	ownerA, adminA, principalA, teacherA := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ownerB, platformAdmin := uuid.New(), uuid.New()
	tenantA, tenantB := uuid.New(), uuid.New()
	for _, u := range []struct {
		id    uuid.UUID
		email string
	}{
		{ownerA, "structure-owner-a@school.invalid"},
		{adminA, "structure-admin-a@school.invalid"},
		{principalA, "structure-principal-a@school.invalid"},
		{teacherA, "structure-teacher-a@school.invalid"},
		{ownerB, "structure-owner-b@school.invalid"},
		{platformAdmin, "structure-platform@school.invalid"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, full_name, status)
            VALUES ($1, $2, 'fictional-test-hash', 'Fictional User', 'ACTIVE')`, u.id, u.email); err != nil {
			t.Fatalf("create test user: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO platform_admins (user_id) VALUES ($1)`, platformAdmin); err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []struct {
		id   uuid.UUID
		slug string
	}{
		{tenantA, "structure-school-a"}, {tenantB, "structure-school-b"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name, status, locale, timezone)
            VALUES ($1, $2::citext, $2::text, 'ACTIVE', 'en-NG', 'Africa/Lagos')`, tenant.id, tenant.slug); err != nil {
			t.Fatal("create test school:", err)
		}
	}
	for _, m := range []struct {
		tenant, user uuid.UUID
		role         string
	}{
		{tenantA, ownerA, "SCHOOL_OWNER"},
		{tenantA, adminA, "SCHOOL_ADMIN"},
		{tenantA, principalA, "PRINCIPAL"},
		{tenantA, teacherA, "TEACHER"},
		{tenantB, ownerB, "SCHOOL_OWNER"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO memberships (id, tenant_id, user_id, role, status)
            VALUES ($1, $2, $3, $4, 'ACTIVE')`, uuid.New(), m.tenant, m.user, m.role); err != nil {
			t.Fatal("create test membership:", err)
		}
	}

	svc := NewStructureService(NewStructureRepository(pool))
	sessionSvc := NewService(NewPostgresRepository(pool))
	sessionA, err := sessionSvc.CreateSession(ctx, ownerA, "structure-school-a", CreateSessionRequest{
		Name: "2026/2027", StartsOn: "2026-09-01", EndsOn: "2027-07-31",
	}, "structure-session-a")
	if err != nil {
		t.Fatal("create school A session:", err)
	}
	sessionB, err := sessionSvc.CreateSession(ctx, ownerB, "structure-school-b", CreateSessionRequest{
		Name: "2026/2027", StartsOn: "2026-09-01", EndsOn: "2027-07-31",
	}, "structure-session-b")
	if err != nil {
		t.Fatal("create school B session:", err)
	}

	levelA, err := svc.CreateGradeLevel(ctx, ownerA, "structure-school-a", CreateGradeLevelRequest{
		Stage: "primary", Code: "p3", Name: " Primary 3 ", SortOrder: 3,
	}, "structure-level-a")
	if err != nil {
		t.Fatal("owner creates grade level:", err)
	}
	if levelA.Code != "P3" || levelA.Stage != StagePrimary || levelA.Name != "Primary 3" || levelA.Status != StructureActive {
		t.Fatalf("unexpected grade normalization or status: %+v", levelA)
	}
	if _, err := svc.CreateGradeLevel(ctx, teacherA, "structure-school-a", CreateGradeLevelRequest{
		Stage: StagePrimary, Code: "P4", Name: "Primary 4", SortOrder: 4,
	}, "structure-teacher-create"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("teacher should not create grade level: %v", err)
	}
	if _, err := svc.CreateGradeLevel(ctx, ownerB, "structure-school-a", CreateGradeLevelRequest{
		Stage: StagePrimary, Code: "P5", Name: "Primary 5", SortOrder: 5,
	}, "structure-cross-school"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("school B owner cannot mutate school A: %v", err)
	}
	if _, err := svc.ListGradeLevels(ctx, platformAdmin, "structure-school-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("platform admin without membership cannot read levels: %v", err)
	}
	if _, err := svc.CreateGradeLevel(ctx, adminA, "structure-school-a", CreateGradeLevelRequest{
		Stage: StagePrimary, Code: "P3", Name: "Duplicate", SortOrder: 30,
	}, "structure-level-duplicate"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate tenant-stage-code must conflict: %v", err)
	}
	if _, err := svc.CreateGradeLevel(ctx, principalA, "structure-school-a", CreateGradeLevelRequest{
		Stage: StagePrimary, Code: "P4", Name: "Primary 4", SortOrder: 4,
	}, "structure-principal-create"); err != nil {
		t.Fatalf("principal should be permitted to create level: %v", err)
	}
	levelB, err := svc.CreateGradeLevel(ctx, ownerB, "structure-school-b", CreateGradeLevelRequest{
		Stage: StagePrimary, Code: "P3", Name: "Primary 3", SortOrder: 3,
	}, "structure-level-b")
	if err != nil {
		t.Fatal("school B may reuse P3 code:", err)
	}

	classA, err := svc.CreateClass(ctx, ownerA, "structure-school-a", CreateClassRequest{
		SessionID: sessionA.ID, GradeLevelID: levelA.ID, Code: "p3", Name: "Primary 3",
	}, "structure-class-a")
	if err != nil {
		t.Fatal("create class:", err)
	}
	if classA.Code != "P3" {
		t.Fatalf("class code was not normalized: %s", classA.Code)
	}
	if _, err := svc.CreateClass(ctx, ownerA, "structure-school-a", CreateClassRequest{
		SessionID: sessionA.ID, GradeLevelID: levelA.ID, Code: "ANOTHER", Name: "Another Primary 3",
	}, "structure-duplicate-level-class"); !errors.Is(err, ErrConflict) {
		t.Fatalf("one class per grade/session required: %v", err)
	}
	if _, err := svc.CreateClass(ctx, ownerA, "structure-school-a", CreateClassRequest{
		SessionID: sessionB.ID, GradeLevelID: levelA.ID, Code: "CROSSSESSION", Name: "Invalid",
	}, "structure-cross-session"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-tenant session FK must reject: %v", err)
	}
	if _, err := svc.CreateClass(ctx, ownerA, "structure-school-a", CreateClassRequest{
		SessionID: sessionA.ID, GradeLevelID: levelB.ID, Code: "CROSSLEVEL", Name: "Invalid",
	}, "structure-cross-level"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-tenant level FK must reject: %v", err)
	}
	if _, err := svc.CreateClass(ctx, teacherA, "structure-school-a", CreateClassRequest{
		SessionID: sessionA.ID, GradeLevelID: levelA.ID, Code: "TEACHER", Name: "Invalid",
	}, "structure-teacher-class"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("teacher should not create class: %v", err)
	}

	armA, err := svc.CreateClassArm(ctx, adminA, "structure-school-a", CreateClassArmRequest{
		ClassID: classA.ID, Code: "blue", Name: "Blue",
	}, "structure-arm-a")
	if err != nil {
		t.Fatal("school admin creates arm:", err)
	}
	if armA.Code != "BLUE" {
		t.Fatalf("arm code was not normalized: %s", armA.Code)
	}
	if _, err := svc.CreateClassArm(ctx, ownerA, "structure-school-a", CreateClassArmRequest{
		ClassID: classA.ID, Code: "BLUE", Name: "Duplicate",
	}, "structure-arm-duplicate"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate arm code must conflict: %v", err)
	}
	if _, err := svc.CreateClassArm(ctx, ownerB, "structure-school-b", CreateClassArmRequest{
		ClassID: classA.ID, Code: "A", Name: "Arm A",
	}, "structure-arm-cross-school"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-tenant class FK must reject: %v", err)
	}

	if levels, err := svc.ListGradeLevels(ctx, teacherA, "structure-school-a"); err != nil || len(levels) != 2 {
		t.Fatalf("teacher can list own school's two levels: count=%d err=%v", len(levels), err)
	}
	if levels, err := svc.ListGradeLevels(ctx, ownerB, "structure-school-b"); err != nil || len(levels) != 1 {
		t.Fatalf("school B must see only its own level: count=%d err=%v", len(levels), err)
	}
	if classes, err := svc.ListClasses(ctx, teacherA, "structure-school-a", sessionA.ID); err != nil || len(classes) != 1 {
		t.Fatalf("teacher may list own session's classes: count=%d err=%v", len(classes), err)
	}
	if classes, err := svc.ListClasses(ctx, ownerB, "structure-school-b", sessionA.ID); err != nil || len(classes) != 0 {
		t.Fatalf("school B must not see school A class: count=%d err=%v", len(classes), err)
	}
	if arms, err := svc.ListClassArms(ctx, teacherA, "structure-school-a", classA.ID); err != nil || len(arms) != 1 {
		t.Fatalf("teacher may list own class arms: count=%d err=%v", len(arms), err)
	}
	if arms, err := svc.ListClassArms(ctx, ownerB, "structure-school-b", classA.ID); err != nil || len(arms) != 0 {
		t.Fatalf("school B must not see school A class arms: count=%d err=%v", len(arms), err)
	}

	for _, audit := range []struct {
		action    string
		subjectID uuid.UUID
	}{
		{"academic.grade_level_created", levelA.ID},
		{"academic.class_created", classA.ID},
		{"academic.class_arm_created", armA.ID},
	} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events
           WHERE tenant_id = $1 AND action = $2 AND subject_id = $3`,
			tenantA, audit.action, audit.subjectID.String()).Scan(&count); err != nil {
			t.Fatal("query audit event:", err)
		}
		if count != 1 {
			t.Fatalf("expected one audit event %s, got %d", audit.action, count)
		}
	}
	// A failed duplicate creation must not produce an extra audit event.
	var duplicateAudits int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id = 'structure-level-duplicate'`).Scan(&duplicateAudits); err != nil {
		t.Fatal(err)
	}
	if duplicateAudits != 0 {
		t.Fatalf("failed duplicate must not have audit event, found %d", duplicateAudits)
	}

	for _, item := range []struct {
		table string
		id    uuid.UUID
	}{
		{"class_arms", armA.ID}, {"classes", classA.ID}, {"grade_levels", levelA.ID},
	} {
		// table names are hardcoded in the test, never received from users.
		query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, item.table)
		if _, err := pool.Exec(ctx, query, item.id); err == nil {
			t.Fatalf("hard delete on %s should be rejected by trigger", item.table)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE class_arms SET status = 'ARCHIVED' WHERE id = $1`, armA.ID); err != nil {
		t.Fatal("archive arm:", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE class_arms SET name = 'Renamed' WHERE id = $1`, armA.ID); err == nil {
		t.Fatal("archived arm should be immutable")
	}
	if _, err := pool.Exec(ctx, `UPDATE classes SET status = 'ARCHIVED' WHERE id = $1`, classA.ID); err != nil {
		t.Fatal("archive class:", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE classes SET name = 'Renamed' WHERE id = $1`, classA.ID); err == nil {
		t.Fatal("archived class should be immutable")
	}
	if _, err := pool.Exec(ctx, `UPDATE grade_levels SET status = 'ARCHIVED' WHERE id = $1`, levelA.ID); err != nil {
		t.Fatal("archive grade level:", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE grade_levels SET name = 'Renamed' WHERE id = $1`, levelA.ID); err == nil {
		t.Fatal("archived grade level should be immutable")
	}
}
