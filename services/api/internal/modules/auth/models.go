package auth

import (
	"time"

	"github.com/google/uuid"
)

type UserStatus string

const (
	UserStatusPendingActivation UserStatus = "PENDING_ACTIVATION"
	UserStatusActive            UserStatus = "ACTIVE"
	UserStatusSuspended         UserStatus = "SUSPENDED"
)

type InvitationStatus string

const (
	InvitationStatusPending  InvitationStatus = "PENDING"
	InvitationStatusAccepted InvitationStatus = "ACCEPTED"
	InvitationStatusExpired  InvitationStatus = "EXPIRED"
	InvitationStatusRevoked  InvitationStatus = "REVOKED"
)

type User struct {
	ID           uuid.UUID  `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	FullName     string     `json:"full_name"`
	Status       UserStatus `json:"status"`
	VerifiedAt   *time.Time `json:"verified_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type UserDTO struct {
	ID        uuid.UUID  `json:"id"`
	Email     string     `json:"email"`
	FullName  string     `json:"full_name"`
	Status    UserStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
}

func (u *User) ToDTO() UserDTO {
	return UserDTO{
		ID:        u.ID,
		Email:     u.Email,
		FullName:  u.FullName,
		Status:    u.Status,
		CreatedAt: u.CreatedAt,
	}
}

type Session struct {
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	TokenHash     string     `json:"-"`
	CSRFTokenHash string     `json:"-"`
	IPAddress     string     `json:"ip_address,omitempty"`
	UserAgent     string     `json:"user_agent,omitempty"`
	ExpiresAt     time.Time  `json:"expires_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type SessionDTO struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Session) ToDTO() SessionDTO {
	return SessionDTO{
		ID:        s.ID,
		CreatedAt: s.CreatedAt,
		ExpiresAt: s.ExpiresAt,
	}
}

type Invitation struct {
	ID           uuid.UUID        `json:"id"`
	TenantID     *uuid.UUID       `json:"tenant_id,omitempty"`
	InvitedEmail string           `json:"invited_email"`
	TargetRole   string           `json:"target_role"`
	TokenHash    string           `json:"-"`
	Status       InvitationStatus `json:"status"`
	ExpiresAt    time.Time        `json:"expires_at"`
	UsedAt       *time.Time       `json:"used_at,omitempty"`
	CreatedBy    *uuid.UUID       `json:"created_by,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
}

type PasswordReset struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type AuditEvent struct {
	ID               uuid.UUID      `json:"id"`
	ActorID          *uuid.UUID     `json:"actor_id,omitempty"`
	Action           string         `json:"action"`
	SubjectType      string         `json:"subject_type"`
	SubjectID        string         `json:"subject_id"`
	RequestID        string         `json:"request_id"`
	MetadataRedacted map[string]any `json:"metadata_redacted"`
	CreatedAt        time.Time      `json:"created_at"`
}

// Request and Response DTOs
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponseData struct {
	User      UserDTO `json:"user"`
	CSRFToken string  `json:"csrf_token"`
}

type CurrentUserResponseData struct {
	User      UserDTO    `json:"user"`
	Session   SessionDTO `json:"session"`
	CSRFToken string     `json:"csrf_token"`
}

type AcceptInvitationRequest struct {
	Token    string `json:"token"`
	FullName string `json:"full_name"`
	Password string `json:"password"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type RateLimitResult struct {
	Allowed    bool
	RetryAfter time.Duration
}
