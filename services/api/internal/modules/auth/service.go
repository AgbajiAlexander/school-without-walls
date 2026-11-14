package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials       = errors.New("invalid email or password")
	ErrInvalidFullName          = errors.New("full name must contain 1 to 150 valid characters")
	ErrAccountSuspended         = errors.New("this account has been suspended")
	ErrAccountPendingActivation = errors.New("account is pending activation; please use your invitation link")
	ErrInvalidEmail             = errors.New("invalid email format")
	ErrAdminAlreadyExists       = errors.New("platform administrator already exists; bootstrap denied")
	ErrCSRFMismatch             = errors.New("invalid or missing CSRF token")
)

type Service struct {
	repo       Repository
	sessionTTL time.Duration
	resetTTL   time.Duration
	inviteTTL  time.Duration
}

func NewService(repo Repository) *Service {
	return &Service{
		repo:       repo,
		sessionTTL: 7 * 24 * time.Hour, // 7 days session lifetime
		resetTTL:   15 * time.Minute,   // 15 minutes password reset
		inviteTTL:  7 * 24 * time.Hour, // 7 days invitation window
	}
}

// Login validates user credentials, generates session and CSRF tokens, and returns active user.
func (s *Service) Login(ctx context.Context, email, password, ip, userAgent, requestID string) (*User, string, string, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	address, parseErr := mail.ParseAddress(cleanEmail)
	if cleanEmail == "" || parseErr != nil || address.Address != cleanEmail ||
		password == "" || !utf8.ValidString(password) || utf8.RuneCountInString(password) > 128 {
		return nil, "", "", ErrInvalidCredentials
	}

	user, err := s.repo.GetUserByEmail(ctx, cleanEmail)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Timing-safe fallback simulation
			_, _ = VerifyPassword("dummy-password", "$argon2id$v=19$m=65536,t=3,p=2$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g")
			return nil, "", "", ErrInvalidCredentials
		}
		return nil, "", "", fmt.Errorf("failed to lookup user: %w", err)
	}

	match, err := VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to verify stored password hash: %w", err)
	}
	if !match {
		return nil, "", "", ErrInvalidCredentials
	}

	if user.Status == UserStatusSuspended {
		return nil, "", "", ErrAccountSuspended
	}
	if user.Status == UserStatusPendingActivation {
		return nil, "", "", ErrAccountPendingActivation
	}

	rawSessionToken, err := GenerateSecureToken(32)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to generate session token: %w", err)
	}

	rawCSRFToken, err := GenerateSecureToken(32)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to generate CSRF token: %w", err)
	}

	session := &Session{
		ID:            uuid.New(),
		UserID:        user.ID,
		TokenHash:     HashToken(rawSessionToken),
		CSRFTokenHash: HashToken(rawCSRFToken),
		IPAddress:     ip,
		UserAgent:     userAgent,
		ExpiresAt:     time.Now().UTC().Add(s.sessionTTL),
	}

	event := &AuditEvent{
		ActorID:          &user.ID,
		Action:           "auth.login.success",
		SubjectType:      "user",
		SubjectID:        user.ID.String(),
		RequestID:        requestID,
		MetadataRedacted: map[string]any{},
	}
	if err := s.repo.CreateSessionWithAudit(ctx, session, event); err != nil {
		return nil, "", "", fmt.Errorf("failed to create session: %w", err)
	}

	return user, rawSessionToken, rawCSRFToken, nil
}

// Logout revokes the session in the database.
func (s *Service) Logout(ctx context.Context, sessionID, actorID uuid.UUID, requestID string) error {
	event := &AuditEvent{
		ActorID:          &actorID,
		Action:           "auth.logout",
		SubjectType:      "session",
		SubjectID:        sessionID.String(),
		RequestID:        requestID,
		MetadataRedacted: map[string]any{},
	}
	if err := s.repo.RevokeSessionWithAudit(ctx, sessionID, event); err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}
	return nil
}

