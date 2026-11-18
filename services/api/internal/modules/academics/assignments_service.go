package academics

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// TeacherAssignmentService handles request validation. The repository and
// database remain authoritative for roles, membership state and offering state.
type TeacherAssignmentService struct{ repo TeacherAssignmentRepository }

func NewTeacherAssignmentService(repo TeacherAssignmentRepository) *TeacherAssignmentService {
	return &TeacherAssignmentService{repo: repo}
}

func (s *TeacherAssignmentService) CreateTeacherAssignment(ctx context.Context, actorID uuid.UUID, slug string, req CreateTeacherAssignmentRequest, requestID string) (*TeacherAssignment, error) {
	if actorID == uuid.Nil || strings.TrimSpace(slug) == "" || req.OfferingID == uuid.Nil || req.TeacherMembershipID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.repo.CreateTeacherAssignment(ctx, slug, actorID, req, requestID)
}

func (s *TeacherAssignmentService) ListTeacherAssignments(ctx context.Context, actorID uuid.UUID, slug string) ([]TeacherAssignment, error) {
	if actorID == uuid.Nil || strings.TrimSpace(slug) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ListTeacherAssignments(ctx, slug, actorID)
}

func (s *TeacherAssignmentService) DeactivateTeacherAssignment(ctx context.Context, actorID uuid.UUID, slug string, assignmentID uuid.UUID, requestID string) (*TeacherAssignment, error) {
	if actorID == uuid.Nil || assignmentID == uuid.Nil || strings.TrimSpace(slug) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.DeactivateTeacherAssignment(ctx, slug, actorID, assignmentID, requestID)
}
