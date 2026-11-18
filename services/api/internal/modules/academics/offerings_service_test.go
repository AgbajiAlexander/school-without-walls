package academics

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type offeringServiceStub struct {
	createCalls, listCalls, archiveCalls int
	gotRequest                           CreateSubjectOfferingRequest
	gotSlug                              string
	gotActor                             uuid.UUID
}

func (s *offeringServiceStub) CreateSubjectOffering(_ context.Context, slug string, actor uuid.UUID, req CreateSubjectOfferingRequest, _ string) (*SubjectOffering, error) {
	s.createCalls++
	s.gotSlug, s.gotActor, s.gotRequest = slug, actor, req
	return &SubjectOffering{ID: uuid.New(), SubjectID: req.SubjectID, TermID: req.TermID, ClassID: req.ClassID, ClassArmID: req.ClassArmID, Status: "ACTIVE"}, nil
}
func (s *offeringServiceStub) ListSubjectOfferings(_ context.Context, slug string, actor uuid.UUID) ([]SubjectOffering, error) {
	s.listCalls++
	s.gotSlug, s.gotActor = slug, actor
	return []SubjectOffering{}, nil
}
func (s *offeringServiceStub) ArchiveSubjectOffering(_ context.Context, slug string, actor, id uuid.UUID, _ string) (*SubjectOffering, error) {
	s.archiveCalls++
	s.gotSlug, s.gotActor = slug, actor
	return &SubjectOffering{ID: id, Status: "ARCHIVED"}, nil
}

func TestSubjectOfferingServiceForwardsValidRequests(t *testing.T) {
	stub := &offeringServiceStub{}
	svc := NewSubjectOfferingService(stub)
	actor := uuid.New()
	arm := uuid.New()
	req := CreateSubjectOfferingRequest{SubjectID: uuid.New(), TermID: uuid.New(), ClassID: uuid.New(), ClassArmID: &arm}
	created, err := svc.CreateSubjectOffering(context.Background(), actor, "school-a", req, "req-1")
	if err != nil || created == nil || created.Status != "ACTIVE" || stub.createCalls != 1 || stub.gotSlug != "school-a" || stub.gotActor != actor || stub.gotRequest.ClassArmID == nil || *stub.gotRequest.ClassArmID != arm {
		t.Fatalf("create did not forward valid request: offering=%+v err=%v calls=%d", created, err, stub.createCalls)
	}
	items, err := svc.ListSubjectOfferings(context.Background(), actor, "school-a")
	if err != nil || items == nil || stub.listCalls != 1 {
		t.Fatalf("list: items=%v err=%v calls=%d", items, err, stub.listCalls)
	}
	archived, err := svc.ArchiveSubjectOffering(context.Background(), actor, "school-a", created.ID, "req-2")
	if err != nil || archived == nil || archived.Status != "ARCHIVED" || stub.archiveCalls != 1 {
		t.Fatalf("archive: value=%+v err=%v calls=%d", archived, err, stub.archiveCalls)
	}
}

func TestSubjectOfferingServiceRejectsInvalidRequests(t *testing.T) {
	stub := &offeringServiceStub{}
	svc := NewSubjectOfferingService(stub)
	actor, subject, term, class := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	zero := uuid.Nil
	cases := []struct {
		name  string
		actor uuid.UUID
		slug  string
		req   CreateSubjectOfferingRequest
	}{
		{"missing actor", zero, "school-a", CreateSubjectOfferingRequest{SubjectID: subject, TermID: term, ClassID: class}},
		{"blank slug", actor, "  ", CreateSubjectOfferingRequest{SubjectID: subject, TermID: term, ClassID: class}},
		{"missing subject", actor, "school-a", CreateSubjectOfferingRequest{TermID: term, ClassID: class}},
		{"missing term", actor, "school-a", CreateSubjectOfferingRequest{SubjectID: subject, ClassID: class}},
		{"missing class", actor, "school-a", CreateSubjectOfferingRequest{SubjectID: subject, TermID: term}},
		{"zero class arm", actor, "school-a", CreateSubjectOfferingRequest{SubjectID: subject, TermID: term, ClassID: class, ClassArmID: &zero}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateSubjectOffering(context.Background(), tc.actor, tc.slug, tc.req, "req")
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput; got %v", err)
			}
		})
	}
	if _, err := svc.ListSubjectOfferings(context.Background(), zero, "school-a"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing actor list: %v", err)
	}
	if _, err := svc.ListSubjectOfferings(context.Background(), actor, " "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank slug list: %v", err)
	}
	if _, err := svc.ArchiveSubjectOffering(context.Background(), actor, "school-a", zero, "req"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing offering ID archive: %v", err)
	}
	if _, err := svc.ArchiveSubjectOffering(context.Background(), zero, "school-a", uuid.New(), "req"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing actor archive: %v", err)
	}
	if _, err := svc.ArchiveSubjectOffering(context.Background(), actor, "  ", uuid.New(), "req"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank slug archive: %v", err)
	}
	if stub.createCalls != 0 || stub.listCalls != 0 || stub.archiveCalls != 0 {
		t.Fatalf("invalid requests reached repository: %+v", stub)
	}
}
