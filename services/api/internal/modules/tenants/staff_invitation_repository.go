package tenants

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/school-without-walls/api/internal/modules/auth"
)

// CreateStaffInvitation atomically verifies the active school owner,
// detects conflicting invitations or memberships, and records an audit event.
//
// Locking the tenant row serializes competing invitation creations for a
// particular school. The caller must deliver the message separately.
func (r *postgresRepository) CreateStaffInvitation(
	ctx context.Context,
	slug string,
	actorID uuid.UUID,
	invitation *auth.Invitation,
	event *auth.AuditEvent,
) (string, error) {
	if actorID == uuid.Nil || invitation == nil || event == nil {
		return "", ErrInvalidInput
	}
	if invitation.TargetRole != "SCHOOL_ADMIN" &&
		invitation.TargetRole != "PRINCIPAL" {
		return "", ErrInvalidInput
	}

	return r.createManagedInvitation(ctx, slug, actorID, invitation, event, uuid.Nil)
}

func (r *postgresRepository) CreateProfileInvitation(ctx context.Context, slug string, actorID, profileID uuid.UUID, invitation *auth.Invitation, event *auth.AuditEvent) (string, error) {
	if actorID == uuid.Nil || profileID == uuid.Nil || invitation == nil || event == nil ||
		(invitation.TargetRole != "STUDENT" && invitation.TargetRole != "PARENT") {
		return "", ErrInvalidInput
	}
	return r.createManagedInvitation(ctx, slug, actorID, invitation, event, profileID)
}

