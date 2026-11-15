package academics

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/modules/auth"
)

const dateLayout = "2006-01-02"

func (s *Service) CreateSession(ctx context.Context, actorID uuid.UUID, slug string, request CreateSessionRequest, requestID string) (*Session, error) {
	request.Name = strings.TrimSpace(request.Name)
	if !validName(request.Name, 32) || !validDateRange(request.StartsOn, request.EndsOn) {
		return nil, ErrInvalidInput
	}
	now := time.Now().UTC()
	session := &Session{
		ID: uuid.New(), Name: request.Name, StartsOn: request.StartsOn, EndsOn: request.EndsOn,
		Status: SessionDraft, CreatedAt: now, UpdatedAt: now,
	}
	event := academicAuditEvent(actorID, requestID, "academic.session_created", session.ID,
		map[string]any{"updated_fields": []string{"name", "starts_on", "ends_on"}})
	return s.repo.CreateSession(ctx, slug, actorID, session, event)
}

func (s *Service) ListSessions(ctx context.Context, actorID uuid.UUID, slug string) ([]Session, error) {
	return s.repo.ListSessions(ctx, slug, actorID)
}

func (s *Service) GetSession(ctx context.Context, actorID uuid.UUID, slug string, sessionID uuid.UUID) (*Session, error) {
	return s.repo.GetSession(ctx, slug, actorID, sessionID)
}

func (s *Service) UpdateSession(ctx context.Context, actorID uuid.UUID, slug string, sessionID uuid.UUID, request UpdateSessionRequest, requestID string) (*Session, error) {
	fields := make([]string, 0, 3)
	if request.Name != nil {
		value := strings.TrimSpace(*request.Name)
		if !validName(value, 32) {
			return nil, ErrInvalidInput
		}
		request.Name = &value
		fields = append(fields, "name")
	}
	if request.StartsOn != nil {
		if _, ok := parseDate(*request.StartsOn); !ok {
			return nil, ErrInvalidInput
		}
		fields = append(fields, "starts_on")
	}
	if request.EndsOn != nil {
		if _, ok := parseDate(*request.EndsOn); !ok {
			return nil, ErrInvalidInput
		}
		fields = append(fields, "ends_on")
	}
	if len(fields) == 0 {
		return nil, ErrInvalidInput
	}
	event := academicAuditEvent(actorID, requestID, "academic.session_updated", sessionID,
		map[string]any{"updated_fields": fields})
	return s.repo.UpdateSession(ctx, slug, actorID, sessionID, request, event)
}

func (s *Service) GetTerms(ctx context.Context, actorID uuid.UUID, slug string, sessionID uuid.UUID) ([]Term, error) {
	return s.repo.GetTerms(ctx, slug, actorID, sessionID)
}

func (s *Service) ConfigureTerms(ctx context.Context, actorID uuid.UUID, slug string, sessionID uuid.UUID, request ConfigureTermsRequest, requestID string) ([]Term, error) {
	if len(request.Terms) != 3 {
		return nil, ErrInvalidInput
	}
	session, err := s.repo.GetSession(ctx, slug, actorID, sessionID)
	if err != nil {
		return nil, err
	}
	sessionStart, _ := parseDate(session.StartsOn)
	sessionEnd, _ := parseDate(session.EndsOn)

	terms := append([]TermInput(nil), request.Terms...)
	for i := range terms {
		terms[i].Name = strings.TrimSpace(terms[i].Name)
		if terms[i].Sequence < 1 || terms[i].Sequence > 3 || !validName(terms[i].Name, 80) ||
			!validDateRange(terms[i].StartsOn, terms[i].EndsOn) {
			return nil, ErrInvalidInput
		}
		start, _ := parseDate(terms[i].StartsOn)
		end, _ := parseDate(terms[i].EndsOn)
		if start.Before(sessionStart) || end.After(sessionEnd) {
			return nil, ErrInvalidInput
		}
	}
	sort.Slice(terms, func(i, j int) bool { return terms[i].Sequence < terms[j].Sequence })
	for i, term := range terms {
		if term.Sequence != i+1 {
			return nil, ErrInvalidInput
		}
		if i > 0 {
			previousEnd, _ := parseDate(terms[i-1].EndsOn)
			currentStart, _ := parseDate(term.StartsOn)
			if !currentStart.After(previousEnd) {
				return nil, ErrConflict
			}
		}
	}

	event := academicAuditEvent(actorID, requestID, "academic.terms_configured", sessionID,
		map[string]any{"term_sequences": []int{1, 2, 3}})
	return s.repo.ConfigureTerms(ctx, slug, actorID, sessionID, terms, event)
}

func (s *Service) ActivateSession(ctx context.Context, actorID uuid.UUID, slug string, sessionID uuid.UUID, requestID string) (*Session, error) {
	event := academicAuditEvent(actorID, requestID, "academic.session_activated", sessionID,
		map[string]any{"status": SessionActive})
	return s.repo.SetSessionStatus(ctx, slug, actorID, sessionID, SessionActive, event)
}

func (s *Service) ArchiveSession(ctx context.Context, actorID uuid.UUID, slug string, sessionID uuid.UUID, requestID string) (*Session, error) {
	event := academicAuditEvent(actorID, requestID, "academic.session_archived", sessionID,
		map[string]any{"status": SessionArchived})
	return s.repo.SetSessionStatus(ctx, slug, actorID, sessionID, SessionArchived, event)
}

func academicAuditEvent(actorID uuid.UUID, requestID, action string, subjectID uuid.UUID, metadata map[string]any) *auth.AuditEvent {
	return &auth.AuditEvent{
		ActorID: &actorID, Action: action, SubjectType: "academic_session",
		SubjectID: subjectID.String(), RequestID: requestID, MetadataRedacted: metadata,
	}
}

func validName(value string, max int) bool {
	return len(value) > 0 && len(value) <= max && strings.TrimSpace(value) == value
}

func validDateRange(startValue, endValue string) bool {
	start, startOK := parseDate(startValue)
	end, endOK := parseDate(endValue)
	return startOK && endOK && !start.After(end)
}

func parseDate(value string) (time.Time, bool) {
	parsed, err := time.Parse(dateLayout, value)
	return parsed, err == nil && parsed.Format(dateLayout) == value
}
