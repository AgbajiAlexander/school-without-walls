package tenants

import (
	"context"
	"errors"
	"fmt"

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

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin staff invitation: %w", err)
	}
	defer tx.Rollback(ctx)

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

	var allowed bool
	err = tx.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT 1
            FROM memberships m
            JOIN users u ON u.id = m.user_id
            WHERE m.tenant_id = $1
              AND m.user_id = $2
              AND m.role = 'SCHOOL_OWNER'
              AND m.status = 'ACTIVE'
              AND u.status = 'ACTIVE'
        )
    `, tenantID, actorID).Scan(&allowed)

	if err != nil {
		return "", fmt.Errorf("authorize staff invitation: %w", err)
	}
	if !allowed {
		return "", ErrForbidden
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
              AND i.expires_at > CURRENT_TIMESTAMP
        )
    `, tenantID, invitation.InvitedEmail, invitation.TargetRole).Scan(&conflict)

	if err != nil {
		return "", fmt.Errorf("check staff invitation conflict: %w", err)
	}
	if conflict {
		return "", ErrConflict
	}

	invitation.TenantID = &tenantID
	invitation.CreatedBy = &actorID

	_, err = tx.Exec(ctx, `
        INSERT INTO invitations (
            id, tenant_id, invited_email, target_role,
            token_hash, status, expires_at, created_by, created_at
        )
        VALUES ($1, $2, $3, $4, $5, 'AWAITING_DELIVERY', $6, $7, $8)
    `, invitation.ID, tenantID, invitation.InvitedEmail,
		invitation.TargetRole, invitation.TokenHash,
		invitation.ExpiresAt, actorID, invitation.CreatedAt)

	if err != nil {
		return "", fmt.Errorf("insert staff invitation: %w", err)
	}

	event.TenantID = &tenantID
	event.ActorID = &actorID
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

// ConfirmStaffInvitationDelivery makes a still-valid staff invitation redeemable.
func (r *postgresRepository) ConfirmStaffInvitationDelivery(ctx context.Context, invitationID, tenantID uuid.UUID) error {
	command, err := r.pool.Exec(ctx, `
		UPDATE invitations SET status = 'PENDING'
		WHERE id = $1 AND tenant_id = $2
		  AND target_role IN ('SCHOOL_ADMIN', 'PRINCIPAL')
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
          AND target_role IN ('SCHOOL_ADMIN', 'PRINCIPAL')
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
