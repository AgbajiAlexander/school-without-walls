package auth

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MockRepository provides an in-memory thread-safe implementation of Repository for testing.
type MockRepository struct {
	mu             sync.RWMutex
	Users          map[uuid.UUID]*User
	UsersByEmail   map[string]*User
	Sessions       map[string]*Session       // key: tokenHash
	Invitations    map[string]*Invitation    // key: tokenHash
	PasswordResets map[string]*PasswordReset // key: tokenHash
	AuditEvents    []*AuditEvent
	PlatformAdmins map[uuid.UUID]bool
	rateLimiter    *RateLimiter
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		Users:          make(map[uuid.UUID]*User),
		UsersByEmail:   make(map[string]*User),
		Sessions:       make(map[string]*Session),
		Invitations:    make(map[string]*Invitation),
		PasswordResets: make(map[string]*PasswordReset),
		AuditEvents:    make([]*AuditEvent, 0),
		PlatformAdmins: make(map[uuid.UUID]bool),
		rateLimiter:    NewRateLimiter(5, 5*time.Minute),
	}
}

func (m *MockRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, exists := m.UsersByEmail[strings.ToLower(strings.TrimSpace(email))]
	if !exists {
		return nil, ErrUserNotFound
	}
	cpy := *u
	return &cpy, nil
}

func (m *MockRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, exists := m.Users[id]
	if !exists {
		return nil, ErrUserNotFound
	}
	cpy := *u
	return &cpy, nil
}

func (m *MockRepository) CreateUser(ctx context.Context, user *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cleanEmail := strings.ToLower(strings.TrimSpace(user.Email))
	if _, exists := m.UsersByEmail[cleanEmail]; exists {
		return ErrDuplicateEmail
	}
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	cpy := *user
	m.Users[user.ID] = &cpy
	m.UsersByEmail[cleanEmail] = &cpy
	return nil
}

func (m *MockRepository) UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, exists := m.Users[userID]
	if !exists {
		return ErrUserNotFound
	}
	u.PasswordHash = passwordHash
	u.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MockRepository) SuspendUser(ctx context.Context, userID, actorID uuid.UUID, event *AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	user, exists := m.Users[userID]
	if !exists || user.Status == UserStatusSuspended {
		return ErrUserNotFound
	}
	user.Status = UserStatusSuspended
	user.UpdatedAt = time.Now().UTC()
	now := time.Now().UTC()
	for _, session := range m.Sessions {
		if session.UserID == userID && session.RevokedAt == nil {
			session.RevokedAt = &now
		}
	}
	event.ActorID = &actorID
	event.SubjectType = "user"
	event.SubjectID = userID.String()
	m.addAuditLocked(event)
	return nil
}

func (m *MockRepository) CountUsers(ctx context.Context) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return int64(len(m.Users)), nil
}

func (m *MockRepository) CreateSession(ctx context.Context, s *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	s.CreatedAt = time.Now().UTC()
	cpy := *s
	m.Sessions[s.TokenHash] = &cpy
	return nil
}

func (m *MockRepository) CreateSessionWithAudit(ctx context.Context, session *Session, event *AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addSessionLocked(session)
	m.addAuditLocked(event)
	return nil
}

func (m *MockRepository) GetActiveSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, *User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, exists := m.Sessions[tokenHash]
	if !exists || s.RevokedAt != nil || time.Now().After(s.ExpiresAt) {
		return nil, nil, ErrSessionNotFound
	}
	u, userExists := m.Users[s.UserID]
	if !userExists {
		return nil, nil, ErrUserNotFound
	}
	sCpy := *s
	uCpy := *u
	return &sCpy, &uCpy, nil
}

func (m *MockRepository) RevokeSession(ctx context.Context, sessionID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.Sessions {
		if s.ID == sessionID {
			now := time.Now().UTC()
			s.RevokedAt = &now
			return nil
		}
	}
	return nil
}

func (m *MockRepository) RevokeSessionWithAudit(ctx context.Context, sessionID uuid.UUID, event *AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, session := range m.Sessions {
		if session.ID == sessionID && session.RevokedAt == nil {
			now := time.Now().UTC()
			session.RevokedAt = &now
			m.addAuditLocked(event)
			return nil
		}
	}
	return ErrSessionNotFound
}

