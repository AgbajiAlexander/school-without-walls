package academics

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// UpdateGradeLevel validates an administrator's partial changes. Tenant
// membership and write permissions are enforced again by the repository.
func (s *StructureService) UpdateGradeLevel(ctx context.Context, actorID uuid.UUID, slug string, levelID uuid.UUID, req UpdateGradeLevelRequest, requestID string) (*GradeLevel, error) {
	if levelID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if req.Stage == nil && req.Code == nil && req.Name == nil && req.SortOrder == nil {
		return nil, ErrInvalidInput
	}
	if req.Stage != nil {
		value := strings.ToUpper(strings.TrimSpace(*req.Stage))
		if !validEducationStage(value) {
			return nil, ErrInvalidInput
		}
		req.Stage = &value
	}
	if req.Code != nil {
		value := normalizeStructureCode(*req.Code)
		if !validStructureCode(value) {
			return nil, ErrInvalidInput
		}
		req.Code = &value
	}
	if req.Name != nil {
		value := strings.TrimSpace(*req.Name)
		if !validStructureName(value) {
			return nil, ErrInvalidInput
		}
		req.Name = &value
	}
	if req.SortOrder != nil && (*req.SortOrder < 1 || *req.SortOrder > 999) {
		return nil, ErrInvalidInput
	}
	return s.repo.UpdateGradeLevel(ctx, slug, actorID, levelID, req, requestID)
}

func (s *StructureService) ArchiveGradeLevel(ctx context.Context, actorID uuid.UUID, slug string, levelID uuid.UUID, requestID string) (*GradeLevel, error) {
	if levelID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.repo.ArchiveGradeLevel(ctx, slug, actorID, levelID, requestID)
}

func (s *StructureService) UpdateClass(ctx context.Context, actorID uuid.UUID, slug string, classID uuid.UUID, req UpdateClassRequest, requestID string) (*Class, error) {
	if classID == uuid.Nil || (req.Code == nil && req.Name == nil) {
		return nil, ErrInvalidInput
	}
	if req.Code != nil {
		value := normalizeStructureCode(*req.Code)
		if !validStructureCode(value) {
			return nil, ErrInvalidInput
		}
		req.Code = &value
	}
	if req.Name != nil {
		value := strings.TrimSpace(*req.Name)
		if !validStructureName(value) {
			return nil, ErrInvalidInput
		}
		req.Name = &value
	}
	return s.repo.UpdateClass(ctx, slug, actorID, classID, req, requestID)
}

func (s *StructureService) ArchiveClass(ctx context.Context, actorID uuid.UUID, slug string, classID uuid.UUID, requestID string) (*Class, error) {
	if classID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.repo.ArchiveClass(ctx, slug, actorID, classID, requestID)
}

func (s *StructureService) UpdateClassArm(ctx context.Context, actorID uuid.UUID, slug string, armID uuid.UUID, req UpdateClassArmRequest, requestID string) (*ClassArm, error) {
	if armID == uuid.Nil || (req.Code == nil && req.Name == nil) {
		return nil, ErrInvalidInput
	}
	if req.Code != nil {
		value := normalizeStructureCode(*req.Code)
		if !validStructureCode(value) {
			return nil, ErrInvalidInput
		}
		req.Code = &value
	}
	if req.Name != nil {
		value := strings.TrimSpace(*req.Name)
		if !validStructureName(value) {
			return nil, ErrInvalidInput
		}
		req.Name = &value
	}
	return s.repo.UpdateClassArm(ctx, slug, actorID, armID, req, requestID)
}

func (s *StructureService) ArchiveClassArm(ctx context.Context, actorID uuid.UUID, slug string, armID uuid.UUID, requestID string) (*ClassArm, error) {
	if armID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.repo.ArchiveClassArm(ctx, slug, actorID, armID, requestID)
}
