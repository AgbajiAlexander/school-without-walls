package academics

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestSessionLifecycleAndAudit(t *testing.T) {
	repo := NewMockRepository()
	actorID, tenantID := uuid.New(), uuid.New()
	repo.AddMembership("school-a", tenantID, actorID, "SCHOOL_OWNER")
	service := NewService(repo)

	session, err := service.CreateSession(context.Background(), actorID, "school-a", CreateSessionRequest{
		Name: " 2026/2027 ", StartsOn: "2026-09-01", EndsOn: "2027-07-31",
	}, "request-create")
	if err != nil {
		t.Fatalf("create valid academic session: %v", err)
	}
	if session.Name != "2026/2027" || session.Status != SessionDraft {
		t.Fatalf("unexpected draft academic session: %+v", session)
	}
	draftUpdate, err := service.UpdateSession(context.Background(), actorID, "school-a", session.ID,
		UpdateSessionRequest{EndsOn: stringPointer("2027-08-15")}, "request-draft-update")
	if err != nil || draftUpdate.EndsOn != "2027-08-15" || draftUpdate.Status != SessionDraft {
		t.Fatalf("authorized draft date update should succeed, session=%+v err=%v", draftUpdate, err)
	}
	listed, err := service.ListSessions(context.Background(), actorID, "school-a")
	if err != nil || len(listed) != 1 || listed[0].ID != session.ID {
		t.Fatalf("session list should return the tenant's session, sessions=%+v err=%v", listed, err)
	}

	terms := validTerms()
	configured, err := service.ConfigureTerms(context.Background(), actorID, "school-a", session.ID,
		ConfigureTermsRequest{Terms: terms}, "request-terms")
	if err != nil {
		t.Fatalf("configure three valid terms: %v", err)
	}
	if len(configured) != 3 {
		t.Fatalf("expected three configured terms, got %d", len(configured))
	}
	readTerms, err := service.GetTerms(context.Background(), actorID, "school-a", session.ID)
	if err != nil || len(readTerms) != 3 {
		t.Fatalf("authorized term lookup should return all terms, terms=%+v err=%v", readTerms, err)
	}
	if _, err := service.ActivateSession(context.Background(), actorID, "school-a", session.ID, "request-activate"); err != nil {
		t.Fatalf("activate configured session: %v", err)
	}
	active, err := service.GetSession(context.Background(), actorID, "school-a", session.ID)
	if err != nil || active.Status != SessionActive {
		t.Fatalf("expected active session, session=%+v err=%v", active, err)
	}
	if len(repo.AuditEvents) != 4 {
		t.Fatalf("expected audit events for create, draft update, term configuration and activation, got %d", len(repo.AuditEvents))
	}
	for _, event := range repo.AuditEvents {
		if event.TenantID == nil || *event.TenantID != tenantID || event.ActorID == nil || *event.ActorID != actorID {
			t.Fatalf("audit event must be tenant- and actor-scoped: %+v", event)
		}
	}
}

func TestCreateSessionRejectsInvalidDates(t *testing.T) {
	repo := NewMockRepository()
	actorID := uuid.New()
	repo.AddMembership("school-a", uuid.New(), actorID, "SCHOOL_ADMIN")
	service := NewService(repo)
	for _, request := range []CreateSessionRequest{
		{Name: "2026/2027", StartsOn: "2026-02-30", EndsOn: "2027-07-31"},
		{Name: "2026/2027", StartsOn: "2027-07-31", EndsOn: "2026-09-01"},
		{Name: " ", StartsOn: "2026-09-01", EndsOn: "2027-07-31"},
	} {
		if _, err := service.CreateSession(context.Background(), actorID, "school-a", request, "invalid"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid session request to fail validation, got %v", err)
		}
	}
	if len(repo.AuditEvents) != 0 || len(repo.Sessions["school-a"]) != 0 {
		t.Fatal("invalid session input must not mutate data or create an audit event")
	}
}

