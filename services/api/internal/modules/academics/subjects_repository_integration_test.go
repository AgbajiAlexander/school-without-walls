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

// TestPostgresSubjectRepository exercises real authorization, transactions and
// audit behavior in a disposable schema; never point it at sww_dev.
func TestPostgresSubjectRepository(t *testing.T) {
	url := strings.TrimSpace(os.Getenv("SWW_TEST_DATABASE_URL"))
	if url == "" {
		if os.Getenv("SWW_REQUIRE_POSTGRES_TESTS") == "1" {
			t.Fatal("SWW_TEST_DATABASE_URL required")
		}
		t.Skip("set SWW_TEST_DATABASE_URL to an isolated PostgreSQL test database")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	if cfg.ConnConfig.Database != "sww_structure_test" && cfg.ConnConfig.Database != "sww_ci_test" {
		t.Fatalf("refusing repository test on database %q", cfg.ConnConfig.Database)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "sww_subject_repo_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanupCtx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
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
		"000001_initial_schema.up.sql", "000002_auth_identity.up.sql", "000003_auth_hardening.up.sql", "000004_tenants.up.sql", "000005_academics.up.sql", "000006_academic_structure.up.sql", "000007_subjects_offerings.up.sql",
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
	owner, adminUser, principal, teacher, outsider, multiRole := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for i, id := range []uuid.UUID{owner, adminUser, principal, teacher, outsider, multiRole} {
		must(`INSERT INTO users (id,email,password_hash,full_name,status) VALUES ($1,$2,'fictional-hash','Fictional User','ACTIVE')`, id, "subject-repo-"+string(rune('a'+i))+"@school.invalid")
	}
	must(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'subject-repo-a','School A','ACTIVE')`, tenantA)
	must(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'subject-repo-b','School B','ACTIVE')`, tenantB)
	for _, m := range []struct {
		tenant, user uuid.UUID
		role         string
	}{
		{tenantA, owner, "SCHOOL_OWNER"}, {tenantA, adminUser, "SCHOOL_ADMIN"}, {tenantA, principal, "PRINCIPAL"}, {tenantA, teacher, "TEACHER"}, {tenantB, outsider, "SCHOOL_ADMIN"}, {tenantA, multiRole, "SCHOOL_OWNER"}, {tenantA, multiRole, "SCHOOL_ADMIN"},
	} {
		must(`INSERT INTO memberships (id,tenant_id,user_id,role,status) VALUES ($1,$2,$3,$4,'ACTIVE')`, uuid.New(), m.tenant, m.user, m.role)
	}
	service := NewSubjectService(NewSubjectRepository(pool))
	// Subject mutations and events must commit together.
	created, err := service.CreateSubject(ctx, adminUser, "subject-repo-a", CreateSubjectRequest{Code: " math ", Name: " Mathematics "}, "create-test")
	if err != nil {
		t.Fatal(err)
	}
	if created.Code != "MATH" || created.Name != "Mathematics" {
		t.Fatalf("unexpected normalized subject: %+v", created)
	}
	auditCount := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND subject_type='subject'`, tenantA).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := auditCount(); got != 1 {
		t.Fatalf("creation audit count = %d, want 1", got)
	}
	t.Run("owner and teacher cannot write", func(t *testing.T) {
		for _, actor := range []uuid.UUID{owner, teacher} {
			if _, err := service.CreateSubject(ctx, actor, "subject-repo-a", CreateSubjectRequest{Code: "SCI", Name: "Science"}, "denied"); !errors.Is(err, ErrForbidden) {
				t.Fatalf("expected forbidden for %s, got %v", actor, err)
			}
			if _, err := service.ArchiveSubject(ctx, actor, "subject-repo-a", created.ID, "denied"); !errors.Is(err, ErrForbidden) {
				t.Fatalf("archive: want forbidden, got %v", err)
			}
		}
		if got := auditCount(); got != 1 {
			t.Fatalf("denied operations changed audit count to %d", got)
		}
	})
	t.Run("read tenant isolation", func(t *testing.T) {
		for _, actor := range []uuid.UUID{owner, teacher, principal} {
			items, err := service.ListSubjects(ctx, actor, "subject-repo-a")
			if err != nil || len(items) != 1 || items[0].ID != created.ID {
				t.Fatalf("authorized listing: %v %+v", err, items)
			}
		}
		if _, err := service.ListSubjects(ctx, outsider, "subject-repo-a"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("outsider read: %v", err)
		}
		if _, err := service.CreateSubject(ctx, outsider, "subject-repo-a", CreateSubjectRequest{Code: "SCI", Name: "Science"}, "cross-tenant"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("outsider write: %v", err)
		}
	})
	t.Run("principal update and audit", func(t *testing.T) {
		name := "Core Mathematics"
		got, err := service.UpdateSubject(ctx, principal, "subject-repo-a", created.ID, UpdateSubjectRequest{Name: &name}, "update-test")
		if err != nil || got.Name != name {
			t.Fatalf("principal update: %v %+v", err, got)
		}
		if n := auditCount(); n != 2 {
			t.Fatalf("audit count %d, want 2", n)
		}
	})
	t.Run("multi-role administrator can write", func(t *testing.T) {
		got, err := service.CreateSubject(ctx, multiRole, "subject-repo-a", CreateSubjectRequest{Code: "ENG", Name: "English"}, "multi-role")
		if err != nil || got.Code != "ENG" {
			t.Fatalf("multi-role create: %v %+v", err, got)
		}
		if n := auditCount(); n != 3 {
			t.Fatalf("audit count %d, want 3", n)
		}
	})
	t.Run("duplicate rollback leaves no audit", func(t *testing.T) {
		if _, err := service.CreateSubject(ctx, adminUser, "subject-repo-a", CreateSubjectRequest{Code: "MATH", Name: "Duplicate"}, "duplicate"); !errors.Is(err, ErrConflict) {
			t.Fatalf("duplicate: want conflict, got %v", err)
		}
		if n := auditCount(); n != 3 {
			t.Fatalf("duplicate changed audit count to %d", n)
		}
	})
	t.Run("suspended administrator cannot write", func(t *testing.T) {
		must(`UPDATE memberships SET status='SUSPENDED' WHERE tenant_id=$1 AND user_id=$2 AND role='SCHOOL_ADMIN'`, tenantA, adminUser)
		if _, err := service.CreateSubject(ctx, adminUser, "subject-repo-a", CreateSubjectRequest{Code: "HIS", Name: "History"}, "suspended"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("suspended: want not found, got %v", err)
		}
		if n := auditCount(); n != 3 {
			t.Fatalf("suspension changed audit count to %d", n)
		}
	})
	t.Run("archive and historical immutability", func(t *testing.T) {
		got, err := service.ArchiveSubject(ctx, principal, "subject-repo-a", created.ID, "archive-test")
		if err != nil || got.Status != "ARCHIVED" {
			t.Fatalf("archive: %v %+v", err, got)
		}
		if n := auditCount(); n != 4 {
			t.Fatalf("archive audit count %d, want 4", n)
		}
		name := "Changed Again"
		if _, err := service.UpdateSubject(ctx, principal, "subject-repo-a", created.ID, UpdateSubjectRequest{Name: &name}, "no-update"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("archived update: %v", err)
		}
		if n := auditCount(); n != 4 {
			t.Fatalf("archived update changed audit count to %d", n)
		}
	})
}
