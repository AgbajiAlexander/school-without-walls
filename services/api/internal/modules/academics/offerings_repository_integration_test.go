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

// TestPostgresSubjectOfferingRepository runs only against an allowlisted test database.
// Each run creates and drops its own schema. Never run it against sww_dev.
func TestPostgresSubjectOfferingRepository(t *testing.T) {
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
		t.Fatalf("refusing offering repository test on %q", cfg.ConnConfig.Database)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "sww_offering_repo_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
		must(`INSERT INTO users (id,email,password_hash,full_name,status) VALUES ($1,$2,'fictional-hash','Fictional Offering User','ACTIVE')`, id, "offering-repo-"+string(rune('a'+i))+"@school.invalid")
	}
	must(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'offering-repo-a','School A','ACTIVE')`, tenantA)
	must(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'offering-repo-b','School B','ACTIVE')`, tenantB)
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

	service := NewSubjectOfferingService(NewSubjectOfferingRepository(pool))
	req := CreateSubjectOfferingRequest{SubjectID: subjectA, TermID: termA, ClassID: classA}
	countAudit := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND subject_type='subject_offering'`, tenantA).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	created, err := service.CreateSubjectOffering(ctx, adminID, "offering-repo-a", req, "create")
	if err != nil || created == nil || created.Status != "ACTIVE" || created.ClassArmID != nil {
		t.Fatalf("create: %v %+v", err, created)
	}
	if countAudit() != 1 {
		t.Fatalf("expected one creation audit event, got %d", countAudit())
	}

	t.Run("owner and teacher cannot mutate", func(t *testing.T) {
		before := countAudit()
		for _, actor := range []uuid.UUID{ownerID, teacherID} {
			if _, err := service.CreateSubjectOffering(ctx, actor, "offering-repo-a", req, "denied"); !errors.Is(err, ErrForbidden) {
				t.Fatalf("create actor %s: %v", actor, err)
			}
			if _, err := service.ArchiveSubjectOffering(ctx, actor, "offering-repo-a", created.ID, "denied"); !errors.Is(err, ErrForbidden) {
				t.Fatalf("archive actor %s: %v", actor, err)
			}
		}
		if countAudit() != before {
			t.Fatal("denied writes added audit events")
		}
	})
	t.Run("tenant isolated listings and writes", func(t *testing.T) {
		for _, actor := range []uuid.UUID{ownerID, teacherID, principalID} {
			items, err := service.ListSubjectOfferings(ctx, actor, "offering-repo-a")
			if err != nil || len(items) != 1 || items[0].ID != created.ID {
				t.Fatalf("listing actor %s: %v %+v", actor, err, items)
			}
		}
		if _, err := service.ListSubjectOfferings(ctx, outsiderID, "offering-repo-a"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross tenant list: %v", err)
		}
		if _, err := service.ArchiveSubjectOffering(ctx, outsiderID, "offering-repo-a", created.ID, "outsider"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross tenant archive: %v", err)
		}
	})
	t.Run("duplicate classwide offering rolls back audit", func(t *testing.T) {
		before := countAudit()
		if _, err := service.CreateSubjectOffering(ctx, principalID, "offering-repo-a", req, "duplicate"); !errors.Is(err, ErrConflict) {
			t.Fatalf("duplicate: %v", err)
		}
		if countAudit() != before {
			t.Fatal("duplicate added audit event")
		}
	})
	t.Run("invalid parents and scopes rejected", func(t *testing.T) {
		before := countAudit()
		for _, tc := range []struct {
			name    string
			request CreateSubjectOfferingRequest
		}{
			{"cross-school subject", CreateSubjectOfferingRequest{SubjectID: subjectB, TermID: termA, ClassID: classA}},
			{"cross-school class", CreateSubjectOfferingRequest{SubjectID: subjectA, TermID: termA, ClassID: classB}},
			{"term session mismatch", CreateSubjectOfferingRequest{SubjectID: subjectA, TermID: termA2, ClassID: classA}},
			{"cross-school arm", CreateSubjectOfferingRequest{SubjectID: subjectA, TermID: termA, ClassID: classA, ClassArmID: &armB}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if _, err := service.CreateSubjectOffering(ctx, principalID, "offering-repo-a", tc.request, "invalid"); !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("want invalid input, got %v", err)
				}
			})
		}
		if countAudit() != before {
			t.Fatal("invalid parent added audit event")
		}
	})
	t.Run("arm specific offering and archive", func(t *testing.T) {
		armReq := CreateSubjectOfferingRequest{SubjectID: subjectA, TermID: termA, ClassID: classA, ClassArmID: &armA}
		armOffering, err := service.CreateSubjectOffering(ctx, principalID, "offering-repo-a", armReq, "arm-create")
		if err != nil || armOffering == nil || armOffering.ClassArmID == nil || *armOffering.ClassArmID != armA {
			t.Fatalf("arm offering: %v %+v", err, armOffering)
		}
		if countAudit() != 2 {
			t.Fatalf("expected two audits, got %d", countAudit())
		}
		archived, err := service.ArchiveSubjectOffering(ctx, principalID, "offering-repo-a", armOffering.ID, "arm-archive")
		if err != nil || archived == nil || archived.Status != "ARCHIVED" {
			t.Fatalf("archive: %v %+v", err, archived)
		}
		if countAudit() != 3 {
			t.Fatalf("expected three audits, got %d", countAudit())
		}
		if _, err := service.ArchiveSubjectOffering(ctx, principalID, "offering-repo-a", armOffering.ID, "archive-twice"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("repeat archive: %v", err)
		}
		if countAudit() != 3 {
			t.Fatal("repeat archive added audit event")
		}
	})
	t.Run("suspended administrator cannot write", func(t *testing.T) {
		must(`UPDATE memberships SET status='SUSPENDED' WHERE tenant_id=$1 AND user_id=$2 AND role='SCHOOL_ADMIN'`, tenantA, adminID)
		if _, err := service.ArchiveSubjectOffering(ctx, adminID, "offering-repo-a", created.ID, "suspended"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("suspended admin: %v", err)
		}
		if countAudit() != 3 {
			t.Fatal("suspended admin added audit event")
		}
	})
}
