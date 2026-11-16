package academics

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/modules/auth"
)

// StructureRepository contains the initial SWW-004B persistence operations.
// It is intentionally separate from the SWW-004A Repository interface so
// existing services and their mocks remain backward compatible.
type StructureRepository interface {
	CreateGradeLevel(context.Context, string, uuid.UUID, CreateGradeLevelRequest, string) (*GradeLevel, error)
	ListGradeLevels(context.Context, string, uuid.UUID) ([]GradeLevel, error)
	CreateClass(context.Context, string, uuid.UUID, CreateClassRequest, string) (*Class, error)
	ListClasses(context.Context, string, uuid.UUID, uuid.UUID) ([]Class, error)
	CreateClassArm(context.Context, string, uuid.UUID, CreateClassArmRequest, string) (*ClassArm, error)
	ListClassArms(context.Context, string, uuid.UUID, uuid.UUID) ([]ClassArm, error)

	UpdateGradeLevel(context.Context, string, uuid.UUID, uuid.UUID, UpdateGradeLevelRequest, string) (*GradeLevel, error)
	ArchiveGradeLevel(context.Context, string, uuid.UUID, uuid.UUID, string) (*GradeLevel, error)

	UpdateClass(context.Context, string, uuid.UUID, uuid.UUID, UpdateClassRequest, string) (*Class, error)
	ArchiveClass(context.Context, string, uuid.UUID, uuid.UUID, string) (*Class, error)

	UpdateClassArm(context.Context, string, uuid.UUID, uuid.UUID, UpdateClassArmRequest, string) (*ClassArm, error)
	ArchiveClassArm(context.Context, string, uuid.UUID, uuid.UUID, string) (*ClassArm, error)
}

// NewStructureRepository reuses the same PostgreSQL pool and tenant
// authorization rules as the existing academic-session repository.
func NewStructureRepository(pool *pgxpool.Pool) StructureRepository {
	return &postgresRepository{pool: pool}
}

func structureAuditEvent(actorID uuid.UUID, requestID, action, subjectType string, subjectID uuid.UUID) *auth.AuditEvent {
	return &auth.AuditEvent{
		ActorID:          &actorID,
		Action:           action,
		SubjectType:      subjectType,
		SubjectID:        subjectID.String(),
		RequestID:        requestID,
		MetadataRedacted: map[string]any{},
	}
}

func (r *postgresRepository) CreateGradeLevel(ctx context.Context, slug string, actorID uuid.UUID, req CreateGradeLevelRequest, requestID string) (*GradeLevel, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin grade level creation: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	var level GradeLevel
	err = tx.QueryRow(ctx, `
		INSERT INTO grade_levels (id, tenant_id, stage, code, name, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, stage, code, name, sort_order, status, created_at, updated_at
	`, id, tenantID, req.Stage, req.Code, req.Name, req.SortOrder).Scan(
		&level.ID, &level.Stage, &level.Code, &level.Name, &level.SortOrder,
		&level.Status, &level.CreatedAt, &level.UpdatedAt,
	)
	if err != nil {
		return nil, mapAcademicError(err)
	}
	event := structureAuditEvent(actorID, requestID, "academic.grade_level_created", "grade_level", id)
	setAcademicAuditTenant(event, tenantID)
	if err = writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(err)
	}
	return &level, nil
}

func (r *postgresRepository) ListGradeLevels(ctx context.Context, slug string, actorID uuid.UUID) ([]GradeLevel, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin grade level listing: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := resolveTenant(ctx, tx, slug, actorID, false, true)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, stage, code, name, sort_order, status, created_at, updated_at
		FROM grade_levels WHERE tenant_id = $1 ORDER BY stage, sort_order, name, id
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query grade levels: %w", err)
	}
	levels := make([]GradeLevel, 0)
	for rows.Next() {
		var v GradeLevel
		if err = rows.Scan(&v.ID, &v.Stage, &v.Code, &v.Name, &v.SortOrder, &v.Status, &v.CreatedAt, &v.UpdatedAt); err != nil {
			break
		}
		levels = append(levels, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read grade levels: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit grade level listing: %w", err)
	}
	return levels, nil
}

