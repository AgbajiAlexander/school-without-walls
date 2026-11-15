package academics

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"
	"github.com/school-without-walls/api/internal/modules/auth"
)

type MockRepository struct {
	mu          sync.RWMutex
	TenantIDs   map[string]uuid.UUID
	Memberships map[string]string
	Sessions    map[string]map[uuid.UUID]*Session
	Terms       map[string]map[uuid.UUID]map[int]*Term
	AuditEvents []*auth.AuditEvent
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		TenantIDs:   make(map[string]uuid.UUID),
		Memberships: make(map[string]string),
		Sessions:    make(map[string]map[uuid.UUID]*Session),
		Terms:       make(map[string]map[uuid.UUID]map[int]*Term),
	}
}

func (m *MockRepository) AddMembership(slug string, tenantID, userID uuid.UUID, role string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TenantIDs[slug] = tenantID
	m.Memberships[academicMembershipKey(slug, userID)] = role
	if m.Sessions[slug] == nil {
		m.Sessions[slug] = make(map[uuid.UUID]*Session)
	}
	if m.Terms[slug] == nil {
		m.Terms[slug] = make(map[uuid.UUID]map[int]*Term)
	}
}

func (m *MockRepository) CreateSession(_ context.Context, slug string, actorID uuid.UUID, session *Session, event *auth.AuditEvent) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.authorize(slug, actorID, true); err != nil {
		return nil, err
	}
	for _, current := range m.Sessions[slug] {
		if current.Name == session.Name {
			return nil, ErrConflict
		}
	}
	copy := *session
	m.Sessions[slug][copy.ID] = &copy
	m.recordEvent(slug, event)
	return &copy, nil
}

func (m *MockRepository) ListSessions(_ context.Context, slug string, actorID uuid.UUID) ([]Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, err := m.authorize(slug, actorID, false); err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(m.Sessions[slug]))
	for _, session := range m.Sessions[slug] {
		sessions = append(sessions, *session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].StartsOn == sessions[j].StartsOn {
			return sessions[i].Name < sessions[j].Name
		}
		return sessions[i].StartsOn > sessions[j].StartsOn
	})
	return sessions, nil
}

func (m *MockRepository) GetSession(_ context.Context, slug string, actorID, sessionID uuid.UUID) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, err := m.authorize(slug, actorID, false); err != nil {
		return nil, err
	}
	session := m.Sessions[slug][sessionID]
	if session == nil {
		return nil, ErrNotFound
	}
	copy := *session
	return &copy, nil
}

func (m *MockRepository) UpdateSession(_ context.Context, slug string, actorID, sessionID uuid.UUID, request UpdateSessionRequest, event *auth.AuditEvent) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.authorize(slug, actorID, true); err != nil {
		return nil, err
	}
	session := m.Sessions[slug][sessionID]
	if session == nil {
		return nil, ErrNotFound
	}
	if session.Status == SessionArchived {
		return nil, ErrConflict
	}
	nextName, nextStart, nextEnd := session.Name, session.StartsOn, session.EndsOn
	if request.Name != nil {
		for id, current := range m.Sessions[slug] {
			if id != sessionID && current.Name == *request.Name {
				return nil, ErrConflict
			}
		}
		nextName = *request.Name
	}
	if request.StartsOn != nil {
		nextStart = *request.StartsOn
	}
	if request.EndsOn != nil {
		nextEnd = *request.EndsOn
	}
	if !validName(nextName, 32) || !validDateRange(nextStart, nextEnd) {
		return nil, ErrInvalidInput
	}
	for _, term := range m.Terms[slug][sessionID] {
		if term.StartsOn < nextStart || term.EndsOn > nextEnd {
			return nil, ErrInvalidInput
		}
	}
	session.Name, session.StartsOn, session.EndsOn = nextName, nextStart, nextEnd
	m.recordEvent(slug, event)
	copy := *session
	return &copy, nil
}

