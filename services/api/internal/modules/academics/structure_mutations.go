package academics

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The SWW-004B mutations are methods of postgresRepository, which already owns
// the connection pool. Each write and its audit event share one transaction.
// The database triggers remain authoritative for lifecycle constraints.

func structureMutationError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		switch pgErr.ConstraintName {
		case "grade_level_archived_immutable", "grade_level_archive_only", "grade_level_identity_immutable",
			"class_archived_immutable", "class_archive_only", "class_identity_immutable",
			"class_arm_archived_immutable", "class_arm_archive_only", "class_arm_identity_immutable",
			"class_session_unavailable", "class_grade_level_unavailable",
			"class_arm_class_unavailable", "class_arm_session_unavailable", "class_arm_grade_level_unavailable":
			return ErrConflict
		}
	}
	return mapAcademicError(err)
}

func (r *postgresRepository) UpdateGradeLevel(ctx context.Context, slug string, actorID, levelID uuid.UUID, req UpdateGradeLevelRequest, requestID string) (*GradeLevel, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin grade level update: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}

	var level GradeLevel
	err = tx.QueryRow(ctx, `
        UPDATE grade_levels
        SET stage = COALESCE($3::varchar, stage),
            code = COALESCE($4::varchar, code),
            name = COALESCE($5::varchar, name),
            sort_order = COALESCE($6::smallint, sort_order)
        WHERE tenant_id = $1 AND id = $2 AND status = 'ACTIVE'
        RETURNING id, stage, code, name, sort_order, status, created_at, updated_at
    `, tenantID, levelID, req.Stage, req.Code, req.Name, req.SortOrder).Scan(
		&level.ID, &level.Stage, &level.Code, &level.Name, &level.SortOrder,
		&level.Status, &level.CreatedAt, &level.UpdatedAt,
	)
	if err != nil {
		return nil, structureMutationError(err)
	}

	event := structureAuditEvent(actorID, requestID, "academic.grade_level_updated", "grade_level", levelID)
	setAcademicAuditTenant(event, tenantID)
	if err = writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, structureMutationError(err)
	}
	return &level, nil
}

func (r *postgresRepository) ArchiveGradeLevel(ctx context.Context, slug string, actorID, levelID uuid.UUID, requestID string) (*GradeLevel, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin grade level archive: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}
	var level GradeLevel
	err = tx.QueryRow(ctx, `
        UPDATE grade_levels SET status = 'ARCHIVED'
        WHERE tenant_id = $1 AND id = $2 AND status = 'ACTIVE'
        RETURNING id, stage, code, name, sort_order, status, created_at, updated_at
    `, tenantID, levelID).Scan(&level.ID, &level.Stage, &level.Code, &level.Name,
		&level.SortOrder, &level.Status, &level.CreatedAt, &level.UpdatedAt)
	if err != nil {
		return nil, structureMutationError(err)
	}
	event := structureAuditEvent(actorID, requestID, "academic.grade_level_archived", "grade_level", levelID)
	setAcademicAuditTenant(event, tenantID)
	if err = writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, structureMutationError(err)
	}
	return &level, nil
}

func (r *postgresRepository) UpdateClass(ctx context.Context, slug string, actorID, classID uuid.UUID, req UpdateClassRequest, requestID string) (*Class, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin class update: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}
	var item Class
	err = tx.QueryRow(ctx, `
        UPDATE classes SET code = COALESCE($3::varchar, code), name = COALESCE($4::varchar, name)
        WHERE tenant_id = $1 AND id = $2 AND status = 'ACTIVE'
        RETURNING id, session_id, grade_level_id, code, name, status, created_at, updated_at
    `, tenantID, classID, req.Code, req.Name).Scan(
		&item.ID, &item.SessionID, &item.GradeLevelID, &item.Code,
		&item.Name, &item.Status, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, structureMutationError(err)
	}
	event := structureAuditEvent(actorID, requestID, "academic.class_updated", "class", classID)
	setAcademicAuditTenant(event, tenantID)
	if err = writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, structureMutationError(err)
	}
	return &item, nil
}

func (r *postgresRepository) ArchiveClass(ctx context.Context, slug string, actorID, classID uuid.UUID, requestID string) (*Class, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin class archive: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}
	var item Class
	err = tx.QueryRow(ctx, `
        UPDATE classes SET status = 'ARCHIVED'
        WHERE tenant_id = $1 AND id = $2 AND status = 'ACTIVE'
        RETURNING id, session_id, grade_level_id, code, name, status, created_at, updated_at
    `, tenantID, classID).Scan(&item.ID, &item.SessionID, &item.GradeLevelID,
		&item.Code, &item.Name, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, structureMutationError(err)
	}
	event := structureAuditEvent(actorID, requestID, "academic.class_archived", "class", classID)
	setAcademicAuditTenant(event, tenantID)
	if err = writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, structureMutationError(err)
	}
	return &item, nil
}

func (r *postgresRepository) UpdateClassArm(ctx context.Context, slug string, actorID, armID uuid.UUID, req UpdateClassArmRequest, requestID string) (*ClassArm, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin class arm update: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}
	var item ClassArm
	err = tx.QueryRow(ctx, `
        UPDATE class_arms SET code = COALESCE($3::varchar, code), name = COALESCE($4::varchar, name)
        WHERE tenant_id = $1 AND id = $2 AND status = 'ACTIVE'
        RETURNING id, class_id, code, name, status, created_at, updated_at
    `, tenantID, armID, req.Code, req.Name).Scan(
		&item.ID, &item.ClassID, &item.Code, &item.Name,
		&item.Status, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, structureMutationError(err)
	}
	event := structureAuditEvent(actorID, requestID, "academic.class_arm_updated", "class_arm", armID)
	setAcademicAuditTenant(event, tenantID)
	if err = writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, structureMutationError(err)
	}
	return &item, nil
}

func (r *postgresRepository) ArchiveClassArm(ctx context.Context, slug string, actorID, armID uuid.UUID, requestID string) (*ClassArm, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin class arm archive: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}
	var item ClassArm
	err = tx.QueryRow(ctx, `
        UPDATE class_arms SET status = 'ARCHIVED'
        WHERE tenant_id = $1 AND id = $2 AND status = 'ACTIVE'
        RETURNING id, class_id, code, name, status, created_at, updated_at
    `, tenantID, armID).Scan(&item.ID, &item.ClassID, &item.Code,
		&item.Name, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, structureMutationError(err)
	}
	event := structureAuditEvent(actorID, requestID, "academic.class_arm_archived", "class_arm", armID)
	setAcademicAuditTenant(event, tenantID)
	if err = writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, structureMutationError(err)
	}
	return &item, nil
}
