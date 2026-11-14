package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresAuthLifecycle(t *testing.T) {
	databaseURL := requireIntegrationDatabaseURL(t)
	if databaseURL == "" {
		t.Skip("set SWW_TEST_DATABASE_URL to an isolated PostgreSQL database to run integration tests")
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
	schema := "sww_auth_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create isolated auth schema: %v", err)
	}
	defer func() {
		if _, err := adminPool.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("drop isolated auth schema: %v", err)
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
	} {
		applyMigrationForTest(t, ctx, pool, filepath.Join("..", "..", "..", "migrations", name))
	}

	repo := NewPostgresRepository(pool)
	service := NewService(repo)

	var bootstrapWG sync.WaitGroup
	bootstrapResults := make(chan error, 2)
	for i := 0; i < 2; i++ {
		bootstrapWG.Add(1)
		go func(i int) {
			defer bootstrapWG.Done()
			_, err := service.BootstrapAdmin(ctx, "admin"+string(rune('a'+i))+"@school.invalid",
				"Test Operator", "a sufficiently long test passphrase", "bootstrap-test")
			bootstrapResults <- err
		}(i)
	}
	bootstrapWG.Wait()
	close(bootstrapResults)
	successes, denied := 0, 0
	for err := range bootstrapResults {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAdminAlreadyExists):
			denied++
		default:
			t.Fatalf("unexpected bootstrap result: %v", err)
		}
	}

	if successes != 1 || denied != 1 {
		t.Fatalf("expected one successful bootstrap and one denial, got successes=%d denied=%d", successes, denied)
	}
	var adminGrantCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_admins`).Scan(&adminGrantCount); err != nil || adminGrantCount != 1 {
		t.Fatalf("expected one platform-admin grant, count=%d err=%v", adminGrantCount, err)
	}

	rawInvite, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateInvitation(ctx, &Invitation{
		InvitedEmail: "invitee@school.invalid", TargetRole: "TEACHER",
		TokenHash: HashToken(rawInvite), Status: InvitationStatusPending,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	var acceptWG sync.WaitGroup
	acceptResults := make(chan error, 2)
	for i := 0; i < 2; i++ {
		acceptWG.Add(1)
		go func() {
			defer acceptWG.Done()
			_, _, _, err := service.AcceptInvitation(ctx, rawInvite, "Fictional Test User",
				"a different long test passphrase", "127.0.0.1", "integration-test", "invite-test")
			acceptResults <- err
		}()
	}
	acceptWG.Wait()
	close(acceptResults)
	accepted, replayed := 0, 0
	for err := range acceptResults {
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, ErrInvitationNotFound):
			replayed++
		default:
			t.Fatalf("unexpected invitation acceptance result: %v", err)
		}
	}
	if accepted != 1 || replayed != 1 {
		t.Fatalf("expected one acceptance and one replay rejection, got accepted=%d replayed=%d", accepted, replayed)
	}
	invitee, err := repo.GetUserByEmail(ctx, "invitee@school.invalid")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := repo.ListActiveSessions(ctx, invitee.ID)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("expected one active invitation session, sessions=%d err=%v", len(sessions), err)
	}
	var rawTokenCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM invitations WHERE token_hash = $1`, rawInvite).Scan(&rawTokenCount); err != nil || rawTokenCount != 0 {
		t.Fatalf("raw invitation token must not be stored, count=%d err=%v", rawTokenCount, err)
	}

	tenantID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status, locale, timezone)
		VALUES ($1, 'integration-school', 'Integration School', 'PENDING_SETUP', 'en-NG', 'Africa/Lagos')
	`, tenantID); err != nil {
		t.Fatalf("create tenant for invitation integration test: %v", err)
	}
	rawOwnerInvite, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateInvitation(ctx, &Invitation{
		TenantID: &tenantID, InvitedEmail: "owner@school.invalid", TargetRole: "SCHOOL_OWNER",
		TokenHash: HashToken(rawOwnerInvite), Status: InvitationStatusPending,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create tenant owner invitation: %v", err)
	}
	ownerInvitation, err := repo.GetInvitationByTokenHash(ctx, HashToken(rawOwnerInvite))
	if err != nil || ownerInvitation.TenantSlug != "integration-school" || ownerInvitation.TenantName != "Integration School" {
		t.Fatalf("tenant invitation verification should return the tenant identity, invitation=%+v err=%v", ownerInvitation, err)
	}
	owner, _, _, err := service.AcceptInvitation(ctx, rawOwnerInvite, "Fictional School Owner",
		"a separate fictional owner test passphrase", "127.0.0.1", "integration-test", "owner-invite-test")
	if err != nil {
		t.Fatalf("accept tenant owner invitation: %v", err)
	}
	var ownerRole, tenantStatus string
	if err := pool.QueryRow(ctx, `
		SELECT m.role, t.status::text
		FROM memberships m JOIN tenants t ON t.id = m.tenant_id
		WHERE m.user_id = $1 AND t.id = $2
	`, owner.ID, tenantID).Scan(&ownerRole, &tenantStatus); err != nil {
		t.Fatalf("verify tenant owner membership: %v", err)
	}
	if ownerRole != "SCHOOL_OWNER" || tenantStatus != "ACTIVE" {
		t.Fatalf("expected owner membership to activate tenant, role=%s status=%s", ownerRole, tenantStatus)
	}
	existingPassword := "fictional existing account passphrase"
	existingPasswordHash, err := HashPassword(existingPassword)
	if err != nil {
		t.Fatal(err)
	}
	existingAccount := &User{
		ID: uuid.New(), Email: "existing-admin@school.invalid",
		PasswordHash: existingPasswordHash, FullName: "Fictional Existing Admin", Status: UserStatusActive,
	}
	if err := repo.CreateUser(ctx, existingAccount); err != nil {
		t.Fatalf("create existing tenant administrator: %v", err)
	}
	rawExistingInvite, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateInvitation(ctx, &Invitation{
		TenantID: &tenantID, InvitedEmail: existingAccount.Email, TargetRole: "SCHOOL_ADMIN",
		TokenHash: HashToken(rawExistingInvite), Status: InvitationStatusPending,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create invitation for existing account: %v", err)
	}
	if _, _, _, err := service.AcceptInvitation(ctx, rawExistingInvite, "Ignored Proposed Name",
		"wrong existing account password", "127.0.0.1", "integration-test", "wrong-existing-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("existing account invitation must reject wrong password: %v", err)
	}
	acceptedExisting, _, _, err := service.AcceptInvitation(ctx, rawExistingInvite, "Ignored Proposed Name",
		existingPassword, "127.0.0.1", "integration-test", "existing-account-accept")
	if err != nil {
		t.Fatalf("existing active account should accept invitation: %v", err)
	}
	if acceptedExisting.ID != existingAccount.ID || acceptedExisting.FullName != existingAccount.FullName ||
		acceptedExisting.PasswordHash != existingPasswordHash {
		t.Fatal("existing account invitation acceptance must preserve global identity credentials")
	}

	token, err := service.RequestPasswordReset(ctx, "invitee@school.invalid", "reset-test")
	if err != nil || token == "" {
		t.Fatalf("expected reset token for active test account, token=%q err=%v", token, err)
	}
	var rawResetCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM password_resets WHERE token_hash = $1`, token).Scan(&rawResetCount); err != nil || rawResetCount != 0 {
		t.Fatalf("raw reset token must not be stored, count=%d err=%v", rawResetCount, err)
	}
	if err := service.ResetPassword(ctx, token, "updated test passphrase for account", "reset-complete"); err != nil {
		t.Fatalf("complete password reset: %v", err)
	}
	if err := service.ResetPassword(ctx, token, "another updated test passphrase", "reset-replay"); !errors.Is(err, ErrPasswordResetNotFound) {
		t.Fatalf("expected reset replay rejection, got %v", err)
	}
	sessions, err = repo.ListActiveSessions(ctx, invitee.ID)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("password reset should revoke all active sessions, sessions=%d err=%v", len(sessions), err)
	}

	if retry, err := repo.AllowRateLimit(ctx, "integration", HashToken("test-key"), 1, time.Minute); err != nil || retry != 0 {
		t.Fatalf("first rate-limit attempt should pass, retry=%s err=%v", retry, err)
	}
	if retry, err := repo.AllowRateLimit(ctx, "integration", HashToken("test-key"), 1, time.Minute); err != nil || retry <= 0 {
		t.Fatalf("second rate-limit attempt should be blocked, retry=%s err=%v", retry, err)
	}

	applyMigrationForTest(t, ctx, pool, filepath.Join("..", "..", "..", "migrations", "000004_tenants.down.sql"))
	applyMigrationForTest(t, ctx, pool, filepath.Join("..", "..", "..", "migrations", "000003_auth_hardening.down.sql"))
}

func requireIntegrationDatabaseURL(t *testing.T) string {
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
		t.Fatal("authentication integration tests require a test-only database name")
	}
	return databaseURL
}

func applyMigrationForTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool, path string) {
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
