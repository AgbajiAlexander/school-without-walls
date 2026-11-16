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

// TestPostgresAcademicStructureMutations verifies the real mutation repository
// in an isolated temporary schema. No public-school records are changed.
func TestPostgresAcademicStructureMutations(t *testing.T) {
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
	if cfg.ConnConfig.Database != "sww_structure_test" {
		t.Fatalf("refusing mutation test on database %q", cfg.ConnConfig.Database)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal("connect to test database:", err)
	}
	defer adminPool.Close()

	schema := "sww_mutations_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal("create test schema:", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := adminPool.Exec(cleanupCtx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("remove temporary test schema: %v", err)
		}
	}()

	// Shadow public.schema_migrations before applying 000001. This avoids
	// migration-ledger search_path leakage into the shared public schema.
	if _, err := adminPool.Exec(ctx, `CREATE TABLE `+schema+`.schema_migrations (
  version VARCHAR(64) PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
 )`); err != nil {
		t.Fatal("create isolated migration ledger:", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("open isolated schema connection:", err)
	}
	defer pool.Close()
	for _, name := range []string{
		"000001_initial_schema.up.sql", "000002_auth_identity.up.sql",
		"000003_auth_hardening.up.sql", "000004_tenants.up.sql",
		"000005_academics.up.sql", "000006_academic_structure.up.sql",
	} {
		applyAcademicMigrationForTest(t, ctx, pool, filepath.Join("..", "..", "..", "migrations", name))
	}

	owner, admin, principal, teacher, otherOwner := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	tenantA, tenantB := uuid.New(), uuid.New()
	for _, user := range []struct {
		id    uuid.UUID
		email string
	}{
		{owner, "mutation-owner@school.invalid"}, {admin, "mutation-admin@school.invalid"},
		{principal, "mutation-principal@school.invalid"}, {teacher, "mutation-teacher@school.invalid"},
		{otherOwner, "mutation-other-owner@school.invalid"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, full_name, status)
   VALUES ($1, $2, 'fictional-test-hash', 'Fictional User', 'ACTIVE')`, user.id, user.email); err != nil {
			t.Fatal("insert test user:", err)
		}
	}
	for _, tenant := range []struct {
		id   uuid.UUID
		slug string
	}{{tenantA, "mutation-school-a"}, {tenantB, "mutation-school-b"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name, status, locale, timezone)
   VALUES ($1, $2::citext, $2::text, 'ACTIVE', 'en-NG', 'Africa/Lagos')`, tenant.id, tenant.slug); err != nil {
			t.Fatal("insert test tenant:", err)
		}
	}
	for _, member := range []struct {
		tenant, user uuid.UUID
		role         string
	}{
		{tenantA, owner, "SCHOOL_OWNER"}, {tenantA, admin, "SCHOOL_ADMIN"},
		{tenantA, principal, "PRINCIPAL"}, {tenantA, teacher, "TEACHER"},
		{tenantB, otherOwner, "SCHOOL_OWNER"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO memberships (id, tenant_id, user_id, role, status)
   VALUES ($1, $2, $3, $4, 'ACTIVE')`, uuid.New(), member.tenant, member.user, member.role); err != nil {
			t.Fatal("insert membership:", err)
		}
	}

	svc := NewStructureService(NewStructureRepository(pool))
	sessionSvc := NewService(NewPostgresRepository(pool))
	session, err := sessionSvc.CreateSession(ctx, owner, "mutation-school-a", CreateSessionRequest{
		Name: "2026/2027", StartsOn: "2026-09-01", EndsOn: "2027-07-31",
	}, "mutation-session")
	if err != nil {
		t.Fatal("create session:", err)
	}
	level, err := svc.CreateGradeLevel(ctx, owner, "mutation-school-a", CreateGradeLevelRequest{
		Stage: "PRIMARY", Code: "P3", Name: "Primary 3", SortOrder: 3,
	}, "mutation-level-create")
	if err != nil {
		t.Fatal("create grade level:", err)
	}
	class, err := svc.CreateClass(ctx, owner, "mutation-school-a", CreateClassRequest{
		SessionID: session.ID, GradeLevelID: level.ID, Code: "P3", Name: "Primary 3",
	}, "mutation-class-create")
	if err != nil {
		t.Fatal("create class:", err)
	}
	arm, err := svc.CreateClassArm(ctx, owner, "mutation-school-a", CreateClassArmRequest{
		ClassID: class.ID, Code: "A", Name: "Arm A",
	}, "mutation-arm-create")
	if err != nil {
		t.Fatal("create arm:", err)
	}

	newLevelName := "Primary Three"
	updatedLevel, err := svc.UpdateGradeLevel(ctx, admin, "mutation-school-a", level.ID,
		UpdateGradeLevelRequest{Name: &newLevelName}, "mutation-level-update")
	if err != nil || updatedLevel.Name != newLevelName {
		t.Fatalf("admin updates grade: result=%+v err=%v", updatedLevel, err)
	}
	newClassCode := "P3_NEW"
	updatedClass, err := svc.UpdateClass(ctx, principal, "mutation-school-a", class.ID,
		UpdateClassRequest{Code: &newClassCode}, "mutation-class-update")
	if err != nil || updatedClass.Code != newClassCode {
		t.Fatalf("principal updates class: result=%+v err=%v", updatedClass, err)
	}
	newArmName := "Blue Arm"
	updatedArm, err := svc.UpdateClassArm(ctx, owner, "mutation-school-a", arm.ID,
		UpdateClassArmRequest{Name: &newArmName}, "mutation-arm-update")
	if err != nil || updatedArm.Name != newArmName {
		t.Fatalf("owner updates arm: result=%+v err=%v", updatedArm, err)
	}

	if _, err := svc.UpdateGradeLevel(ctx, teacher, "mutation-school-a", level.ID,
		UpdateGradeLevelRequest{Name: &newLevelName}, "teacher-denied"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("teacher grade update must be forbidden; got %v", err)
	}
	if _, err := svc.ArchiveClass(ctx, teacher, "mutation-school-a", class.ID, "teacher-archive-denied"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("teacher archive must be forbidden; got %v", err)
	}
	if _, err := svc.UpdateClassArm(ctx, otherOwner, "mutation-school-a", arm.ID,
		UpdateClassArmRequest{Name: &newArmName}, "other-owner-denied"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-school update should not disclose school; got %v", err)
	}
	if _, err := svc.ArchiveGradeLevel(ctx, otherOwner, "mutation-school-a", level.ID, "other-owner-archive-denied"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-school archive should not disclose school; got %v", err)
	}
	if _, err := svc.UpdateClass(ctx, owner, "mutation-school-a", class.ID,
		UpdateClassRequest{}, "empty-update"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty class update must be invalid; got %v", err)
	}

	// All six successful writes (three creates and three updates) must have
	// transactionally persisted audit records scoped to the test tenant.
	for _, action := range []string{
		"academic.grade_level_created", "academic.class_created", "academic.class_arm_created",
		"academic.grade_level_updated", "academic.class_updated", "academic.class_arm_updated",
	} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE tenant_id=$1 AND action=$2`, tenantA, action).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("expected one tenant-scoped audit for %q; got %d", action, count)
		}
	}

	// Archive from the leaf upward to preserve parent lifecycle restrictions.
	archivedArm, err := svc.ArchiveClassArm(ctx, admin, "mutation-school-a", arm.ID, "mutation-arm-archive")
	if err != nil || archivedArm.Status != StructureArchived {
		t.Fatalf("archive arm: result=%+v err=%v", archivedArm, err)
	}
	if _, err := svc.UpdateClassArm(ctx, owner, "mutation-school-a", arm.ID,
		UpdateClassArmRequest{Name: &newArmName}, "archived-arm-update"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived arm update should be denied; got %v", err)
	}
	archivedClass, err := svc.ArchiveClass(ctx, owner, "mutation-school-a", class.ID, "mutation-class-archive")
	if err != nil || archivedClass.Status != StructureArchived {
		t.Fatalf("archive class: result=%+v err=%v", archivedClass, err)
	}
	if _, err := svc.UpdateClass(ctx, owner, "mutation-school-a", class.ID,
		UpdateClassRequest{Name: &newLevelName}, "archived-class-update"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived class update should be denied; got %v", err)
	}
	archivedLevel, err := svc.ArchiveGradeLevel(ctx, principal, "mutation-school-a", level.ID, "mutation-level-archive")
	if err != nil || archivedLevel.Status != StructureArchived {
		t.Fatalf("archive grade: result=%+v err=%v", archivedLevel, err)
	}
	if _, err := svc.UpdateGradeLevel(ctx, owner, "mutation-school-a", level.ID,
		UpdateGradeLevelRequest{Name: &newLevelName}, "archived-grade-update"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived grade update should be denied; got %v", err)
	}
	for _, action := range []string{
		"academic.class_arm_archived", "academic.class_archived", "academic.grade_level_archived",
	} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE tenant_id=$1 AND action=$2`, tenantA, action).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("expected one archive audit %q; got %d", action, count)
		}
	}

	// Enforce archived immutability even for someone issuing SQL directly.
	if _, err := pool.Exec(ctx, `UPDATE grade_levels SET name='illegal edit' WHERE id=$1`, level.ID); err == nil {
		t.Fatal("database must reject changes to archived grade level")
	}
}
