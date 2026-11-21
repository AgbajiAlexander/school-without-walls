package people

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

var ErrNotFound = errors.New("student or relationship not found")

// StudentSummary omits private profile and guardian contact information.
type StudentSummary struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Stage       string    `json:"stage"`
}

// Student reads both the record and its current access relationship in one database snapshot.
func (r *Repository) Student(ctx context.Context, slug string, actor, studentID uuid.UUID) (StudentSummary, error) {
	var student StudentSummary
	err := r.pool.QueryRow(ctx, `
		SELECT s.id, s.display_name, s.stage
		FROM students s
		JOIN tenants t ON t.id = s.tenant_id
		JOIN users u ON u.id = $2 AND u.status = 'ACTIVE'
		WHERE t.slug = $1::citext AND t.status = 'ACTIVE' AND s.id = $3
		  AND EXISTS (
			SELECT 1 FROM memberships m
			WHERE m.tenant_id = s.tenant_id AND m.user_id = u.id AND m.status = 'ACTIVE'
			  AND (m.role IN ('SCHOOL_OWNER', 'SCHOOL_ADMIN', 'PRINCIPAL')
			       OR (m.role = 'STUDENT' AND s.user_id = u.id AND s.status = 'ACTIVE')
			       OR (m.role = 'PARENT' AND s.status = 'ACTIVE' AND EXISTS (
			           SELECT 1 FROM student_guardians link
			           JOIN guardians g ON g.tenant_id = link.tenant_id AND g.id = link.guardian_id
			           WHERE link.tenant_id = s.tenant_id AND link.student_id = s.id
			             AND link.active AND g.user_id = u.id
			       )))
		  )
	`, slug, actor, studentID).Scan(&student.ID, &student.DisplayName, &student.Stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return StudentSummary{}, ErrNotFound
	}
	return student, err
}

// UnlinkGuardian serializes authority checks before changing the link and commits its audit with the change.
func (r *Repository) UnlinkGuardian(ctx context.Context, slug string, actor, studentID, guardianID uuid.UUID, requestID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tenantID, err := lockManager(ctx, tx, slug, actor)
	if err != nil {
		return err
	}

	result, err := tx.Exec(ctx, `
		UPDATE student_guardians SET active = FALSE, updated_at = CURRENT_TIMESTAMP
		WHERE tenant_id = $1 AND student_id = $2 AND guardian_id = $3 AND active
	`, tenantID, studentID, guardianID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id, tenant_id, actor_id, action, subject_type, subject_id, request_id, metadata_redacted)
		VALUES ($1, $2, $3, 'STUDENT_GUARDIAN_UNLINKED', 'student', $4, $5, jsonb_build_object('guardian_id', $6::text))
	`, uuid.New(), tenantID, actor, studentID.String(), requestID, guardianID.String())
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scopeError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func lockManager(ctx context.Context, tx pgx.Tx, slug string, actor uuid.UUID) (uuid.UUID, error) {
	var tenantID, lockedID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM tenants WHERE slug = $1::citext AND status = 'ACTIVE' FOR SHARE`, slug).Scan(&tenantID); err != nil {
		return uuid.Nil, scopeError(err)
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 AND status = 'ACTIVE' FOR SHARE`, actor).Scan(&lockedID); err != nil {
		return uuid.Nil, scopeError(err)
	}
	if err := tx.QueryRow(ctx, `
		SELECT id FROM memberships
		WHERE tenant_id = $1 AND user_id = $2 AND status = 'ACTIVE'
		  AND role IN ('SCHOOL_OWNER', 'SCHOOL_ADMIN', 'PRINCIPAL')
		ORDER BY id LIMIT 1 FOR SHARE
	`, tenantID, actor).Scan(&lockedID); err != nil {
		return uuid.Nil, scopeError(err)
	}

	return tenantID, nil
}
