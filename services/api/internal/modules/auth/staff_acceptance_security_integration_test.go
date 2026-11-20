package auth

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresStaffAcceptanceSecurity(t *testing.T) {
	databaseURL := requireIntegrationDatabaseURL(t)
	if databaseURL == "" {
		t.Skip("isolated PostgreSQL test database not configured")
	}

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("invalid staff acceptance test database configuration")
	}
	if cfg.ConnConfig.Database != "sww_structure_test" ||
		(cfg.ConnConfig.Port != 5432 && databaseURL != "postgres://sww_browser@127.0.0.1:15432/sww_structure_test?sslmode=disable") ||
		(cfg.ConnConfig.Host != "localhost" &&
			cfg.ConnConfig.Host != "127.0.0.1") {
		t.Fatalf("unsafe integration test database target")
	}
	for _, fallback := range cfg.ConnConfig.Fallbacks {
		if fallback.Host != cfg.ConnConfig.Host || fallback.Port != cfg.ConnConfig.Port {
			t.Fatal("unsafe integration test database fallback")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	admin, err := pgxpool.NewWithConfig(ctx, cfg.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	schema := "sww_staff_accept_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")

	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 15*time.Second)
		defer stop()
		if _, err := admin.Exec(
			cleanupCtx, "DROP SCHEMA "+schema+" CASCADE",
		); err != nil {
			t.Errorf("cleanup security test schema: %v", err)
		}
	}()

	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	// The concurrency cases need a blocker, an acceptance transaction and a lock observer.
	cfg.MaxConns = max(cfg.MaxConns, 3)

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	for _, name := range []string{
		"000001_initial_schema.up.sql",
		"000002_auth_identity.up.sql",
		"000003_auth_hardening.up.sql",
		"000004_tenants.up.sql",
	} {
		applyMigrationForTest(t, ctx, pool,
			filepath.Join("..", "..", "..", "migrations", name))
	}

	repo := NewPostgresRepository(pool)
	service := NewService(repo)
	password := "a sufficiently long fictional staff test passphrase"

	passwordHash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name           string
		tenantStatus   string
		targetRole     string
		existingRole   string
		existingStatus string
		shouldAccept   bool
	}{
		{
			name:         "active school accepts new principal",
			tenantStatus: "ACTIVE",
			targetRole:   "PRINCIPAL",
			shouldAccept: true,
		},
		{
			name:         "archived school rejects administrator",
			tenantStatus: "ARCHIVED",
			targetRole:   "SCHOOL_ADMIN",
		},
		{
			name:         "suspended school rejects principal",
			tenantStatus: "SUSPENDED",
			targetRole:   "PRINCIPAL",
		},
		{
			name:         "pending school rejects administrator",
			tenantStatus: "PENDING_SETUP",
			targetRole:   "SCHOOL_ADMIN",
		},
		{
			name:         "active school rejects new owner invitation",
			tenantStatus: "ACTIVE",
			targetRole:   "SCHOOL_OWNER",
		},
		{
			name:           "duplicate active role rejected",
			tenantStatus:   "ACTIVE",
			targetRole:     "SCHOOL_ADMIN",
			existingRole:   "SCHOOL_ADMIN",
			existingStatus: "ACTIVE",
		},
		{
			name:           "suspended matching membership rejected",
			tenantStatus:   "ACTIVE",
			targetRole:     "PRINCIPAL",
			existingRole:   "PRINCIPAL",
			existingStatus: "SUSPENDED",
		},
		{
			name:           "suspended different role cannot be bypassed",
			tenantStatus:   "ACTIVE",
			targetRole:     "SCHOOL_ADMIN",
			existingRole:   "TEACHER",
			existingStatus: "SUSPENDED",
		},
		{
			name:           "active different role can accept",
			tenantStatus:   "ACTIVE",
			targetRole:     "PRINCIPAL",
			existingRole:   "TEACHER",
			existingStatus: "ACTIVE",
			shouldAccept:   true,
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tenantID := uuid.New()
			userID := uuid.New()
			slug := "staff-accept-test-" + uuid.NewString()
			email := "staff-accept-" + uuid.NewString() + "@school.invalid"

			_, err := pool.Exec(ctx, `
                INSERT INTO tenants (id, slug, name, status)
                VALUES ($1, $2, 'Security Test School', $3::tenant_status)
            `, tenantID, slug, tc.tenantStatus)
			if err != nil {
				t.Fatal(err)
			}

			_, err = pool.Exec(ctx, `
                INSERT INTO users
                    (id, email, password_hash, full_name, status)
                VALUES ($1, $2, $3, 'Fictional Staff', 'ACTIVE')
            `, userID, email, passwordHash)
			if err != nil {
				t.Fatal(err)
			}

			if tc.existingRole != "" {
				_, err = pool.Exec(ctx, `
                    INSERT INTO memberships
                        (id, tenant_id, user_id, role, status)
                    VALUES ($1, $2, $3, $4, $5::membership_status)
                `, uuid.New(), tenantID, userID,
					tc.existingRole, tc.existingStatus)
				if err != nil {
					t.Fatal(err)
				}
			}

			token, err := GenerateSecureToken(32)
			if err != nil {
				t.Fatal(err)
			}

			if err := repo.CreateInvitation(ctx, &Invitation{
				TenantID:     &tenantID,
				InvitedEmail: email,
				TargetRole:   tc.targetRole,
				TokenHash:    HashToken(token),
				Status:       InvitationStatusPending,
				ExpiresAt:    time.Now().UTC().Add(time.Hour),
			}); err != nil {
				t.Fatal(err)
			}

			before := 0
			if err := pool.QueryRow(ctx, `
                SELECT count(*) FROM memberships
                WHERE tenant_id = $1 AND user_id = $2
            `, tenantID, userID).Scan(&before); err != nil {
				t.Fatal(err)
			}

			accepted, _, _, acceptErr := service.AcceptInvitation(
				ctx, token, "Fictional Staff", password,
				"127.0.0.1", "security-test",
				"staff-security-"+uuid.NewString(),
			)

			if tc.shouldAccept {
				if acceptErr != nil {
					t.Fatalf("expected acceptance: %v", acceptErr)
				}
				if accepted == nil || accepted.ID != userID {
					t.Fatal("unexpected accepted account")
				}
			} else if !errors.Is(acceptErr, ErrInvitationNotFound) {
				t.Fatalf(
					"expected ErrInvitationNotFound, got %v",
					acceptErr,
				)
			}

			var after, granted, pending, consumed int
			if err := pool.QueryRow(ctx, `
                SELECT count(*) FROM memberships
                WHERE tenant_id = $1 AND user_id = $2
            `, tenantID, userID).Scan(&after); err != nil {
				t.Fatal(err)
			}

			if err := pool.QueryRow(ctx, `
                SELECT count(*) FROM memberships
                WHERE tenant_id = $1 AND user_id = $2
                  AND role = $3 AND status = 'ACTIVE'
            `, tenantID, userID, tc.targetRole).Scan(&granted); err != nil {
				t.Fatal(err)
			}

			if err := pool.QueryRow(ctx, `
                SELECT
                    count(*) FILTER (WHERE status = 'PENDING'),
                    count(*) FILTER (WHERE status = 'ACCEPTED')
                FROM invitations WHERE token_hash = $1
            `, HashToken(token)).Scan(&pending, &consumed); err != nil {
				t.Fatal(err)
			}

			sessions, err := repo.ListActiveSessions(ctx, userID)
			if err != nil {
				t.Fatal(err)
			}

			if tc.shouldAccept {
				if after != before+1 || granted != 1 ||
					pending != 0 || consumed != 1 || len(sessions) != 1 {
					t.Fatalf(
						"unexpected acceptance state: before=%d after=%d granted=%d pending=%d accepted=%d sessions=%d",
						before, after, granted, pending, consumed, len(sessions),
					)
				}

				if _, _, _, replayErr := service.AcceptInvitation(
					ctx, token, "Fictional Staff", password,
					"127.0.0.1", "security-test", "replay-"+uuid.NewString(),
				); !errors.Is(replayErr, ErrInvitationNotFound) {
					t.Fatalf("replay not rejected: %v", replayErr)
				}
			} else {
				if after != before || pending != 1 ||
					consumed != 0 || len(sessions) != 0 {
					t.Fatalf(
						"rejected invitation changed state: before=%d after=%d pending=%d accepted=%d sessions=%d",
						before, after, pending, consumed, len(sessions),
					)
				}
			}

			_ = i
		})
	}

	t.Run("concurrency", func(t *testing.T) {
		testStaffAcceptanceConcurrency(t, pool, repo, service, password, passwordHash)
	})
}
