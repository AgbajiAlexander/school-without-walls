package academics

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SubjectOffering associates a tenant's subject with a term, class, and optional class arm.
// Tenant identity is resolved from authenticated school membership in the repository.
type SubjectOffering struct {
	ID         uuid.UUID  `json:"id"`
	SubjectID  uuid.UUID  `json:"subject_id"`
	TermID     uuid.UUID  `json:"term_id"`
	ClassID    uuid.UUID  `json:"class_id"`
	ClassArmID *uuid.UUID `json:"class_arm_id"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type CreateSubjectOfferingRequest struct {
	SubjectID  uuid.UUID  `json:"subject_id"`
	TermID     uuid.UUID  `json:"term_id"`
	ClassID    uuid.UUID  `json:"class_id"`
	ClassArmID *uuid.UUID `json:"class_arm_id"`
}

type SubjectOfferingRepository interface {
	CreateSubjectOffering(context.Context, string, uuid.UUID, CreateSubjectOfferingRequest, string) (*SubjectOffering, error)
	ListSubjectOfferings(context.Context, string, uuid.UUID) ([]SubjectOffering, error)
	ArchiveSubjectOffering(context.Context, string, uuid.UUID, uuid.UUID, string) (*SubjectOffering, error)
}
