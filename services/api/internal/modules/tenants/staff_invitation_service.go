package tenants

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/modules/auth"
)

const staffInvitationCreateAction = "tenant.staff_invitation.create"

// StaffInvitationRepository supplies tenant-authorized, audited operations.
type StaffInvitationRepository interface {
	CreateStaffInvitation(
		context.Context,
		string,
		uuid.UUID,
		*auth.Invitation,
		*auth.AuditEvent,
	) (string, error)

	ConfirmStaffInvitationDelivery(context.Context, uuid.UUID, uuid.UUID) error

	RevokeStaffInvitation(
		context.Context,
		uuid.UUID,
		uuid.UUID,
		*auth.AuditEvent,
	) error
}

// StaffInvitationMailer must send a staff-specific invitation,
// never the existing School Owner onboarding message.
type StaffInvitationMailer interface {
	SendStaffInvitation(
		context.Context,
		string,
		string,
		string,
		string,
		time.Time,
	) error
}

type StaffInvitationService struct {
	repo   StaffInvitationRepository
	mailer StaffInvitationMailer
}

type StaffInvitationResult struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewStaffInvitationService(
	repo StaffInvitationRepository,
	mailer StaffInvitationMailer,
) *StaffInvitationService {
	return &StaffInvitationService{
		repo:   repo,
		mailer: mailer,
	}
}

func (s *StaffInvitationService) InviteStaff(
	ctx context.Context,
	actorID uuid.UUID,
	slug string,
	request StaffInvitationRequest,
	requestID string,
) (*StaffInvitationResult, error) {
	if actorID == uuid.Nil {
		return nil, ErrForbidden
	}

	normalizedSlug, normalizedRequest, err :=
		validateStaffInvitationInput(slug, request)
	if err != nil {
		return nil, err
	}

	if s.repo == nil || s.mailer == nil {
		return nil, errors.New(
			"staff invitation infrastructure is not configured",
		)
	}

	rawToken, err := auth.GenerateSecureToken(32)
	if err != nil {
		return nil, fmt.Errorf(
			"generate staff invitation token: %w", err,
		)
	}

	now := time.Now().UTC()

	invitation := &auth.Invitation{
		ID:           uuid.New(),
		InvitedEmail: normalizedRequest.Email,
		TargetRole:   normalizedRequest.Role,
		TokenHash:    auth.HashToken(rawToken),
		Status:       auth.InvitationStatusPending,
		ExpiresAt:    now.Add(7 * 24 * time.Hour),
		CreatedAt:    now,
	}

	event := &auth.AuditEvent{
		Action:    staffInvitationCreateAction,
		ActorID:   &actorID,
		RequestID: requestID,
		MetadataRedacted: map[string]any{
			"target_role": normalizedRequest.Role,
		},
	}

	schoolName, err := s.repo.CreateStaffInvitation(
		ctx, normalizedSlug, actorID, invitation, event,
	)
	if err != nil {
		return nil, err
	}

	if invitation.TenantID == nil {
		return nil, errors.New(
			"staff invitation missing tenant after creation",
		)
	}

	return s.deliverInvitation(ctx, invitation, actorID, requestID, rawToken, schoolName, "tenant.staff_invitation.delivery_failed")
}

func (s *StaffInvitationService) deliverInvitation(ctx context.Context, invitation *auth.Invitation, actorID uuid.UUID, requestID, rawToken, schoolName, failureAction string) (*StaffInvitationResult, error) {
	err := s.mailer.SendStaffInvitation(
		ctx,
		invitation.InvitedEmail,
		rawToken,
		schoolName,
		invitation.TargetRole,
		invitation.ExpiresAt,
	)

	if err != nil {
		failureEvent := &auth.AuditEvent{
			Action:    failureAction,
			ActorID:   &actorID,
			RequestID: requestID,
			MetadataRedacted: map[string]any{
				"invitation_id": invitation.ID.String(),
			},
		}

		cleanupCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			5*time.Second,
		)
		defer cancel()

		revokeErr := s.repo.RevokeStaffInvitation(
			cleanupCtx,
			invitation.ID,
			*invitation.TenantID,
			failureEvent,
		)

		if revokeErr != nil {
			return nil, fmt.Errorf(
				"staff invitation delivery and revocation failed: %w",
				errors.Join(err, revokeErr),
			)
		}

		return nil, errors.New(
			"staff invitation delivery failed; invitation revoked",
		)
	}

	// Mail handoff and the database are separate systems. Complete the database
	// transition even if the request disconnects after a successful handoff.
	confirmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.repo.ConfirmStaffInvitationDelivery(confirmCtx, invitation.ID, *invitation.TenantID); err != nil {
		return nil, fmt.Errorf("staff invitation delivery could not be confirmed: %w", err)
	}

	return &StaffInvitationResult{
		ID:        invitation.ID,
		Email:     invitation.InvitedEmail,
		Role:      invitation.TargetRole,
		ExpiresAt: invitation.ExpiresAt,
	}, nil
}