// ValidateSession verifies a raw session token and returns the session and user.
func (s *Service) ValidateSession(ctx context.Context, rawSessionToken string) (*Session, *User, error) {
	tokenHash := HashToken(rawSessionToken)
	session, user, err := s.repo.GetActiveSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil, nil, ErrSessionNotFound
		}
		return nil, nil, fmt.Errorf("failed to validate session: %w", err)
	}

	if user.Status == UserStatusSuspended {
		return nil, nil, ErrAccountSuspended
	}

	return session, user, nil
}

// VerifyCSRF checks if the provided raw CSRF token matches the session hash.
func (s *Service) VerifyCSRF(session *Session, rawCSRFToken string) bool {
	if session == nil || rawCSRFToken == "" {
		return false
	}
	expectedHash := HashToken(rawCSRFToken)
	if len(expectedHash) != len(session.CSRFTokenHash) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expectedHash), []byte(session.CSRFTokenHash)) == 1
}

func (s *Service) AllowRateLimit(ctx context.Context, action, key string, maxAttempts int, window time.Duration) (time.Duration, error) {
	return s.repo.AllowRateLimit(ctx, action, HashToken(key), maxAttempts, window)
}

func (s *Service) ActiveSessions(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	return s.repo.ListActiveSessions(ctx, userID)
}

func (s *Service) RevokeUserSession(ctx context.Context, userID, sessionID uuid.UUID, requestID string) error {
	event := &AuditEvent{
		Action: "auth.session.revoke", RequestID: requestID, MetadataRedacted: map[string]any{},
	}
	return s.repo.RevokeUserSession(ctx, userID, sessionID, event)
}

func (s *Service) SuspendUser(ctx context.Context, userID, actorID uuid.UUID, requestID string) error {
	event := &AuditEvent{
		Action: "auth.user.suspend", RequestID: requestID, MetadataRedacted: map[string]any{},
	}
	if err := s.repo.SuspendUser(ctx, userID, actorID, event); err != nil {
		return fmt.Errorf("failed to suspend user: %w", err)
	}
	return nil
}

func (s *Service) UpdateCSRFToken(ctx context.Context, sessionID uuid.UUID, rawToken string) error {
	if rawToken == "" {
		return ErrCSRFMismatch
	}
	return s.repo.UpdateSessionCSRF(ctx, sessionID, HashToken(rawToken))
}

// VerifyInvitation checks if an invitation token is valid and returns its metadata.
func (s *Service) VerifyInvitation(ctx context.Context, rawToken string) (*Invitation, error) {
	if strings.TrimSpace(rawToken) == "" {
		return nil, ErrInvitationNotFound
	}
	tokenHash := HashToken(rawToken)
	return s.repo.GetInvitationByTokenHash(ctx, tokenHash)
}

// AcceptInvitation activates an account, consumes the invitation token, and starts a session.
func (s *Service) AcceptInvitation(ctx context.Context, rawToken, fullName, password, ip, userAgent, requestID string) (*User, string, string, error) {
	if strings.TrimSpace(rawToken) == "" {
		return nil, "", "", ErrInvitationNotFound
	}
	trimmedName := strings.TrimSpace(fullName)
	if !utf8.ValidString(trimmedName) || utf8.RuneCountInString(trimmedName) == 0 ||
		utf8.RuneCountInString(trimmedName) > 150 || strings.IndexFunc(trimmedName, unicode.IsControl) >= 0 {
		return nil, "", "", ErrInvalidFullName
	}
	if err := ValidatePasswordPolicy(password); err != nil {
		return nil, "", "", err
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to hash password: %w", err)
	}
	rawSessionToken, err := GenerateSecureToken(32)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to generate session token: %w", err)
	}
	rawCSRFToken, err := GenerateSecureToken(32)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to generate CSRF token: %w", err)
	}

	user := &User{
		ID:           uuid.New(),
		PasswordHash: passwordHash,
		FullName:     trimmedName,
		Status:       UserStatusActive,
	}
	session := &Session{
		ID:            uuid.New(),
		TokenHash:     HashToken(rawSessionToken),
		CSRFTokenHash: HashToken(rawCSRFToken),
		IPAddress:     ip,
		UserAgent:     userAgent,
		ExpiresAt:     time.Now().UTC().Add(s.sessionTTL),
	}
	event := &AuditEvent{
		Action:           "auth.invitation.accept",
		RequestID:        requestID,
		MetadataRedacted: map[string]any{},
	}
	acceptedUser, err := s.repo.AcceptInvitation(ctx, HashToken(rawToken), user, password, session, event)
	if err != nil {
		if errors.Is(err, ErrInvitationNotFound) {
			return nil, "", "", err
		}
		return nil, "", "", fmt.Errorf("failed to accept invitation: %w", err)
	}
	return acceptedUser, rawSessionToken, rawCSRFToken, nil
}

