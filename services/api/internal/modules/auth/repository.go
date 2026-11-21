package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUserNotFound          = errors.New("user not found")
	ErrSessionNotFound       = errors.New("session not found or expired")
	ErrInvitationNotFound    = errors.New("invitation not found, expired, or already used")
	ErrPasswordResetNotFound = errors.New("password reset token not found, expired, or already used")
	ErrDuplicateEmail        = errors.New("email already in use")
)

type Repository interface {
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	CreateUser(ctx context.Context, user *User) error
	UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error
	SuspendUser(ctx context.Context, userID, actorID uuid.UUID, event *AuditEvent) error
	CountUsers(ctx context.Context) (int64, error)

	CreateSession(ctx context.Context, session *Session) error
	CreateSessionWithAudit(ctx context.Context, session *Session, event *AuditEvent) error
	GetActiveSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, *User, error)
	RevokeSession(ctx context.Context, sessionID uuid.UUID) error
	RevokeSessionWithAudit(ctx context.Context, sessionID uuid.UUID, event *AuditEvent) error
	UpdateSessionCSRF(ctx context.Context, sessionID uuid.UUID, tokenHash string) error
	RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error
	ListActiveSessions(ctx context.Context, userID uuid.UUID) ([]Session, error)
	RevokeUserSession(ctx context.Context, userID, sessionID uuid.UUID, event *AuditEvent) error

	CreateInvitation(ctx context.Context, inv *Invitation) error
	GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*Invitation, error)
	MarkInvitationUsed(ctx context.Context, id uuid.UUID) error
	AcceptInvitation(ctx context.Context, tokenHash string, user *User, password string, session *Session, event *AuditEvent) (*User, error)

	CreatePasswordReset(ctx context.Context, reset *PasswordReset) error
	CreatePasswordResetWithAudit(ctx context.Context, reset *PasswordReset, event *AuditEvent) error
	GetActivePasswordResetByTokenHash(ctx context.Context, tokenHash string) (*PasswordReset, error)
	MarkPasswordResetUsed(ctx context.Context, id uuid.UUID) error
	CompletePasswordReset(ctx context.Context, tokenHash, passwordHash string, event *AuditEvent) error
	AllowRateLimit(ctx context.Context, action, keyHash string, maxAttempts int, window time.Duration) (time.Duration, error)
	BootstrapAdmin(ctx context.Context, user *User, event *AuditEvent) error

	RecordAuditEvent(ctx context.Context, event *AuditEvent) error
}

type pgRepository struct {
	pool           *pgxpool.Pool
	rateLimitCalls atomic.Uint64
}

func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{pool: pool}
}

func (r *pgRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, email, password_hash, full_name, status, verified_at, created_at, updated_at
		FROM users
		WHERE email = $1;
	`
	var u User
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Status, &u.VerifiedAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user by email: %w", err)
	}
	return &u, nil
}

func (r *pgRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	query := `
		SELECT id, email, password_hash, full_name, status, verified_at, created_at, updated_at
		FROM users
		WHERE id = $1;
	`
	var u User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Status, &u.VerifiedAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user by id: %w", err)
	}
	return &u, nil
}

func (r *pgRepository) CreateUser(ctx context.Context, user *User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	query := `
		INSERT INTO users (id, email, password_hash, full_name, status, verified_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
	`
	_, err := r.pool.Exec(ctx, query,
		user.ID, user.Email, user.PasswordHash, user.FullName, user.Status, user.VerifiedAt, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert user: %w", err)
	}
	return nil
}

func (r *pgRepository) CreateSessionWithAudit(ctx context.Context, session *Session, event *AuditEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin session creation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := insertSession(ctx, tx, session); err != nil {
		return err
	}
	if err := insertAuditEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit session creation: %w", err)
	}
	return nil
}

func (r *pgRepository) UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	query := `
		UPDATE users
		SET password_hash = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2;
	`
	cmd, err := r.pool.Exec(ctx, query, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("failed to update user password: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *pgRepository) SuspendUser(ctx context.Context, userID, actorID uuid.UUID, event *AuditEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin account suspension: %w", err)
	}
	defer tx.Rollback(ctx)
	cmd, err := tx.Exec(ctx, `
		UPDATE users SET status = 'SUSPENDED', updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status <> 'SUSPENDED'
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to suspend user: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID); err != nil {
		return fmt.Errorf("failed to revoke sessions for suspended user: %w", err)
	}
	event.ActorID = &actorID
	event.SubjectType = "user"
	event.SubjectID = userID.String()
	if err := insertAuditEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit account suspension: %w", err)
	}
	return nil
}

