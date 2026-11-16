package academics

import (
	"time"

	"github.com/google/uuid"
)

// Education stages are aligned with the existing tenant settings vocabulary.
const (
	StageEarlyYears      = "EARLY_YEARS"
	StagePrimary         = "PRIMARY"
	StageJuniorSecondary = "JUNIOR_SECONDARY"
	StageSeniorSecondary = "SENIOR_SECONDARY"

	StructureActive   = "ACTIVE"
	StructureArchived = "ARCHIVED"
)

// GradeLevel belongs to one school and may be reused across academic sessions.
type GradeLevel struct {
	ID        uuid.UUID `json:"id"`
	Stage     string    `json:"stage"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	SortOrder int16     `json:"sort_order"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Class connects a school's grade level to a particular academic session.
type Class struct {
	ID           uuid.UUID `json:"id"`
	SessionID    uuid.UUID `json:"session_id"`
	GradeLevelID uuid.UUID `json:"grade_level_id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ClassArm represents a division within a class, such as A or Blue.
type ClassArm struct {
	ID        uuid.UUID `json:"id"`
	ClassID   uuid.UUID `json:"class_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Create requests deliberately omit tenant_id, actor_id, role and status.
// The backend must derive authority from the authenticated session.
type CreateGradeLevelRequest struct {
	Stage     string `json:"stage"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	SortOrder int16  `json:"sort_order"`
}

type UpdateGradeLevelRequest struct {
	Stage     *string `json:"stage"`
	Code      *string `json:"code"`
	Name      *string `json:"name"`
	SortOrder *int16  `json:"sort_order"`
}

type CreateClassRequest struct {
	SessionID    uuid.UUID `json:"session_id"`
	GradeLevelID uuid.UUID `json:"grade_level_id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
}

type UpdateClassRequest struct {
	Code *string `json:"code"`
	Name *string `json:"name"`
}

type CreateClassArmRequest struct {
	ClassID uuid.UUID `json:"class_id"`
	Code    string    `json:"code"`
	Name    string    `json:"name"`
}

type UpdateClassArmRequest struct {
	Code *string `json:"code"`
	Name *string `json:"name"`
}
