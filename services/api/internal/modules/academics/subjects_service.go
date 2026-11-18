package academics

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

type SubjectService struct{ repo SubjectRepository }

func NewSubjectService(repo SubjectRepository) *SubjectService {
	return &SubjectService{repo: repo}
}

func subjectNameOK(s string) bool {
	return s != "" && utf8.ValidString(s) && len(s) <= 100 && utf8.RuneCountInString(s) <= 100
}

func subjectDescriptionOK(s string) bool {
	return utf8.ValidString(s) && len(s) <= 500 && utf8.RuneCountInString(s) <= 500
}

func (s *SubjectService) CreateSubject(ctx context.Context, actorID uuid.UUID, slug string, req CreateSubjectRequest, requestID string) (*Subject, error) {
	req.Code = normalizeStructureCode(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	if actorID == uuid.Nil || strings.TrimSpace(slug) == "" || !validStructureCode(req.Code) || !subjectNameOK(req.Name) {
		return nil, ErrInvalidInput
	}
	if req.Description != nil {
		v := strings.TrimSpace(*req.Description)
		if !subjectDescriptionOK(v) {
			return nil, ErrInvalidInput
		}
		req.Description = &v
	}
	return s.repo.CreateSubject(ctx, slug, actorID, req, requestID)
}

func (s *SubjectService) ListSubjects(ctx context.Context, actorID uuid.UUID, slug string) ([]Subject, error) {
	if actorID == uuid.Nil || strings.TrimSpace(slug) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ListSubjects(ctx, slug, actorID)
}

func (s *SubjectService) UpdateSubject(ctx context.Context, actorID uuid.UUID, slug string, subjectID uuid.UUID, req UpdateSubjectRequest, requestID string) (*Subject, error) {
	if actorID == uuid.Nil || subjectID == uuid.Nil || strings.TrimSpace(slug) == "" ||
		(req.Code == nil && req.Name == nil && req.Description == nil) {
		return nil, ErrInvalidInput
	}
	if req.Code != nil {
		v := normalizeStructureCode(*req.Code)
		if !validStructureCode(v) {
			return nil, ErrInvalidInput
		}
		req.Code = &v
	}
	if req.Name != nil {
		v := strings.TrimSpace(*req.Name)
		if !subjectNameOK(v) {
			return nil, ErrInvalidInput
		}
		req.Name = &v
	}
	if req.Description != nil {
		v := strings.TrimSpace(*req.Description)
		if !subjectDescriptionOK(v) {
			return nil, ErrInvalidInput
		}
		req.Description = &v
	}
	return s.repo.UpdateSubject(ctx, slug, actorID, subjectID, req, requestID)
}

func (s *SubjectService) ArchiveSubject(ctx context.Context, actorID uuid.UUID, slug string, subjectID uuid.UUID, requestID string) (*Subject, error) {
	if actorID == uuid.Nil || subjectID == uuid.Nil || strings.TrimSpace(slug) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ArchiveSubject(ctx, slug, actorID, subjectID, requestID)
}