func (m *MockRepository) UpdateSessionCSRF(ctx context.Context, sessionID uuid.UUID, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, session := range m.Sessions {
		if session.ID == sessionID && session.RevokedAt == nil && session.ExpiresAt.After(time.Now()) {
			session.CSRFTokenHash = tokenHash
			return nil
		}
	}
	return ErrSessionNotFound
}

func (m *MockRepository) RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for _, s := range m.Sessions {
		if s.UserID == userID && s.RevokedAt == nil {
			s.RevokedAt = &now
		}
	}
	return nil
}

func (m *MockRepository) ListActiveSessions(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := time.Now()
	sessions := make([]Session, 0)
	for _, session := range m.Sessions {
		if session.UserID == userID && session.RevokedAt == nil && session.ExpiresAt.After(now) {
			sessions = append(sessions, Session{
				ID: session.ID, UserID: session.UserID,
				CreatedAt: session.CreatedAt, ExpiresAt: session.ExpiresAt,
			})
		}
	}
	return sessions, nil
}

func (m *MockRepository) RevokeUserSession(ctx context.Context, userID, sessionID uuid.UUID, event *AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, session := range m.Sessions {
		if session.ID == sessionID && session.UserID == userID && session.RevokedAt == nil {
			now := time.Now().UTC()
			session.RevokedAt = &now
			event.ActorID = &userID
			event.SubjectType = "session"
			event.SubjectID = sessionID.String()
			m.addAuditLocked(event)
			return nil
		}
	}
	return ErrSessionNotFound
}

func (m *MockRepository) CreateInvitation(ctx context.Context, inv *Invitation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inv.ID == uuid.Nil {
		inv.ID = uuid.New()
	}
	inv.CreatedAt = time.Now().UTC()
	cpy := *inv
	m.Invitations[inv.TokenHash] = &cpy
	return nil
}

func (m *MockRepository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*Invitation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inv, exists := m.Invitations[tokenHash]
	if !exists || inv.Status != InvitationStatusPending || time.Now().After(inv.ExpiresAt) {
		return nil, ErrInvitationNotFound
	}
	cpy := *inv
	return &cpy, nil
}

func (m *MockRepository) MarkInvitationUsed(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inv := range m.Invitations {
		if inv.ID == id {
			now := time.Now().UTC()
			inv.Status = InvitationStatusAccepted
			inv.UsedAt = &now
			return nil
		}
	}
	return ErrInvitationNotFound
}

func (m *MockRepository) AcceptInvitation(ctx context.Context, tokenHash string, invitedUser *User, session *Session, event *AuditEvent) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	invitation, exists := m.Invitations[tokenHash]
	if !exists || invitation.Status != InvitationStatusPending || !invitation.ExpiresAt.After(time.Now()) {
		return nil, ErrInvitationNotFound
	}
	key := strings.ToLower(strings.TrimSpace(invitation.InvitedEmail))
	user, exists := m.UsersByEmail[key]
	if exists && user.Status != UserStatusPendingActivation {
		return nil, ErrInvitationIdentityExists
	}

	now := time.Now().UTC()
	if exists {
		user.FullName = invitedUser.FullName
		user.PasswordHash = invitedUser.PasswordHash
		user.Status = UserStatusActive
		user.VerifiedAt = &now
		user.UpdatedAt = now
	} else {
		user = invitedUser
		user.Email = invitation.InvitedEmail
		if user.ID == uuid.Nil {
			user.ID = uuid.New()
		}
		user.Status = UserStatusActive
		user.VerifiedAt = &now
		user.CreatedAt, user.UpdatedAt = now, now
		m.Users[user.ID] = user
		m.UsersByEmail[key] = user
	}
	invitation.Status = InvitationStatusAccepted
	invitation.UsedAt = &now
	session.UserID = user.ID
	m.addSessionLocked(session)
	event.ActorID = &user.ID
	event.SubjectType = "invitation"
	event.SubjectID = invitation.ID.String()
	m.addAuditLocked(event)
	result := *user
	return &result, nil
}

