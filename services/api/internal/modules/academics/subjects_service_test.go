package academics

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type subjectServiceStub struct {
	create       CreateSubjectRequest
	update       UpdateSubjectRequest
	createCalls  int
	updateCalls  int
	listCalls    int
	archiveCalls int
}

func (s *subjectServiceStub) CreateSubject(_ context.Context, _ string, _ uuid.UUID, req CreateSubjectRequest, _ string) (*Subject, error) {
	s.createCalls++
	s.create = req
	return &Subject{Code: req.Code, Name: req.Name, Description: req.Description}, nil
}
func (s *subjectServiceStub) ListSubjects(context.Context, string, uuid.UUID) ([]Subject, error) {
	s.listCalls++
	return []Subject{}, nil
}
func (s *subjectServiceStub) UpdateSubject(_ context.Context, _ string, _ uuid.UUID, _ uuid.UUID, req UpdateSubjectRequest, _ string) (*Subject, error) {
	s.updateCalls++
	s.update = req
	return &Subject{}, nil
}
func (s *subjectServiceStub) ArchiveSubject(context.Context, string, uuid.UUID, uuid.UUID, string) (*Subject, error) {
	s.archiveCalls++
	return &Subject{}, nil
}

func TestSubjectServiceNormalizesCreateAndUpdate(t *testing.T) {
	actor, id := uuid.New(), uuid.New()
	repo := &subjectServiceStub{}
	svc := NewSubjectService(repo)
	desc := "  Number and algebra  "
	if _, err := svc.CreateSubject(context.Background(), actor, "school-a", CreateSubjectRequest{Code: " math_1 ", Name: " Mathematics ", Description: &desc}, "req"); err != nil {
		t.Fatal(err)
	}
	if repo.create.Code != "MATH_1" || repo.create.Name != "Mathematics" || *repo.create.Description != "Number and algebra" {
		t.Fatalf("unexpected normalized create: %+v", repo.create)
	}
	code := " eng "
	name := " English "
	empty := " "
	if _, err := svc.UpdateSubject(context.Background(), actor, "school-a", id, UpdateSubjectRequest{Code: &code, Name: &name, Description: &empty}, "req"); err != nil {
		t.Fatal(err)
	}
	if *repo.update.Code != "ENG" || *repo.update.Name != "English" || *repo.update.Description != "" {
		t.Fatalf("unexpected normalized update: %+v", repo.update)
	}
}

func TestSubjectServiceRejectsInvalidMutationsBeforeRepository(t *testing.T) {
	repo := &subjectServiceStub{}
	svc := NewSubjectService(repo)
	actor, id := uuid.New(), uuid.New()
	invalidCreates := []CreateSubjectRequest{
		{Code: "bad space", Name: "Math"},
		{Code: "MATH", Name: "   "},
		{Code: "MATH", Name: "Math", Description: func() *string { v := string(make([]byte, 501)); return &v }()},
	}
	for _, req := range invalidCreates {
		if _, err := svc.CreateSubject(context.Background(), actor, "school-a", req, "req"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for %+v, got %v", req, err)
		}
	}
	if _, err := svc.CreateSubject(context.Background(), uuid.Nil, "school-a", CreateSubjectRequest{Code: "MATH", Name: "Math"}, "req"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil actor accepted: %v", err)
	}
	if _, err := svc.UpdateSubject(context.Background(), actor, "school-a", id, UpdateSubjectRequest{}, "req"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty update accepted: %v", err)
	}
	if _, err := svc.ArchiveSubject(context.Background(), actor, "school-a", uuid.Nil, "req"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil subject ID accepted: %v", err)
	}
	if repo.createCalls != 0 || repo.updateCalls != 0 || repo.archiveCalls != 0 {
		t.Fatal("invalid operation reached repository")
	}
}