func TestConfigureTermsRejectsMissingDuplicateAndOverlappingTerms(t *testing.T) {
	repo, service, actorID, session := newDraftSession(t)
	tests := []struct {
		name  string
		terms []TermInput
		want  error
	}{
		{name: "missing", terms: validTerms()[:2], want: ErrInvalidInput},
		{name: "duplicate sequence", terms: []TermInput{
			{Sequence: 1, Name: "Term 1", StartsOn: "2026-09-01", EndsOn: "2026-11-30"},
			{Sequence: 1, Name: "Term 2", StartsOn: "2026-12-01", EndsOn: "2027-03-31"},
			{Sequence: 3, Name: "Term 3", StartsOn: "2027-04-01", EndsOn: "2027-07-31"},
		}, want: ErrInvalidInput},
		{name: "overlap", terms: []TermInput{
			{Sequence: 1, Name: "Term 1", StartsOn: "2026-09-01", EndsOn: "2026-12-01"},
			{Sequence: 2, Name: "Term 2", StartsOn: "2026-12-01", EndsOn: "2027-03-31"},
			{Sequence: 3, Name: "Term 3", StartsOn: "2027-04-01", EndsOn: "2027-07-31"},
		}, want: ErrConflict},
		{name: "sequence order", terms: []TermInput{
			{Sequence: 1, Name: "Term 1", StartsOn: "2027-01-05", EndsOn: "2027-02-01"},
			{Sequence: 2, Name: "Term 2", StartsOn: "2026-09-01", EndsOn: "2026-12-15"},
			{Sequence: 3, Name: "Term 3", StartsOn: "2027-04-01", EndsOn: "2027-07-31"},
		}, want: ErrConflict},
		{name: "outside session", terms: []TermInput{
			{Sequence: 1, Name: "Term 1", StartsOn: "2026-08-31", EndsOn: "2026-11-30"},
			{Sequence: 2, Name: "Term 2", StartsOn: "2026-12-01", EndsOn: "2027-03-31"},
			{Sequence: 3, Name: "Term 3", StartsOn: "2027-04-01", EndsOn: "2027-07-31"},
		}, want: ErrInvalidInput},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.ConfigureTerms(context.Background(), actorID, "school-a", session.ID,
				ConfigureTermsRequest{Terms: test.terms}, "invalid-terms")
			if !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
		})
	}
	if len(repo.Terms["school-a"][session.ID]) != 0 || len(repo.AuditEvents) != 1 {
		t.Fatal("invalid term configuration must not persist terms or additional audit events")
	}
}

func TestSessionPermissionsAndCrossTenantIsolation(t *testing.T) {
	repo := NewMockRepository()
	ownerA, ownerB, teacher, bursar := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo.AddMembership("school-a", uuid.New(), ownerA, "SCHOOL_OWNER")
	repo.AddMembership("school-b", uuid.New(), ownerB, "SCHOOL_OWNER")
	repo.AddMembership("school-a", repo.TenantIDs["school-a"], teacher, "TEACHER")
	repo.AddMembership("school-a", repo.TenantIDs["school-a"], bursar, "BURSAR")
	service := NewService(repo)

	sessionA, err := service.CreateSession(context.Background(), ownerA, "school-a", CreateSessionRequest{
		Name: "2026/2027", StartsOn: "2026-09-01", EndsOn: "2027-07-31",
	}, "create-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetSession(context.Background(), ownerB, "school-a", sessionA.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another school's owner must not retrieve this school session, got %v", err)
	}
	if _, err := service.UpdateSession(context.Background(), ownerB, "school-a", sessionA.ID,
		UpdateSessionRequest{Name: stringPointer("tampered")}, "cross-tenant"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-school write must be denied without disclosing the school, got %v", err)
	}
	if _, err := service.CreateSession(context.Background(), teacher, "school-a", CreateSessionRequest{
		Name: "teacher-attempt", StartsOn: "2026-09-01", EndsOn: "2027-07-31",
	}, "teacher"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("teacher must not manage academic sessions, got %v", err)
	}
	if _, err := service.ListSessions(context.Background(), bursar, "school-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("bursar without academic read permission must be denied, got %v", err)
	}
}

