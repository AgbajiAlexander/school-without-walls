package tenants

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/middleware"
	apphttp "github.com/school-without-walls/api/internal/platform/response"
)

const profileInvitationCreateAction = "tenant.profile_invitation.create"

type profileInvitationRepository interface {
	CreateProfileInvitation(context.Context, string, uuid.UUID, uuid.UUID, *auth.Invitation, *auth.AuditEvent) (string, error)
}

func (h *StaffInvitationHandler) CreateStudent(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email string `json:"email"`
	}
	if err := decodeTenantRequest(w, r, &request); err != nil {
		profileInvitationError(w, r, ErrInvalidInput)
		return
	}
	h.createProfile(w, r, "studentID", "STUDENT", request.Email)
}

func (h *StaffInvitationHandler) CreateGuardian(w http.ResponseWriter, r *http.Request) {
	if err := decodeTenantRequest(w, r, &struct{}{}); err != nil {
		profileInvitationError(w, r, ErrInvalidInput)
		return
	}
	h.createProfile(w, r, "guardianID", "PARENT", "")
}

func (h *StaffInvitationHandler) createProfile(w http.ResponseWriter, r *http.Request, param, role, email string) {
	actor, ok := tenantActor(w, r)
	if !ok {
		return
	}
	profileID, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil || profileID == uuid.Nil {
		profileInvitationError(w, r, ErrInvalidInput)
		return
	}
	result, err := h.service.inviteProfile(r.Context(), actor.ID, chi.URLParam(r, "slug"),
		profileID, role, email, middleware.GetRequestID(r.Context()))
	if err != nil {
		profileInvitationError(w, r, err)
		return
	}
	apphttp.JSON(w, r, http.StatusCreated, struct {
		ID        uuid.UUID `json:"id"`
		ExpiresAt time.Time `json:"expires_at"`
	}{ID: result.ID, ExpiresAt: result.ExpiresAt})
}

func (s *StaffInvitationService) inviteProfile(ctx context.Context, actor uuid.UUID, slug string, profile uuid.UUID,
	role, email, requestID string) (*StaffInvitationResult, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if len(slug) > 80 || !slugPattern.MatchString(slug) {
		return nil, ErrInvalidInput
	}
	if role == "STUDENT" {
		var err error
		email, err = normalizeInvitationEmail(email)
		if err != nil {
			return nil, err
		}
	}
	repo, ok := s.repo.(profileInvitationRepository)
	if !ok || s.mailer == nil {
		return nil, errors.New("profile invitation unavailable")
	}
	token, err := auth.GenerateSecureToken(32)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	invitation := &auth.Invitation{
		ID: uuid.New(), InvitedEmail: email, TargetRole: role, TokenHash: auth.HashToken(token),
		CreatedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
	}
	event := &auth.AuditEvent{
		RequestID: requestID, MetadataRedacted: map[string]any{"target_role": role},
	}
	schoolName, err := repo.CreateProfileInvitation(ctx, slug, actor, profile, invitation, event)
	if err != nil {
		return nil, err
	}
	return s.deliverInvitation(ctx, invitation, actor, requestID, token, schoolName, "tenant.profile_invitation.delivery_failed")
}

func profileInvitationError(w http.ResponseWriter, r *http.Request, err error) {
	var limited *staffInvitationRateLimitError
	switch {
	case errors.As(err, &limited):
		seconds := int((limited.retryAfter + time.Second - 1) / time.Second)
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		apphttp.ErrorJSON(w, r, http.StatusTooManyRequests, apphttp.ErrCodeRateLimited, "Invitation limit reached. Try again later.", nil)
	case errors.Is(err, ErrInvalidInput):
		apphttp.ErrorJSON(w, r, http.StatusUnprocessableEntity, apphttp.ErrCodeInvalidInput, "Invalid account invitation details", nil)
	case errors.Is(err, ErrForbidden):
		apphttp.ErrorJSON(w, r, http.StatusForbidden, apphttp.ErrCodeForbidden, "Access denied", nil)
	case errors.Is(err, ErrNotFound):
		apphttp.ErrorJSON(w, r, http.StatusNotFound, apphttp.ErrCodeNotFound, "Profile is not available for invitation", nil)
	case errors.Is(err, ErrConflict):
		apphttp.ErrorJSON(w, r, http.StatusConflict, "PROFILE_INVITATION_CONFLICT", "An invitation or membership already exists", nil)
	default:
		slog.Error("profile invitation failed", "request_id", middleware.GetRequestID(r.Context()))
		apphttp.ErrorJSON(w, r, http.StatusInternalServerError, apphttp.ErrCodeInternalError, "Unable to issue account invitation", nil)
	}
}
