package academics

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/modules/auth"
)

var (
	ErrForbidden    = errors.New("academic operation forbidden")
	ErrNotFound     = errors.New("academic resource not found")
	ErrInvalidInput = errors.New("invalid academic input")
	ErrConflict     = errors.New("academic resource conflict")
)

const (
	SessionDraft    = "DRAFT"
	SessionActive   = "ACTIVE"
	SessionArchived = "ARCHIVED"
)

type Session struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	StartsOn  string    `json:"starts_on"`
	EndsOn    string    `json:"ends_on"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Term struct {
	ID        uuid.UUID `json:"id"`
	SessionID uuid.UUID `json:"session_id"`
	Sequence  int       `json:"sequence"`
	Name      string    `json:"name"`
	StartsOn  string    `json:"starts_on"`
	EndsOn    string    `json:"ends_on"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateSessionRequest struct {
	Name     string `json:"name"`
	StartsOn string `json:"starts_on"`
	EndsOn   string `json:"ends_on"`
}

type UpdateSessionRequest struct {
	Name     *string `json:"name"`
	StartsOn *string `json:"starts_on"`
	EndsOn   *string `json:"ends_on"`
}

type TermInput struct {
	Sequence int    `json:"sequence"`
	Name     string `json:"name"`
	StartsOn string `json:"starts_on"`
	EndsOn   string `json:"ends_on"`
}

type ConfigureTermsRequest struct {
	Terms []TermInput `json:"terms"`
}

type Repository interface {
	CreateSession(context.Context, string, uuid.UUID, *Session, *auth.AuditEvent) (*Session, error)
	ListSessions(context.Context, string, uuid.UUID) ([]Session, error)
	GetSession(context.Context, string, uuid.UUID, uuid.UUID) (*Session, error)
	UpdateSession(context.Context, string, uuid.UUID, uuid.UUID, UpdateSessionRequest, *auth.AuditEvent) (*Session, error)
	GetTerms(context.Context, string, uuid.UUID, uuid.UUID) ([]Term, error)
	ConfigureTerms(context.Context, string, uuid.UUID, uuid.UUID, []TermInput, *auth.AuditEvent) ([]Term, error)
	SetSessionStatus(context.Context, string, uuid.UUID, uuid.UUID, string, *auth.AuditEvent) (*Session, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}
