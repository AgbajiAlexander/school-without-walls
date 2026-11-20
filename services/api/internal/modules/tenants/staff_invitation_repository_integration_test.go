package tenants

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/modules/auth"
)

type staffTestMailer func(context.Context, string) error

func (send staffTestMailer) SendStaffInvitation(ctx context.Context, _, token, _, _ string, _ time.Time) error {
	return send(ctx, token)
}

func TestPostgresStaffInvitationRepository(t *testing.T) {
	databaseURL := requireTenantIntegrationDatabaseURL(t)
	if databaseURL == "" {
		t.Skip("isolated test database is not configured")
	}

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Database != "sww_structure_test" {
		t.Fatalf("unsafe test database: %q", cfg.ConnConfig.Database)
	}
	if cfg.ConnConfig.Host != "localhost" &&
		cfg.ConnConfig.Host != "127.0.0.1" {
		t.Fatalf("unsafe test host: %q", cfg.ConnConfig.Host)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer adminPool.Close()

	schema := "sww_staff_inv_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, err := adminPool.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("clean up test schema: %v", err)
		}
	}()

	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `
        CREATE TABLE schema_migrations (
            version VARCHAR(64) PRIMARY KEY,
            applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
        )
    `); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"000001_initial_schema.up.sql",
		"000002_auth_identity.up.sql",
		"000003_auth_hardening.up.sql",
		"000004_tenants.up.sql",
		"000008_staff_invitation_delivery.up.sql",
	} {
		applyTenantMigrationForTest(t, ctx, pool,
			filepath.Join("..", "..", "..", "migrations", name))
	}

	must := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("seed test data: %v", err)
		}
	}

	ownerA := uuid.New()
	ownerB := uuid.New()
	teacher := uuid.New()
	suspendedOwner := uuid.New()
	existingPrincipal := uuid.New()
	tenantA := uuid.New()
	tenantB := uuid.New()

	identities := []struct {
		id    uuid.UUID
		email string
	}{
		{ownerA, "staff-owner-a@school.invalid"},
		{ownerB, "staff-owner-b@school.invalid"},
		{teacher, "staff-teacher@school.invalid"},
		{suspendedOwner, "staff-suspended@school.invalid"},
		{existingPrincipal, "staff-existing@school.invalid"},
	}

	for _, u := range identities {
		must(`INSERT INTO users
            (id,email,password_hash,full_name,status)
            VALUES ($1,$2,'test-hash','Test User','ACTIVE')`,
			u.id, u.email)
	}

	must(`INSERT INTO tenants (id,slug,name,status)
        VALUES ($1,'staff-test-a','Test School A','ACTIVE')`, tenantA)
	must(`INSERT INTO tenants (id,slug,name,status)
        VALUES ($1,'staff-test-b','Test School B','ACTIVE')`, tenantB)

	memberships := []struct {
		tenant uuid.UUID
		user   uuid.UUID
		role   string
		status string
	}{
		{tenantA, ownerA, "SCHOOL_OWNER", "ACTIVE"},
		{tenantB, ownerB, "SCHOOL_OWNER", "ACTIVE"},
		{tenantA, teacher, "TEACHER", "ACTIVE"},
		{tenantA, suspendedOwner, "SCHOOL_OWNER", "SUSPENDED"},
		{tenantA, existingPrincipal, "PRINCIPAL", "ACTIVE"},
	}

	for _, m := range memberships {
		must(`INSERT INTO memberships (id,tenant_id,user_id,role,status)
            VALUES ($1,$2,$3,$4,$5::membership_status)`,
			uuid.New(), m.tenant, m.user, m.role, m.status)
	}

	repo := NewPostgresRepository(pool).(*postgresRepository)

	newInvitation := func(email, role string) *auth.Invitation {
		return &auth.Invitation{
			ID:           uuid.New(),
			InvitedEmail: email,
			TargetRole:   role,
			TokenHash:    auth.HashToken(uuid.NewString()),
			Status:       auth.InvitationStatusPending,
			ExpiresAt:    time.Now().UTC().Add(24 * time.Hour),
			CreatedAt:    time.Now().UTC(),
		}
	}

	newEvent := func() *auth.AuditEvent {
		return &auth.AuditEvent{
			Action:           "tenant.staff_invitation.create",
			RequestID:        uuid.NewString(),
			MetadataRedacted: map[string]any{},
		}
	}

	t.Run("unconfirmed delivery cannot grant access even when revocation rolls back", func(t *testing.T) {
		const email = "staff-ambiguous-delivery@school.invalid"
		must(`ALTER TABLE invitations ADD CONSTRAINT staff_test_revocation_failure
			CHECK (invited_email <> 'staff-ambiguous-delivery@school.invalid' OR status <> 'REVOKED')`)
		authService := auth.NewService(auth.NewPostgresRepository(pool))
		var token string
		redeem := func(ctx context.Context, raw string) error {
			_, _, _, err := authService.AcceptInvitation(ctx, raw, "Fictional Invitee",
				"fictional test passphrase for staff", "127.0.0.1", "staff-test", "staff-ambiguous-accept")
			return err
		}
		service := NewStaffInvitationService(repo, staffTestMailer(func(ctx context.Context, raw string) error {
			token = raw
			if _, err := authService.VerifyInvitation(ctx, raw); !errors.Is(err, auth.ErrInvitationNotFound) {
				t.Fatal("unconfirmed invitation was visible during mail handoff")
			}
			if err := redeem(ctx, raw); !errors.Is(err, auth.ErrInvitationNotFound) {
				t.Fatal("unconfirmed token was redeemable during mail handoff")
			}
			// Simulate the caller losing the SMTP response after the remote server received the email.
			return errors.New("fictional SMTP confirmation lost")
		}))
		result, err := service.InviteStaff(ctx, ownerA, "staff-test-a",
			StaffInvitationRequest{Email: email, Role: "PRINCIPAL"}, "staff-ambiguous-send")
		if err == nil || result != nil || token == "" {
			t.Fatal("expected an unconfirmed delivery error without a receipt")
		}
		if err := redeem(ctx, token); !errors.Is(err, auth.ErrInvitationNotFound) {
			t.Fatal("token became redeemable after failed cleanup")
		}
		var pending, noAccount, noAcceptance bool
		if err := pool.QueryRow(ctx, `SELECT
			(SELECT status='AWAITING_DELIVERY' AND used_at IS NULL
			 FROM invitations WHERE tenant_id=$1 AND invited_email=$2),
			NOT EXISTS (SELECT 1 FROM users WHERE email=$2),
			NOT EXISTS (SELECT 1 FROM audit_events WHERE request_id='staff-ambiguous-accept')`,
			tenantA, email).Scan(&pending, &noAccount, &noAcceptance); err != nil {
			t.Fatal(err)
		}
		if !pending || !noAccount || !noAcceptance {
			t.Fatal("failed revocation did not preserve an unused, gated invitation")
		}
	})

	t.Run("delivery finalization and revocation cannot both win", func(t *testing.T) {
		inv := newInvitation("staff-finalize-race@school.invalid", "PRINCIPAL")
		if _, err := repo.CreateStaffInvitation(ctx, "staff-test-a", ownerA, inv, newEvent()); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		confirmed, revoked := make(chan error, 1), make(chan error, 1)
		go func() {
			<-start
			confirmed <- repo.ConfirmStaffInvitationDelivery(ctx, inv.ID, tenantA)
		}()
		go func() {
			<-start
			revoked <- repo.RevokeStaffInvitation(ctx, inv.ID, tenantA, newEvent())
		}()
		close(start)
		confirmErr, revokeErr := <-confirmed, <-revoked
		if !((confirmErr == nil && errors.Is(revokeErr, ErrConflict)) ||
			(revokeErr == nil && errors.Is(confirmErr, ErrConflict))) {
			t.Fatalf("expected exactly one state transition, got confirmation=%v revocation=%v", confirmErr, revokeErr)
		}
		var status string
		if err := pool.QueryRow(ctx, "SELECT status::text FROM invitations WHERE id=$1", inv.ID).
			Scan(&status); err != nil {
			t.Fatal(err)
		}
		if confirmErr == nil && status != "PENDING" || revokeErr == nil && status != "REVOKED" {
			t.Fatal("unexpected final invitation state")
		}
	})

	t.Run("redemption cannot overtake delivery-failure revocation", func(t *testing.T) {
		inv := newInvitation("staff-revoke-race@school.invalid", "PRINCIPAL")
		token := uuid.NewString()
		inv.TokenHash = auth.HashToken(token)
		if _, err := repo.CreateStaffInvitation(ctx, "staff-test-a", ownerA, inv, newEvent()); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		redeemed, revoked := make(chan error, 1), make(chan error, 1)
		go func() {
			<-start
			_, _, _, err := auth.NewService(auth.NewPostgresRepository(pool)).AcceptInvitation(
				ctx, token, "Fictional Invitee", "fictional test passphrase for staff",
				"127.0.0.1", "staff-test", "staff-revocation-race")
			redeemed <- err
		}()
		go func() {
			<-start
			revoked <- repo.RevokeStaffInvitation(ctx, inv.ID, tenantA, newEvent())
		}()
		close(start)
		if acceptErr, revokeErr := <-redeemed, <-revoked; !errors.Is(acceptErr, auth.ErrInvitationNotFound) || revokeErr != nil {
			t.Fatal("a token without delivery confirmation raced past revocation")
		}
	})

	t.Run("active owner creates staff invitation with audit", func(t *testing.T) {
		inv := newInvitation("staff-new-admin@school.invalid", "SCHOOL_ADMIN")

		name, err := repo.CreateStaffInvitation(
			ctx, "staff-test-a", ownerA, inv, newEvent())
		if err != nil {
			t.Fatal(err)
		}
		if name != "Test School A" {
			t.Fatalf("unexpected school name: %q", name)
		}
		if inv.TenantID == nil || *inv.TenantID != tenantA {
			t.Fatal("invitation not scoped to School A")
		}

		var invitationCount, auditCount int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM invitations WHERE id=$1 AND tenant_id=$2
             AND target_role='SCHOOL_ADMIN' AND status='AWAITING_DELIVERY'`,
			inv.ID, tenantA).Scan(&invitationCount); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM audit_events WHERE tenant_id=$1
             AND subject_type='invitation' AND subject_id=$2`,
			tenantA, inv.ID.String()).Scan(&auditCount); err != nil {
			t.Fatal(err)
		}
		if invitationCount != 1 || auditCount != 1 {
			t.Fatalf("invitation=%d audit=%d, want 1 each",
				invitationCount, auditCount)
		}
	})

	t.Run("duplicate pending invitation rejected", func(t *testing.T) {
		inv := newInvitation("staff-new-admin@school.invalid", "SCHOOL_ADMIN")
		_, err := repo.CreateStaffInvitation(
			ctx, "staff-test-a", ownerA, inv, newEvent())
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("want ErrConflict, got %v", err)
		}
	})

	t.Run("existing principal membership rejected", func(t *testing.T) {
		inv := newInvitation("staff-existing@school.invalid", "PRINCIPAL")
		_, err := repo.CreateStaffInvitation(
			ctx, "staff-test-a", ownerA, inv, newEvent())
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("want ErrConflict, got %v", err)
		}
	})

	t.Run("teacher cannot invite", func(t *testing.T) {
		inv := newInvitation("staff-new-teacher@school.invalid", "PRINCIPAL")
		_, err := repo.CreateStaffInvitation(
			ctx, "staff-test-a", teacher, inv, newEvent())
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("want ErrForbidden, got %v", err)
		}
	})

	t.Run("cross-school owner cannot invite", func(t *testing.T) {
		inv := newInvitation("staff-cross-school@school.invalid", "PRINCIPAL")
		_, err := repo.CreateStaffInvitation(
			ctx, "staff-test-b", ownerA, inv, newEvent())
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("want ErrForbidden, got %v", err)
		}
	})

	t.Run("suspended owner cannot invite", func(t *testing.T) {
		inv := newInvitation("staff-suspended-invite@school.invalid", "PRINCIPAL")
		_, err := repo.CreateStaffInvitation(
			ctx, "staff-test-a", suspendedOwner, inv, newEvent())
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("want ErrForbidden, got %v", err)
		}
	})

	t.Run("owner role cannot be granted", func(t *testing.T) {
		inv := newInvitation("staff-privilege@school.invalid", "SCHOOL_OWNER")
		_, err := repo.CreateStaffInvitation(
			ctx, "staff-test-a", ownerA, inv, newEvent())
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("want ErrInvalidInput, got %v", err)
		}
	})

	t.Run("revocation consumes pending invitation", func(t *testing.T) {
		inv := newInvitation("staff-revoke@school.invalid", "PRINCIPAL")
		_, err := repo.CreateStaffInvitation(
			ctx, "staff-test-a", ownerA, inv, newEvent())
		if err != nil {
			t.Fatal(err)
		}

		event := newEvent()
		event.Action = "tenant.staff_invitation.delivery_failed"
		event.ActorID = &ownerA

		if err := repo.RevokeStaffInvitation(
			ctx, inv.ID, tenantA, event); err != nil {
			t.Fatal(err)
		}

		var status string
		if err := pool.QueryRow(ctx,
			`SELECT status::text FROM invitations WHERE id=$1`,
			inv.ID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "REVOKED" {
			t.Fatalf("want REVOKED, got %s", status)
		}

		if err := repo.RevokeStaffInvitation(
			ctx, inv.ID, tenantA, newEvent()); !errors.Is(err, ErrConflict) {
			t.Fatalf("repeated revocation: want ErrConflict, got %v", err)
		}
	})

	t.Run("concurrent duplicate invitations", func(t *testing.T) {
		email := "staff-concurrent@school.invalid"

		results := make(chan error, 2)

		for i := 0; i < 2; i++ {
			go func() {
				inv := newInvitation(email, "SCHOOL_ADMIN")
				_, err := repo.CreateStaffInvitation(
					ctx, "staff-test-a", ownerA, inv, newEvent())
				results <- err
			}()
		}

		successCount := 0
		conflictCount := 0

		for i := 0; i < 2; i++ {
			err := <-results

			switch {
			case err == nil:
				successCount++
			case errors.Is(err, ErrConflict):
				conflictCount++
			default:
				t.Fatalf("unexpected concurrency error: %v", err)
			}
		}

		if successCount != 1 || conflictCount != 1 {
			t.Fatalf(
				"success=%d conflict=%d, expected one each",
				successCount, conflictCount,
			)
		}

		var invitationCount int
		if err := pool.QueryRow(ctx, `
            SELECT count(*) FROM invitations
            WHERE tenant_id=$1 AND invited_email=$2
              AND target_role='SCHOOL_ADMIN'
              AND status='AWAITING_DELIVERY'
        `, tenantA, email).Scan(&invitationCount); err != nil {
			t.Fatal(err)
		}

		if invitationCount != 1 {
			t.Fatalf("expected one invitation, got %d", invitationCount)
		}
	})

	t.Run("failed audit rolls back invitation", func(t *testing.T) {
		email := "staff-rollback@school.invalid"
		inv := newInvitation(email, "PRINCIPAL")

		event := newEvent()
		event.MetadataRedacted = map[string]any{
			"invalid_value": make(chan int),
		}

		_, err := repo.CreateStaffInvitation(
			ctx, "staff-test-a", ownerA, inv, event)
		if err == nil {
			t.Fatal("expected audit serialization failure")
		}

		var count int
		if err := pool.QueryRow(ctx, `
            SELECT count(*) FROM invitations
            WHERE tenant_id=$1 AND invited_email=$2
        `, tenantA, email).Scan(&count); err != nil {
			t.Fatal(err)
		}

		if count != 0 {
			t.Fatalf(
				"invitation survived failed transaction: %d",
				count,
			)
		}
	})

	t.Run("suspended existing membership blocks invitation", func(t *testing.T) {
		userID := uuid.New()
		email := "staff-already-suspended@school.invalid"

		must(`INSERT INTO users
            (id,email,password_hash,full_name,status)
            VALUES ($1,$2,'test-hash','Suspended Staff','ACTIVE')`,
			userID, email)

		must(`INSERT INTO memberships
            (id,tenant_id,user_id,role,status)
            VALUES ($1,$2,$3,'SCHOOL_ADMIN','SUSPENDED')`,
			uuid.New(), tenantA, userID)

		inv := newInvitation(email, "SCHOOL_ADMIN")

		_, err := repo.CreateStaffInvitation(
			ctx, "staff-test-a", ownerA, inv, newEvent())

		if !errors.Is(err, ErrConflict) {
			t.Fatalf("expected ErrConflict, got %v", err)
		}
	})
}