func (r *postgresRepository) createManagedInvitation(ctx context.Context, slug string, actorID uuid.UUID, invitation *auth.Invitation, event *auth.AuditEvent, profileID uuid.UUID) (string, error) {
	isProfile := profileID != uuid.Nil
	// Quota and duplicate checks need fresh snapshots after the tenant lock.
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return "", fmt.Errorf("begin staff invitation: %w", err)
	}
	defer tx.Rollback(ctx)

	// Reject obvious denials without taking another school's issuance lock.
	// The locked reads below remain authoritative.
	var eligible bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM tenants t
			JOIN memberships m ON m.tenant_id = t.id
			JOIN users u ON u.id = m.user_id
			WHERE t.slug = $1::citext AND t.status = 'ACTIVE' AND m.user_id = $2
			  AND (m.role = 'SCHOOL_OWNER' OR ($3 AND m.role IN ('SCHOOL_ADMIN', 'PRINCIPAL')))
			  AND m.status = 'ACTIVE' AND u.status = 'ACTIVE'
		)
	`, slug, actorID, isProfile).Scan(&eligible)
	if err != nil {
		return "", fmt.Errorf("precheck staff invitation owner: %w", err)
	}
	if !eligible {
		return "", ErrForbidden
	}

	var tenantID uuid.UUID
	var tenantName string

	err = tx.QueryRow(ctx, `
        SELECT id, name
        FROM tenants
        WHERE slug = $1::citext AND status = 'ACTIVE'
        FOR UPDATE
    `, slug).Scan(&tenantID, &tenantName)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	if err != nil {
		return "", fmt.Errorf("find active invitation tenant: %w", err)
	}

	// Lock tenant -> actor -> owner membership. Status updates that win either
	// row lock are observed here. Later updates wait for issuance to commit.
	var lockedID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id FROM users WHERE id = $1 AND status = 'ACTIVE' FOR SHARE
	`, actorID).Scan(&lockedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	if err != nil {
		return "", fmt.Errorf("lock staff invitation actor: %w", err)
	}
	err = tx.QueryRow(ctx, `
		SELECT id FROM memberships
		WHERE tenant_id = $1 AND user_id = $2 AND status = 'ACTIVE'
		  AND (role = 'SCHOOL_OWNER' OR ($3 AND role IN ('SCHOOL_ADMIN', 'PRINCIPAL')))
		ORDER BY id LIMIT 1 FOR SHARE
	`, tenantID, actorID, isProfile).Scan(&lockedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	if err != nil {
		return "", fmt.Errorf("lock staff invitation owner membership: %w", err)
	}

	var studentID, guardianID *uuid.UUID
	if isProfile {
		if invitation.TargetRole == "STUDENT" {
			err = tx.QueryRow(ctx, `SELECT id FROM students
				WHERE tenant_id = $1 AND id = $2 AND status = 'ACTIVE' AND user_id IS NULL
				FOR SHARE`, tenantID, profileID).Scan(&lockedID)
			studentID = &profileID
		} else {
			err = tx.QueryRow(ctx, `SELECT contact_email FROM guardians
				WHERE tenant_id = $1 AND id = $2 AND user_id IS NULL AND contact_email IS NOT NULL
				FOR SHARE`, tenantID, profileID).Scan(&invitation.InvitedEmail)
			guardianID = &profileID
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		if err != nil {
			return "", fmt.Errorf("lock invitation profile: %w", err)
		}
		email, err := normalizeInvitationEmail(invitation.InvitedEmail)
		if err != nil {
			return "", err
		}
		invitation.InvitedEmail = email

		var pending bool
		err = tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM invitations WHERE tenant_id = $1
			  AND (student_id = $2 OR guardian_id = $3)
			  AND status IN ('AWAITING_DELIVERY', 'PENDING') AND expires_at > statement_timestamp()
		)`, tenantID, studentID, guardianID).Scan(&pending)
		if err != nil {
			return "", fmt.Errorf("check pending profile invitation: %w", err)
		}
		if pending {
			return "", ErrConflict
		}
	}

	var conflict bool
	err = tx.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT 1
            FROM memberships m
            JOIN users u ON u.id = m.user_id
            WHERE m.tenant_id = $1
              AND u.email = $2::citext
              AND m.role = $3
        )
        OR EXISTS (
            SELECT 1
            FROM invitations i
            WHERE i.tenant_id = $1
              AND i.invited_email = $2::citext
              AND i.target_role = $3
              AND i.status IN ('AWAITING_DELIVERY', 'PENDING')
              AND i.expires_at > statement_timestamp()
        )
    `, tenantID, invitation.InvitedEmail, invitation.TargetRole).Scan(&conflict)

	if err != nil {
		return "", fmt.Errorf("check staff invitation conflict: %w", err)
	}
	if conflict {
		return "", ErrConflict
	}

	issuedAt, err := r.checkStaffInvitationQuota(ctx, tx, tenantID, actorID, nil)
	if err != nil {
		return "", err
	}
	invitation.TenantID = &tenantID
	invitation.CreatedBy = &actorID

	if isProfile {
		_, err = tx.Exec(ctx, `
			INSERT INTO invitations (id, tenant_id, invited_email, target_role, token_hash, status,
				expires_at, created_by, created_at, student_id, guardian_id)
			VALUES ($1,$2,$3,$4,$5,'AWAITING_DELIVERY',$6,$7,$8,$9,$10)
		`, invitation.ID, tenantID, invitation.InvitedEmail, invitation.TargetRole, invitation.TokenHash,
			invitation.ExpiresAt, actorID, invitation.CreatedAt, studentID, guardianID)
	} else {
		_, err = tx.Exec(ctx, `
        INSERT INTO invitations (
            id, tenant_id, invited_email, target_role,
            token_hash, status, expires_at, created_by, created_at
        )
        VALUES ($1, $2, $3, $4, $5, 'AWAITING_DELIVERY', $6, $7, $8)
    `, invitation.ID, tenantID, invitation.InvitedEmail,
			invitation.TargetRole, invitation.TokenHash,
			invitation.ExpiresAt, actorID, invitation.CreatedAt)
	}

	if err != nil {
		return "", fmt.Errorf("insert staff invitation: %w", err)
	}

	event.TenantID = &tenantID
	event.ActorID = &actorID
	event.Action = staffInvitationCreateAction
	if isProfile {
		event.Action = profileInvitationCreateAction
	}
	event.CreatedAt = issuedAt
	event.SubjectType = "invitation"
	event.SubjectID = invitation.ID.String()

	if err := writeAuditEvent(ctx, tx, event); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit staff invitation: %w", err)
	}

	return tenantName, nil
}