func (r *pgRepository) RevokeSessionWithAudit(ctx context.Context, sessionID uuid.UUID, event *AuditEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin session revocation: %w", err)
	}
	defer tx.Rollback(ctx)
	cmd, err := tx.Exec(ctx, `
		UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND revoked_at IS NULL
	`, sessionID)
	if err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	if err := insertAuditEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit session revocation: %w", err)
	}
	return nil
}

func (r *pgRepository) UpdateSessionCSRF(ctx context.Context, sessionID uuid.UUID, tokenHash string) error {
	cmd, err := r.pool.Exec(ctx, `
		UPDATE sessions SET csrf_token_hash = $1
		WHERE id = $2 AND revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP
	`, tokenHash, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update session CSRF token: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (r *pgRepository) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM users;").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}
	return count, nil
}

func (r *pgRepository) CreateSession(ctx context.Context, s *Session) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	s.CreatedAt = time.Now().UTC()

	query := `
		INSERT INTO sessions (id, user_id, token_hash, csrf_token_hash, ip_address, user_agent, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
	`
	_, err := r.pool.Exec(ctx, query,
		s.ID, s.UserID, s.TokenHash, s.CSRFTokenHash, s.IPAddress, s.UserAgent, s.ExpiresAt, s.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert session: %w", err)
	}
	return nil
}

func (r *pgRepository) GetActiveSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, *User, error) {
	query := `
		SELECT
			s.id, s.user_id, s.token_hash, s.csrf_token_hash, s.ip_address, s.user_agent, s.expires_at, s.revoked_at, s.created_at,
			u.id, u.email, u.password_hash, u.full_name, u.status, u.verified_at, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > CURRENT_TIMESTAMP;
	`
	var s Session
	var u User
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(
		&s.ID, &s.UserID, &s.TokenHash, &s.CSRFTokenHash, &s.IPAddress, &s.UserAgent, &s.ExpiresAt, &s.RevokedAt, &s.CreatedAt,
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Status, &u.VerifiedAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrSessionNotFound
		}
		return nil, nil, fmt.Errorf("failed to query active session: %w", err)
	}
	return &s, &u, nil
}

func (r *pgRepository) RevokeSession(ctx context.Context, sessionID uuid.UUID) error {
	query := `
		UPDATE sessions
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND revoked_at IS NULL;
	`
	_, err := r.pool.Exec(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}
	return nil
}

func (r *pgRepository) RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE sessions
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND revoked_at IS NULL;
	`
	_, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke all sessions for user: %w", err)
	}
	return nil
}

func (r *pgRepository) ListActiveSessions(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, expires_at, created_at
		FROM sessions
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list active sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]Session, 0)
	for rows.Next() {
		var session Session
		if err := rows.Scan(&session.ID, &session.UserID, &session.ExpiresAt, &session.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan active session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate active sessions: %w", err)
	}
	return sessions, nil
}

func (r *pgRepository) RevokeUserSession(ctx context.Context, userID, sessionID uuid.UUID, event *AuditEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin session revocation: %w", err)
	}
	defer tx.Rollback(ctx)
	cmd, err := tx.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
	`, sessionID, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke user session: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	event.ActorID = &userID
	event.SubjectType = "session"
	event.SubjectID = sessionID.String()
	if err := insertAuditEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit session revocation: %w", err)
	}
	return nil
}

func (r *pgRepository) CreateInvitation(ctx context.Context, inv *Invitation) error {
	if inv.ID == uuid.Nil {
		inv.ID = uuid.New()
	}
	inv.CreatedAt = time.Now().UTC()

	query := `
		INSERT INTO invitations (id, tenant_id, invited_email, target_role, token_hash, status, expires_at, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
	`
	_, err := r.pool.Exec(ctx, query,
		inv.ID, inv.TenantID, inv.InvitedEmail, inv.TargetRole, inv.TokenHash, inv.Status, inv.ExpiresAt, inv.CreatedBy, inv.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create invitation: %w", err)
	}
	return nil
}

