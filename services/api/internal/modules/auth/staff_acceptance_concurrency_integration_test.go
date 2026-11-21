package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testStaffAcceptanceConcurrency(t *testing.T, pool *pgxpool.Pool, repo Repository, service *Service, password, passwordHash string) {
	cases := []struct {
		name       string
		seedRole   string
		lockSQL    string
		waitFor    string
		expire     bool
		inverted   bool
		wantRoles  int
		wantTarget int
		wantPaused int
	}{
		{
			name: "matching membership created while acceptance waits",
			lockSQL: `INSERT INTO memberships (id, tenant_id, user_id, role, status)
				VALUES ($3, $1, $2, 'PRINCIPAL', 'ACTIVE')`,
			waitFor: "FROM tenants", wantRoles: 1, wantTarget: 1,
		},
		{
			name: "suspended different role created while acceptance waits",
			lockSQL: `INSERT INTO memberships (id, tenant_id, user_id, role, status)
				VALUES ($3, $1, $2, 'TEACHER', 'SUSPENDED')`,
			waitFor: "FROM tenants", wantRoles: 1, wantPaused: 1,
		},
		{
			name:     "existing membership suspended while acceptance waits",
			seedRole: "TEACHER",
			lockSQL: `UPDATE memberships SET status = 'SUSPENDED'
				WHERE tenant_id = $1 AND user_id = $2 AND id = $3`,
			waitFor: "FROM memberships", wantRoles: 1, wantPaused: 1,
		},
		{
			name:    "invitation expires while acceptance waits for tenant lock",
			lockSQL: `SELECT id FROM tenants WHERE id = $1 FOR UPDATE`,
			waitFor: "FROM tenants", expire: true,
		},
		{
			name:     "inverted membership writer cannot commit together with acceptance",
			seedRole: "TEACHER",
			lockSQL: `UPDATE memberships SET status='SUSPENDED'
				WHERE tenant_id=$1 AND user_id=$2 AND id=$3`,
			waitFor: "FROM memberships", inverted: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
			defer cancel()
			tenantID, userID, memberID := uuid.New(), uuid.New(), uuid.New()
			email := "staff-concurrency-" + userID.String() + "@school.invalid"
			exec := func(query string, args ...any) {
				t.Helper()
				if _, err := pool.Exec(ctx, query, args...); err != nil {
					t.Fatal(err)
				}
			}
			exec(`INSERT INTO tenants (id, slug, name, status)
				VALUES ($1, $2, 'Fictional Concurrency School', 'ACTIVE')`, tenantID, "staff-race-"+tenantID.String())
			exec(`INSERT INTO users (id, email, password_hash, full_name, status)
				VALUES ($1, $2, $3, 'Fictional Staff', 'ACTIVE')`, userID, email, passwordHash)
			if tc.seedRole != "" {
				exec(`INSERT INTO memberships (id, tenant_id, user_id, role, status)
					VALUES ($1, $2, $3, $4, 'ACTIVE')`, memberID, tenantID, userID, tc.seedRole)
			}
			token := uuid.NewString()
			invitation := &Invitation{
				ID: uuid.New(), TenantID: &tenantID, InvitedEmail: email, TargetRole: "PRINCIPAL",
				TokenHash: HashToken(token), Status: InvitationStatusPending, ExpiresAt: time.Now().Add(time.Hour),
			}
			if err := repo.CreateInvitation(ctx, invitation); err != nil {
				t.Fatal(err)
			}

			blocker, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(context.Background())
			lockArgs := []any{tenantID}
			if !tc.expire {
				lockArgs = append(lockArgs, userID, memberID)
			}
			if _, err := blocker.Exec(ctx, tc.lockSQL, lockArgs...); err != nil {
				t.Fatal(err)
			}
			var expiresAt time.Time
			if tc.expire {
				// Set the deadline with the database clock and outside the blocking transaction.
				if err := pool.QueryRow(ctx, `UPDATE invitations
					SET expires_at = clock_timestamp() + interval '8 seconds'
					WHERE id = $1 RETURNING expires_at`, invitation.ID).Scan(&expiresAt); err != nil {
					t.Fatal(err)
				}
			}

			pid := int32(blocker.Conn().PgConn().PID())
			requestID := "staff-race-" + uuid.NewString()
			result := make(chan error, 1)
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _, _, err := service.AcceptInvitation(ctx, token, "Fictional Staff", password, "127.0.0.1", "security-test", requestID)
				result <- err
			}()
			defer func() {
				cancel()
				<-done
			}()

			waitForStaffAcceptanceLock(t, ctx, pool, result, pid, tc.waitFor, expiresAt)
			if tc.expire {
				for {
					var expired bool
					if err := pool.QueryRow(ctx, "SELECT clock_timestamp() >= $1::timestamptz", expiresAt).Scan(&expired); err != nil {
						t.Fatal(err)
					}
					if expired {
						break
					}
					select {
					case err := <-result:
						t.Fatalf("acceptance finished before the lock was released: %v", err)
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					case <-time.After(25 * time.Millisecond):
					}
				}
			}
			writerAborted := false
			if tc.inverted {
				// Deliberately take the tenant lock after the membership lock.
				if _, err := blocker.Exec(ctx, "SELECT id FROM tenants WHERE id=$1 FOR UPDATE", tenantID); err != nil {
					var pgErr *pgconn.PgError
					if !errors.As(err, &pgErr) || pgErr.Code != "40P01" {
						t.Fatal(err)
					}
					writerAborted = true
					if err := blocker.Rollback(ctx); err != nil {
						t.Fatal(err)
					}
				}
			}
			if !writerAborted {
				if err := blocker.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			}
			var acceptErr error
			select {
			case acceptErr = <-result:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}

			var roles, target, suspended, sessions, audits int
			var invitationPending bool
			if err := pool.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM memberships WHERE tenant_id=$1 AND user_id=$2),
				(SELECT count(*) FROM memberships WHERE tenant_id=$1 AND user_id=$2 AND role='PRINCIPAL'),
				(SELECT count(*) FROM memberships WHERE tenant_id=$1 AND user_id=$2 AND status='SUSPENDED'),
				(SELECT count(*) FROM sessions WHERE user_id=$2),
				(SELECT count(*) FROM audit_events WHERE request_id=$4),
				(SELECT status='PENDING' AND used_at IS NULL FROM invitations WHERE id=$3)`,
				tenantID, userID, invitation.ID, requestID).
				Scan(&roles, &target, &suspended, &sessions, &audits, &invitationPending); err != nil {
				t.Fatal(err)
			}
			if tc.inverted {
				if writerAborted {
					if acceptErr != nil || roles != 2 || target != 1 || suspended != 0 || sessions != 1 || audits != 1 || invitationPending {
						t.Fatal("acceptance did not preserve atomic state after the inverted writer rolled back")
					}
				} else {
					var pgErr *pgconn.PgError
					if !errors.As(acceptErr, &pgErr) || pgErr.Code != "40P01" ||
						roles != 1 || target != 0 || suspended != 1 || sessions != 0 || audits != 0 || !invitationPending {
						t.Fatal("acceptance left side effects after losing the database deadlock")
					}
				}
				return
			}
			if !errors.Is(acceptErr, ErrInvitationNotFound) || roles != tc.wantRoles || target != tc.wantTarget ||
				suspended != tc.wantPaused || sessions != 0 || audits != 0 || !invitationPending {
				t.Errorf("expected rejection with only the concurrent writer's changes: error=%v roles=%d target=%d suspended=%d sessions=%d audits=%d pending=%t",
					acceptErr, roles, target, suspended, sessions, audits, invitationPending)
			}
		})
	}
}

func waitForStaffAcceptanceLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, result <-chan error, blockerPID int32, queryFragment string, expiresAt time.Time) {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked, beforeExpiry bool
		var deadline *time.Time
		if !expiresAt.IsZero() {
			deadline = &expiresAt
		}
		if err := pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'
			  AND $1 = ANY(pg_blocking_pids(pid)) AND strpos(query, $2) > 0
		), COALESCE(clock_timestamp() < $3::timestamptz, true)`,
			blockerPID, queryFragment, deadline).Scan(&blocked, &beforeExpiry); err != nil {
			t.Fatal(err)
		}
		if !beforeExpiry {
			t.Fatal("invitation expired before the acceptance lock wait was observed")
		}
		if blocked {
			return
		}
		select {
		case err := <-result:
			t.Fatalf("acceptance finished without the expected lock wait: %v", err)
		case <-ctx.Done():
			t.Fatalf("acceptance lock wait was not observed: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}