func (m *MockRepository) GetTerms(_ context.Context, slug string, actorID, sessionID uuid.UUID) ([]Term, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, err := m.authorize(slug, actorID, false); err != nil {
		return nil, err
	}
	if m.Sessions[slug][sessionID] == nil {
		return nil, ErrNotFound
	}
	bySequence := m.Terms[slug][sessionID]
	terms := make([]Term, 0, len(bySequence))
	for _, term := range bySequence {
		terms = append(terms, *term)
	}
	sort.Slice(terms, func(i, j int) bool { return terms[i].Sequence < terms[j].Sequence })
	return terms, nil
}

func (m *MockRepository) ConfigureTerms(_ context.Context, slug string, actorID, sessionID uuid.UUID, inputs []TermInput, event *auth.AuditEvent) ([]Term, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.authorize(slug, actorID, true); err != nil {
		return nil, err
	}
	session := m.Sessions[slug][sessionID]
	if session == nil {
		return nil, ErrNotFound
	}
	if session.Status == SessionArchived {
		return nil, ErrConflict
	}
	if m.Terms[slug][sessionID] == nil {
		m.Terms[slug][sessionID] = make(map[int]*Term)
	}
	bySequence := m.Terms[slug][sessionID]
	for _, input := range inputs {
		term := bySequence[input.Sequence]
		if term == nil {
			term = &Term{ID: uuid.New(), SessionID: sessionID, Sequence: input.Sequence}
			bySequence[input.Sequence] = term
		}
		term.Name = input.Name
		term.StartsOn = input.StartsOn
		term.EndsOn = input.EndsOn
	}
	m.recordEvent(slug, event)
	terms := make([]Term, 0, len(bySequence))
	for _, term := range bySequence {
		terms = append(terms, *term)
	}
	sort.Slice(terms, func(i, j int) bool { return terms[i].Sequence < terms[j].Sequence })
	return terms, nil
}

func (m *MockRepository) SetSessionStatus(_ context.Context, slug string, actorID, sessionID uuid.UUID, targetStatus string, event *auth.AuditEvent) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.authorize(slug, actorID, true); err != nil {
		return nil, err
	}
	session := m.Sessions[slug][sessionID]
	if session == nil {
		return nil, ErrNotFound
	}
	if session.Status == SessionArchived || session.Status == targetStatus {
		return nil, ErrConflict
	}
	if targetStatus == SessionActive {
		if session.Status != SessionDraft || len(m.Terms[slug][sessionID]) != 3 {
			return nil, ErrConflict
		}
		for _, other := range m.Sessions[slug] {
			if other.ID != sessionID && other.Status == SessionActive {
				return nil, ErrConflict
			}
		}
	}
	session.Status = targetStatus
	m.recordEvent(slug, event)
	copy := *session
	return &copy, nil
}

func (m *MockRepository) authorize(slug string, actorID uuid.UUID, manage bool) (uuid.UUID, error) {
	tenantID, exists := m.TenantIDs[slug]
	if !exists {
		return uuid.Nil, ErrNotFound
	}
	role, member := m.Memberships[academicMembershipKey(slug, actorID)]
	if !member {
		return uuid.Nil, ErrNotFound
	}
	if !canRead(role) {
		return uuid.Nil, ErrForbidden
	}
	if manage && !canManage(role) {
		return uuid.Nil, ErrForbidden
	}
	return tenantID, nil
}

func (m *MockRepository) recordEvent(slug string, event *auth.AuditEvent) {
	copy := *event
	tenantID := m.TenantIDs[slug]
	copy.TenantID = &tenantID
	if event.MetadataRedacted != nil {
		copy.MetadataRedacted = make(map[string]any, len(event.MetadataRedacted))
		for key, value := range event.MetadataRedacted {
			copy.MetadataRedacted[key] = value
		}
	}
	m.AuditEvents = append(m.AuditEvents, &copy)
}

func academicMembershipKey(slug string, userID uuid.UUID) string {
	return slug + ":" + userID.String()
}

var _ Repository = (*MockRepository)(nil)
