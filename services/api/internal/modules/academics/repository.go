package academics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/modules/auth"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) CreateSession(ctx context.Context, slug string, actorID uuid.UUID, session *Session, event *auth.AuditEvent) (*Session, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin academic session creation: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}
	created, err := scanSession(tx.QueryRow(ctx, `
		INSERT INTO academic_sessions (id, tenant_id, name, starts_on, ends_on, status)
		VALUES ($1, $2, $3, $4::date, $5::date, 'DRAFT')
		RETURNING id, name, starts_on, ends_on, status, created_at, updated_at
	`, session.ID, tenantID, session.Name, session.StartsOn, session.EndsOn))
	if err != nil {
		return nil, mapAcademicError(err)
	}
	setAcademicAuditTenant(event, tenantID)
	if err := writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(fmt.Errorf("commit academic session creation: %w", err))
	}
	return created, nil
}

func (r *postgresRepository) ListSessions(ctx context.Context, slug string, actorID uuid.UUID) ([]Session, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin academic session listing: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := resolveTenant(ctx, tx, slug, actorID, false, true)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, name, starts_on, ends_on, status, created_at, updated_at
		FROM academic_sessions
		WHERE tenant_id = $1
		ORDER BY starts_on DESC, name
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list academic sessions: %w", err)
	}
	defer rows.Close()
	sessions := make([]Session, 0)
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan academic session: %w", err)
		}
		sessions = append(sessions, *session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate academic sessions: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("finish academic session listing: %w", err)
	}
	return sessions, nil
}

func (r *postgresRepository) GetSession(ctx context.Context, slug string, actorID, sessionID uuid.UUID) (*Session, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin academic session lookup: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := resolveTenant(ctx, tx, slug, actorID, false, true)
	if err != nil {
		return nil, err
	}
	session, err := scanSession(tx.QueryRow(ctx, `
		SELECT id, name, starts_on, ends_on, status, created_at, updated_at
		FROM academic_sessions
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, sessionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get academic session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("finish academic session lookup: %w", err)
	}
	return session, nil
}

func (r *postgresRepository) UpdateSession(ctx context.Context, slug string, actorID, sessionID uuid.UUID, request UpdateSessionRequest, event *auth.AuditEvent) (*Session, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin academic session update: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}
	var name *string
	var startsOn, endsOn *string
	if request.Name != nil {
		name = request.Name
	}
	if request.StartsOn != nil {
		startsOn = request.StartsOn
	}
	if request.EndsOn != nil {
		endsOn = request.EndsOn
	}
	session, err := scanSession(tx.QueryRow(ctx, `
		UPDATE academic_sessions
		SET name = COALESCE($3::varchar, name),
			starts_on = COALESCE($4::date, starts_on),
			ends_on = COALESCE($5::date, ends_on)
		WHERE tenant_id = $1 AND id = $2
		RETURNING id, name, starts_on, ends_on, status, created_at, updated_at
	`, tenantID, sessionID, name, startsOn, endsOn))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, mapAcademicError(err)
	}
	setAcademicAuditTenant(event, tenantID)
	if err := writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(fmt.Errorf("commit academic session update: %w", err))
	}
	return session, nil
}

func (r *postgresRepository) GetTerms(ctx context.Context, slug string, actorID, sessionID uuid.UUID) ([]Term, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin academic term lookup: %w", err)
	}
	defer tx.Rollback(ctx)
	tenantID, err := resolveTenant(ctx, tx, slug, actorID, false, true)
	if err != nil {
		return nil, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM academic_sessions WHERE tenant_id = $1 AND id = $2
		)
	`, tenantID, sessionID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check academic session for terms: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := tx.Query(ctx, `
		SELECT id, session_id, sequence, name, starts_on, ends_on, created_at, updated_at
		FROM academic_terms
		WHERE tenant_id = $1 AND session_id = $2
		ORDER BY sequence
	`, tenantID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list academic terms: %w", err)
	}
	defer rows.Close()
	terms := make([]Term, 0, 3)
	for rows.Next() {
		term, err := scanTerm(rows)
		if err != nil {
			return nil, fmt.Errorf("scan academic term: %w", err)
		}
		terms = append(terms, *term)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate academic terms: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("finish academic term lookup: %w", err)
	}
	return terms, nil
}

func (r *postgresRepository) ConfigureTerms(ctx context.Context, slug string, actorID, sessionID uuid.UUID, inputs []TermInput, event *auth.AuditEvent) ([]Term, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin academic term configuration: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}
	var status string
	err = tx.QueryRow(ctx, `
		SELECT status FROM academic_sessions
		WHERE tenant_id = $1 AND id = $2
		FOR UPDATE
	`, tenantID, sessionID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock academic session for term configuration: %w", err)
	}
	if status == SessionArchived {
		return nil, ErrConflict
	}

	terms := make([]Term, 0, len(inputs))
	for _, input := range inputs {
		term, err := scanTerm(tx.QueryRow(ctx, `
			INSERT INTO academic_terms (id, tenant_id, session_id, sequence, name, starts_on, ends_on)
			VALUES ($1, $2, $3, $4, $5, $6::date, $7::date)
			ON CONFLICT (tenant_id, session_id, sequence)
			DO UPDATE SET name = EXCLUDED.name, starts_on = EXCLUDED.starts_on, ends_on = EXCLUDED.ends_on
			RETURNING id, session_id, sequence, name, starts_on, ends_on, created_at, updated_at
		`, uuid.New(), tenantID, sessionID, input.Sequence, input.Name, input.StartsOn, input.EndsOn))
		if err != nil {
			return nil, mapAcademicError(err)
		}
		terms = append(terms, *term)
	}
	setAcademicAuditTenant(event, tenantID)
	if err := writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(fmt.Errorf("commit academic term configuration: %w", err))
	}
	return terms, nil
}

