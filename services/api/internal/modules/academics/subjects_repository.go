package academics

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSubjectRepository implements school-scoped subject catalog operations.
// It intentionally does not reuse resolveTenant(..., manage=true): unlike
// academic sessions, the PRD makes school owners read-only for subjects.
type PostgresSubjectRepository struct{ pool *pgxpool.Pool }

func NewSubjectRepository(pool *pgxpool.Pool) SubjectRepository {
	return &PostgresSubjectRepository{pool: pool}
}

// authorizeSubjectTenant checks every active membership rather than selecting a
// single arbitrary role. A person may have multiple roles in the same school.
// Writers require an active SCHOOL_ADMIN or PRINCIPAL membership; readers
// require any active academic role, with PLATFORM_ADMIN never implying access.
// All checks run inside the caller's transaction; writes lock the matching
// tenant/membership rows to serialize against membership suspensions.
func authorizeSubjectTenant(ctx context.Context, tx pgx.Tx, slug string, actorID uuid.UUID, write bool) (uuid.UUID, error) {
	var tenantID uuid.UUID
	var permitted bool
	query := `
        SELECT t.id, EXISTS (
            SELECT 1 FROM memberships m
            WHERE m.tenant_id = t.id AND m.user_id = $2
              AND m.status = 'ACTIVE'
              AND m.role IN ('SCHOOL_ADMIN', 'PRINCIPAL')
        )
        FROM tenants t
        WHERE t.slug = $1::citext AND t.status = 'ACTIVE'
          AND EXISTS (
              SELECT 1 FROM memberships m
              WHERE m.tenant_id = t.id AND m.user_id = $2
                AND m.status = 'ACTIVE'
                AND m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL','TEACHER','STUDENT','PARENT')
          )`
	if write {
		// Read locks prevent concurrent membership suspension from overtaking
		// the transaction's authorization check.
		query += ` FOR SHARE OF t`
	}
	err := tx.QueryRow(ctx, query, slug, actorID).Scan(&tenantID, &permitted)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("authorize subject tenant: %w", err)
	}
	if write && !permitted {
		return uuid.Nil, ErrForbidden
	}
	if write {
		// Lock the actual privileged membership row. Multiple active roles
		// are supported; one sufficient privileged membership is required.
		var membershipID uuid.UUID
		err = tx.QueryRow(ctx, `
            SELECT id FROM memberships
            WHERE tenant_id = $1 AND user_id = $2 AND status = 'ACTIVE'
              AND role IN ('SCHOOL_ADMIN', 'PRINCIPAL')
            ORDER BY id LIMIT 1 FOR SHARE`, tenantID, actorID).Scan(&membershipID)
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrForbidden
		}
		if err != nil {
			return uuid.Nil, fmt.Errorf("lock subject administrator membership: %w", err)
		}
	}
	return tenantID, nil
}

func scanSubject(row pgx.Row) (*Subject, error) {
	var subject Subject
	err := row.Scan(&subject.ID, &subject.Code, &subject.Name,
		&subject.Description, &subject.Status, &subject.CreatedAt, &subject.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &subject, nil
}

const subjectColumns = `id, code, name, description, status, created_at, updated_at`

func subjectAudit(ctx context.Context, tx pgx.Tx, tenantID, actorID, subjectID uuid.UUID, requestID, action string) error {
	event := structureAuditEvent(actorID, requestID, action, "subject", subjectID)
	setAcademicAuditTenant(event, tenantID)
	return writeAcademicAudit(ctx, tx, event)
}

func (r *PostgresSubjectRepository) CreateSubject(ctx context.Context, slug string, actorID uuid.UUID, req CreateSubjectRequest, requestID string) (*Subject, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin subject creation: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := authorizeSubjectTenant(ctx, tx, slug, actorID, true)
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	subject, err := scanSubject(tx.QueryRow(ctx, `
        INSERT INTO subjects (id, tenant_id, code, name, description)
        VALUES ($1,$2,$3,$4,$5) RETURNING `+subjectColumns,
		id, tenantID, req.Code, req.Name, req.Description))
	if err != nil {
		return nil, mapAcademicError(err)
	}
	if err := subjectAudit(ctx, tx, tenantID, actorID, id, requestID, "academic.subject_created"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(err)
	}
	return subject, nil
}

func (r *PostgresSubjectRepository) ListSubjects(ctx context.Context, slug string, actorID uuid.UUID) ([]Subject, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin subject listing: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := authorizeSubjectTenant(ctx, tx, slug, actorID, false)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT `+subjectColumns+` FROM subjects WHERE tenant_id=$1 ORDER BY name, id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list subjects: %w", err)
	}
	subjects := make([]Subject, 0)
	for rows.Next() {
		var s Subject
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.Description, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan subject: %w", err)
		}
		subjects = append(subjects, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read subjects: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit subject listing: %w", err)
	}
	return subjects, nil
}

func (r *PostgresSubjectRepository) UpdateSubject(ctx context.Context, slug string, actorID, subjectID uuid.UUID, req UpdateSubjectRequest, requestID string) (*Subject, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin subject update: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := authorizeSubjectTenant(ctx, tx, slug, actorID, true)
	if err != nil {
		return nil, err
	}
	// The service ensures at least one field is present. Explicit description
	// clearing is a separate future API concern, not silently inferred from nil.
	subject, err := scanSubject(tx.QueryRow(ctx, `
        UPDATE subjects
        SET code = CASE WHEN $3::boolean THEN $4::varchar ELSE code END,
            name = CASE WHEN $5::boolean THEN $6::varchar ELSE name END,
            description = CASE WHEN $7::boolean THEN $8::varchar ELSE description END
        WHERE tenant_id=$1 AND id=$2 AND status='ACTIVE'
        RETURNING `+subjectColumns,
		tenantID, subjectID, req.Code != nil, req.Code, req.Name != nil, req.Name,
		req.Description != nil, req.Description))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, mapAcademicError(err)
	}
	if err := subjectAudit(ctx, tx, tenantID, actorID, subjectID, requestID, "academic.subject_updated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(err)
	}
	return subject, nil
}

func (r *PostgresSubjectRepository) ArchiveSubject(ctx context.Context, slug string, actorID, subjectID uuid.UUID, requestID string) (*Subject, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin subject archive: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := authorizeSubjectTenant(ctx, tx, slug, actorID, true)
	if err != nil {
		return nil, err
	}
	subject, err := scanSubject(tx.QueryRow(ctx, `
        UPDATE subjects SET status='ARCHIVED'
        WHERE tenant_id=$1 AND id=$2 AND status='ACTIVE'
        RETURNING `+subjectColumns, tenantID, subjectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, mapAcademicError(err)
	}
	if err := subjectAudit(ctx, tx, tenantID, actorID, subjectID, requestID, "academic.subject_archived"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(err)
	}
	return subject, nil
}
