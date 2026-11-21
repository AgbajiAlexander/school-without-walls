package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Lock the target after the tenant, user and membership locks. The guardian
// address must still match the address approved when the invitation was sent.
func lockInvitationProfile(ctx context.Context, tx pgx.Tx, invitation Invitation) (uuid.UUID, error) {
	var query string
	switch invitation.TargetRole {
	case "STUDENT":
		query = `
            SELECT s.id FROM invitations i
            JOIN students s ON s.tenant_id = i.tenant_id AND s.id = i.student_id
            WHERE i.id = $1 AND s.status = 'ACTIVE' AND s.user_id IS NULL
            FOR UPDATE OF s`
	case "PARENT":
		query = `
            SELECT g.id FROM invitations i
            JOIN guardians g ON g.tenant_id = i.tenant_id AND g.id = i.guardian_id
            WHERE i.id = $1 AND g.contact_email = i.invited_email AND g.user_id IS NULL
            FOR UPDATE OF g`
	default:
		return uuid.Nil, nil
	}

	var id uuid.UUID
	err := tx.QueryRow(ctx, query, invitation.ID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrInvitationNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lock invited school profile: %w", err)
	}
	return id, nil
}

func bindInvitationProfile(ctx context.Context, tx pgx.Tx, role string, tenantID, profileID, userID uuid.UUID) error {
	query := `UPDATE students SET user_id = $3, updated_at = clock_timestamp()
        WHERE tenant_id = $1 AND id = $2 AND user_id IS NULL`
	if role == "PARENT" {
		query = `UPDATE guardians SET user_id = $3, updated_at = clock_timestamp()
            WHERE tenant_id = $1 AND id = $2 AND user_id IS NULL`
	}
	result, err := tx.Exec(ctx, query, tenantID, profileID, userID)
	if err != nil {
		return fmt.Errorf("bind invited school profile: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrInvitationNotFound
	}
	return nil
}
