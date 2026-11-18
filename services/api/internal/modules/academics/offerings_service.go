package academics

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// SubjectOfferingService validates inputs before invoking the school-scoped repository.
// Authorization and parent-record consistency remain repository/database duties.
type SubjectOfferingService struct{ repo SubjectOfferingRepository }

func NewSubjectOfferingService(repo SubjectOfferingRepository) *SubjectOfferingService {
	return &SubjectOfferingService{repo: repo}
}

func (s *SubjectOfferingService) CreateSubjectOffering(ctx context.Context, actorID uuid.UUID, slug string, req CreateSubjectOfferingRequest, requestID string) (*SubjectOffering, error) {
	if actorID == uuid.Nil || strings.TrimSpace(slug) == "" || req.SubjectID == uuid.Nil || req.TermID == uuid.Nil || req.ClassID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if req.ClassArmID != nil && *req.ClassArmID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.repo.CreateSubjectOffering(ctx, slug, actorID, req, requestID)
}

func (s *SubjectOfferingService) ListSubjectOfferings(ctx context.Context, actorID uuid.UUID, slug string) ([]SubjectOffering, error) {
	if actorID == uuid.Nil || strings.TrimSpace(slug) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ListSubjectOfferings(ctx, slug, actorID)
}

func (s *SubjectOfferingService) ArchiveSubjectOffering(ctx context.Context, actorID uuid.UUID, slug string, offeringID uuid.UUID, requestID string) (*SubjectOffering, error) {
	if actorID == uuid.Nil || offeringID == uuid.Nil || strings.TrimSpace(slug) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ArchiveSubjectOffering(ctx, slug, actorID, offeringID, requestID)
}