func (r *postgresRepository) CreateClass(ctx context.Context, slug string, actorID uuid.UUID, req CreateClassRequest, requestID string) (*Class, error) {
	if req.SessionID == uuid.Nil || req.GradeLevelID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin class creation: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	var class Class
	err = tx.QueryRow(ctx, `
		INSERT INTO classes (id, tenant_id, session_id, grade_level_id, code, name)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, session_id, grade_level_id, code, name, status, created_at, updated_at
	`, id, tenantID, req.SessionID, req.GradeLevelID, req.Code, req.Name).Scan(
		&class.ID, &class.SessionID, &class.GradeLevelID, &class.Code, &class.Name,
		&class.Status, &class.CreatedAt, &class.UpdatedAt,
	)
	if err != nil {
		return nil, mapAcademicError(err)
	}
	event := structureAuditEvent(actorID, requestID, "academic.class_created", "class", id)
	setAcademicAuditTenant(event, tenantID)
	if err = writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(err)
	}
	return &class, nil
}

// ListClasses requires an explicit academic session to avoid accidentally
// returning records across all years of a school.
func (r *postgresRepository) ListClasses(ctx context.Context, slug string, actorID, sessionID uuid.UUID) ([]Class, error) {
	if sessionID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin class listing: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := resolveTenant(ctx, tx, slug, actorID, false, true)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, session_id, grade_level_id, code, name, status, created_at, updated_at
		FROM classes WHERE tenant_id = $1 AND session_id = $2 ORDER BY name, id
	`, tenantID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query classes: %w", err)
	}
	classes := make([]Class, 0)
	for rows.Next() {
		var v Class
		if err = rows.Scan(&v.ID, &v.SessionID, &v.GradeLevelID, &v.Code, &v.Name, &v.Status, &v.CreatedAt, &v.UpdatedAt); err != nil {
			break
		}
		classes = append(classes, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read classes: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit class listing: %w", err)
	}
	return classes, nil
}

func (r *postgresRepository) CreateClassArm(ctx context.Context, slug string, actorID uuid.UUID, req CreateClassArmRequest, requestID string) (*ClassArm, error) {
	if req.ClassID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin class arm creation: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	var arm ClassArm
	err = tx.QueryRow(ctx, `
		INSERT INTO class_arms (id, tenant_id, class_id, code, name)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, class_id, code, name, status, created_at, updated_at
	`, id, tenantID, req.ClassID, req.Code, req.Name).Scan(
		&arm.ID, &arm.ClassID, &arm.Code, &arm.Name, &arm.Status, &arm.CreatedAt, &arm.UpdatedAt,
	)
	if err != nil {
		return nil, mapAcademicError(err)
	}
	event := structureAuditEvent(actorID, requestID, "academic.class_arm_created", "class_arm", id)
	setAcademicAuditTenant(event, tenantID)
	if err = writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(err)
	}
	return &arm, nil
}

func (r *postgresRepository) ListClassArms(ctx context.Context, slug string, actorID, classID uuid.UUID) ([]ClassArm, error) {
	if classID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin class arm listing: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := resolveTenant(ctx, tx, slug, actorID, false, true)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, class_id, code, name, status, created_at, updated_at
		FROM class_arms WHERE tenant_id = $1 AND class_id = $2 ORDER BY code, id
	`, tenantID, classID)
	if err != nil {
		return nil, fmt.Errorf("query class arms: %w", err)
	}
	arms := make([]ClassArm, 0)
	for rows.Next() {
		var v ClassArm
		if err = rows.Scan(&v.ID, &v.ClassID, &v.Code, &v.Name, &v.Status, &v.CreatedAt, &v.UpdatedAt); err != nil {
			break
		}
		arms = append(arms, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read class arms: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit class arm listing: %w", err)
	}
	return arms, nil
}