func (r *pgRepository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*Invitation, error) {
	query := `
		SELECT i.id, i.tenant_id, i.invited_email, i.target_role, i.token_hash, i.status,
			i.expires_at, i.used_at, i.created_by, i.created_at, t.slug::text, t.name
		FROM invitations i
		LEFT JOIN tenants t ON t.id = i.tenant_id
		WHERE i.token_hash = $1 AND i.status = 'PENDING' AND i.expires_at > CURRENT_TIMESTAMP;
	`
	var inv Invitation
	var tenantSlug, tenantName *string
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(
		&inv.ID, &inv.TenantID, &inv.InvitedEmail, &inv.TargetRole, &inv.TokenHash, &inv.Status,
		&inv.ExpiresAt, &inv.UsedAt, &inv.CreatedBy, &inv.CreatedAt, &tenantSlug, &tenantName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvitationNotFound
		}
		return nil, fmt.Errorf("failed to query invitation: %w", err)
	}
	if tenantSlug != nil {
		inv.TenantSlug = *tenantSlug
	}
	if tenantName != nil {
		inv.TenantName = *tenantName
	}
	return &inv, nil
}

func (r *pgRepository) MarkInvitationUsed(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE invitations
		SET status = 'ACCEPTED', used_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status = 'PENDING';
	`
	cmd, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to mark invitation used: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrInvitationNotFound
	}
	return nil
}

func (r *pgRepository) AcceptInvitation(ctx context.Context, tokenHash string, invitedUser *User, password string, session *Session, event *AuditEvent) (*User, error) {
	// Lock invitation, tenant, user, then memberships. Membership writers must
	// acquire tenant and user locks before updating membership rows.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin invitation acceptance: %w", err)
	}
	defer tx.Rollback(ctx)

	var invitation Invitation
	err = tx.QueryRow(ctx, `
		SELECT id, tenant_id, invited_email, target_role, status, expires_at
		FROM invitations
		WHERE token_hash = $1 AND status = 'PENDING' AND expires_at > clock_timestamp()
		FOR UPDATE
	`, tokenHash).Scan(&invitation.ID, &invitation.TenantID, &invitation.InvitedEmail, &invitation.TargetRole, &invitation.Status, &invitation.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvitationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lock invitation: %w", err)
	}

	if invitation.TenantID != nil {
		var tenantStatus string
		err = tx.QueryRow(ctx, `
            SELECT status::text
            FROM tenants
            WHERE id = $1
            FOR UPDATE
        `, *invitation.TenantID).Scan(&tenantStatus)

		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvitationNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("lock invited tenant status: %w", err)
		}

		switch invitation.TargetRole {
		case "SCHOOL_OWNER":
			if tenantStatus != "PENDING_SETUP" {
				return nil, ErrInvitationNotFound
			}
		case "SCHOOL_ADMIN", "PRINCIPAL", "TEACHER",
			"STUDENT", "PARENT", "BURSAR":
			if tenantStatus != "ACTIVE" {
				return nil, ErrInvitationNotFound
			}
		default:
			return nil, ErrInvitationNotFound
		}
	}

	var user User
	err = tx.QueryRow(ctx, `
		SELECT id, email, password_hash, full_name, status, verified_at, created_at, updated_at
		FROM users
		WHERE email = $1
		FOR UPDATE
	`, invitation.InvitedEmail).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.FullName, &user.Status,
		&user.VerifiedAt, &user.CreatedAt, &user.UpdatedAt,
	)
	switch {
	case err == nil:
		switch user.Status {
		case UserStatusActive:
			matches, err := VerifyPassword(password, user.PasswordHash)
			if err != nil {
				return nil, fmt.Errorf("verify existing invited account: %w", err)
			}
			if !matches {
				return nil, ErrInvalidCredentials
			}
		case UserStatusPendingActivation:
			user.FullName = invitedUser.FullName
			user.PasswordHash = invitedUser.PasswordHash
			user.Status = UserStatusActive
			now := time.Now().UTC()
			user.VerifiedAt = &now
			if _, err := tx.Exec(ctx, `
				UPDATE users SET password_hash = $1, full_name = $2, status = $3,
					verified_at = $4, updated_at = $4 WHERE id = $5
			`, user.PasswordHash, user.FullName, user.Status, now, user.ID); err != nil {
				return nil, fmt.Errorf("failed to activate invited user: %w", err)
			}
		default:
			return nil, ErrAccountSuspended
		}
	case errors.Is(err, pgx.ErrNoRows):
		user = *invitedUser
		user.Email = invitation.InvitedEmail
		if user.ID == uuid.Nil {
			user.ID = uuid.New()
		}
		now := time.Now().UTC()
		user.CreatedAt, user.UpdatedAt = now, now
		user.VerifiedAt = &now
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, email, password_hash, full_name, status, verified_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, user.ID, user.Email, user.PasswordHash, user.FullName, UserStatusActive, now, now, now); err != nil {
			return nil, fmt.Errorf("failed to create invited user: %w", err)
		}
	default:
		return nil, fmt.Errorf("failed to find invited user: %w", err)
	}
	if invitation.TenantID != nil {
		// Lock existing memberships to prevent suspended access
		// or duplicate role grants during invitation acceptance.
		rows, err := tx.Query(ctx, `
            SELECT role, status::text
            FROM memberships
            WHERE tenant_id = $1 AND user_id = $2
            FOR UPDATE
        `, *invitation.TenantID, user.ID)
		if err != nil {
			return nil, fmt.Errorf("check existing invitation membership: %w", err)
		}

		blocked := false
		for rows.Next() {
			var existingRole, existingStatus string
			if err := rows.Scan(&existingRole, &existingStatus); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan invited membership: %w", err)
			}
			if existingRole == invitation.TargetRole ||
				existingStatus == "SUSPENDED" {
				blocked = true
			}
		}
		rowErr := rows.Err()
		rows.Close()
		if rowErr != nil {
			return nil, fmt.Errorf("read invited memberships: %w", rowErr)
		}
		if blocked {
			return nil, ErrInvitationNotFound
		}
	}

	profileID, err := lockInvitationProfile(ctx, tx, invitation)
	if err != nil {
		return nil, err
	}

	// The invitation may have expired while waiting for a row lock.
	// Check the database wall clock again before granting access.
	var unexpired bool
	if err := tx.QueryRow(ctx, `SELECT $1::timestamptz > clock_timestamp()`, invitation.ExpiresAt).Scan(&unexpired); err != nil {
		return nil, fmt.Errorf("recheck invitation expiration: %w", err)
	}
	if !unexpired {
		return nil, ErrInvitationNotFound
	}

	session.UserID = user.ID
	if invitation.TenantID != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO memberships (id, tenant_id, user_id, role, status, created_at)
			VALUES ($1, $2, $3, $4, 'ACTIVE', CURRENT_TIMESTAMP)
		`, uuid.New(), invitation.TenantID, user.ID, invitation.TargetRole); err != nil {
			return nil, fmt.Errorf("failed to create invited tenant membership: %w", err)
		}
		if profileID != uuid.Nil {
			if err := bindInvitationProfile(ctx, tx, invitation.TargetRole, *invitation.TenantID, profileID, user.ID); err != nil {
				return nil, err
			}
		}
		event.TenantID = invitation.TenantID
		if invitation.TargetRole == "SCHOOL_OWNER" {
			if _, err := tx.Exec(ctx, `
				UPDATE tenants SET status = 'ACTIVE', updated_at = CURRENT_TIMESTAMP
				WHERE id = $1 AND status = 'PENDING_SETUP'
			`, invitation.TenantID); err != nil {
				return nil, fmt.Errorf("activate owner-provisioned tenant: %w", err)
			}
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE invitations SET status = 'ACCEPTED', used_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status = 'PENDING'
	`, invitation.ID); err != nil {
		return nil, fmt.Errorf("failed to consume invitation: %w", err)
	}
	if err := insertSession(ctx, tx, session); err != nil {
		return nil, err
	}
	event.ActorID = &user.ID
	event.SubjectType = "invitation"
	event.SubjectID = invitation.ID.String()
	if err := insertAuditEvent(ctx, tx, event); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit invitation acceptance: %w", err)
	}
	return &user, nil
}

func (r *pgRepository) CreatePasswordReset(ctx context.Context, reset *PasswordReset) error {
	if reset.ID == uuid.Nil {
		reset.ID = uuid.New()
	}
	reset.CreatedAt = time.Now().UTC()

	query := `
		INSERT INTO password_resets (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5);
	`
	_, err := r.pool.Exec(ctx, query, reset.ID, reset.UserID, reset.TokenHash, reset.ExpiresAt, reset.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert password reset: %w", err)
	}
	return nil
}

func (r *pgRepository) CreatePasswordResetWithAudit(ctx context.Context, reset *PasswordReset, event *AuditEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin password reset request: %w", err)
	}
	defer tx.Rollback(ctx)
	if reset.ID == uuid.Nil {
		reset.ID = uuid.New()
	}
	reset.CreatedAt = time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		INSERT INTO password_resets (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, reset.ID, reset.UserID, reset.TokenHash, reset.ExpiresAt, reset.CreatedAt); err != nil {
		return fmt.Errorf("failed to insert password reset: %w", err)
	}
	event.ActorID = &reset.UserID
	event.SubjectType = "user"
	event.SubjectID = reset.UserID.String()
	if err := insertAuditEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit password reset request: %w", err)
	}
	return nil
}

func (r *pgRepository) GetActivePasswordResetByTokenHash(ctx context.Context, tokenHash string) (*PasswordReset, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, used_at, created_at
		FROM password_resets
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP;
	`
	var reset PasswordReset
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(
		&reset.ID, &reset.UserID, &reset.TokenHash, &reset.ExpiresAt, &reset.UsedAt, &reset.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPasswordResetNotFound
		}
		return nil, fmt.Errorf("failed to query password reset: %w", err)
	}
	return &reset, nil
}

