package academics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/modules/auth"
)

func TestPostgresAcademicSessionIsolationConstraintsAndLifecycle(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("SWW_TEST_DATABASE_URL"))
	if databaseURL == "" {
		if os.Getenv("SWW_REQUIRE_POSTGRES_TESTS") == "1" {
			t.Fatal("SWW_TEST_DATABASE_URL is required when PostgreSQL integration tests are mandatory")
		}
		t.Skip("set SWW_TEST_DATABASE_URL to an isolated disposable PostgreSQL test database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	schema := "sww_academics_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create isolated academic test schema: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect using isolated academic test schema: %v", err)
	}
	defer func() {
		if _, err := adminPool.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("drop isolated academic test schema: %v", err)
		}
	}()
	defer pool.Close()

	for _, name := range []string{
		"000001_initial_schema.up.sql",
		"000002_auth_identity.up.sql",
		"000003_auth_hardening.up.sql",
		"000004_tenants.up.sql",
		"000005_academics.up.sql",
	} {
		applyAcademicMigrationForTest(t, ctx, pool, filepath.Join("..", "..", "..", "migrations", name))
	}

	ownerA, adminA, principalA, teacherA, ownerB, platformAdmin := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	tenantA, tenantB := uuid.New(), uuid.New()
	for id, email := range map[uuid.UUID]string{
		ownerA: "owner-a@school.invalid", adminA: "admin-a@school.invalid",
		principalA: "principal-a@school.invalid", teacherA: "teacher-a@school.invalid",
		ownerB: "owner-b@school.invalid", platformAdmin: "platform@school.invalid",
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO users (id, email, password_hash, full_name, status)
			VALUES ($1, $2, 'fictional-test-hash', 'Fictional Test User', 'ACTIVE')
		`, id, email); err != nil {
			t.Fatalf("insert fictional academic test identity: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO platform_admins (user_id) VALUES ($1)`, platformAdmin); err != nil {
		t.Fatalf("insert fictional platform admin: %v", err)
	}
	for _, tenant := range []struct {
		id   uuid.UUID
		slug string
	}{
		{tenantA, "academic-school-a"},
		{tenantB, "academic-school-b"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO tenants (id, slug, name, status, locale, timezone)
			VALUES ($1, $2::citext, $2::text, 'ACTIVE', 'en-NG', 'Africa/Lagos')
		`, tenant.id, tenant.slug); err != nil {
			t.Fatalf("insert fictional tenant: %v", err)
		}
	}
	for _, membership := range []struct {
		id       uuid.UUID
		tenantID uuid.UUID
		userID   uuid.UUID
		role     string
	}{
		{uuid.New(), tenantA, ownerA, "SCHOOL_OWNER"},
		{uuid.New(), tenantA, adminA, "SCHOOL_ADMIN"},
		{uuid.New(), tenantA, principalA, "PRINCIPAL"},
		{uuid.New(), tenantA, teacherA, "TEACHER"},
		{uuid.New(), tenantB, ownerB, "SCHOOL_OWNER"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO memberships (id, tenant_id, user_id, role, status)
			VALUES ($1, $2, $3, $4, 'ACTIVE')
		`, membership.id, membership.tenantID, membership.userID, membership.role); err != nil {
			t.Fatalf("insert fictional school membership: %v", err)
		}
	}

	repository := NewPostgresRepository(pool)
	service := NewService(repository)
	session, err := service.CreateSession(ctx, ownerA, "academic-school-a", CreateSessionRequest{
		Name: "2026/2027", StartsOn: "2026-09-01", EndsOn: "2027-07-31",
	}, "postgres-create")
	if err != nil {
		t.Fatalf("create valid academic session: %v", err)
	}
	if _, err := service.CreateSession(ctx, adminA, "academic-school-a", CreateSessionRequest{
		Name: "2026/2027", StartsOn: "2026-09-01", EndsOn: "2027-07-31",
	}, "postgres-duplicate"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate school session should conflict, got %v", err)
	}
	if _, err := service.CreateSession(ctx, teacherA, "academic-school-a", CreateSessionRequest{
		Name: "teacher-attempt", StartsOn: "2026-09-01", EndsOn: "2027-07-31",
	}, "postgres-teacher"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("teacher must not create academic sessions, got %v", err)
	}
	if _, err := service.GetSession(ctx, ownerB, "academic-school-a", session.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant lookup must be hidden as not found, got %v", err)
	}
	if _, err := service.UpdateSession(ctx, ownerB, "academic-school-a", session.ID,
		UpdateSessionRequest{Name: stringPointer("cross-tenant-edit")}, "postgres-cross-tenant-update"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant update must be denied without exposing the school, got %v", err)
	}
	if _, err := service.ListSessions(ctx, platformAdmin, "academic-school-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("platform admin without school membership must not list academic data, got %v", err)
	}

	missingTermsSession, err := service.CreateSession(ctx, ownerA, "academic-school-a", CreateSessionRequest{
		Name: "2027/2028", StartsOn: "2027-09-01", EndsOn: "2028-07-31",
	}, "postgres-missing-terms")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateSession(ctx, ownerA, "academic-school-a", missingTermsSession.ID, "postgres-early-activate"); !errors.Is(err, ErrConflict) {
		t.Fatalf("activating without three terms must fail at PostgreSQL constraint, got %v", err)
	}

	terms := validTerms()
	if _, err := service.ConfigureTerms(ctx, adminA, "academic-school-a", session.ID,
		ConfigureTermsRequest{Terms: terms}, "postgres-terms"); err != nil {
		t.Fatalf("configure three valid terms: %v", err)
	}
	count, err := countAcademicTerms(ctx, pool, tenantA, session.ID)
	if err != nil || count != 3 {
		t.Fatalf("expected three persisted terms, count=%d err=%v", count, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO academic_sessions (id, tenant_id, name, starts_on, ends_on, status)
		VALUES ($1, $2, 'invalid-range', '2027-07-31', '2026-09-01', 'DRAFT')
	`, uuid.New(), tenantA); err == nil {
		t.Fatal("PostgreSQL must reject academic sessions with an inverted date range")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO academic_terms (id, tenant_id, session_id, sequence, name, starts_on, ends_on)
		VALUES ($1, $2, $3, 1, 'Duplicate sequence', '2026-09-01', '2026-09-02')
	`, uuid.New(), tenantA, session.ID); err == nil {
		t.Fatal("PostgreSQL must reject duplicate term sequences in one session")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO academic_terms (id, tenant_id, session_id, sequence, name, starts_on, ends_on)
		VALUES ($1, $2, $3, 1, 'Outside term', '2026-08-31', '2026-09-02')
	`, uuid.New(), tenantA, session.ID); err == nil {
		t.Fatal("PostgreSQL must reject term dates outside their session")
	}
	if _, err := service.ConfigureTerms(ctx, ownerA, "academic-school-a", session.ID,
		ConfigureTermsRequest{Terms: []TermInput{
			{Sequence: 1, Name: "Term 1", StartsOn: "2026-09-01", EndsOn: "2026-12-01"},
			{Sequence: 2, Name: "Term 2", StartsOn: "2026-12-01", EndsOn: "2027-03-31"},
			{Sequence: 3, Name: "Term 3", StartsOn: "2027-04-01", EndsOn: "2027-07-31"},
		}}, "postgres-overlap"); !errors.Is(err, ErrConflict) {
		t.Fatalf("overlapping inclusive term dates must be rejected, got %v", err)
	}
	if err := assertTermDates(ctx, pool, tenantA, session.ID, "2026-12-15"); err != nil {
		t.Fatalf("rejected term configuration must roll back completely: %v", err)
	}

	// Bypass service validation to prove the deferred PostgreSQL constraint rejects an overlap.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `
		UPDATE academic_terms SET ends_on = '2027-01-10'
		WHERE tenant_id = $1 AND session_id = $2 AND sequence = 1
	`, tenantA, session.ID)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("stage deliberately overlapping term update: %v", err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("database must reject overlapping terms at transaction commit")
	}
	if err := assertTermDates(ctx, pool, tenantA, session.ID, "2026-12-15"); err != nil {
		t.Fatalf("overlap constraint failure must roll back the update: %v", err)
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, update := range []struct {
		sequence int
		startsOn string
		endsOn   string
	}{
		{1, "2027-05-01", "2027-05-20"},
		{2, "2027-01-05", "2027-01-20"},
		{3, "2027-03-01", "2027-03-20"},
	} {
		if _, err := tx.Exec(ctx, `
			UPDATE academic_terms SET starts_on = $3::date, ends_on = $4::date
			WHERE tenant_id = $1 AND session_id = $2 AND sequence = $5
		`, tenantA, session.ID, update.startsOn, update.endsOn, update.sequence); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("stage deliberately out-of-order terms: %v", err)
		}
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("database must reject term dates that do not follow their configured sequence")
	}
	if err := assertTermDates(ctx, pool, tenantA, session.ID, "2026-12-15"); err != nil {
		t.Fatalf("sequence-order constraint failure must roll back all term updates: %v", err)
	}

	if _, err := service.ConfigureTerms(ctx, ownerA, "academic-school-a", session.ID,
		ConfigureTermsRequest{Terms: terms}, "postgres-valid-terms-again"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateSession(ctx, principalA, "academic-school-a", session.ID, "postgres-activate"); err != nil {
		t.Fatalf("principal should be able to activate a configured session: %v", err)
	}
	activeDateUpdate, err := service.UpdateSession(ctx, adminA, "academic-school-a", session.ID,
		UpdateSessionRequest{EndsOn: stringPointer("2027-08-31")}, "postgres-active-date-valid")
	if err != nil {
		t.Fatalf("authorized active session date extension should succeed: %v", err)
	}
	if activeDateUpdate.EndsOn != "2027-08-31" {
		t.Fatalf("active session date update was not persisted: %+v", activeDateUpdate)
	}
	if _, err := service.UpdateSession(ctx, adminA, "academic-school-a", session.ID,
		UpdateSessionRequest{StartsOn: stringPointer("2026-10-01")}, "postgres-active-date-break"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("active session date update that excludes a term must fail, got %v", err)
	}
	if err := verifyAuditRows(ctx, pool, tenantA, session.ID); err != nil {
		t.Fatal(err)
	}

	// Composite tenant FK rejects a term whose tenant does not own its referenced session.
	if _, err := pool.Exec(ctx, `
		INSERT INTO academic_terms (id, tenant_id, session_id, sequence, name, starts_on, ends_on)
		VALUES ($1, $2, $3, 1, 'Cross Tenant', '2026-09-01', '2026-09-02')
	`, uuid.New(), tenantB, session.ID); err == nil {
		t.Fatal("tenant-aware term/session foreign key must reject a cross-tenant reference")
	}

	// A failing audit insert must roll back its corresponding session mutation.
	rollbackSession := &Session{
		ID: uuid.New(), Name: "audit-rollback", StartsOn: "2028-09-01", EndsOn: "2029-07-31", Status: SessionDraft,
	}
	invalidActor := uuid.New()
	rollbackEvent := &auth.AuditEvent{
		ActorID: &invalidActor, Action: "academic.session_created", SubjectType: "academic_session",
		SubjectID: rollbackSession.ID.String(), RequestID: "rollback-test",
		MetadataRedacted: map[string]any{"updated_fields": []string{"name"}},
	}
	if _, err := repository.CreateSession(ctx, "academic-school-a", ownerA, rollbackSession, rollbackEvent); err == nil {
		t.Fatal("invalid audit actor should fail the transaction")
	}
	var persisted bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM academic_sessions WHERE id = $1)`, rollbackSession.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted {
		t.Fatal("academic session must roll back when its audit insert fails")
	}

	if _, err := service.ArchiveSession(ctx, ownerA, "academic-school-a", session.ID, "postgres-archive"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateSession(ctx, ownerA, "academic-school-a", session.ID,
		UpdateSessionRequest{Name: stringPointer("archived-edit")}, "postgres-archived-update"); !errors.Is(err, ErrConflict) {
		t.Fatalf("archived session update must be rejected, got %v", err)
	}
	if _, err := service.ConfigureTerms(ctx, ownerA, "academic-school-a", session.ID,
		ConfigureTermsRequest{Terms: terms}, "postgres-archived-terms"); !errors.Is(err, ErrConflict) {
		t.Fatalf("archived terms update must be rejected, got %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE academic_sessions SET name = 'direct-archived-edit'
		WHERE tenant_id = $1 AND id = $2
	`, tenantA, session.ID); err == nil {
		t.Fatal("PostgreSQL must reject direct updates to archived sessions")
	}
	if _, err := pool.Exec(ctx, `
		UPDATE academic_terms SET name = 'direct-archived-term-edit'
		WHERE tenant_id = $1 AND session_id = $2 AND sequence = 1
	`, tenantA, session.ID); err == nil {
		t.Fatal("PostgreSQL must reject direct updates to terms of archived sessions")
	}

	if err := testConcurrentActivation(ctx, service, ownerB); err != nil {
		t.Fatal(err)
	}
}

func testConcurrentActivation(ctx context.Context, service *Service, actorID uuid.UUID) error {
	const slug = "academic-school-b"
	sessions := make([]*Session, 2)
	termSets := [][]TermInput{
		{
			{Sequence: 1, Name: "First Term", StartsOn: "2028-09-01", EndsOn: "2028-12-15"},
			{Sequence: 2, Name: "Second Term", StartsOn: "2029-01-05", EndsOn: "2029-03-31"},
			{Sequence: 3, Name: "Third Term", StartsOn: "2029-04-01", EndsOn: "2029-07-31"},
		},
		{
			{Sequence: 1, Name: "First Term", StartsOn: "2029-09-01", EndsOn: "2029-12-15"},
			{Sequence: 2, Name: "Second Term", StartsOn: "2030-01-05", EndsOn: "2030-03-31"},
			{Sequence: 3, Name: "Third Term", StartsOn: "2030-04-01", EndsOn: "2030-07-31"},
		},
	}
	for i := range sessions {
		startYear := 2028 + i
		session, err := service.CreateSession(ctx, actorID, slug, CreateSessionRequest{
			Name:     fmt.Sprintf("%d/%d", startYear, startYear+1),
			StartsOn: fmt.Sprintf("%d-09-01", startYear), EndsOn: fmt.Sprintf("%d-07-31", startYear+1),
		}, "concurrent-create")
		if err != nil {
			return err
		}
		sessions[i] = session
		if _, err := service.ConfigureTerms(ctx, actorID, slug, session.ID,
			ConfigureTermsRequest{Terms: termSets[i]}, "concurrent-terms"); err != nil {
			return err
		}
	}

	start := make(chan struct{})
	results := make(chan error, len(sessions))
	var wait sync.WaitGroup
	for _, session := range sessions {
		wait.Add(1)
		go func(sessionID uuid.UUID) {
			defer wait.Done()
			<-start
			_, err := service.ActivateSession(ctx, actorID, slug, sessionID, "concurrent-activate")
			results <- err
		}(session.ID)
	}
	close(start)
	wait.Wait()
	close(results)

	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			return fmt.Errorf("unexpected concurrent activation result: %w", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		return fmt.Errorf("expected one active-session winner and one conflict, got successes=%d conflicts=%d", successes, conflicts)
	}
	return nil
}

func countAcademicTerms(ctx context.Context, pool *pgxpool.Pool, tenantID, sessionID uuid.UUID) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM academic_terms WHERE tenant_id = $1 AND session_id = $2
	`, tenantID, sessionID).Scan(&count)
	return count, err
}

func assertTermDates(ctx context.Context, pool *pgxpool.Pool, tenantID, sessionID uuid.UUID, expectedEnd string) error {
	var end string
	if err := pool.QueryRow(ctx, `
		SELECT ends_on::text FROM academic_terms
		WHERE tenant_id = $1 AND session_id = $2 AND sequence = 1
	`, tenantID, sessionID).Scan(&end); err != nil {
		return err
	}
	if end != expectedEnd {
		return fmt.Errorf("term date changed despite rejected transaction: got %s want %s", end, expectedEnd)
	}
	return nil
}

func verifyAuditRows(ctx context.Context, pool *pgxpool.Pool, tenantID, sessionID uuid.UUID) error {
	var count int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE tenant_id = $1 AND subject_type = 'academic_session' AND subject_id = $2
	`, tenantID, sessionID.String()).Scan(&count)
	if err != nil {
		return fmt.Errorf("read academic audit events: %w", err)
	}
	if count != 5 {
		return fmt.Errorf("expected session create, two term configurations, activation and active-date update audit rows; got %d", count)
	}
	return nil
}

func applyAcademicMigrationForTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool, path string) {
	t.Helper()
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", filepath.Base(path), err)
	}
	if _, err := pool.Exec(ctx, string(sql), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatalf("apply migration %s: %v", filepath.Base(path), err)
	}
}