// RequestPasswordReset generates a one-use reset token. Does not reveal if the email exists.
func (s *Service) RequestPasswordReset(ctx context.Context, email, requestID string) (string, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	user, err := s.repo.GetUserByEmail(ctx, cleanEmail)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("failed to find account for password reset: %w", err)
	}
	if user.Status != UserStatusActive {
		return "", nil
	}

	rawResetToken, err := GenerateSecureToken(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate reset token: %w", err)
	}

	reset := &PasswordReset{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: HashToken(rawResetToken),
		ExpiresAt: time.Now().UTC().Add(s.resetTTL),
	}

	event := &AuditEvent{
		Action:           "auth.password_reset.request",
		RequestID:        requestID,
		MetadataRedacted: map[string]any{},
	}
	if err := s.repo.CreatePasswordResetWithAudit(ctx, reset, event); err != nil {
		return "", fmt.Errorf("failed to create password reset record: %w", err)
	}

	return rawResetToken, nil
}

// ResetPassword validates the reset token, updates password, and revokes all active sessions.
func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword, requestID string) error {
	if strings.TrimSpace(rawToken) == "" {
		return ErrPasswordResetNotFound
	}
	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return err
	}
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	event := &AuditEvent{
		Action:           "auth.password_reset.confirm",
		RequestID:        requestID,
		MetadataRedacted: map[string]any{},
	}
	if err := s.repo.CompletePasswordReset(ctx, HashToken(rawToken), passwordHash, event); err != nil {
		if errors.Is(err, ErrPasswordResetNotFound) {
			return ErrPasswordResetNotFound
		}
		return fmt.Errorf("failed to complete password reset: %w", err)
	}
	return nil
}

// BootstrapAdmin creates the first platform administrator if the database has 0 users.
func (s *Service) BootstrapAdmin(ctx context.Context, email, fullName, password, requestID string) (*User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(cleanEmail)
	if err != nil || address.Address != cleanEmail {
		return nil, ErrInvalidEmail
	}
	name := strings.TrimSpace(fullName)
	if name == "" || len(name) > 150 {
		return nil, errors.New("full name is required and must be at most 150 characters")
	}
	if err := ValidatePasswordPolicy(password); err != nil {
		return nil, err
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	admin := &User{
		Email:        cleanEmail,
		PasswordHash: passwordHash,
		FullName:     name,
		Status:       UserStatusActive,
	}
	event := &AuditEvent{
		Action:           "auth.bootstrap.admin",
		RequestID:        requestID,
		MetadataRedacted: map[string]any{},
	}
	if err := s.repo.BootstrapAdmin(ctx, admin, event); err != nil {
		if errors.Is(err, ErrAdminAlreadyExists) {
			return nil, ErrAdminAlreadyExists
		}
		return nil, fmt.Errorf("failed to bootstrap platform administrator: %w", err)
	}
	return admin, nil
}
