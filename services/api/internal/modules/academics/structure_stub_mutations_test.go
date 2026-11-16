package academics

import (
	"context"

	"github.com/google/uuid"
)

// These methods complete the existing test stub's StructureRepository
// implementation. Future mutation-specific tests should use a
// configurable mock to assert these operations directly.

func (r *structureRepoStub) UpdateGradeLevel(
	ctx context.Context, slug string, actorID, levelID uuid.UUID,
	req UpdateGradeLevelRequest, requestID string,
) (*GradeLevel, error) {
	return nil, ErrNotFound
}

func (r *structureRepoStub) ArchiveGradeLevel(
	ctx context.Context, slug string, actorID, levelID uuid.UUID,
	requestID string,
) (*GradeLevel, error) {
	return nil, ErrNotFound
}

func (r *structureRepoStub) UpdateClass(
	ctx context.Context, slug string, actorID, classID uuid.UUID,
	req UpdateClassRequest, requestID string,
) (*Class, error) {
	return nil, ErrNotFound
}

func (r *structureRepoStub) ArchiveClass(
	ctx context.Context, slug string, actorID, classID uuid.UUID,
	requestID string,
) (*Class, error) {
	return nil, ErrNotFound
}

func (r *structureRepoStub) UpdateClassArm(
	ctx context.Context, slug string, actorID, armID uuid.UUID,
	req UpdateClassArmRequest, requestID string,
) (*ClassArm, error) {
	return nil, ErrNotFound
}

func (r *structureRepoStub) ArchiveClassArm(
	ctx context.Context, slug string, actorID, armID uuid.UUID,
	requestID string,
) (*ClassArm, error) {
	return nil, ErrNotFound
}
