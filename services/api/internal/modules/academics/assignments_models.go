package academics

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TeacherAssignment links an active teacher membership to a subject offering.
// Tenant identity is resolved from the authenticated actor, never the request.
// Assignment status alone is not a permission grant; later teacher-facing
// operations must recheck membership and offering status in PostgreSQL.
type TeacherAssignment struct {
	ID                  uuid.UUID `json:"id"`
	OfferingID          uuid.UUID `json:"offering_id"`
	TeacherMembershipID uuid.UUID `json:"teacher_membership_id"`
	Status              string    `json:"status"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type CreateTeacherAssignmentRequest struct {
	OfferingID          uuid.UUID `json:"offering_id"`
	TeacherMembershipID uuid.UUID `json:"teacher_membership_id"`
}

// TeacherAssignmentRepository must enforce tenant isolation and School Admin /
// Principal write access. Mutations must be audited in their DB transactions.
type TeacherAssignmentRepository interface {
	CreateTeacherAssignment(context.Context, string, uuid.UUID, CreateTeacherAssignmentRequest, string) (*TeacherAssignment, error)
	ListTeacherAssignments(context.Context, string, uuid.UUID) ([]TeacherAssignment, error)
	DeactivateTeacherAssignment(context.Context, string, uuid.UUID, uuid.UUID, string) (*TeacherAssignment, error)
}