func (r *pgRepository) MarkPasswordResetUsed(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE password_resets
		SET used_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND used_at IS NULL;
	`
	cmd, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to mark password reset used: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrPasswordResetNotFound
	}
	return nil
}

func (r *pgRepository) CompletePasswordReset(ctx context.Context, tokenHash, passwordHash string, event *AuditEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin password reset: %w", err)
	}
	defer tx.Rollback(ctx)

	var reset PasswordReset
	err = tx.QueryRow(ctx, `
		SELECT id, user_id FROM password_resets
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP
		FOR UPDATE
	`, tokenHash).Scan(&reset.ID, &reset.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPasswordResetNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to lock password reset: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users SET password_hash = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2
	`, passwordHash, reset.UserID); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE password_resets SET used_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND used_at IS NULL
	`, reset.UserID); err != nil {
		return fmt.Errorf("failed to consume outstanding password resets: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND revoked_at IS NULL
	`, reset.UserID); err != nil {
		return fmt.Errorf("failed to revoke sessions after password reset: %w", err)
	}
	event.ActorID = &reset.UserID
	event.SubjectType = "user"
	event.SubjectID = reset.UserID.String()
	if err := insertAuditEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit password reset: %w", err)
	}
	return nil
}

func (r *pgRepository) AllowRateLimit(ctx context.Context, action, keyHash string, maxAttempts int, window time.Duration) (time.Duration, error) {
	var attempts int
	var windowStarted time.Time
	var blockedUntil *time.Time
	err := r.pool.QueryRow(ctx, `
		INSERT INTO auth_rate_limits (action, key_hash, window_started_at, attempt_count, updated_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1, CURRENT_TIMESTAMP)
		ON CONFLICT (action, key_hash) DO UPDATE SET
			window_started_at = CASE
				WHEN auth_rate_limits.window_started_at + ($3 * INTERVAL '1 second') <= CURRENT_TIMESTAMP THEN CURRENT_TIMESTAMP
				ELSE auth_rate_limits.window_started_at END,
			attempt_count = CASE
				WHEN auth_rate_limits.window_started_at + ($3 * INTERVAL '1 second') <= CURRENT_TIMESTAMP THEN 1
				ELSE LEAST(auth_rate_limits.attempt_count + 1, $4 + 1) END,
			blocked_until = CASE
				WHEN auth_rate_limits.window_started_at + ($3 * INTERVAL '1 second') <= CURRENT_TIMESTAMP THEN NULL
				WHEN auth_rate_limits.attempt_count + 1 > $4
					THEN auth_rate_limits.window_started_at + ($3 * INTERVAL '1 second')
				ELSE auth_rate_limits.blocked_until END,
			updated_at = CURRENT_TIMESTAMP
		RETURNING attempt_count, window_started_at, blocked_until
	`, action, keyHash, window.Seconds(), maxAttempts).Scan(&attempts, &windowStarted, &blockedUntil)
	if err != nil {
		return 0, fmt.Errorf("failed to update authentication rate limit: %w", err)
	}
	if r.rateLimitCalls.Add(1)%100 == 0 {
		if _, err := r.pool.Exec(ctx, `
			DELETE FROM auth_rate_limits
			WHERE updated_at < CURRENT_TIMESTAMP - INTERVAL '2 days'
		`); err != nil {
			return 0, fmt.Errorf("failed to prune expired authentication rate limits: %w", err)
		}
	}
	if attempts <= maxAttempts || blockedUntil == nil || !blockedUntil.After(time.Now()) {
		return 0, nil
	}
	return time.Until(*blockedUntil), nil
}

