package academics

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresTeacherAssignmentRepository manages school-scoped teacher assignments.
// Assignment rows are not authorization grants: teacher-facing endpoints must
// independently check the teacher's current active membership and offering.
type PostgresTeacherAssignmentRepository struct {
	pool *pgxpool.Pool
}

func NewTeacherAssignmentRepository(pool *pgxpool.Pool) TeacherAssignmentRepository {
	return &PostgresTeacherAssignmentRepository{pool: pool}
}

const teacherAssignmentColumns = `id, offering_id, teacher_membership_id, status, created_at, updated_at`

func scanTeacherAssignment(row pgx.Row) (*TeacherAssignment, error) {
	var assignment TeacherAssignment
	err := row.Scan(&assignment.ID, &assignment.OfferingID, &assignment.TeacherMembershipID,
		&assignment.Status, &assignment.CreatedAt, &assignment.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &assignment, nil
}

func teacherAssignmentAudit(ctx context.Context, tx pgx.Tx, tenantID, actorID, assignmentID uuid.UUID, requestID, action string) error {
	event := structureAuditEvent(actorID, requestID, action, "teacher_assignment", assignmentID)
	setAcademicAuditTenant(event, tenantID)
	return writeAcademicAudit(ctx, tx, event)
}

func (r *PostgresTeacherAssignmentRepository) CreateTeacherAssignment(
	ctx context.Context, slug string, actorID uuid.UUID, req CreateTeacherAssignmentRequest, requestID string,
) (*TeacherAssignment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin teacher assignment creation: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := authorizeSubjectTenant(ctx, tx, slug, actorID, true)
	if err != nil {
		return nil, err
	}

	assignmentID := uuid.New()
	assignment, err := scanTeacherAssignment(tx.QueryRow(ctx, `
        INSERT INTO teacher_assignments (id, tenant_id, offering_id, teacher_membership_id)
        VALUES ($1, $2, $3, $4)
        RETURNING `+teacherAssignmentColumns,
		assignmentID, tenantID, req.OfferingID, req.TeacherMembershipID))
	if err != nil {
		return nil, mapAcademicError(err)
	}

	if err := teacherAssignmentAudit(ctx, tx, tenantID, actorID, assignmentID, requestID, "academic.teacher_assignment_created"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(err)
	}
	return assignment, nil
}

func (r *PostgresTeacherAssignmentRepository) ListTeacherAssignments(
	ctx context.Context, slug string, actorID uuid.UUID,
) ([]TeacherAssignment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin teacher assignment listing: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := authorizeSubjectTenant(ctx, tx, slug, actorID, false)
	if err != nil {
		return nil, err
	}

	var canListAll, canListSelf bool
	err = tx.QueryRow(ctx, `
    SELECT
        EXISTS (
            SELECT 1 FROM memberships
            WHERE tenant_id=$1 AND user_id=$2
              AND status='ACTIVE'
              AND role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL')
        ),
        EXISTS (
            SELECT 1 FROM memberships
            WHERE tenant_id=$1 AND user_id=$2
              AND status='ACTIVE' AND role='TEACHER'
        )`,
		tenantID, actorID,
	).Scan(&canListAll, &canListSelf)
	if err != nil {
		return nil, fmt.Errorf("authorize teacher assignment listing: %w", err)
	}
	if !canListAll && !canListSelf {
		return nil, ErrForbidden
	}

	rows, err := tx.Query(ctx, `SELECT `+teacherAssignmentColumns+`
    FROM teacher_assignments
    WHERE tenant_id=$1
      AND (
          $2::boolean
          OR EXISTS (
              SELECT 1 FROM memberships m
              WHERE m.tenant_id=teacher_assignments.tenant_id
                AND m.id=teacher_assignments.teacher_membership_id
                AND m.user_id=$3
                AND m.status='ACTIVE'
                AND m.role='TEACHER'
          )
      )
    ORDER BY offering_id, teacher_membership_id, id`,
		tenantID, canListAll, actorID)
	if err != nil {
		return nil, fmt.Errorf("list teacher assignments: %w", err)
	}

	assignments := make([]TeacherAssignment, 0)
	for rows.Next() {
		var assignment TeacherAssignment
		if err := rows.Scan(&assignment.ID, &assignment.OfferingID, &assignment.TeacherMembershipID,
			&assignment.Status, &assignment.CreatedAt, &assignment.UpdatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan teacher assignment: %w", err)
		}
		assignments = append(assignments, assignment)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read teacher assignments: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit teacher assignment listing: %w", err)
	}
	return assignments, nil
}

func (r *PostgresTeacherAssignmentRepository) DeactivateTeacherAssignment(
	ctx context.Context, slug string, actorID, assignmentID uuid.UUID, requestID string,
) (*TeacherAssignment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin teacher assignment deactivation: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := authorizeSubjectTenant(ctx, tx, slug, actorID, true)
	if err != nil {
		return nil, err
	}

	assignment, err := scanTeacherAssignment(tx.QueryRow(ctx, `
        UPDATE teacher_assignments SET status='INACTIVE'
        WHERE tenant_id=$1 AND id=$2 AND status='ACTIVE'
        RETURNING `+teacherAssignmentColumns, tenantID, assignmentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, mapAcademicError(err)
	}

	if err := teacherAssignmentAudit(ctx, tx, tenantID, actorID, assignmentID, requestID, "academic.teacher_assignment_deactivated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(err)
	}
	return assignment, nil
}