func (r *postgresRepository) SetSessionStatus(ctx context.Context, slug string, actorID, sessionID uuid.UUID, targetStatus string, event *auth.AuditEvent) (*Session, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin academic session lifecycle change: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID, err := resolveTenant(ctx, tx, slug, actorID, true, true)
	if err != nil {
		return nil, err
	}
	var currentStatus string
	err = tx.QueryRow(ctx, `
		SELECT status FROM academic_sessions WHERE tenant_id = $1 AND id = $2 FOR UPDATE
	`, tenantID, sessionID).Scan(&currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock academic session lifecycle: %w", err)
	}
	if currentStatus == SessionArchived || currentStatus == targetStatus ||
		(targetStatus == SessionActive && currentStatus != SessionDraft) {
		return nil, ErrConflict
	}
	session, err := scanSession(tx.QueryRow(ctx, `
		UPDATE academic_sessions SET status = $3
		WHERE tenant_id = $1 AND id = $2
		RETURNING id, name, starts_on, ends_on, status, created_at, updated_at
	`, tenantID, sessionID, targetStatus))
	if err != nil {
		return nil, mapAcademicError(err)
	}
	setAcademicAuditTenant(event, tenantID)
	if err := writeAcademicAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, mapAcademicError(fmt.Errorf("commit academic session lifecycle change: %w", err))
	}
	return session, nil
}

type rowScanner interface {
	Scan(...any) error
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func resolveTenant(ctx context.Context, queryer queryRower, slug string, actorID uuid.UUID, manage, lockMembership bool) (uuid.UUID, error) {
	var tenantID uuid.UUID
	var role string
	query := `
		SELECT t.id, m.role
		FROM tenants t
		JOIN memberships m ON m.tenant_id = t.id
		WHERE t.slug = $1::citext AND t.status = 'ACTIVE'
			AND m.user_id = $2 AND m.status = 'ACTIVE'
		ORDER BY CASE
			WHEN m.role IN ('SCHOOL_OWNER', 'SCHOOL_ADMIN', 'PRINCIPAL') THEN 0
			WHEN m.role IN ('TEACHER', 'STUDENT', 'PARENT') THEN 1
			ELSE 2
		END
		LIMIT 1
	`
	if lockMembership {
		query += ` FOR SHARE OF t, m`
	}
	err := queryer.QueryRow(ctx, query, slug, actorID).Scan(&tenantID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve academic tenant membership: %w", err)
	}
	if !canRead(role) {
		return uuid.Nil, ErrForbidden
	}
	if manage && !canManage(role) {
		return uuid.Nil, ErrForbidden
	}
	return tenantID, nil
}

func canRead(role string) bool {
	switch role {
	case "SCHOOL_OWNER", "SCHOOL_ADMIN", "PRINCIPAL", "TEACHER", "STUDENT", "PARENT":
		return true
	default:
		return false
	}
}

func canManage(role string) bool {
	switch role {
	case "SCHOOL_OWNER", "SCHOOL_ADMIN", "PRINCIPAL":
		return true
	default:
		return false
	}
}

func scanSession(row rowScanner) (*Session, error) {
	var session Session
	var startsOn, endsOn time.Time
	err := row.Scan(&session.ID, &session.Name, &startsOn, &endsOn, &session.Status, &session.CreatedAt, &session.UpdatedAt)
	if err != nil {
		return nil, err
	}
	session.StartsOn = startsOn.Format(dateLayout)
	session.EndsOn = endsOn.Format(dateLayout)
	return &session, nil
}

func scanTerm(row rowScanner) (*Term, error) {
	var term Term
	var startsOn, endsOn time.Time
	err := row.Scan(&term.ID, &term.SessionID, &term.Sequence, &term.Name, &startsOn, &endsOn, &term.CreatedAt, &term.UpdatedAt)
	if err != nil {
		return nil, err
	}
	term.StartsOn = startsOn.Format(dateLayout)
	term.EndsOn = endsOn.Format(dateLayout)
	return &term, nil
}

func setAcademicAuditTenant(event *auth.AuditEvent, tenantID uuid.UUID) {
	event.TenantID = &tenantID
}

func writeAcademicAudit(ctx context.Context, tx pgx.Tx, event *auth.AuditEvent) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	metadata, err := json.Marshal(event.MetadataRedacted)
	if err != nil {
		return fmt.Errorf("encode academic audit metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id, tenant_id, actor_id, action, subject_type, subject_id, request_id, metadata_redacted, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_TIMESTAMP)
	`, event.ID, event.TenantID, event.ActorID, event.Action, event.SubjectType, event.SubjectID, event.RequestID, metadata)
	if err != nil {
		return fmt.Errorf("write academic audit event: %w", err)
	}
	return nil
}

func mapAcademicError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		switch pgError.Code {
		case "23505", "23P01":
			return ErrConflict
		case "23503":
			return ErrInvalidInput
		case "23514":
			switch pgError.ConstraintName {
			case "academic_session_archived_immutable", "academic_term_archived_immutable",
				"academic_session_identity_immutable", "academic_term_identity_immutable",
				"academic_term_delete_forbidden", "academic_session_transition",
				"academic_session_terms_required":
				return ErrConflict
			default:
				return ErrInvalidInput
			}
		}
	}
	return err
}
