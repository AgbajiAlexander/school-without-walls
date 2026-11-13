package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresAuthLifecycle(t *testing.T) {
	databaseURL := os.Getenv("SWW_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set SWW_TEST_DATABASE_URL to an isolated PostgreSQL database to run integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to isolated test database: %v", err)
	}
	defer adminPool.Close()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(config.ConnConfig.Database), "test") {
		t.Skip("refusing to create an auth integration schema outside a database whose name contains 'test'")
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

	applyMigrationForTest(t, ctx, pool, filepath.Join("..", "..", "..", "migrations", "000003_auth_hardening.down.sql"))
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