func (r *pgRepository) BootstrapAdmin(ctx context.Context, user *User, event *AuditEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin administrator bootstrap: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(735104223)`); err != nil {
		return fmt.Errorf("failed to lock administrator bootstrap: %w", err)
	}
	var count int64
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return fmt.Errorf("failed to check existing users: %w", err)
	}
	if count != 0 {
		return ErrAdminAlreadyExists
	}
	now := time.Now().UTC()
	user.ID = uuid.New()
	user.Status = UserStatusActive
	user.VerifiedAt = &now
	user.CreatedAt, user.UpdatedAt = now, now
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, full_name, status, verified_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, user.ID, user.Email, user.PasswordHash, user.FullName, user.Status, now, now, now); err != nil {
		return fmt.Errorf("failed to create initial administrator: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO platform_admins (user_id) VALUES ($1)`, user.ID); err != nil {
		return fmt.Errorf("failed to grant initial platform administrator access: %w", err)
	}
	event.ActorID = &user.ID
	event.SubjectType = "user"
	event.SubjectID = user.ID.String()
	if err := insertAuditEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit administrator bootstrap: %w", err)
	}
	return nil
}

func (r *pgRepository) RecordAuditEvent(ctx context.Context, event *AuditEvent) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	event.CreatedAt = time.Now().UTC()

	metaJSON, err := json.Marshal(event.MetadataRedacted)
	if err != nil {
		return fmt.Errorf("failed to encode audit metadata: %w", err)
	}

	query := `
		INSERT INTO audit_events (id, tenant_id, actor_id, action, subject_type, subject_id, request_id, metadata_redacted, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
	`
	_, err = r.pool.Exec(ctx, query,
		event.ID, event.TenantID, event.ActorID, event.Action, event.SubjectType, event.SubjectID, event.RequestID, metaJSON, event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to record audit event: %w", err)
	}
	return nil
}

type authTx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertSession(ctx context.Context, tx authTx, session *Session) error {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	session.CreatedAt = time.Now().UTC()
	_, err := tx.Exec(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, csrf_token_hash, ip_address, user_agent, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, session.ID, session.UserID, session.TokenHash, session.CSRFTokenHash, session.IPAddress,
		session.UserAgent, session.ExpiresAt, session.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

func insertAuditEvent(ctx context.Context, tx authTx, event *AuditEvent) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	event.CreatedAt = time.Now().UTC()
	metaJSON, err := json.Marshal(event.MetadataRedacted)
	if err != nil {
		return fmt.Errorf("failed to encode audit metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id, tenant_id, actor_id, action, subject_type, subject_id, request_id, metadata_redacted, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, event.ID, event.TenantID, event.ActorID, event.Action, event.SubjectType, event.SubjectID, event.RequestID, metaJSON, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to record audit event: %w", err)
	}
	return nil
}
