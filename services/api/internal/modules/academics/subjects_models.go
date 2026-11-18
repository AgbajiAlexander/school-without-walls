package academics

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Subject is a school-specific subject. Tenant identity is never supplied by API callers.
type Subject struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateSubjectRequest struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type UpdateSubjectRequest struct {
	Code        *string `json:"code"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// SubjectRepository is separate from older academic interfaces to preserve existing mocks.
// Implementations must derive the tenant and authorization from the actor and slug.
type SubjectRepository interface {
	CreateSubject(context.Context, string, uuid.UUID, CreateSubjectRequest, string) (*Subject, error)
	ListSubjects(context.Context, string, uuid.UUID) ([]Subject, error)
	UpdateSubject(context.Context, string, uuid.UUID, uuid.UUID, UpdateSubjectRequest, string) (*Subject, error)
	ArchiveSubject(context.Context, string, uuid.UUID, uuid.UUID, string) (*Subject, error)
}
