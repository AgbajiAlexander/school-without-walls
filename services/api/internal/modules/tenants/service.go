package tenants

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/modules/auth"
)

var (
	slugPattern   = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	localePattern = regexp.MustCompile(`^[a-z]{2}(?:-[A-Z]{2})?$`)
	colorPattern  = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	stageSet      = map[string]bool{
		"EARLY_YEARS": true, "PRIMARY": true, "JUNIOR_SECONDARY": true,
		"SENIOR_SECONDARY": true, "BRITISH_ENRICHMENT": true,
	}
)

func (s *Service) Provision(ctx context.Context, actorID uuid.UUID, request ProvisionRequest, requestID string) (*Tenant, error) {
	allowed, err := s.repo.IsPlatformAdmin(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}
	if s.mailer == nil {
		return nil, errors.New("invitation delivery is not configured")
	}
	request.Slug = strings.TrimSpace(strings.ToLower(request.Slug))
	request.Name = strings.TrimSpace(request.Name)
	request.OwnerEmail = strings.ToLower(strings.TrimSpace(request.OwnerEmail))
	if !slugPattern.MatchString(request.Slug) || len(request.Slug) > 80 || len(request.Name) == 0 || len(request.Name) > 160 {
		return nil, ErrInvalidInput
	}
	recipient, err := mail.ParseAddress(request.OwnerEmail)
	if err != nil || recipient.Address != request.OwnerEmail || len(request.OwnerEmail) > 254 {
		return nil, ErrInvalidInput
	}
	if request.Locale == "" {
		request.Locale = "en-NG"
	}
	if !localePattern.MatchString(request.Locale) {
		return nil, ErrInvalidInput
	}
	if request.Timezone == "" {
		request.Timezone = "Africa/Lagos"
	}
	if len(request.Timezone) > 80 {
		return nil, ErrInvalidInput
	}
	if _, err := time.LoadLocation(request.Timezone); err != nil {
		return nil, ErrInvalidInput
	}

	token, err := auth.GenerateSecureToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate owner invitation: %w", err)
	}
	now := time.Now().UTC()
	tenant := &Tenant{
		ID: uuid.New(), Slug: request.Slug, Name: request.Name, Status: "PENDING_SETUP",
		Locale: request.Locale, Timezone: request.Timezone,
		Branding: map[string]any{}, FeatureFlags: map[string]bool{},
		EducationStages: []string{"EARLY_YEARS", "PRIMARY", "JUNIOR_SECONDARY", "SENIOR_SECONDARY"},
		CreatedAt:       now, UpdatedAt: now,
	}
	invitation := &auth.Invitation{
		ID: uuid.New(), TenantID: &tenant.ID, InvitedEmail: request.OwnerEmail,
		TargetRole: "SCHOOL_OWNER", TokenHash: auth.HashToken(token),
		Status: auth.InvitationStatusPending, ExpiresAt: now.Add(7 * 24 * time.Hour),
		CreatedBy: &actorID, CreatedAt: now,
	}
	event := &auth.AuditEvent{
		TenantID: &tenant.ID, ActorID: &actorID, Action: "tenant.provisioned",
		SubjectType: "tenant", SubjectID: tenant.ID.String(), RequestID: requestID,
		MetadataRedacted: map[string]any{"slug": tenant.Slug, "invitation_id": invitation.ID.String()},
	}
	if err := s.repo.Provision(ctx, tenant, invitation, event); err != nil {
		return nil, err
	}
	if err := s.mailer.SendInvitation(ctx, invitation.InvitedEmail, token, tenant.Name, invitation.ExpiresAt); err != nil {
		cancelEvent := &auth.AuditEvent{
			TenantID: &tenant.ID, ActorID: &actorID, Action: "tenant.provisioning_delivery_failed",
			SubjectType: "tenant", SubjectID: tenant.ID.String(), RequestID: requestID,
			MetadataRedacted: map[string]any{"invitation_id": invitation.ID.String()},
		}
		cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if cancelErr := s.repo.CancelProvision(cleanupContext, tenant.ID, invitation.ID, cancelEvent); cancelErr != nil {
			return nil, fmt.Errorf("owner invitation delivery failed and provision rollback failed: %w", errors.Join(err, cancelErr))
		}
		return nil, errors.New("owner invitation delivery failed; tenant provision was rolled back")
	}
	return tenant, nil
}

func (s *Service) List(ctx context.Context, actorID uuid.UUID) ([]Tenant, error) {
	allowed, err := s.repo.IsPlatformAdmin(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}
	return s.repo.List(ctx)
}

func (s *Service) IsPlatformAdmin(ctx context.Context, actorID uuid.UUID) (bool, error) {
	return s.repo.IsPlatformAdmin(ctx, actorID)
}

func (s *Service) GetSettings(ctx context.Context, actorID uuid.UUID, slug string) (*Tenant, error) {
	return s.repo.GetForMember(ctx, strings.TrimSpace(slug), actorID)
}

func (s *Service) UpdateSettings(ctx context.Context, actorID uuid.UUID, slug string, request *SettingsRequest, requestID string) (*Tenant, error) {
	request.Name = strings.TrimSpace(request.Name)
	if len(request.Name) == 0 || len(request.Name) > 160 || !localePattern.MatchString(request.Locale) {
		return nil, ErrInvalidInput
	}
	if len(request.Timezone) > 80 {
		return nil, ErrInvalidInput
	}
	if _, err := time.LoadLocation(request.Timezone); err != nil {
		return nil, ErrInvalidInput
	}
	if len(request.EducationStages) == 0 || len(request.EducationStages) > len(stageSet) {
		return nil, ErrInvalidInput
	}
	seenStages := make(map[string]bool, len(request.EducationStages))
	for _, stage := range request.EducationStages {
		if !stageSet[stage] || seenStages[stage] {
			return nil, ErrInvalidInput
		}
		seenStages[stage] = true
	}
	if request.Branding == nil {
		request.Branding = map[string]any{}
	}
	if len(request.Branding) > 8 {
		return nil, ErrInvalidInput
	}
	for key, value := range request.Branding {
		text, ok := value.(string)
		if !ok || (key != "logo_url" && key != "primary_color" && key != "accent_color") || len(text) > 2048 {
			return nil, ErrInvalidInput
		}
		switch key {
		case "logo_url":
			parsed, err := url.ParseRequestURI(text)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
				return nil, ErrInvalidInput
			}
		case "primary_color", "accent_color":
			if text != "" && !colorPattern.MatchString(text) {
				return nil, ErrInvalidInput
			}
		}
	}
	event := &auth.AuditEvent{
		ActorID: &actorID, Action: "tenant.settings_updated", SubjectType: "tenant",
		SubjectID: strings.TrimSpace(slug), RequestID: requestID,
		MetadataRedacted: map[string]any{"updated_fields": []string{"name", "locale", "timezone", "branding", "education_stages"}},
	}
	return s.repo.UpdateSettings(ctx, strings.TrimSpace(slug), actorID, request, event)
}