func (m *MockRepository) CreatePasswordReset(ctx context.Context, reset *PasswordReset) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if reset.ID == uuid.Nil {
		reset.ID = uuid.New()
	}
	reset.CreatedAt = time.Now().UTC()
	cpy := *reset
	m.PasswordResets[reset.TokenHash] = &cpy
	return nil
}

func (m *MockRepository) CreatePasswordResetWithAudit(ctx context.Context, reset *PasswordReset, event *AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if reset.ID == uuid.Nil {
		reset.ID = uuid.New()
	}
	reset.CreatedAt = time.Now().UTC()
	copy := *reset
	m.PasswordResets[reset.TokenHash] = &copy
	event.ActorID = &reset.UserID
	event.SubjectType = "user"
	event.SubjectID = reset.UserID.String()
	m.addAuditLocked(event)
	return nil
}

func (m *MockRepository) GetActivePasswordResetByTokenHash(ctx context.Context, tokenHash string) (*PasswordReset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	reset, exists := m.PasswordResets[tokenHash]
	if !exists || reset.UsedAt != nil || time.Now().After(reset.ExpiresAt) {
		return nil, ErrPasswordResetNotFound
	}
	cpy := *reset
	return &cpy, nil
}

func (m *MockRepository) MarkPasswordResetUsed(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, reset := range m.PasswordResets {
		if reset.ID == id {
			now := time.Now().UTC()
			reset.UsedAt = &now
			return nil
		}
	}
	return ErrPasswordResetNotFound
}

func (m *MockRepository) CompletePasswordReset(ctx context.Context, tokenHash, passwordHash string, event *AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	reset, exists := m.PasswordResets[tokenHash]
	if !exists || reset.UsedAt != nil || !reset.ExpiresAt.After(time.Now()) {
		return ErrPasswordResetNotFound
	}
	user, exists := m.Users[reset.UserID]
	if !exists {
		return ErrUserNotFound
	}
	user.PasswordHash = passwordHash
	user.UpdatedAt = time.Now().UTC()
	now := time.Now().UTC()
	reset.UsedAt = &now
	for _, session := range m.Sessions {
		if session.UserID == reset.UserID && session.RevokedAt == nil {
			session.RevokedAt = &now
		}
	}
	event.ActorID = &reset.UserID
	event.SubjectType = "user"
	event.SubjectID = reset.UserID.String()
	m.addAuditLocked(event)
	return nil
}

func (m *MockRepository) AllowRateLimit(ctx context.Context, action, keyHash string, maxAttempts int, window time.Duration) (time.Duration, error) {
	m.rateLimiter.mu.Lock()
	m.rateLimiter.maxAttempts = maxAttempts
	m.rateLimiter.window = window
	m.rateLimiter.mu.Unlock()
	allowed, retryAfter := m.rateLimiter.Allow(action + ":" + keyHash)
	if !allowed {
		return retryAfter, nil
	}
	return 0, nil
}

func (m *MockRepository) BootstrapAdmin(ctx context.Context, user *User, event *AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Users) > 0 {
		return ErrAdminAlreadyExists
	}
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	now := time.Now().UTC()
	user.Status = UserStatusActive
	user.VerifiedAt = &now
	user.CreatedAt, user.UpdatedAt = now, now
	copy := *user
	m.Users[user.ID] = &copy
	m.UsersByEmail[strings.ToLower(strings.TrimSpace(user.Email))] = &copy
	m.PlatformAdmins[user.ID] = true
	event.ActorID = &user.ID
	event.SubjectType = "user"
	event.SubjectID = user.ID.String()
	m.addAuditLocked(event)
	return nil
}

func (m *MockRepository) RecordAuditEvent(ctx context.Context, event *AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addAuditLocked(event)
	return nil
}

func (m *MockRepository) addSessionLocked(session *Session) {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	session.CreatedAt = time.Now().UTC()
	copy := *session
	m.Sessions[session.TokenHash] = &copy
}

func (m *MockRepository) addAuditLocked(event *AuditEvent) {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	event.CreatedAt = time.Now().UTC()
	cpy := *event
	m.AuditEvents = append(m.AuditEvents, &cpy)
}