func TestTermActivationAndArchiveLifecycle(t *testing.T) {
	repo, service, actorID, session := newDraftSession(t)
	if _, err := service.ActivateSession(context.Background(), actorID, "school-a", session.ID, "early-activation"); !errors.Is(err, ErrConflict) {
		t.Fatalf("activation without three terms must fail, got %v", err)
	}
	if len(repo.AuditEvents) != 1 {
		t.Fatal("failed activation must not create an audit event")
	}
	if _, err := service.ConfigureTerms(context.Background(), actorID, "school-a", session.ID,
		ConfigureTermsRequest{Terms: validTerms()}, "terms"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateSession(context.Background(), actorID, "school-a", session.ID, "activate"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ArchiveSession(context.Background(), actorID, "school-a", session.ID, "archive"); err != nil {
		t.Fatal(err)
	}
	beforeEvents := len(repo.AuditEvents)
	if _, err := service.UpdateSession(context.Background(), actorID, "school-a", session.ID,
		UpdateSessionRequest{Name: stringPointer("changed")}, "archived-update"); !errors.Is(err, ErrConflict) {
		t.Fatalf("archived session must be immutable, got %v", err)
	}
	if _, err := service.ConfigureTerms(context.Background(), actorID, "school-a", session.ID,
		ConfigureTermsRequest{Terms: validTerms()}, "archived-terms"); !errors.Is(err, ErrConflict) {
		t.Fatalf("terms of an archived session must be immutable, got %v", err)
	}
	if len(repo.AuditEvents) != beforeEvents {
		t.Fatal("rejected archived changes must not create audit events")
	}
}

func TestOnlyOneActiveSessionPerSchool(t *testing.T) {
	repo := NewMockRepository()
	actorID := uuid.New()
	repo.AddMembership("school-a", uuid.New(), actorID, "PRINCIPAL")
	service := NewService(repo)
	first, err := service.CreateSession(context.Background(), actorID, "school-a", CreateSessionRequest{
		Name: "2025/2026", StartsOn: "2025-09-01", EndsOn: "2026-07-31",
	}, "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateSession(context.Background(), actorID, "school-a", CreateSessionRequest{
		Name: "2026/2027", StartsOn: "2026-09-01", EndsOn: "2027-07-31",
	}, "second")
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range []*Session{first, second} {
		terms := []TermInput{
			{Sequence: 1, Name: "Term 1", StartsOn: session.StartsOn, EndsOn: "2025-12-15"},
			{Sequence: 2, Name: "Term 2", StartsOn: "2026-01-05", EndsOn: "2026-03-31"},
			{Sequence: 3, Name: "Term 3", StartsOn: "2026-04-01", EndsOn: session.EndsOn},
		}
		// Use year-matched, non-overlapping terms for the second session.
		if session.ID == second.ID {
			terms = []TermInput{
				{Sequence: 1, Name: "Term 1", StartsOn: "2026-09-01", EndsOn: "2026-12-15"},
				{Sequence: 2, Name: "Term 2", StartsOn: "2027-01-05", EndsOn: "2027-03-31"},
				{Sequence: 3, Name: "Term 3", StartsOn: "2027-04-01", EndsOn: "2027-07-31"},
			}
		}
		if _, err := service.ConfigureTerms(context.Background(), actorID, "school-a", session.ID,
			ConfigureTermsRequest{Terms: terms}, "terms"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.ActivateSession(context.Background(), actorID, "school-a", first.ID, "activate-first"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateSession(context.Background(), actorID, "school-a", second.ID, "activate-second"); !errors.Is(err, ErrConflict) {
		t.Fatalf("second active session must be rejected, got %v", err)
	}
}

func newDraftSession(t *testing.T) (*MockRepository, *Service, uuid.UUID, *Session) {
	t.Helper()
	repo := NewMockRepository()
	actorID := uuid.New()
	repo.AddMembership("school-a", uuid.New(), actorID, "SCHOOL_ADMIN")
	service := NewService(repo)
	session, err := service.CreateSession(context.Background(), actorID, "school-a", CreateSessionRequest{
		Name: "2026/2027", StartsOn: "2026-09-01", EndsOn: "2027-07-31",
	}, "create")
	if err != nil {
		t.Fatal(err)
	}
	return repo, service, actorID, session
}

func validTerms() []TermInput {
	return []TermInput{
		{Sequence: 1, Name: "First Term", StartsOn: "2026-09-01", EndsOn: "2026-12-15"},
		{Sequence: 2, Name: "Second Term", StartsOn: "2027-01-05", EndsOn: "2027-03-31"},
		{Sequence: 3, Name: "Third Term", StartsOn: "2027-04-01", EndsOn: "2027-07-31"},
	}
}

func stringPointer(value string) *string {
	return &value
}
