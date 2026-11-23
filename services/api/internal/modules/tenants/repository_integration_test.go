package tenants

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/modules/auth"
)

func TestPostgresTenantIsolationAndSettings(t *testing.T) {
	databaseURL := requireTenantIntegrationDatabaseURL(t)
	if databaseURL == "" {
		t.Skip("set SWW_TEST_DATABASE_URL to an isolated PostgreSQL database to run tenant integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal("could not connect to isolated PostgreSQL test database")
	}
	defer adminPool.Close()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("SWW_TEST_DATABASE_URL is not a valid PostgreSQL URI")
	}
	if _, err := adminPool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS citext`); err != nil {
		t.Fatalf("enable citext in isolated test database: %v", err)
	}
	if _, err := adminPool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pgcrypto`); err != nil {
		t.Fatalf("enable pgcrypto in isolated test database: %v", err)
	}
	schema := "sww_tenants_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create isolated tenant schema: %v", err)
	}
	defer func() {
		if _, err := adminPool.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("drop isolated tenant schema: %v", err)
		}
	}()
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect using isolated schema: %v", err)
	}
	defer pool.Close()
	for _, name := range []string{
		"000001_initial_schema.up.sql",
		"000002_auth_identity.up.sql",
		"000003_auth_hardening.up.sql",
		"000004_tenants.up.sql",
		"000024_school_operating_mode.up.sql",
	} {
		applyTenantMigrationForTest(t, ctx, pool, filepath.Join("..", "..", "..", "migrations", name))
	}

	adminID, ownerAID, ownerBID, tenantAID, tenantBID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, identity := range []struct {
		id    uuid.UUID
		email string
	}{
		{adminID, "platform@school.invalid"},
		{ownerAID, "owner-a@school.invalid"},
		{ownerBID, "owner-b@school.invalid"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO users (id, email, password_hash, full_name, status)
			VALUES ($1, $2, 'fictional-test-hash', 'Fictional Test User', 'ACTIVE')
		`, identity.id, identity.email); err != nil {
			t.Fatalf("insert fictional user: %v", err)
		}
	}

	if _, err := pool.Exec(ctx, `INSERT INTO platform_admins (user_id) VALUES ($1)`, adminID); err != nil {
		t.Fatalf("grant test platform administrator: %v", err)
	}
	for _, tenant := range []struct {
		id   uuid.UUID
		slug string
	}{
		{tenantAID, "school-a"},
		{tenantBID, "school-b"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO tenants (id, slug, name, status, locale, timezone)
			VALUES ($1, $2::citext, $2::text, 'ACTIVE', 'en-NG', 'Africa/Lagos')
		`, tenant.id, tenant.slug); err != nil {
			t.Fatalf("insert independent tenant: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO memberships (id, tenant_id, user_id, role, status)
		VALUES ($1, $2, $3, 'SCHOOL_OWNER', 'ACTIVE'),
			($4, $5, $6, 'SCHOOL_OWNER', 'ACTIVE')
	`, uuid.New(), tenantAID, ownerAID, uuid.New(), tenantBID, ownerBID); err != nil {
		t.Fatalf("insert isolated tenant memberships: %v", err)
	}

	repository := NewPostgresRepository(pool)
	// database-backed member school isolation
	schoolAList, err := repository.ListForMember(ctx, ownerAID)
	if err != nil {
		t.Fatalf("list school A memberships: %v", err)
	}
	if len(schoolAList) != 1 ||
		schoolAList[0].Slug != "school-a" ||
		schoolAList[0].CurrentRole != "SCHOOL_OWNER" {
		t.Fatalf("owner A received unexpected schools: %+v", schoolAList)
	}

	schoolBList, err := repository.ListForMember(ctx, ownerBID)
	if err != nil {
		t.Fatalf("list school B memberships: %v", err)
	}
	if len(schoolBList) != 1 || schoolBList[0].Slug != "school-b" {
		t.Fatalf("owner B received unexpected schools: %+v", schoolBList)
	}

	noMemberships, err := repository.ListForMember(ctx, adminID)
	if err != nil {
		t.Fatalf("list schools for user without membership: %v", err)
	}
	if len(noMemberships) != 0 {
		t.Fatalf("unrelated user received schools: %+v", noMemberships)
	}
	// database-backed suspended membership isolation
	if _, err := pool.Exec(ctx, `
        UPDATE memberships
        SET status = 'SUSPENDED'
        WHERE tenant_id = $1 AND user_id = $2
    `, tenantAID, ownerAID); err != nil {
		t.Fatalf("suspend test membership: %v", err)
	}

	suspendedMembershipSchools, err := repository.ListForMember(ctx, ownerAID)
	if err != nil || len(suspendedMembershipSchools) != 0 {
		t.Fatalf("suspended membership disclosed schools: %+v err=%v",
			suspendedMembershipSchools, err)
	}

	if _, err := pool.Exec(ctx, `
        UPDATE memberships
        SET status = 'ACTIVE'
        WHERE tenant_id = $1 AND user_id = $2
    `, tenantAID, ownerAID); err != nil {
		t.Fatalf("restore test membership: %v", err)
	}

	// Database-backed suspended and archived school isolation.
	for _, status := range []string{"SUSPENDED", "ARCHIVED"} {
		if _, err := pool.Exec(ctx, `
            UPDATE tenants
            SET status = $1::tenant_status
            WHERE id = $2
        `, status, tenantAID); err != nil {
			t.Fatalf("set test school status %s: %v", status, err)
		}

		hiddenSchools, err := repository.ListForMember(ctx, ownerAID)
		if err != nil || len(hiddenSchools) != 0 {
			t.Fatalf("school with status %s was disclosed: %+v err=%v",
				status, hiddenSchools, err)
		}
	}

	if _, err := pool.Exec(ctx, `
        UPDATE tenants
        SET status = 'ACTIVE'
        WHERE id = $1
    `, tenantAID); err != nil {
		t.Fatalf("restore test school status: %v", err)
	}

	restoredSchools, err := repository.ListForMember(ctx, ownerAID)
	if err != nil || len(restoredSchools) != 1 ||
		restoredSchools[0].Slug != "school-a" {
		t.Fatalf("restored school membership not visible: %+v err=%v",
			restoredSchools, err)
	}
	if allowed, err := repository.IsPlatformAdmin(ctx, adminID); err != nil || !allowed {
		t.Fatalf("database-backed platform admin should be allowed, allowed=%v err=%v", allowed, err)
	}
	if allowed, err := repository.IsPlatformAdmin(ctx, ownerAID); err != nil || allowed {
		t.Fatalf("school owner must not be a platform admin, allowed=%v err=%v", allowed, err)
	}
	if _, err := repository.GetForMember(ctx, "school-b", ownerAID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant member lookup must not disclose a school, got %v", err)
	}
	_, err = repository.UpdateSettings(ctx, "school-b", ownerAID, &SettingsRequest{
		Name: "Unauthorized", Locale: "en-NG", Timezone: "Africa/Lagos",
		Branding: map[string]any{}, EducationStages: []string{"PRIMARY"},
	}, &auth.AuditEvent{
		ActorID: &ownerAID, Action: "tenant.settings_updated", SubjectType: "tenant",
		RequestID: "cross-tenant-test", MetadataRedacted: map[string]any{},
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-tenant settings update must be denied, got %v", err)
	}
	updated, err := repository.UpdateSettings(ctx, "school-a", ownerAID, &SettingsRequest{
		Name: "School A Updated", Locale: "en-NG", Timezone: "Africa/Lagos",
		Branding: map[string]any{}, EducationStages: []string{"PRIMARY"},
	}, &auth.AuditEvent{
		ActorID: &ownerAID, Action: "tenant.settings_updated", SubjectType: "tenant",
		RequestID: "settings-test", MetadataRedacted: map[string]any{},
	})
	if err != nil {
		t.Fatalf("active school owner should update its settings: %v", err)
	}
	if updated.Name != "School A Updated" || updated.CurrentRole != "SCHOOL_OWNER" {
		t.Fatalf("unexpected updated tenant: %+v", updated)
	}
	tenantB, err := repository.GetForMember(ctx, "school-b", ownerBID)
	if err != nil {
		t.Fatalf("tenant B owner should remain scoped to tenant B: %v", err)
	}
	if tenantB.Name != "school-b" {
		t.Fatalf("updating tenant A must not change tenant B settings, got %q", tenantB.Name)
	}

	duplicate := &Tenant{
		ID: uuid.New(), Slug: "school-a", Name: "Duplicate School", Locale: "en-NG",
		Timezone: "Africa/Lagos", EducationStages: []string{"PRIMARY"}, CreatedAt: time.Now().UTC(),
	}
	if err := repository.Provision(ctx, duplicate, &auth.Invitation{
		ID: uuid.New(), TenantID: &duplicate.ID, InvitedEmail: "duplicate@school.invalid",
		TargetRole: "SCHOOL_OWNER", TokenHash: auth.HashToken("fictional-test-token"),
		Status: auth.InvitationStatusPending, ExpiresAt: time.Now().Add(time.Hour),
	}, &auth.AuditEvent{
		ActorID: &adminID, Action: "tenant.provisioned", SubjectType: "tenant",
		RequestID: "duplicate-test", MetadataRedacted: map[string]any{},
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate slug should be rejected, got %v", err)
	}
}

func requireTenantIntegrationDatabaseURL(t *testing.T) string {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("SWW_TEST_DATABASE_URL"))
	if databaseURL == "" {
		if os.Getenv("SWW_REQUIRE_POSTGRES_TESTS") == "1" {
			t.Fatal("SWW_TEST_DATABASE_URL is required when PostgreSQL integration tests are mandatory")
		}
		return ""
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("SWW_TEST_DATABASE_URL is not a valid PostgreSQL URI")
	}
	databaseName := strings.ToLower(config.ConnConfig.Database)
	testSegment := regexp.MustCompile(`(^|[^a-z0-9])test([^a-z0-9]|$)`)
	unsafeSegment := regexp.MustCompile(`(^|[^a-z0-9])(dev|prod|production)([^a-z0-9]|$)`)
	if !testSegment.MatchString(databaseName) || unsafeSegment.MatchString(databaseName) {
		t.Fatal("tenant integration tests require a test-only database name")
	}
	return databaseURL
}

func applyTenantMigrationForTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool, path string) {
	t.Helper()
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", path, err)
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire migration connection: %v", err)
	}
	defer connection.Release()
	results, err := connection.Conn().PgConn().Exec(ctx, string(sql)).ReadAll()
	if err != nil {
		t.Fatalf("execute migration %s: %v", path, err)
	}
	for _, result := range results {
		if result.Err != nil {
			t.Fatalf("execute migration %s: %v", path, result.Err)
		}
	}
}
