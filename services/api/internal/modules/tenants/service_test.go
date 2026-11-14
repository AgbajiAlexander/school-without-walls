package tenants

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/modules/auth"
)

type testInvitationMailer struct {
	email, token, tenantName string
	expiresAt                time.Time
	err                      error
}

func (m *testInvitationMailer) SendInvitation(_ context.Context, email, token, tenantName string, expiresAt time.Time) error {
	m.email, m.token, m.tenantName, m.expiresAt = email, token, tenantName, expiresAt
	return m.err
}

func TestProvisionRequiresPlatformAdminAndStoresOnlyTokenHash(t *testing.T) {
	repo := NewMockRepository()
	adminID := uuid.New()
	repo.PlatformAdmins[adminID] = true
	mailer := &testInvitationMailer{}
	service := NewService(repo, mailer)

	tenant, err := service.Provision(context.Background(), adminID, ProvisionRequest{
		Slug: "  demo-school ", Name: " Demo School ", OwnerEmail: "OWNER@school.invalid",
	}, "test-request")
	if err != nil {
		t.Fatalf("provision tenant: %v", err)
	}
	if tenant.Status != "PENDING_SETUP" || tenant.Slug != "demo-school" || tenant.Locale != "en-NG" {
		t.Fatalf("unexpected tenant data: %+v", tenant)
	}
	if mailer.email != "owner@school.invalid" || mailer.tenantName != "Demo School" || mailer.token == "" {
		t.Fatalf("owner invitation was not delivered: %+v", mailer)
	}
	var invitation auth.Invitation
	for _, current := range repo.Invitations {
		invitation = current
	}
	if invitation.TargetRole != "SCHOOL_OWNER" || invitation.TenantID == nil || *invitation.TenantID != tenant.ID {
		t.Fatalf("stored invitation does not bind owner role and tenant: %+v", invitation)
	}
	if invitation.TokenHash == mailer.token || invitation.TokenHash != auth.HashToken(mailer.token) {
		t.Fatal("repository must store only the invitation token hash")
	}
}

func TestProvisionRejectsNonAdminAndInvalidFields(t *testing.T) {
	repo := NewMockRepository()
	mailer := &testInvitationMailer{}
	service := NewService(repo, mailer)
	actorID := uuid.New()
	request := ProvisionRequest{Slug: "safe-school", Name: "Safe School", OwnerEmail: "owner@school.invalid"}

	if _, err := service.Provision(context.Background(), actorID, request, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected non-admin rejection, got %v", err)
	}
	if len(repo.Tenants) != 0 || mailer.token != "" {
		t.Fatal("unauthorized provisioning must not create tenants or send invitations")
	}
	repo.PlatformAdmins[actorID] = true
	request.Slug = "../unsafe"
	if _, err := service.Provision(context.Background(), actorID, request, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected slug validation error, got %v", err)
	}
	request.Slug = "safe-school"
	request.OwnerEmail = "not-an-email"
	if _, err := service.Provision(context.Background(), actorID, request, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected email validation error, got %v", err)
	}
}

func TestProvisionRollsBackWhenInvitationDeliveryFails(t *testing.T) {
	repo := NewMockRepository()
	actorID := uuid.New()
	repo.PlatformAdmins[actorID] = true
	mailer := &testInvitationMailer{err: errors.New("test mail delivery failure")}
	service := NewService(repo, mailer)
	tenant, err := service.Provision(context.Background(), actorID, ProvisionRequest{
		Slug: "delivery-failure", Name: "Delivery Failure School", OwnerEmail: "owner@school.invalid",
	}, "request")
	if err == nil || tenant != nil {
		t.Fatal("provision must fail when invitation delivery fails")
	}
	stored := repo.Tenants["delivery-failure"]
	if stored == nil || stored.Status != "ARCHIVED" {
		t.Fatalf("undelivered provision should be archived, got %+v", stored)
	}
	for _, invitation := range repo.Invitations {
		if invitation.Status != auth.InvitationStatusRevoked {
			t.Fatalf("undelivered invitation must be revoked, got %s", invitation.Status)
		}
	}
}

func TestTenantSettingsRequireActiveMembershipAndOwnerOrAdmin(t *testing.T) {
	repo := NewMockRepository()
	tenantID := uuid.New()
	repo.Tenants["example-school"] = &Tenant{
		ID: tenantID, Slug: "example-school", Name: "Example School", Status: "ACTIVE",
		Locale: "en-NG", Timezone: "Africa/Lagos",
	}
	ownerID, teacherID := uuid.New(), uuid.New()
	repo.AddMembership("example-school", ownerID, "SCHOOL_OWNER")
	repo.AddMembership("example-school", teacherID, "TEACHER")
	service := NewService(repo, nil)
	request := &SettingsRequest{
		Name: "Updated School", Locale: "en-NG", Timezone: "Africa/Lagos",
		EducationStages: []string{"PRIMARY"},
	}

	if _, err := service.GetSettings(context.Background(), teacherID, "other-school"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant lookup must be denied, got %v", err)
	}
	if _, err := service.UpdateSettings(context.Background(), teacherID, "example-school", request, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("teacher must not change school settings, got %v", err)
	}
	if _, err := service.UpdateSettings(context.Background(), ownerID, "example-school", request, ""); err != nil {
		t.Fatalf("school owner should update settings: %v", err)
	}
	if repo.Tenants["example-school"].Name != "Updated School" {
		t.Fatal("authorized settings update was not saved")
	}
}

func TestSettingsRejectUnversionedOrUnknownEducationStages(t *testing.T) {
	service := NewService(NewMockRepository(), nil)
	request := &SettingsRequest{
		Name: "School", Locale: "en-NG", Timezone: "Africa/Lagos",
		EducationStages: []string{"CLIENT_SUPPLIED_ROLE"},
	}
	if _, err := service.UpdateSettings(context.Background(), uuid.New(), "school", request, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown education stage should be rejected: %v", err)
	}
}

func TestSettingsRejectUnsafeBrandingURL(t *testing.T) {
	service := NewService(NewMockRepository(), nil)
	request := &SettingsRequest{
		Name: "School", Locale: "en-NG", Timezone: "Africa/Lagos",
		Branding:        map[string]any{"logo_url": "javascript:alert(1)"},
		EducationStages: []string{"PRIMARY"},
	}
	if _, err := service.UpdateSettings(context.Background(), uuid.New(), "school", request, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unsafe branding URL should be rejected: %v", err)
	}
}
