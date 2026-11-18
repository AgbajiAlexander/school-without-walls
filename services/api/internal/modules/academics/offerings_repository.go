package academics

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSubjectOfferingRepository implements school-scoped subject offerings.
// All mutations are authorized and audited within a single database transaction.
type PostgresSubjectOfferingRepository struct{ pool *pgxpool.Pool }

func NewSubjectOfferingRepository(pool *pgxpool.Pool) SubjectOfferingRepository {
	return &PostgresSubjectOfferingRepository{pool: pool}
}

const subjectOfferingColumns = `id, subject_id, term_id, class_id, class_arm_id, status, created_at, updated_at`

func scanSubjectOffering(row pgx.Row) (*SubjectOffering, error) {
	var offering SubjectOffering
	err := row.Scan(&offering.ID, &offering.SubjectID, &offering.TermID, &offering.ClassID,
		&offering.ClassArmID, &offering.Status, &offering.CreatedAt, &offering.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &offering, nil
}

func offeringAudit(ctx context.Context, tx pgx.Tx, tenantID, actorID, offeringID uuid.UUID, requestID, action string) error {
	event := structureAuditEvent(actorID, requestID, action, "subject_offering", offeringID)
	setAcademicAuditTenant(event, tenantID)
	return writeAcademicAudit(ctx, tx, event)
}

func (r *PostgresSubjectOfferingRepository) CreateSubjectOffering(
	ctx context.Context, slug string, actorID uuid.UUID, req CreateSubjectOfferingRequest, requestID string,
) (*SubjectOffering, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin subject offering creation: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := authorizeSubjectTenant(ctx, tx, slug, actorID, true)
	if err != nil {
		return nil, err
	}

	id := uuid.New()
	offering, err := scanSubjectOffering(tx.QueryRow(ctx, `
        INSERT INTO subject_offerings (id, tenant_id, subject_id, term_id, class_id, class_arm_id)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING `+subjectOfferingColumns,
		id, tenantID, req.SubjectID, req.TermID, req.ClassID, req.ClassArmID))
	if err != nil {
		return nil, mapAcademicError(err)
	}

	if err := offeringAudit(ctx, tx, tenantID, actorID, id, requestID, "academic.subject_offering_created"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(err)
	}
	return offering, nil
}

func (r *PostgresSubjectOfferingRepository) ListSubjectOfferings(
	ctx context.Context, slug string, actorID uuid.UUID,
) ([]SubjectOffering, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin subject offering listing: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := authorizeSubjectTenant(ctx, tx, slug, actorID, false)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `SELECT `+subjectOfferingColumns+`
        FROM subject_offerings WHERE tenant_id=$1
        ORDER BY term_id, class_id, subject_id, class_arm_id NULLS FIRST, id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list subject offerings: %w", err)
	}

	offerings := make([]SubjectOffering, 0)
	for rows.Next() {
		var offering SubjectOffering
		if err := rows.Scan(&offering.ID, &offering.SubjectID, &offering.TermID, &offering.ClassID,
			&offering.ClassArmID, &offering.Status, &offering.CreatedAt, &offering.UpdatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan subject offering: %w", err)
		}
		offerings = append(offerings, offering)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read subject offerings: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit subject offering listing: %w", err)
	}
	return offerings, nil
}

func (r *PostgresSubjectOfferingRepository) ArchiveSubjectOffering(
	ctx context.Context, slug string, actorID, offeringID uuid.UUID, requestID string,
) (*SubjectOffering, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin subject offering archive: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := authorizeSubjectTenant(ctx, tx, slug, actorID, true)
	if err != nil {
		return nil, err
	}

	offering, err := scanSubjectOffering(tx.QueryRow(ctx, `
        UPDATE subject_offerings SET status='ARCHIVED'
        WHERE tenant_id=$1 AND id=$2 AND status='ACTIVE'
        RETURNING `+subjectOfferingColumns, tenantID, offeringID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, mapAcademicError(err)
	}

	if err := offeringAudit(ctx, tx, tenantID, actorID, offeringID, requestID, "academic.subject_offering_archived"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(err)
	}
	return offering, nil
}
