package academics

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type teacherAssignmentServiceStub struct {
	createCalls, listCalls, deactivateCalls int
	lastSlug, lastRequestID                 string
	lastActor, lastID                       uuid.UUID
	lastCreate                              CreateTeacherAssignmentRequest
}

func (r *teacherAssignmentServiceStub) CreateTeacherAssignment(_ context.Context, slug string, actor uuid.UUID, req CreateTeacherAssignmentRequest, requestID string) (*TeacherAssignment, error) {
	r.createCalls++
	r.lastSlug, r.lastActor, r.lastCreate, r.lastRequestID = slug, actor, req, requestID
	return &TeacherAssignment{ID: uuid.New(), OfferingID: req.OfferingID, TeacherMembershipID: req.TeacherMembershipID, Status: "ACTIVE"}, nil
}
func (r *teacherAssignmentServiceStub) ListTeacherAssignments(_ context.Context, slug string, actor uuid.UUID) ([]TeacherAssignment, error) {
	r.listCalls++
	r.lastSlug, r.lastActor = slug, actor
	return []TeacherAssignment{}, nil
}
func (r *teacherAssignmentServiceStub) DeactivateTeacherAssignment(_ context.Context, slug string, actor, id uuid.UUID, requestID string) (*TeacherAssignment, error) {
	r.deactivateCalls++
	r.lastSlug, r.lastActor, r.lastID, r.lastRequestID = slug, actor, id, requestID
	return &TeacherAssignment{ID: id, Status: "INACTIVE"}, nil
}

func TestTeacherAssignmentServiceForwardsValidRequests(t *testing.T) {
	repo := &teacherAssignmentServiceStub{}
	svc := NewTeacherAssignmentService(repo)
	actor, offering, membership, assignment := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	req := CreateTeacherAssignmentRequest{OfferingID: offering, TeacherMembershipID: membership}
	created, err := svc.CreateTeacherAssignment(t.Context(), actor, "school-a", req, "create-request")
	if err != nil || created == nil || created.OfferingID != offering || created.TeacherMembershipID != membership || repo.createCalls != 1 || repo.lastActor != actor || repo.lastSlug != "school-a" || repo.lastRequestID != "create-request" {
		t.Fatalf("create forwarding: value=%+v err=%v repo=%+v", created, err, repo)
	}
	items, err := svc.ListTeacherAssignments(t.Context(), actor, "school-a")
	if err != nil || items == nil || repo.listCalls != 1 {
		t.Fatalf("list forwarding: value=%+v err=%v", items, err)
	}
	deactivated, err := svc.DeactivateTeacherAssignment(t.Context(), actor, "school-a", assignment, "deactivate-request")
	if err != nil || deactivated == nil || deactivated.Status != "INACTIVE" || repo.deactivateCalls != 1 || repo.lastID != assignment || repo.lastRequestID != "deactivate-request" {
		t.Fatalf("deactivate forwarding: value=%+v err=%v repo=%+v", deactivated, err, repo)
	}
}

func TestTeacherAssignmentServiceRejectsInvalidRequests(t *testing.T) {
	actor, offering, membership, assignment := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	valid := CreateTeacherAssignmentRequest{OfferingID: offering, TeacherMembershipID: membership}
	tests := []struct {
		name string
		run  func(*TeacherAssignmentService) error
	}{
		{"create_missing_actor", func(s *TeacherAssignmentService) error {
			_, err := s.CreateTeacherAssignment(context.Background(), uuid.Nil, "school-a", valid, "r")
			return err
		}},
		{"create_blank_slug", func(s *TeacherAssignmentService) error {
			_, err := s.CreateTeacherAssignment(context.Background(), actor, "  ", valid, "r")
			return err
		}},
		{"create_missing_offering", func(s *TeacherAssignmentService) error {
			_, err := s.CreateTeacherAssignment(context.Background(), actor, "school-a", CreateTeacherAssignmentRequest{TeacherMembershipID: membership}, "r")
			return err
		}},
		{"create_missing_teacher_membership", func(s *TeacherAssignmentService) error {
			_, err := s.CreateTeacherAssignment(context.Background(), actor, "school-a", CreateTeacherAssignmentRequest{OfferingID: offering}, "r")
			return err
		}},
		{"list_missing_actor", func(s *TeacherAssignmentService) error {
			_, err := s.ListTeacherAssignments(context.Background(), uuid.Nil, "school-a")
			return err
		}},
		{"list_blank_slug", func(s *TeacherAssignmentService) error {
			_, err := s.ListTeacherAssignments(context.Background(), actor, " ")
			return err
		}},
		{"deactivate_missing_actor", func(s *TeacherAssignmentService) error {
			_, err := s.DeactivateTeacherAssignment(context.Background(), uuid.Nil, "school-a", assignment, "r")
			return err
		}},
		{"deactivate_blank_slug", func(s *TeacherAssignmentService) error {
			_, err := s.DeactivateTeacherAssignment(context.Background(), actor, " ", assignment, "r")
			return err
		}},
		{"deactivate_missing_assignment", func(s *TeacherAssignmentService) error {
			_, err := s.DeactivateTeacherAssignment(context.Background(), actor, "school-a", uuid.Nil, "r")
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &teacherAssignmentServiceStub{}
			err := tc.run(NewTeacherAssignmentService(repo))
			if !errors.Is(err, ErrInvalidInput) || repo.createCalls+repo.listCalls+repo.deactivateCalls != 0 {
				t.Fatalf("want ErrInvalidInput without repo access, got err=%v repo=%+v", err, repo)
			}
		})
	}
}