// checkStaffInvitationQuota runs under the tenant lock. The issuance audit row
// is the quota charge and commits atomically with the invitation.
func (r *postgresRepository) checkStaffInvitationQuota(ctx context.Context, tx pgx.Tx, tenantID, actorID uuid.UUID, asOf *time.Time) (time.Time, error) {
	limits := r.staffLimits
	var now time.Time
	var tenantBoundary, actorBoundary *time.Time
	err := tx.QueryRow(ctx, `
		WITH quota_clock AS (SELECT COALESCE($7::timestamptz, statement_timestamp()) AS now),
		recent AS MATERIALIZED (
			SELECT created_at, actor_id FROM audit_events CROSS JOIN quota_clock
			WHERE tenant_id = $1 AND action IN ($6, 'tenant.profile_invitation.create')
			  AND created_at > quota_clock.now - make_interval(secs => $3)
		)
		SELECT quota_clock.now,
			(SELECT created_at FROM recent ORDER BY created_at DESC OFFSET $4 LIMIT 1),
			(SELECT created_at FROM recent WHERE actor_id = $2 ORDER BY created_at DESC OFFSET $5 LIMIT 1)
		FROM quota_clock
	`, tenantID, actorID, limits.Window.Seconds(), limits.PerTenant-1, limits.PerActor-1, staffInvitationCreateAction, asOf).
		Scan(&now, &tenantBoundary, &actorBoundary)
	if err != nil {
		return time.Time{}, fmt.Errorf("check staff invitation quota: %w", err)
	}
	var retryAfter time.Duration
	for _, boundary := range []*time.Time{tenantBoundary, actorBoundary} {
		if boundary != nil {
			retryAfter = max(retryAfter, boundary.Add(limits.Window).Sub(now))
		}
	}
	if retryAfter > 0 {
		return time.Time{}, &staffInvitationRateLimitError{retryAfter: retryAfter}
	}
	return now, nil
}

type staffInvitationRateLimitError struct{ retryAfter time.Duration }

func (*staffInvitationRateLimitError) Error() string { return "staff invitation quota exceeded" }

// ConfirmStaffInvitationDelivery makes a still-valid staff invitation redeemable.
func (r *postgresRepository) ConfirmStaffInvitationDelivery(ctx context.Context, invitationID, tenantID uuid.UUID) error {
	command, err := r.pool.Exec(ctx, `
		UPDATE invitations SET status = 'PENDING'
		WHERE id = $1 AND tenant_id = $2
		  AND target_role IN ('SCHOOL_ADMIN', 'PRINCIPAL', 'STUDENT', 'PARENT')
		  AND status = 'AWAITING_DELIVERY' AND expires_at > clock_timestamp()
	`, invitationID, tenantID)
	if err != nil {
		return fmt.Errorf("confirm staff invitation delivery: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

// RevokeStaffInvitation cleans up an unconfirmed invitation after delivery failure.
func (r *postgresRepository) RevokeStaffInvitation(
	ctx context.Context,
	invitationID uuid.UUID,
	tenantID uuid.UUID,
	event *auth.AuditEvent,
) error {
	if invitationID == uuid.Nil || tenantID == uuid.Nil || event == nil {
		return ErrInvalidInput
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin staff invitation revocation: %w", err)
	}
	defer tx.Rollback(ctx)

	command, err := tx.Exec(ctx, `
        UPDATE invitations
        SET status = 'REVOKED'
        WHERE id = $1
          AND tenant_id = $2
          AND status = 'AWAITING_DELIVERY'
          AND target_role IN ('SCHOOL_ADMIN', 'PRINCIPAL', 'STUDENT', 'PARENT')
    `, invitationID, tenantID)

	if err != nil {
		return fmt.Errorf("revoke staff invitation: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrConflict
	}

	event.TenantID = &tenantID
	event.SubjectType = "invitation"
	event.SubjectID = invitationID.String()

	if err := writeAuditEvent(ctx, tx, event); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit staff invitation revocation: %w", err)
	}
	return nil
}
