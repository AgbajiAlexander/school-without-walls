package academics

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// StructureService validates structure requests before delegating to the
// repository. Authorization and tenant isolation remain repository-enforced.
type StructureService struct {
	repo StructureRepository
}

func NewStructureService(repo StructureRepository) *StructureService {
	return &StructureService{repo: repo}
}

var structureCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,31}$`)

func validStructureCode(code string) bool {
	return structureCodePattern.MatchString(code)
}

func validStructureName(name string) bool {
	return name != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= 100 && len(name) <= 100
}

func validEducationStage(stage string) bool {
	switch stage {
	case StageEarlyYears, StagePrimary, StageJuniorSecondary, StageSeniorSecondary:
		return true
	default:
		return false
	}
}

func normalizeStructureCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func (s *StructureService) CreateGradeLevel(ctx context.Context, actorID uuid.UUID, slug string, req CreateGradeLevelRequest, requestID string) (*GradeLevel, error) {
	req.Stage = strings.ToUpper(strings.TrimSpace(req.Stage))
	req.Code = normalizeStructureCode(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	if actorID == uuid.Nil || !validEducationStage(req.Stage) || !validStructureCode(req.Code) || !validStructureName(req.Name) || req.SortOrder < 1 || req.SortOrder > 999 {
		return nil, ErrInvalidInput
	}
	return s.repo.CreateGradeLevel(ctx, slug, actorID, req, requestID)
}

func (s *StructureService) ListGradeLevels(ctx context.Context, actorID uuid.UUID, slug string) ([]GradeLevel, error) {
	if actorID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.repo.ListGradeLevels(ctx, slug, actorID)
}

func (s *StructureService) CreateClass(ctx context.Context, actorID uuid.UUID, slug string, req CreateClassRequest, requestID string) (*Class, error) {
	req.Code = normalizeStructureCode(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	if actorID == uuid.Nil || req.SessionID == uuid.Nil || req.GradeLevelID == uuid.Nil || !validStructureCode(req.Code) || !validStructureName(req.Name) {
		return nil, ErrInvalidInput
	}
	return s.repo.CreateClass(ctx, slug, actorID, req, requestID)
}

func (s *StructureService) ListClasses(ctx context.Context, actorID uuid.UUID, slug string, sessionID uuid.UUID) ([]Class, error) {
	if actorID == uuid.Nil || sessionID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.repo.ListClasses(ctx, slug, actorID, sessionID)
}

func (s *StructureService) CreateClassArm(ctx context.Context, actorID uuid.UUID, slug string, req CreateClassArmRequest, requestID string) (*ClassArm, error) {
	req.Code = normalizeStructureCode(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	if actorID == uuid.Nil || req.ClassID == uuid.Nil || !validStructureCode(req.Code) || !validStructureName(req.Name) {
		return nil, ErrInvalidInput
	}
	return s.repo.CreateClassArm(ctx, slug, actorID, req, requestID)
}

func (s *StructureService) ListClassArms(ctx context.Context, actorID uuid.UUID, slug string, classID uuid.UUID) ([]ClassArm, error) {
	if actorID == uuid.Nil || classID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.repo.ListClassArms(ctx, slug, actorID, classID)
}
