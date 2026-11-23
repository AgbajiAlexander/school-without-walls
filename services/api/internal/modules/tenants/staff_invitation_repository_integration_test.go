package tenants

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/config"
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
		"000024_school_operating_mode.up.sql",
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

	t.Run("active owner capability includes multi-role owners only once", func(t *testing.T) {
		must(`INSERT INTO memberships (id, tenant_id, user_id, role, status)
			VALUES ($1,$3,$4,'PRINCIPAL','ACTIVE'), ($2,$3,$5,'PRINCIPAL','ACTIVE')`,
			uuid.New(), uuid.New(), tenantA, ownerA, suspendedOwner)
		must(`INSERT INTO memberships (id, tenant_id, user_id, role, status)
			VALUES ($1,$4,$5,'BURSAR','ACTIVE'), ($2,$4,$6,'BURSAR','ACTIVE'),
			($3,$4,$6,'PRINCIPAL','SUSPENDED')`, uuid.New(), uuid.New(), uuid.New(), tenantA, existingPrincipal, teacher)
		for _, tc := range []struct {
			actor          uuid.UUID
			role           string
			invite, manage bool
		}{
			{ownerA, "PRINCIPAL", true, true},
			{suspendedOwner, "PRINCIPAL", false, true},
			{existingPrincipal, "BURSAR", false, true},
			{teacher, "BURSAR", false, false},
		} {
			school, err := repo.GetForMember(ctx, "staff-test-a", tc.actor)
			if err != nil || school.CurrentRole != tc.role || school.CanInviteStaff != tc.invite || school.CanManageAcademics != tc.manage {
				t.Fatalf("unexpected school capability: %v", err)
			}
			schools, err := repo.ListForMember(ctx, tc.actor)
			if err != nil || len(schools) != 1 || schools[0].ID != tenantA || schools[0].CanInviteStaff != tc.invite || schools[0].CanManageAcademics != tc.manage {
				t.Fatalf("unexpected dashboard capability: %v", err)
			}
		}
		if _, err := repo.GetForMember(ctx, "staff-test-b", ownerA); !errors.Is(err, ErrNotFound) {
			t.Fatal("multi-role owner was able to read another school")
		}
	})

	t.Run("quota window uses an exclusive lower boundary", func(t *testing.T) {
		boundaryTenant := uuid.New()
		must(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'quota-boundary','Quota Boundary','ACTIVE')`, boundaryTenant)
		limited := NewPostgresRepositoryWithStaffLimits(pool, config.StaffInvitationLimits{PerActor: 1, PerTenant: 1, Window: time.Hour}).(*postgresRepository)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		var now time.Time
		if err := tx.QueryRow(ctx, `SELECT statement_timestamp() FROM tenants WHERE id=$1 FOR UPDATE`, boundaryTenant).Scan(&now); err != nil {
			t.Fatal(err)
		}
		id := uuid.New()
		for _, tc := range []struct {
			name    string
			offset  time.Duration
			blocked bool
		}{
			{"just inside", time.Microsecond, true},
			{"exact lower boundary", 0, false},
			{"just outside", -time.Microsecond, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if _, err := tx.Exec(ctx, `INSERT INTO audit_events
					(id, tenant_id, actor_id, action, subject_type, subject_id, request_id, created_at)
					VALUES ($1,$2,$3,$4,'invitation','fixture','fixture',$5)
					ON CONFLICT (id) DO UPDATE SET created_at = EXCLUDED.created_at`,
					id, boundaryTenant, ownerA, staffInvitationCreateAction, now.Add(-time.Hour+tc.offset)); err != nil {
					t.Fatal(err)
				}
				_, err := limited.checkStaffInvitationQuota(ctx, tx, boundaryTenant, ownerA, &now)
				var rejected *staffInvitationRateLimitError
				if tc.blocked {
					if !errors.As(err, &rejected) || rejected.retryAfter != time.Microsecond {
						t.Fatalf("inside issuance did not consume quota: %v", err)
					}
				} else if err != nil {
					t.Fatalf("issuance at or outside the boundary consumed quota: %v", err)
				}
			})
		}
	})

	t.Run("failed delivery charges quota and duplicates do not", func(t *testing.T) {
		school := uuid.New()
		must(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'quota-delivery','Quota Delivery','ACTIVE')`, school)
		must(`INSERT INTO memberships (id,tenant_id,user_id,role,status) VALUES ($1,$2,$3,'SCHOOL_OWNER','ACTIVE')`, uuid.New(), school, ownerA)
		limited := NewPostgresRepositoryWithStaffLimits(pool, config.StaffInvitationLimits{PerActor: 2, PerTenant: 2, Window: time.Hour}).(*postgresRepository)
		var sends int
		failDelivery := false
		service := NewStaffInvitationService(limited, staffTestMailer(func(context.Context, string) error {
			sends++
			if failDelivery {
				return errors.New("fictional delivery failure")
			}
			return nil
		}))
		issue := func(email string) error {
			_, err := service.InviteStaff(ctx, ownerA, "quota-delivery", StaffInvitationRequest{Email: email, Role: "PRINCIPAL"}, uuid.NewString())
			return err
		}
		const first = "quota-confirmed@school.invalid"
		if err := issue(first); err != nil {
			t.Fatal(err)
		}
		if err := issue(first); !errors.Is(err, ErrConflict) {
			t.Fatalf("duplicate: %v", err)
		}
		failDelivery = true
		if err := issue("quota-failed@school.invalid"); err == nil || sends != 2 {
			t.Fatalf("expected second issuance to attempt delivery despite duplicate: %v", err)
		}
		var rejected *staffInvitationRateLimitError
		if err := issue("quota-blocked@school.invalid"); !errors.As(err, &rejected) || sends != 2 {
			t.Fatalf("expected failed delivery to retain quota without another send: %v", err)
		}
		var revoked, charges int
		if err := pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM invitations WHERE tenant_id=$1 AND status='REVOKED'),
			(SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND action=$2)`,
			school, staffInvitationCreateAction).Scan(&revoked, &charges); err != nil || revoked != 1 || charges != 2 {
			t.Fatalf("unexpected revoked=%d charges=%d: %v", revoked, charges, err)
		}
	})

	t.Run("concurrent distinct recipients respect actor and tenant quotas across repositories", func(t *testing.T) {
		limitedTenant := uuid.New()
		must(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'quota-test','Test Quota School','ACTIVE')`, limitedTenant)
		for _, actor := range []uuid.UUID{ownerA, ownerB} {
			must(`INSERT INTO memberships (id,tenant_id,user_id,role,status) VALUES ($1,$2,$3,'SCHOOL_OWNER','ACTIVE')`, uuid.New(), limitedTenant, actor)
		}
		limits := config.StaffInvitationLimits{PerActor: 2, PerTenant: 3, Window: time.Hour}
		otherConfig := cfg.Copy()
		otherConfig.ConnConfig.RuntimeParams["default_transaction_isolation"] = "repeatable read"
		otherPool, err := pgxpool.NewWithConfig(ctx, otherConfig)
		if err != nil {
			t.Fatal(err)
		}
		defer otherPool.Close()
		instances := []*postgresRepository{
			NewPostgresRepositoryWithStaffLimits(pool, limits).(*postgresRepository),
			NewPostgresRepositoryWithStaffLimits(otherPool, limits).(*postgresRepository),
		}
		issueConcurrently := func(actor uuid.UUID, prefix string, want int) {
			t.Helper()
			start := make(chan struct{})
			results := make(chan error, 8)
			for i := range 8 {
				go func() {
					<-start
					inv := newInvitation(fmt.Sprintf("%s-%d@school.invalid", prefix, i), "PRINCIPAL")
					_, err := instances[i%len(instances)].CreateStaffInvitation(ctx, "quota-test", actor, inv, newEvent())
					results <- err
				}()
			}
			close(start)
			issued := 0
			for range 8 {
				err := <-results
				if err == nil {
					issued++
					continue
				}
				var limited *staffInvitationRateLimitError
				if !errors.As(err, &limited) || limited.retryAfter <= 0 || limited.retryAfter > time.Hour {
					t.Errorf("unexpected error for distinct recipient: %v", err)
				}
			}
			if issued != want {
				t.Fatalf("issued %d invitations, want %d", issued, want)
			}
		}
		issueConcurrently(ownerA, "quota-actor", 2)
		issueConcurrently(ownerB, "quota-school", 1)
		var count, audited int
		if err := pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM invitations WHERE tenant_id=$1),
			(SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND action=$2)`,
			limitedTenant, staffInvitationCreateAction).Scan(&count, &audited); err != nil || count != 3 || audited != 3 {
			t.Fatalf("quota losers left issuance side effects: invitations=%d audits=%d error=%v", count, audited, err)
		}
		if _, err := instances[0].CreateStaffInvitation(ctx, "staff-test-b", ownerB, newInvitation("quota-other-tenant@school.invalid", "PRINCIPAL"), newEvent()); err != nil {
			t.Fatalf("quota leaked into another school: %v", err)
		}
		must(`UPDATE audit_events SET created_at=clock_timestamp() - interval '2 hours' WHERE tenant_id=$1`, limitedTenant)
		issueConcurrently(ownerA, "quota-next-window", 2)
	})

	for _, tc := range []struct {
		table, update string
		args          []any
	}{
		{"memberships", `UPDATE memberships SET status='SUSPENDED' WHERE tenant_id=$1 AND user_id=$2 AND role='SCHOOL_OWNER'`, []any{tenantA, ownerA}},
		{"users", `UPDATE users SET status='SUSPENDED' WHERE id=$1`, []any{ownerA}},
	} {
		t.Run("issuer suspension during issuance/"+tc.table, func(t *testing.T) {
			waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			blocker, err := pool.Begin(waitCtx)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(ctx)
			if _, err := blocker.Exec(waitCtx, tc.update, tc.args...); err != nil {
				t.Fatal(err)
			}
			mailSent := make(chan struct{}, 1)
			service := NewStaffInvitationService(repo, staffTestMailer(func(context.Context, string) error {
				mailSent <- struct{}{}
				return nil
			}))
			email, requestID := "suspension-"+tc.table+"@school.invalid", uuid.NewString()
			result := make(chan error, 1)
			go func() {
				_, err := service.InviteStaff(waitCtx, ownerA, "staff-test-a",
					StaffInvitationRequest{Email: email, Role: "PRINCIPAL"}, requestID)
				result <- err
			}()
			ticker := time.NewTicker(25 * time.Millisecond)
			defer ticker.Stop()
			for {
				var waiting bool
				if err := pool.QueryRow(waitCtx, `SELECT EXISTS(
					SELECT 1 FROM pg_stat_activity
					WHERE wait_event_type='Lock' AND $1=ANY(pg_blocking_pids(pid)) AND strpos(query, $2)>0)`,
					int32(blocker.Conn().PgConn().PID()), "FROM "+tc.table).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				select {
				case err := <-result:
					t.Fatalf("issuance completed before suspension committed: %v", err)
				case <-waitCtx.Done():
					t.Fatal("expected issuance to wait on the status row")
				case <-ticker.C:
				}
			}
			if err := blocker.Commit(waitCtx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-result:
				if !errors.Is(err, ErrForbidden) {
					t.Fatalf("expected committed suspension to deny issuance: %v", err)
				}
			case <-waitCtx.Done():
				t.Fatal(waitCtx.Err())
			}
			select {
			case <-mailSent:
				t.Fatal("suspended issuer sent an email")
			default:
			}
			var noSideEffects bool
			if err := pool.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM invitations WHERE invited_email=$1)
				AND NOT EXISTS (SELECT 1 FROM audit_events WHERE request_id=$2)`, email, requestID).Scan(&noSideEffects); err != nil || !noSideEffects {
				t.Fatalf("suspended issuer left side effects: %v", err)
			}
			if tc.table == "users" {
				must("UPDATE users SET status='ACTIVE' WHERE id=$1", ownerA)
			} else {
				must("UPDATE memberships SET status='ACTIVE' WHERE tenant_id=$1 AND user_id=$2 AND role='SCHOOL_OWNER'", tenantA, ownerA)
			}
		})
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
