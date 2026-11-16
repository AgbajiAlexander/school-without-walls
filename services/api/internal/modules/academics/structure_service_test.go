package academics

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type structureRepoStub struct {
	gradeRequest CreateGradeLevelRequest
	classRequest CreateClassRequest
	armRequest   CreateClassArmRequest
	called       bool
}

func (s *structureRepoStub) CreateGradeLevel(_ context.Context, _ string, _ uuid.UUID, req CreateGradeLevelRequest, _ string) (*GradeLevel, error) {
	s.called = true
	s.gradeRequest = req
	return &GradeLevel{Code: req.Code, Name: req.Name, Stage: req.Stage}, nil
}
func (s *structureRepoStub) ListGradeLevels(context.Context, string, uuid.UUID) ([]GradeLevel, error) {
	s.called = true
	return []GradeLevel{}, nil
}
func (s *structureRepoStub) CreateClass(_ context.Context, _ string, _ uuid.UUID, req CreateClassRequest, _ string) (*Class, error) {
	s.called = true
	s.classRequest = req
	return &Class{Code: req.Code, Name: req.Name}, nil
}
func (s *structureRepoStub) ListClasses(context.Context, string, uuid.UUID, uuid.UUID) ([]Class, error) {
	s.called = true
	return []Class{}, nil
}
func (s *structureRepoStub) CreateClassArm(_ context.Context, _ string, _ uuid.UUID, req CreateClassArmRequest, _ string) (*ClassArm, error) {
	s.called = true
	s.armRequest = req
	return &ClassArm{Code: req.Code, Name: req.Name}, nil
}
func (s *structureRepoStub) ListClassArms(context.Context, string, uuid.UUID, uuid.UUID) ([]ClassArm, error) {
	s.called = true
	return []ClassArm{}, nil
}

func TestStructureServiceCreateGradeLevelNormalization(t *testing.T) {
	repo := &structureRepoStub{}
	service := NewStructureService(repo)
	_, err := service.CreateGradeLevel(context.Background(), uuid.New(), "school", CreateGradeLevelRequest{
		Stage: " primary ", Code: " p3 ", Name: " Primary 3 ", SortOrder: 3,
	}, "req")
	if err != nil {
		t.Fatal(err)
	}
	if !repo.called || repo.gradeRequest.Stage != StagePrimary || repo.gradeRequest.Code != "P3" || repo.gradeRequest.Name != "Primary 3" {
		t.Fatalf("unexpected request passed to repository: %+v", repo.gradeRequest)
	}
}

func TestStructureServiceRejectsInvalidGradeLevel(t *testing.T) {
	tests := []CreateGradeLevelRequest{
		{Stage: "UNKNOWN", Code: "P3", Name: "Primary 3", SortOrder: 3},
		{Stage: StagePrimary, Code: "bad code", Name: "Primary 3", SortOrder: 3},
		{Stage: StagePrimary, Code: "P3", Name: " ", SortOrder: 3},
		{Stage: StagePrimary, Code: "P3", Name: "Primary 3", SortOrder: 0},
		{Stage: StagePrimary, Code: "P3", Name: strings.Repeat("x", 101), SortOrder: 3},
	}
	for _, req := range tests {
		repo := &structureRepoStub{}
		_, err := NewStructureService(repo).CreateGradeLevel(context.Background(), uuid.New(), "school", req, "req")
		if !errors.Is(err, ErrInvalidInput) || repo.called {
			t.Fatalf("expected validation failure without repository call for %+v; got %v", req, err)
		}
	}
}

func TestStructureServiceCreateClassAndArm(t *testing.T) {
	repo := &structureRepoStub{}
	service := NewStructureService(repo)
	actorID := uuid.New()
	_, err := service.CreateClass(context.Background(), actorID, "school", CreateClassRequest{
		SessionID: uuid.New(), GradeLevelID: uuid.New(), Code: " p3 ", Name: " Primary 3 ",
	}, "req")
	if err != nil || repo.classRequest.Code != "P3" || repo.classRequest.Name != "Primary 3" {
		t.Fatalf("create class: %v, %+v", err, repo.classRequest)
	}
	_, err = service.CreateClassArm(context.Background(), actorID, "school", CreateClassArmRequest{
		ClassID: uuid.New(), Code: " blue ", Name: " Blue ",
	}, "req")
	if err != nil || repo.armRequest.Code != "BLUE" || repo.armRequest.Name != "Blue" {
		t.Fatalf("create class arm: %v, %+v", err, repo.armRequest)
	}
}

func TestStructureServiceRejectsMissingParentIDs(t *testing.T) {
	repo := &structureRepoStub{}
	service := NewStructureService(repo)
	actorID := uuid.New()
	_, classErr := service.CreateClass(context.Background(), actorID, "school", CreateClassRequest{Code: "P3", Name: "Primary 3"}, "req")
	_, armErr := service.CreateClassArm(context.Background(), actorID, "school", CreateClassArmRequest{Code: "A", Name: "A"}, "req")
	_, listClassesErr := service.ListClasses(context.Background(), actorID, "school", uuid.Nil)
	_, listArmsErr := service.ListClassArms(context.Background(), actorID, "school", uuid.Nil)
	for _, err := range []error{classErr, armErr, listClassesErr, listArmsErr} {
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input, got %v", err)
		}
	}
	if repo.called {
		t.Fatal("invalid input must not reach repository")
	}
}
