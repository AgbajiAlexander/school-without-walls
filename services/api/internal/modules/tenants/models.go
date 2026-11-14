package tenants

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/modules/auth"
)

var (
	ErrForbidden         = errors.New("tenant operation forbidden")
	ErrNotFound          = errors.New("tenant not found")
	ErrInvalidInput      = errors.New("invalid tenant input")
	ErrConflict          = errors.New("tenant slug already exists")
)

type Tenant struct {
	ID              uuid.UUID       `json:"id"`
	Slug            string          `json:"slug"`
	Name            string          `json:"name"`
	Status          string          `json:"status"`
	Locale          string          `json:"locale"`
	Timezone        string          `json:"timezone"`
	Branding        map[string]any  `json:"branding"`
	FeatureFlags    map[string]bool `json:"feature_flags"`
	EducationStages []string        `json:"education_stages"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	CurrentRole     string          `json:"current_role,omitempty"`
}

type ProvisionRequest struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	OwnerEmail string `json:"owner_email"`
	Locale     string `json:"locale"`
	Timezone   string `json:"timezone"`
}

type SettingsRequest struct {
	Name            string         `json:"name"`
	Locale          string         `json:"locale"`
	Timezone        string         `json:"timezone"`
	Branding        map[string]any `json:"branding"`
	EducationStages []string       `json:"education_stages"`
}

type InvitationMailer interface {
	SendInvitation(ctx context.Context, email, token, tenantName string, expiresAt time.Time) error
}

type CaptureMessage struct {
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Repository interface {
	IsPlatformAdmin(context.Context, uuid.UUID) (bool, error)
	Provision(context.Context, *Tenant, *auth.Invitation, *auth.AuditEvent) error
	CancelProvision(context.Context, uuid.UUID, uuid.UUID, *auth.AuditEvent) error
	List(context.Context) ([]Tenant, error)
	GetForMember(context.Context, string, uuid.UUID) (*Tenant, error)
	UpdateSettings(context.Context, string, uuid.UUID, *SettingsRequest, *auth.AuditEvent) (*Tenant, error)
}

type Service struct {
	repo   Repository
	mailer InvitationMailer
}

func NewService(repo Repository, mailer InvitationMailer) *Service {
	return &Service{repo: repo, mailer: mailer}
}
