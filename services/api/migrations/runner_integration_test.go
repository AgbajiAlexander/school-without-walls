package migrations

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	testDatabaseName = regexp.MustCompile(`(^|[^a-z0-9])test([^a-z0-9]|$)`)
	devDatabaseName  = regexp.MustCompile(`(^|[^a-z0-9])dev([^a-z0-9]|$)`)
	prodDatabaseName = regexp.MustCompile(`(^|[^a-z0-9])(prod|production)([^a-z0-9]|$)`)
)

func TestMigrationRunnerLedgerAndFailureRollback(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("SWW_TEST_DATABASE_URL"))
	if databaseURL == "" {
		requirePostgresTests := os.Getenv("SWW_REQUIRE_POSTGRES_TESTS") == "1"
		if requirePostgresTests {
			t.Fatal("SWW_TEST_DATABASE_URL is required when PostgreSQL integration tests are mandatory")
		}
		t.Skip("set SWW_TEST_DATABASE_URL to an isolated disposable PostgreSQL test database")
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("SWW_TEST_DATABASE_URL is not a valid PostgreSQL URI")
	}
	databaseName := strings.ToLower(poolConfig.ConnConfig.Database)
	if !testDatabaseName.MatchString(databaseName) ||
		devDatabaseName.MatchString(databaseName) ||
		prodDatabaseName.MatchString(databaseName) {
		t.Fatal("migration integration tests require a dedicated database name containing a test segment and no dev/prod segment")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	adminPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal("could not connect to isolated migration test database")
	}
	defer adminPool.Close()

	schema := "sww_migration_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal("could not create isolated migration test schema")
	}
	defer func() {
		if _, err := adminPool.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("could not remove isolated migration test schema: %v", err)
		}
	}()

	if poolConfig.ConnConfig.RuntimeParams == nil {
		poolConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal("could not connect to isolated migration test schema")
	}
	defer pool.Close()

	secondPoolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("SWW_TEST_DATABASE_URL is not a valid PostgreSQL URI")
	}
	if secondPoolConfig.ConnConfig.RuntimeParams == nil {
		secondPoolConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	secondPoolConfig.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	secondPool, err := pgxpool.NewWithConfig(ctx, secondPoolConfig)
	if err != nil {
		t.Fatal("could not create second migration runner connection")
	}
	defer secondPool.Close()

	// Create the ledger in the temporary schema before invoking Up.
	// Otherwise search_path may resolve the existing public ledger.
	if _, err := adminPool.Exec(ctx, `CREATE TABLE `+schema+`.schema_migrations (
        version VARCHAR(64) PRIMARY KEY,
        applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
    )`); err != nil {
		t.Fatalf("create isolated migration ledger: %v", err)
	}

	runnerErrors := make(chan error, 2)
	go func() { runnerErrors <- Up(ctx, pool) }()
	go func() { runnerErrors <- Up(ctx, secondPool) }()
	var concurrentRunErrors []error
	for range 2 {
		if err := <-runnerErrors; err != nil {
			concurrentRunErrors = append(concurrentRunErrors, err)
		}
	}
	if len(concurrentRunErrors) != 0 {
		t.Fatalf("concurrent migration runs failed: %v", concurrentRunErrors)
	}

	if err := Up(ctx, pool); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	t.Run("people_enrollment_constraints", func(t *testing.T) {
		testPeopleEnrollmentConstraints(t, ctx, pool)
	})

	t.Run("profile_invitation_redemption", func(t *testing.T) {
		testProfileInvitationRedemption(t, ctx, pool)
	})

	t.Run("profile_invitation_issuance", func(t *testing.T) {
		testProfileInvitationIssuance(t, ctx, pool)
	})

	t.Run("curriculum_registry", func(t *testing.T) {
		testCurriculumRegistry(t, ctx, pool)
	})

	statuses, err := ReadStatus(ctx, pool)
	if err != nil {
		t.Fatalf("read migration status: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatal("expected embedded migrations in migration status")
	}
	for _, status := range statuses {
		if !status.Applied {
			t.Errorf("migration %s should be recorded as applied", status.Version)
		}
	}
	var appliedCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&appliedCount); err != nil {
		t.Fatal("could not count applied migration versions")
	}
	if appliedCount != len(statuses) {
		t.Fatalf("expected one ledger entry for each migration: entries=%d migrations=%d", appliedCount, len(statuses))
	}

	concurrentMigration := Migration{
		Version: "999998_concurrent_test",
		SQL: `CREATE TABLE concurrent_migration_marker (id INTEGER);
			SELECT pg_sleep(0.25);`,
	}
	startRunners := make(chan struct{})
	runnerErrors = make(chan error, 2)
	go func() {
		<-startRunners
		runnerErrors <- Apply(ctx, pool, []Migration{concurrentMigration})
	}()
	go func() {
		<-startRunners
		runnerErrors <- Apply(ctx, secondPool, []Migration{concurrentMigration})
	}()
	close(startRunners)
	concurrentRunErrors = nil
	for range 2 {
		if err := <-runnerErrors; err != nil {
			concurrentRunErrors = append(concurrentRunErrors, err)
		}
	}
	if len(concurrentRunErrors) != 0 {
		t.Fatalf("concurrent same-version migration runs failed: %v", concurrentRunErrors)
	}
	var concurrentVersionCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = $1`,
		concurrentMigration.Version).Scan(&concurrentVersionCount); err != nil {
		t.Fatal("could not verify concurrent migration ledger entry")
	}
	if concurrentVersionCount != 1 {
		t.Fatalf("concurrent migration should be recorded once, entries=%d", concurrentVersionCount)
	}

	failedMigration := Migration{
		Version: "999999_test_failure",
		SQL: `CREATE TABLE should_rollback_on_failure (id INTEGER);
			SELECT 1 / 0;`,
	}
	if err := Apply(ctx, pool, []Migration{failedMigration}); err == nil {
		t.Fatal("expected failed migration to stop execution")
	}
	var tableExists, migrationRecorded bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('should_rollback_on_failure') IS NOT NULL`).Scan(&tableExists); err != nil {
		t.Fatal("could not verify failed migration rollback")
	}
	if err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM schema_migrations WHERE version = $1
	)`, failedMigration.Version).Scan(&migrationRecorded); err != nil {
		t.Fatal("could not verify failed migration was not recorded")
	}
	if tableExists || migrationRecorded {
		t.Fatal("failed migration must roll back its schema changes and ledger entry")
	}
}
